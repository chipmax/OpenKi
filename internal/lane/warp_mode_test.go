package lane

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chipmax/openki/internal/warp"
)

// The WARP switch is a pure function of two config values: the dedicated
// boolean (which wins over every proxy mode) and the mode string itself.
func TestUseWarp(t *testing.T) {
	cases := []struct {
		enabled bool
		mode    string
		want    bool
	}{
		{true, "env", true},
		{true, "direct", true},
		{true, "rotate", true},
		{true, "warp", true},
		{false, "warp", true},
		{false, "env", false},
		{false, "direct", false},
		{false, "custom", false},
		{false, "rotate", false},
		{false, "", false},
	}
	for _, c := range cases {
		if got := useWarp(c.enabled, c.mode); got != c.want {
			t.Errorf("useWarp(%v, %q) = %v, want %v", c.enabled, c.mode, got, c.want)
		}
	}
}

// The lease must be released exactly once, when the body closes — closing
// twice (which callers do: defer + explicit) must not double-release the
// exit back into the pool.
func TestLeaseBodyReleasesExactlyOnce(t *testing.T) {
	var releases, closes atomic.Int64
	b := &leaseBody{
		Reader:  strings.NewReader("hello"),
		closer:  closerFunc(func() error { closes.Add(1); return nil }),
		release: func() { releases.Add(1) },
	}
	out, err := io.ReadAll(b)
	if err != nil || string(out) != "hello" {
		t.Fatalf("read = %q, %v", out, err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if releases.Load() != 1 {
		t.Errorf("releases = %d, want exactly 1", releases.Load())
	}
	if closes.Load() != 1 {
		t.Errorf("underlying closes = %d, want exactly 1", closes.Load())
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// When the pool cannot be built or leased (the normal case on a network
// that blocks WARP's UDP), the request must still go out through the base
// transport, and the fallback must be counted.
func TestWarpRoundTripperFallsBackWhenPoolUnavailable(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "via-base")
	}))
	defer up.Close()

	old := warpPoolFor
	warpPoolFor = func(int) (*warp.Pool, error) { return nil, errors.New("udp blocked") }
	defer func() { warpPoolFor = old }()

	// Dial the test server regardless of the (fake, non-loopback) host so
	// the loopback bypass does not mask the fallback path.
	_, serverAddr, _ := strings.Cut(up.URL, "//")
	base := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, serverAddr)
		},
	}
	rt := &warpRoundTripper{base: base, poolSize: 2}
	before := WarpFallbacks()
	req, _ := http.NewRequest(http.MethodGet, "http://upstream.invalid/v1/models", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "via-base" {
		t.Fatalf("resp = %d %q, want 200 via-base", resp.StatusCode, body)
	}
	if delta := WarpFallbacks() - before; delta != 1 {
		t.Errorf("fallback delta = %d, want 1", delta)
	}
}

// Loopback targets bypass WARP entirely (and do not count as fallbacks):
// a local provider is not reachable through a WARP netstack.
func TestWarpRoundTripperBypassesLoopback(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "local")
	}))
	defer up.Close()

	old := warpPoolFor
	warpPoolFor = func(int) (*warp.Pool, error) {
		t.Error("pool must not be consulted for loopback")
		return nil, errors.New("unused")
	}
	defer func() { warpPoolFor = old }()

	rt := &warpRoundTripper{base: &http.Transport{}, poolSize: 2}
	before := WarpFallbacks()
	req, _ := http.NewRequest(http.MethodGet, up.URL+"/v1/models", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "local" {
		t.Fatalf("body = %q, want local", body)
	}
	if delta := WarpFallbacks() - before; delta != 0 {
		t.Errorf("fallback delta = %d, want 0 for loopback bypass", delta)
	}
}

func TestParseRetryAfter(t *testing.T) {
	h := http.Header{}
	if d := parseRetryAfter(h); d != 0 {
		t.Errorf("absent header = %v, want 0", d)
	}
	h.Set("Retry-After", "120")
	if d := parseRetryAfter(h); d != 120*time.Second {
		t.Errorf("seconds form = %v, want 120s", d)
	}
	h.Set("Retry-After", "garbage")
	if d := parseRetryAfter(h); d != 0 {
		t.Errorf("garbage = %v, want 0", d)
	}
	h.Set("Retry-After", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat))
	if d := parseRetryAfter(h); d != 0 {
		t.Errorf("past date = %v, want 0", d)
	}
}
