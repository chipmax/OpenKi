package lane

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestEgressRelayRewritesAndLabels(t *testing.T) {
	var gotPath, gotUpstream, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUpstream = r.Header.Get("X-OpenKi-Relay-Upstream")
		gotToken = r.Header.Get("X-OpenKi-Relay-Token")
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	SetEgress(EgressOptions{
		Modes:    []string{"relay"},
		Relays:   []RelayEndpoint{{ID: "r1", URL: srv.URL, Token: "tok"}},
		BaseMode: "direct",
	})
	defer SetEgress(EgressOptions{})

	req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions?stream=1", strings.NewReader(`{"model":"m"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := laneHTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != `{"ok":true}` {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("relay path = %q", gotPath)
	}
	if gotUpstream != "https://upstream.example" {
		t.Errorf("relay upstream header = %q", gotUpstream)
	}
	if gotToken != "tok" {
		t.Errorf("relay token = %q", gotToken)
	}
	if got := resp.Header.Get("X-OpenKi-Egress"); got != "relay:r1" {
		t.Errorf("egress label = %q", got)
	}
}

func TestEgressRelayFailsOverToNextPeer(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "second")
	}))
	defer srv.Close()

	SetEgress(EgressOptions{
		Modes: []string{"relay"},
		Relays: []RelayEndpoint{
			{ID: "dead", URL: "http://127.0.0.1:1", Token: "a"},
			{ID: "live", URL: srv.URL, Token: "b"},
		},
		BaseMode: "direct",
	})
	defer SetEgress(EgressOptions{})

	req, err := http.NewRequest(http.MethodGet, "https://upstream.example/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := laneHTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "second" {
		t.Fatalf("body = %q, want second peer", body)
	}
	if hits.Load() != 1 {
		t.Fatalf("live relay hits = %d", hits.Load())
	}
}

func TestEgressEmptyModesRestoresLegacy(t *testing.T) {
	SetEgress(EgressOptions{Modes: []string{"relay"}})
	SetEgress(EgressOptions{BaseMode: "direct"})
	if EgressExplicit() {
		t.Fatal("empty mode list must clear the selector")
	}
	if EgressActive() {
		t.Fatal("legacy transport must not report per-request egress")
	}
}

func TestNormalizeEgressModes(t *testing.T) {
	got, err := NormalizeEgressModes([]string{" WARP ", "relay", "warp", ""})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "warp,relay" {
		t.Fatalf("normalized = %v", got)
	}
	if _, err := NormalizeEgressModes([]string{"socks", "proton"}); err == nil {
		t.Fatal("unknown or removed mode must fail validation")
	}
}

// A 429 is one exit's answer, not the turn's: the selector must close that
// response and ask the next equal peer before the lane applies quota rules.
func TestEgressSelectorRetries429OnNextPeer(t *testing.T) {
	var hits []string
	peer := func(name string, status int, body string) egressPeer {
		return egressPeer{name: name, rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			hits = append(hits, name)
			return &http.Response{
				StatusCode: status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})}
	}
	sel := &egressSelector{peers: []egressPeer{
		peer("limited", http.StatusTooManyRequests, `{"error":"limited"}`),
		peer("good", http.StatusOK, `{"ok":true}`),
	}}
	req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", strings.NewReader(`{"model":"m"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := sel.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the next peer's 200", resp.StatusCode)
	}
	if strings.Join(hits, ",") != "limited,good" {
		t.Fatalf("peer order = %v, want limited then good", hits)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A config saved before Proton was removed must not silently fall back to the
// machine's own IP. SetEgress installs a transport that refuses the request so
// the operator sees the error and fixes config.json.
func TestSetEgressFailsClosedOnRemovedMode(t *testing.T) {
	SetEgress(EgressOptions{Modes: []string{"proton"}, BaseMode: "direct"})
	t.Cleanup(func() { SetEgress(EgressOptions{BaseMode: "direct"}) })

	if EgressExplicit() {
		t.Fatal("a rejected mode must not install an egress selector")
	}
	req, err := http.NewRequest(http.MethodGet, "https://upstream.example/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := laneHTTP.Transport.RoundTrip(req); err == nil {
		t.Fatal("a removed egress mode fell back to a working transport")
	}
}
