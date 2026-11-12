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
		gotUpstream = r.Header.Get("X-LoiMo-Relay-Upstream")
		gotToken = r.Header.Get("X-LoiMo-Relay-Token")
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
	if got := resp.Header.Get("X-LoiMo-Egress"); got != "relay:r1" {
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
	if _, err := NormalizeEgressModes([]string{"socks"}); err == nil {
		t.Fatal("unknown mode must fail validation")
	}
}
