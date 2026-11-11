package agents

import (
	"os"
	"path/filepath"
)

// platform.go resolves the second platform when the gateway runs inside WSL.
//
// The gateway in WSL and an agent installed on Windows keep two independent
// configs. They are two entries in the dashboard, never one: enabling
// "OpenCode (Windows)" must not touch the Linux file and vice versa. All
// Windows-side adapters resolve their profile through /mnt/c, and each looks
// for its own marker so only genuinely installed agents appear.

// windowsProfileHolding returns the first Windows user profile under /mnt/c/Users
// that contains rel (a path relative to the profile directory), or "" when
// none does. System profiles are skipped; multiple human profiles would need
// distinct ids, so the first is reported — the common desktop case is one.
func windowsProfileHolding(rel ...string) string {
	const base = "/mnt/c/Users"
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		switch lower(e.Name()) {
		case "public", "default", "default user", "all users":
			continue
		}
		profile := filepath.Join(base, e.Name())
		if _, err := os.Stat(filepath.Join(append([]string{profile}, rel...)...)); err == nil {
			return profile
		}
	}
	return ""
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// windowsVariant builds the Windows-side adapter when this process runs in WSL
// and the marker path exists in some Windows profile; nil otherwise.
func windowsVariant(rel []string, id, name string, mk func(home, id, name string) Agent) Agent {
	if !isWSL() {
		return nil
	}
	home := windowsProfileHolding(rel...)
	if home == "" {
		return nil
	}
	return mk(home, id, name)
}

// hostLabel disambiguates the Linux-side adapter when a Windows entry sits
// next to it, so the dashboard never shows two plain "OpenCode" cards.
func hostLabel(name string) string {
	if isWSL() {
		return name + " (WSL)"
	}
	return name
}
