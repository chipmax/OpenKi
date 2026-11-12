package store

import "github.com/chipmax/openki/internal/lane"

// RelayEndpoints converts the stored relay pool into the lane's egress type.
// Disabled entries are dropped here: a relay the user turned off must not be
// dialed even when "relay" remains in EgressModes.
func (c Config) RelayEndpoints() []lane.RelayEndpoint {
	out := make([]lane.RelayEndpoint, 0, len(c.RelayPool))
	for _, r := range c.RelayPool {
		if !r.Enabled {
			continue
		}
		out = append(out, lane.RelayEndpoint{ID: r.ID, URL: r.URL, Token: r.Token})
	}
	return out
}

// EgressModeList returns a copy of the configured mode strings.
func (c Config) EgressModeList() []string {
	return append([]string(nil), c.EgressModes...)
}

// ValidAgentScope reports whether raw is one of the canonical scope names.
func ValidAgentScope(raw string) bool {
	switch raw {
	case "host", "windows", "wsl", "all", "none":
		return true
	}
	return false
}

// NormalizeAgentScope folds the scope aliases onto the five canonical values.
func NormalizeAgentScope(raw string) string {
	switch raw {
	case "host", "windows", "wsl", "all", "none":
		return raw
	case "":
		return "all"
	default:
		return "host"
	}
}
