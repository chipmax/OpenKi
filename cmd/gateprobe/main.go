// gateprobe checks whether the OpenCode free lane still requires the injected
// read/shell tool fingerprint. It sends one minimal turn with the definitions
// and one without, then reports which side the gate accepted.
//
// This makes two real upstream requests on the free pool. Run it manually when
// the gate appears to change, not from a loop.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"zen-gate/internal/lane"
)

func main() {
	model := flag.String("model", "space-bunny-free", "model id to test")
	timeout := flag.Duration("timeout", 60*time.Second, "per-request timeout")
	asJSON := flag.Bool("json", false, "print the result as JSON")
	base := flag.String("base", "", "override the upstream base URL (default https://opencode.ai)")
	flag.Parse()

	if *base != "" {
		lane.UpstreamBase = *base
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout*2+5*time.Second)
	defer cancel()
	res := lane.CheckFingerprintGate(ctx, *model)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
			os.Exit(2)
		}
		return
	}

	fmt.Printf("model: %s\n", res.Model)
	fmt.Printf("with fingerprint:    %s", gateSide(res.With))
	fmt.Printf("without fingerprint: %s", gateSide(res.Without))
	fmt.Printf("verdict: %s\n", res.Verdict)
	if res.Note != "" {
		fmt.Printf("note: %s\n", res.Note)
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
