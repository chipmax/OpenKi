package lane

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// GateToolDef overrides one fingerprint tool's description and parameter
// schema. It is a config-file escape hatch on purpose: the upstream gate
// compares the body byte for byte, so a dashboard field inviting casual edits
// would invite silent 403s. An override replaces the compiled value whole (the
// schema is not merged field by field); an empty field keeps the compiled
// default.
type GateToolDef struct {
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// gateProfileDef is the validated, canonical form of one override.
type gateProfileDef struct {
	desc   string
	params json.RawMessage
}

var gateProfile = struct {
	sync.RWMutex
	ua   string
	defs map[string]gateProfileDef
}{}

// SetGateProfile installs the optional config.json override for the OpenCode
// fingerprint (user agent + per-tool description/schema). Unknown tool names,
// malformed JSON and blank values are dropped so a typo cannot produce a
// request that is neither the compiled fingerprint nor the one the user meant.
// Pass empty values to restore the compiled defaults.
func SetGateProfile(ua string, defs map[string]GateToolDef) {
	next := map[string]gateProfileDef{}
	for name, def := range defs {
		if _, _, ok := defaultFingerprintDef(name); !ok {
			continue
		}
		pd := gateProfileDef{desc: strings.TrimSpace(def.Description)}
		if len(def.Parameters) > 0 {
			var v any
			if err := json.Unmarshal(def.Parameters, &v); err == nil {
				if m, ok := v.(map[string]any); ok {
					if b, err := json.Marshal(m); err == nil {
						pd.params = b
					}
				}
			}
		}
		if pd.desc == "" && len(pd.params) == 0 {
			continue
		}
		next[name] = pd
	}
	gateProfile.Lock()
	gateProfile.ua = strings.TrimSpace(ua)
	gateProfile.defs = next
	gateProfile.Unlock()
}

// GateUserAgent is the User-Agent the gate currently expects: the config
// override when one is set, otherwise the compiled v2 CLI string.
func GateUserAgent() string {
	gateProfile.RLock()
	ua := gateProfile.ua
	gateProfile.RUnlock()
	if ua != "" {
		return ua
	}
	return ClientUA
}

// gateOverride returns the validated override for one tool, if any.
func gateOverride(name string) (gateProfileDef, bool) {
	gateProfile.RLock()
	def, ok := gateProfile.defs[name]
	gateProfile.RUnlock()
	return def, ok
}

// fingerprintDefs returns the effective exact read/shell definitions keyed by
// name: the compiled v2.0.26 shapes with any config override applied. Values
// are rebuilt per call so callers cannot mutate the shared schema.
func fingerprintDefs(name string) (desc string, params map[string]any, ok bool) {
	desc, params, ok = defaultFingerprintDef(name)
	if !ok {
		return "", nil, false
	}
	if def, over := gateOverride(name); over {
		if def.desc != "" {
			desc = def.desc
		}
		if len(def.params) > 0 {
			var p map[string]any
			if err := json.Unmarshal(def.params, &p); err == nil {
				params = p
			}
		}
	}
	return desc, params, true
}

// FingerprintHash is a short digest of the effective gate profile: user agent,
// tool names, descriptions and parameter schemas. It is logged at boot and by
// gate-check so "did the fingerprint we send change?" is answerable without a
// body diff — the usual cause of a sudden FREE_TIER_BLOCKED after an upgrade.
func FingerprintHash() string {
	h := sha256.New()
	fmt.Fprintf(h, "ua\x00%s\x00", GateUserAgent())
	names := append([]string(nil), FingerprintTools...)
	sort.Strings(names)
	for _, name := range names {
		desc, params, _ := fingerprintDefs(name)
		b, _ := json.Marshal(params)
		fmt.Fprintf(h, "%s\x00%s\x00", name, desc)
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
