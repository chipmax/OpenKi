package lane

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

// The proxy-pool rotator is the opencode2api "proxies" mechanism: a fixed list
// of exits that real traffic is spread across, with the same (exit, model)
// ledger as the subscription sidecar. These tests cover the decisions that
// cannot be observed from a live run without eight real proxies: parsing,
// sticky selection, the ban/revive ledger, dead-endpoint parking, and the
// HTTP CONNECT handshake.

func newTestPool(t *testing.T, entries ...string) *ProxyPool {
	t.Helper()
	p, err := NewProxyPool(entries)
	if err != nil {
		t.Fatalf("NewProxyPool(%v): %v", entries, err)
	}
	return p
}

func TestProxyPoolParsesSupportedSchemes(t *testing.T) {
	p := newTestPool(t,
		"direct",
		"socks5h://127.0.0.1:13001",
		"socks5://user:pass@127.0.0.1:1080",
		"http://127.0.0.1",
		"https://user@127.0.0.1:8443",
	)
	if len(p.nodes) != 5 {
		t.Fatalf("nodes = %d, want 5", len(p.nodes))
	}
	if got := p.nodes[3].addr; got != "127.0.0.1:80" {
		t.Errorf("http default port = %q, want 127.0.0.1:80", got)
	}
	if got := p.nodes[4].addr; got != "127.0.0.1:8443" {
		t.Errorf("https addr = %q, want 127.0.0.1:8443", got)
	}
	for _, n := range p.nodes {
		if strings.Contains(n.id, "pass") || strings.Contains(n.id, "secret") {
			t.Errorf("node id leaks credentials: %q", n.id)
		}
	}
}

func TestProxyPoolNormalizeRejectsBadEntriesWithoutEchoingSecrets(t *testing.T) {
	if _, err := NormalizeProxyPool([]string{"socks5://user:secret@127.0.0.1"}); err == nil {
		t.Fatal("a SOCKS5 entry without a port was accepted")
	} else if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaks the proxy password: %v", err)
	}
	if _, err := NormalizeProxyPool([]string{"ftp://127.0.0.1:21"}); err == nil {
		t.Fatal("an unsupported scheme was accepted")
	}
	clean, err := NormalizeProxyPool([]string{"", "  socks5h://127.0.0.1:13001  "})
	if err != nil || len(clean) != 1 || clean[0] != "socks5h://127.0.0.1:13001" {
		t.Fatalf("clean = %v, err = %v", clean, err)
	}
	if _, err := NewProxyPool(nil); err == nil {
		t.Fatal("an empty pool was accepted")
	}
}

func TestProxyPoolDeduplicatesEntries(t *testing.T) {
	p := newTestPool(t, "socks5h://127.0.0.1:13001", "socks5h://127.0.0.1:13001")
	if len(p.nodes) != 1 {
		t.Fatalf("nodes = %d, want 1 after dedupe", len(p.nodes))
	}
}

// A conversation must keep one exit (prompt-cache affinity), and separate
// conversations must not all land on the same node.
func TestProxyPoolStickyAndSpread(t *testing.T) {
	p := newTestPool(t, "direct", "socks5h://127.0.0.1:13001", "socks5h://127.0.0.1:13002")

	first := p.Pick("conversation-a", "model-x")
	if again := p.Pick("conversation-a", "model-x"); again != first {
		t.Fatalf("sticky pick moved: %q then %q", first, again)
	}
	seen := map[string]bool{first: true}
	for i := range 40 {
		seed := "conversation-" + string(rune('b'+i%26)) + "-" + string(rune('0'+i/26))
		seen[p.Pick(seed, "model-x")] = true
	}
	if len(seen) < 2 {
		t.Fatalf("40 conversations all chose one exit: %v", seen)
	}
}

// A 429 is one exit's answer: the pair is parked, the next pick for the same
// conversation moves on, and a real success (Revive) clears it.
func TestProxyPoolLedgerBansAndRevivesAPair(t *testing.T) {
	p := newTestPool(t, "socks5h://127.0.0.1:13001", "socks5h://127.0.0.1:13002")
	seed := "ledger-conversation"
	first := p.Pick(seed, "model-x")
	p.Report(first, "model-x", "limited")
	if next := p.Pick(seed, "model-x"); next == first {
		t.Fatalf("a rate-limited exit was picked again: %q", next)
	}
	// Another model on the same exit is unaffected — the ledger is per pair.
	if other := p.Pick("other-seed", "model-y"); other == "" {
		t.Fatal("no exit picked for an unrelated model")
	}
	p.Revive(first, "model-x")
	p.mu.Lock()
	_, banned := p.banUntil[poolBanKey(first, "model-x")]
	p.mu.Unlock()
	if banned {
		t.Fatal("Revive left the pair parked")
	}
}

