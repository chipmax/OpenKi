// gateprobe checks whether the OpenCode free lane still requires the injected
// read/shell tool fingerprint. It sends one minimal turn with the definitions
// and one without, then reports which side the gate accepted.
//
// This makes two real upstream requests per run on the free pool. Run it
// manually when the gate appears to change, not from a loop. Repeating a run
// (-n) is for judging a noisy verdict: a single INCONCLUSIVE can be quota, but
// five runs that all say FINGERPRINT_REQUIRED are evidence.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chipmax/loimo/internal/lane"
	"github.com/chipmax/loimo/internal/store"
)

func main() {
	model := flag.String("model", "space-bunny-free", "model id to test")
	timeout := flag.Duration("timeout", 60*time.Second, "per-request timeout")
	runs := flag.Int("n", 1, "number of A/B pairs to run (each pair costs two upstream requests)")
	via := flag.String("via", "direct", "transport for the probe: direct | pool | warp")
	poolList := flag.String("pool", "", "comma-separated exits for -via pool (default: proxyPool from config.json)")
	asJSON := flag.Bool("json", false, "print the result as JSON")
	base := flag.String("base", "", "override the upstream base URL (default https://opencode.ai)")
	flag.Parse()

	if *base != "" {
		lane.UpstreamBase = *base
	}
	if *runs < 1 {
		*runs = 1
	}
	if *runs > 10 {
		fmt.Fprintln(os.Stderr, "-n capped at 10")
		*runs = 10
	}

	// The probe must compare the gate body, not whatever ambient proxy the
	// shell exported: WSL sets HTTPS_PROXY for the host tunnel, and a dead
	// proxy turned a real 429 into a 30-second TIMEOUT in the first run here.
	// Loading the store also applies gateUserAgent/gateTools so the probe
	// measures the profile the gateway would actually send.
	var cfgPool []string
	if st, err := store.Open(); err == nil {
		cfg := st.Config()
		lane.SetGateProfile(cfg.GateUserAgent, cfg.GateTools)
		cfgPool = cfg.ProxyPool
	}
	switch strings.ToLower(strings.TrimSpace(*via)) {
	case "", "direct":
		lane.SetProxy("direct", "")
		*via = "direct"
	case "pool":
		entries := cfgPool
		if strings.TrimSpace(*poolList) != "" {
			entries = nil
			for _, e := range strings.Split(*poolList, ",") {
				if e = strings.TrimSpace(e); e != "" {
					entries = append(entries, e)
				}
			}
		}
		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "-via pool needs -pool or proxyPool in config.json")
			os.Exit(2)
		}
		pool, err := lane.NewProxyPool(entries)
		if err != nil {
			fmt.Fprintln(os.Stderr, "proxy pool:", err)
			os.Exit(2)
		}
		lane.SetProxy("pool", "")
		lane.SetRotator(pool)
	case "warp":
		lane.SetWarpEnabled(true, 2)
		lane.SetProxy("warp", "")
	default:
		fmt.Fprintln(os.Stderr, "unknown -via (use direct, pool or warp):", *via)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "via: %s\n", *via)

	results := make([]lane.FingerprintGateResult, 0, *runs)
	for i := 0; i < *runs; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout*2+5*time.Second)
		results = append(results, lane.CheckFingerprintGate(ctx, *model))
		cancel()
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		var out any = results[0]
		if len(results) > 1 {
			out = results
		}
		if err := enc.Encode(out); err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
			os.Exit(2)
		}
		return
	}

	for i, res := range results {
		if len(results) > 1 {
			fmt.Printf("--- run %d/%d ---\n", i+1, len(results))
		}
		fmt.Printf("model: %s\n", res.Model)
		fmt.Printf("fingerprint hash: %s\n", res.Hash)
		fmt.Printf("with fingerprint:    %s", gateSide(res.With))
		fmt.Printf("without fingerprint: %s", gateSide(res.Without))
		fmt.Printf("verdict: %s\n", res.Verdict)
		if res.Note != "" {
			fmt.Printf("note: %s\n", res.Note)
		}
	}
	if len(results) > 1 {
		counts := map[string]int{}
		for _, res := range results {
			counts[res.Verdict]++
		}
		fmt.Printf("summary: ")
		for _, verdict := range []string{
			lane.GateCheckFingerprintRequired, lane.GateCheckGateOpen, lane.GateCheckInconclusive,
		} {
			if n := counts[verdict]; n > 0 {
				fmt.Printf("%s=%d ", verdict, n)
			}
		}
		fmt.Println()
	}
}

func gateSide(a lane.GateAttempt) string {
	status := "ok"
	if !a.OK {
		status = "failed"
		if a.Code != "" {
			status += " (" + a.Code + ")"
		}
		if a.Status != 0 {
			status += fmt.Sprintf(" HTTP %d", a.Status)
		}
		if a.Message != "" {
			status += ": " + a.Message
		}
	}
	return fmt.Sprintf("%-8s %dms\n", status, a.Millis)
}
