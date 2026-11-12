//go:build linux

package agents

import (
	"fmt"
	"os"
	"path/filepath"
)

const autostartDir = ".config/autostart"

// autostartFile is the XDG autostart desktop entry this app manages.
func autostartFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, autostartDir, "loimo.desktop")
}

// legacyAutostartFile is the entry written before the project rename.
func legacyAutostartFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, autostartDir, "zen-gate.desktop")
}

// execPath points at the running app. When launched from an AppImage the
// mount-point binary vanishes on exit, so the persistent autostart entry must
// reference the AppImage itself.
func execPath() string {
	if img := os.Getenv("APPIMAGE"); img != "" {
		return img
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

// AutostartEnabled reports whether either the current or pre-rename XDG
// autostart entry exists.
func AutostartEnabled() bool {
	for _, p := range []string{autostartFile(), legacyAutostartFile()} {
		if p != "" && autostartFileExists(p) {
			return true
		}
	}
	return false
}

// AutostartSet installs or removes the XDG autostart desktop entry.
func AutostartSet(enable bool) error {
	f := autostartFile()
	if f == "" {
		return fmt.Errorf("no autostart dir available")
	}
	if !enable {
		for _, p := range []string{f, legacyAutostartFile()} {
			if p != "" && autostartFileExists(p) {
				if err := os.Remove(p); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if old := legacyAutostartFile(); old != "" {
		_ = os.Remove(old)
	}
	exe := execPath()
	if exe == "" {
		return fmt.Errorf("cannot resolve current executable for autostart")
	}
	content := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Lối Mở\n" +
		"Comment=Cổng mô hình miễn phí cục bộ\n" +
		"Exec=\"" + exe + "\"\n" +
		"Icon=loimo\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f, []byte(content), 0o644)
}

func autostartFileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
