package store

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestAppDataDirHonoursOverride(t *testing.T) {
	t.Setenv("OPENKI_HOME", "/tmp/zg-override")
	if got := AppDataDir(); got != "/tmp/zg-override" {
		t.Fatalf("OPENKI_HOME ignored: got %s", got)
	}
}

func TestAppDataDirIsPerPlatform(t *testing.T) {
	t.Setenv("OPENKI_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	got := AppDataDir()
	if filepath.Base(got) != "openki" {
		t.Fatalf("data dir must live in a openki folder, got %s", got)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("data dir must be absolute, got %s", got)
	}
	switch runtime.GOOS {
	case "windows":
		// %APPDATA% wins on Windows.
		if want := filepath.Join(home, "AppData", "Roaming", "openki"); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	case "darwin":
		// macOS keeps app state in ~/Library/Application Support, never in the
		// Windows-shaped AppData path the port inherited.
		if want := filepath.Join(home, "Library", "Application Support", "openki"); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	}
}

// The macOS build must not read APPDATA (Windows-only) and must not fall back
// to AppData/Roaming, which does not exist on this platform.
func TestAppDataDirIgnoresWindowsOnlyVars(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-only invariant")
	}
	home := t.TempDir()
	t.Setenv("OPENKI_HOME", "")
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	got := AppDataDir()
	if filepath.Dir(got) != filepath.Join(home, "Library", "Application Support") {
		t.Fatalf("APPDATA leaked into the macOS data dir: %s", got)
	}
}
