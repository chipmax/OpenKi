package gateway

import (
	"net/http/httptest"
	"testing"

	"zen-gate/internal/lane"
)

// Both fields are process-wide policy switches. A panel save that only changed
// in-memory state would look applied until the next boot, so the test asserts
// the store and the live lane agree, and that an unrelated save cannot revert
// them.
func TestSettingsPATCHPersistsEgressPolicy(t *testing.T) {
	up := fakeUpstream(t, []string{"[DONE]"})
	defer up.Close()
	s := newTestServer(t, up)
	ts := httptest.NewServer(s.mux)
	defer ts.Close()
	t.Cleanup(func() { lane.SetFingerprintMode("auto") })

	if code := patchSettings(t, ts.URL, `{"exitTokensPerMinute":60000,"fingerprintMode":"off"}`); code != 200 {
		t.Fatalf("settings = %d, want 200", code)
	}
	if got := s.Store.Config().ExitTokensPerMinute; got != 60000 {
		t.Fatalf("exitTokensPerMinute persisted as %d, want 60000", got)
	}
	if got := s.Store.Config().FingerprintMode; got != "off" {
		t.Fatalf("fingerprintMode persisted as %q, want off", got)
	}
	if got := lane.FingerprintMode(); got != "off" {
		t.Fatalf("live fingerprint mode = %q, want off", got)
	}

	if code := patchSettings(t, ts.URL, `{"autoTagEnabled":false}`); code != 200 {
		t.Fatalf("unrelated settings = %d, want 200", code)
	}
	if got := s.Store.Config().ExitTokensPerMinute; got != 60000 {
		t.Fatalf("unrelated save changed exitTokensPerMinute to %d", got)
	}
	if got := s.Store.Config().FingerprintMode; got != "off" {
		t.Fatalf("unrelated save changed fingerprintMode to %q", got)
	}

	// Turn both back off/auto: neither branch is a one-way door.
	if code := patchSettings(t, ts.URL, `{"exitTokensPerMinute":0,"fingerprintMode":"auto"}`); code != 200 {
		t.Fatalf("settings reset = %d, want 200", code)
	}
	if got := s.Store.Config().ExitTokensPerMinute; got != 0 {
		t.Fatalf("exitTokensPerMinute reset to %d, want 0", got)
	}
	if got := s.Store.Config().FingerprintMode; got != "auto" {
		t.Fatalf("fingerprintMode reset to %q, want auto", got)
	}
	if got := lane.FingerprintMode(); got != "auto" {
		t.Fatalf("live fingerprint mode = %q, want auto", got)
	}
}