func TestProxyPoolPicksAnExitEvenWhenEveryExitIsParked(t *testing.T) {
	p := newTestPool(t, "socks5h://127.0.0.1:13001", "socks5h://127.0.0.1:13002")
	p.Report(p.nodes[0].id, "model-x", "region")
	p.Report(p.nodes[1].id, "model-x", "region")
	got := p.Pick("seed", "model-x")
	if got == "" {
		t.Fatal("an all-parked pool returned no exit, which degrades rotation to a direct dial")
	}
}

func TestProxyPoolParksDeadExitsAndEscalates(t *testing.T) {
	p := newTestPool(t, "socks5h://127.0.0.1:13001", "socks5h://127.0.0.1:13002")
	dead := p.nodes[0].id
	if p.Healthy() != 2 {
		t.Fatalf("Healthy = %d, want 2", p.Healthy())
	}
	p.ReportDead(dead)
	if p.Healthy() != 2 {
		t.Fatal("one strike already parked the exit")
	}
	p.ReportDead(dead)
	if p.Healthy() != 1 {
		t.Fatalf("two strikes did not park the exit: Healthy = %d", p.Healthy())
	}
	firstUntil := p.deadUntil[dead]

	// Rewind the park and strike it out again: the second park must be longer.
	p.mu.Lock()
	p.deadUntil[dead] = time.Now().Add(-time.Second)
	p.mu.Unlock()
	p.ReportDead(dead)
	p.ReportDead(dead)
	if !p.deadUntil[dead].After(firstUntil) {
		t.Fatalf("second park did not escalate: %v then %v", firstUntil, p.deadUntil[dead])
	}
}

// dialHTTPProxy must speak CONNECT (with credentials), reject non-2xx, and
// hand back the tunnel including bytes the header parser buffered.
func TestProxyPoolDialHTTPConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var (
		mu      sync.Mutex
		request string
	)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tp := textproto.NewReader(bufio.NewReader(conn))
		line, _ := tp.ReadLine()
		hdr, _ := tp.ReadMIMEHeader()
		mu.Lock()
		request = line + "|" + hdr.Get("Proxy-Authorization")
		mu.Unlock()
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		_, _ = io.Copy(conn, conn) // echo the tunnel
	}()

	p := newTestPool(t, "http://user:pass@"+ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := p.Dial(ctx, "tcp", "upstream.example:443")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read tunnel: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("tunnel echoed %q, want ping", buf)
	}
	mu.Lock()
	got := request
	mu.Unlock()
	if !strings.HasPrefix(got, "CONNECT upstream.example:443 HTTP/1.1|") {
		t.Fatalf("CONNECT request = %q", got)
	}
	if !strings.HasPrefix(got, "CONNECT upstream.example:443 HTTP/1.1|Basic ") {
		t.Fatalf("Proxy-Authorization = %q, want Basic auth", got)
	}
}

func TestProxyPoolDialHTTPConnectRejectsErrorStatus(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Consume the CONNECT request before closing. Windows turns a close
		// with unread inbound bytes into RST, which can discard the 407 and
		// surface as "connection aborted" instead of the status line.
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		tp := textproto.NewReader(bufio.NewReader(conn))
		for {
			line, err := tp.ReadLine()
			if err != nil || line == "" {
				break
			}
		}
		_, _ = conn.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\n"))
	}()

	p := newTestPool(t, "http://"+ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := p.Dial(ctx, "tcp", "upstream.example:443"); err == nil {
		t.Fatal("a 407 CONNECT was accepted")
	} else if !strings.Contains(err.Error(), "407") {
		t.Fatalf("error = %v, want it to name HTTP 407", err)
	}
}

func TestProxyPoolDialThroughUnknownNode(t *testing.T) {
	p := newTestPool(t, "direct")
	if _, err := p.DialThrough(context.Background(), "tcp", "example.test:80", "missing"); err == nil {
		t.Fatal("DialThrough accepted an unknown node id")
	}
}

func TestProxyPoolSnapshotListsRedactedEndpointsAndParks(t *testing.T) {
	p := newTestPool(t,
		"http://alice:s3cret@127.0.0.1:7890",
		"socks5h://bob:hunter2@127.0.0.1:1080",
	)
	if len(p.nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(p.nodes))
	}
	first := p.nodes[0].id
	p.mu.Lock()
	p.deadUntil[first] = time.Now().Add(2 * time.Minute)
	p.mu.Unlock()

	snap := p.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(snap))
	}
	if snap[0].ID != first || snap[0].Scheme != "http" || snap[0].Addr != "127.0.0.1:7890" {
		t.Fatalf("first snapshot row = %+v", snap[0])
	}
	if snap[0].DeadUntil == 0 {
		t.Fatal("parked endpoint did not carry deadUntil")
	}
	if snap[1].DeadUntil != 0 {
		t.Fatalf("healthy endpoint has deadUntil = %d", snap[1].DeadUntil)
	}
	for _, row := range snap {
		if strings.Contains(row.Addr, "s3cret") || strings.Contains(row.Addr, "hunter2") ||
			strings.Contains(row.Addr, "alice") || strings.Contains(row.Addr, "bob") {
			t.Fatalf("snapshot leaked proxy credentials: %+v", row)
		}
	}
}
