package lane

import (
	"encoding/json"
	"testing"
)

func TestGateProfileOverrideChangesHashAndDefinitions(t *testing.T) {
	t.Cleanup(func() { SetGateProfile("", nil) })
	baseHash := FingerprintHash()
	if baseHash == "" {
		t.Fatal("empty fingerprint hash")
	}
	if GateUserAgent() != ClientUA {
		t.Fatalf("default UA = %q, want %q", GateUserAgent(), ClientUA)
	}

	SetGateProfile("opencode/test/9.9.9/cli", map[string]GateToolDef{
		"read": {
			Description: "custom read description",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	})
	if got := GateUserAgent(); got != "opencode/test/9.9.9/cli" {
		t.Fatalf("UA override = %q", got)
	}
	if FingerprintHash() == baseHash {
		t.Fatal("hash did not change after a profile override")
	}
	desc, params, ok := fingerprintDefs("read")
	if !ok || desc != "custom read description" {
		t.Fatalf("read override not applied: ok=%v desc=%q", ok, desc)
	}
	if params["type"] != "object" {
		t.Fatalf("read schema override not applied: %v", params)
	}
	// A name not in the compiled pair is ignored rather than turning the
	// fingerprint into something the gate has never seen.
	SetGateProfile("", map[string]GateToolDef{
		"write": {Description: "should not exist"},
		"shell": {Parameters: json.RawMessage(`{not json`)},
	})
	if ua := GateUserAgent(); ua != ClientUA {
		t.Fatalf("UA not restored: %q", ua)
	}
	shellDesc, _, _ := fingerprintDefs("shell")
	if shellDesc != realShellDesc {
		t.Fatalf("malformed schema override changed the shell description: %q", shellDesc)
	}
	if _, _, ok := fingerprintDefs("write"); ok {
		t.Fatal("unknown tool name entered the fingerprint")
	}
	if FingerprintHash() != baseHash {
		t.Fatal("hash did not return to the compiled default after overrides were dropped")
	}
}
