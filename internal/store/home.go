package store

import (
	"os"
	"path/filepath"
	"runtime"
)

// AppDataDir returns the per-user application data directory of the host OS:
//
//	Windows %APPDATA%\openki
//	macOS   ~/Library/Application Support/openki
//	other   $XDG_CONFIG_HOME (or ~/.config)/openki
//
// OPENKI_HOME overrides everything.
func AppDataDir() string {
	if h := os.Getenv("OPENKI_HOME"); h != "" {
		return h
	}
	switch runtime.GOOS {
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "openki")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "AppData", "Roaming", "openki")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "openki")
		}
		return filepath.Join(home, "Library", "Application Support", "openki")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "openki")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "openki")
	}
}
