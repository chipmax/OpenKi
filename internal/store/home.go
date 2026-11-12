package store

import (
	"os"
	"path/filepath"
	"runtime"
)

// AppDataDir returns the per-user application data directory of the host OS:
//
//	Windows %APPDATA%\loimo
//	macOS   ~/Library/Application Support/loimo
//	other   $XDG_CONFIG_HOME (or ~/.config)/loimo
//
// LOIMO_HOME overrides everything.
func AppDataDir() string {
	if h := os.Getenv("LOIMO_HOME"); h != "" {
		return h
	}
	switch runtime.GOOS {
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "loimo")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "AppData", "Roaming", "loimo")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "loimo")
		}
		return filepath.Join(home, "Library", "Application Support", "loimo")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "loimo")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "loimo")
	}
}
