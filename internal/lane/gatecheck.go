package lane

import (
	"context"
	"time"
)

// GateAttempt is one side of the fingerprint A/B check.
type GateAttempt struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Status  int    `json:"status,omitempty"`
	Millis  int64  `json:"millis"`
}

// FingerprintGateResult compares the same minimal turn with and without the
// OpenCode read/shell definitions. The gate is a body shape, not a header, so
// this is the only honest way to tell whether it still applies.
type FingerprintGateResult struct {
	Model    string      `json:"model"`
	Hash     string      `json:"hash"`
	With     GateAttempt `json:"withFingerprint"`
	Without  GateAttempt `json:"withoutFingerprint"`
	Verdict  string      `json:"verdict"`
	Note     string      `json:"note,omitempty"`
	TestedAt int64       `json:"testedAt"`
}

// GateCheckFingerprintRequired means the fingerprint still decides access.
const (
	GateCheckFingerprintRequired = "FINGERPRINT_REQUIRED"
	GateCheckGateOpen            = "GATE_OPEN"
	GateCheckInconclusive        = "INCONCLUSIVE"
)

// CheckFingerprintGate runs one pair of minimal chat turns against the real
// upstream. It bills two short free-lane requests, so callers must be explicit
// (CLI flag / dashboard action), never a background probe.
func CheckFingerprintGate(ctx context.Context, model string) FingerprintGateResult {
	if model == "" {
		model = "space-bunny-free"
	}
	res := FingerprintGateResult{Model: model, Hash: FingerprintHash(), TestedAt: time.Now().UnixMilli()}
	res.With = gateCheckOnce(ctx, model, true)
	res.Without = gateCheckOnce(ctx, model, false)
	switch {
	case res.With.OK && !res.Without.OK:
		res.Verdict = GateCheckFingerprintRequired
		res.Note = "the plain body was refused while the fingerprinted body passed; keep fingerprintMode=auto (or force)"
	case res.With.OK && res.Without.OK:
		res.Verdict = GateCheckGateOpen
		res.Note = "both bodies passed; the gate may no longer require the injected tools — fingerprintMode=off is safe to trial"
	case !res.With.OK && res.Without.OK:
		res.Verdict = GateCheckInconclusive
		res.Note = "the fingerprinted body failed while the plain body passed; check the injected definitions before changing the mode"
	default:
		res.Verdict = GateCheckInconclusive
		res.Note = "both bodies failed; a quota, region or network fault may be masking the gate — retry later"
	}
	return res
}

func gateCheckOnce(ctx context.Context, model string, fingerprint bool) GateAttempt {
	start := time.Now()
	body := map[string]any{
		"model":      model,
		"stream":     true,
		"max_tokens": 16,
		"messages":   []map[string]any{{"role": "user", "content": "ping"}},
	}
	if fingerprint {
		ApplyFingerprint(body, mapWireStyle(WireFor(model)))
	}
	// Fresh sessions on each side: quota is accounted per session, and sharing
	// one would make the second half inherit the first half's crowding.
	seed := "gatecheck:plain"
	if fingerprint {
		seed = "gatecheck:fingerprint"
	}
	_, err := PostStreamed(ctx, EndpointFor(model), body, SessionForConversation(seed), MintRequestId(0), func([]byte) error { return nil })
	att := GateAttempt{OK: err == nil, Millis: time.Since(start).Milliseconds()}
	if err != nil {
		if ue, ok := err.(*UpstreamError); ok {
			att.Code = ue.Code
			att.Message = ue.Message
			att.Status = ue.HTTPStatus
		} else {
			att.Code = CodeTransport
			att.Message = err.Error()
		}
	}
	return att
}
