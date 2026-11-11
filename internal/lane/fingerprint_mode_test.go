package lane

import "testing"

func TestFingerprintModeNormalization(t *testing.T) {
	cases := map[string]string{
		"":        "auto",
		"auto":    "auto",
		"OFF":     "off",
		"never":   "off",
		"always":  "force",
		"force":   "force",
		"garbage": "auto",
	}
	for in, want := range cases {
		if got := NormalizeFingerprintMode(in); got != want {
			t.Errorf("NormalizeFingerprintMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFingerprintModeControlsInjection(t *testing.T) {
	t.Cleanup(func() { SetFingerprintMode("auto") })
	SetFingerprintMode("off")
	if FingerprintEnabledFor("") {
		t.Fatal("off still injected the fingerprint")
	}
	body := map[string]any{"messages": []any{}}
	if rename := ApplyFingerprintIfEnabled(body, "chat"); len(rename) != 0 {
		t.Fatal("off still renamed tools")
	}
	if _, ok := body["tools"]; ok {
		t.Fatal("off still added tools")
	}
	SetFingerprintMode("force")
	if !FingerprintEnabledFor(ChannelKilo) {
		t.Fatal("force did not apply to the Kilo channel")
	}
	SetFingerprintMode("auto")
	if FingerprintEnabledFor(ChannelKilo) || !FingerprintEnabledFor("") {
		t.Fatal("auto did not preserve the historical channel split")
	}
}
