package gateway

import (
	"net/http/httptest"
	"testing"

	"github.com/chipmax/openki/internal/lane"
)

// rotate is a real mode — lane.SetProxy tunnels through the subscription pool
// and the sidecar manager implements it — but the settings whitelist predates
// it, so the dashboard's 「订阅轮询 (sing-box)」 choice fell through the switch:
// no 400, no save, no effect.
func TestSettingsAppliesRotateProxyMode(t *testing.T) {
	up := fakeUpstream(t, []string{"[DONE]"})
	defer up.Close()
	s := newTestServer(t, up)
	t.Cleanup(func() { lane.SetProxy("direct", "") })

	ts := httptest.NewServer(s.mux)
	defer ts.Close()
	if code := patchSettings(t, ts.URL, `{"proxyMode":"rotate"}`); code != 200 {
		t.Fatalf("rotate = %d, want 200", code)
	}
	if got := s.Store.Config().ProxyMode; got != "rotate" {
		t.Errorf("ProxyMode = %q, want rotate", got)
	}
}

// A typo in the mode must not read as success while nothing changed.
func TestSettingsRejectsUnknownProxyMode(t *testing.T) {
	up := fakeUpstream(t, []string{"[DONE]"})
	defer up.Close()
	s := newTestServer(t, up)
	t.Cleanup(func() { lane.SetProxy("direct", "") })

	ts := httptest.NewServer(s.mux)
	defer ts.Close()
	if code := patchSettings(t, ts.URL, `{"proxyMode":"roatete"}`); code != 400 {
		t.Fatalf("unknown mode = %d, want 400", code)
	}
	if got := s.Store.Config().ProxyMode; got == "roatete" {
		t.Errorf("rejected mode was still stored")
	}
}

// "pool" is the static-list sibling of the subscription rotation: choosing it
// must install a rotator built from the list in the same request that saves
// the mode, or the transport dials through a nil rotator and silently leaves
// from the machine's own IP.
func TestSettingsPoolModeInstallsStaticRotator(t *testing.T) {
	up := fakeUpstream(t, []string{"[DONE]"})
	defer up.Close()
	s := newTestServer(t, up)
	t.Cleanup(func() { lane.SetProxy("direct", ""); lane.SetRotator(nil) })

	ts := httptest.NewServer(s.mux)
	defer ts.Close()
	if code := patchSettings(t, ts.URL, `{"proxyMode":"pool","proxyPool":["direct","socks5h://127.0.0.1:13001"]}`); code != 200 {
		t.Fatalf("pool = %d, want 200", code)
	}
	cfg := s.Store.Config()
	if cfg.ProxyMode != "pool" {
		t.Errorf("ProxyMode = %q, want pool", cfg.ProxyMode)
	}
	if len(cfg.ProxyPool) != 2 || cfg.ProxyPool[1] != "socks5h://127.0.0.1:13001" {
		t.Errorf("ProxyPool = %v, want both entries persisted", cfg.ProxyPool)
	}
	r := lane.CurrentRotator()
	if r == nil {
		t.Fatal("the pool mode was saved without installing a rotator")
	}
	if r.Healthy() != 2 {
		t.Errorf("rotator Healthy = %d, want 2", r.Healthy())
	}
}

func TestSettingsAppliesProtonPeerAndRejectsBadListener(t *testing.T) {
	up := fakeUpstream(t, []string{"[DONE]"})
	defer up.Close()
	s := newTestServer(t, up)
	t.Cleanup(func() { lane.SetProxy("direct", "") })

	ts := httptest.NewServer(s.mux)
	defer ts.Close()
	if code := patchSettings(t, ts.URL,
		`{"egressModes":["proton"],"protonSocks5":"http://127.0.0.1:8080"}`); code != 400 {
		t.Fatalf("HTTP listener as Proton = %d, want 400", code)
	}
	if got := s.Store.Config().EgressModes; len(got) != 0 {
		t.Fatalf("rejected Proton settings were stored: %v", got)
	}
	if code := patchSettings(t, ts.URL,
		`{"egressModes":["proton"],"protonSocks5":"socks5h://127.0.0.1:1080"}`); code != 200 {
		t.Fatalf("valid Proton settings = %d, want 200", code)
	}
	cfg := s.Store.Config()
	if len(cfg.EgressModes) != 1 || cfg.EgressModes[0] != "proton" {
		t.Fatalf("EgressModes = %v, want [proton]", cfg.EgressModes)
	}
	if cfg.ProtonSocks5 != "socks5h://127.0.0.1:1080" {
		t.Fatalf("ProtonSocks5 = %q", cfg.ProtonSocks5)
	}
	if !lane.EgressExplicit() {
		t.Fatal("saving Proton did not install the egress selector")
	}
}

func TestSettingsRejectsInvalidProxyPoolEntry(t *testing.T) {
	up := fakeUpstream(t, []string{"[DONE]"})
	defer up.Close()
	s := newTestServer(t, up)
	t.Cleanup(func() { lane.SetProxy("direct", ""); lane.SetRotator(nil) })

	ts := httptest.NewServer(s.mux)
	defer ts.Close()
	if code := patchSettings(t, ts.URL, `{"proxyMode":"pool","proxyPool":["socks5://127.0.0.1"]}`); code != 400 {
		t.Fatalf("malformed pool = %d, want 400", code)
	}
	cfg := s.Store.Config()
	if cfg.ProxyMode == "pool" || len(cfg.ProxyPool) != 0 {
		t.Errorf("a rejected pool write was still stored: mode=%q pool=%v", cfg.ProxyMode, cfg.ProxyPool)
	}
}
