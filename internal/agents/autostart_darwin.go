//go:build darwin

package agents

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chipmax/loimo/internal/store"
)

// Autostart on macOS is a per-user launchd job: a plist in
// ~/Library/LaunchAgents with RunAtLoad. That is the same scope as the
// HKCU\...\Run entry used on Windows — it starts the app when the user logs
// in, without asking for admin rights.

// launchdLabel is the reverse-DNS job label; it doubles as the plist name.
const launchdLabel = "io.github.chipmax.loimo"

// legacyLaunchdLabel is the label written before the project rename.
const legacyLaunchdLabel = "com.lagcomcom.zen-gate"

func launchdPlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func legacyLaunchdPlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", legacyLaunchdLabel+".plist")
}

// launchdDomain is the per-user launchd domain (`launchctl bootstrap`'s
// domain-target) and launchdServiceTarget the same job addressed by label
// (`launchctl print`/`bootout` take a service-target instead).
func launchdDomain() string { return "gui/" + strconv.Itoa(os.Getuid()) }
func launchdServiceTarget() string {
	return launchdServiceTargetFor(launchdLabel)
}
func launchdServiceTargetFor(label string) string {
	return launchdDomain() + "/" + label
}

// AutostartEnabled reports whether either the current or pre-rename launchd
// job is installed.
func AutostartEnabled() bool {
	for _, p := range []string{launchdPlistPath(), legacyLaunchdPlistPath()} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// AutostartSet installs or removes the launchd job.
func AutostartSet(enable bool) error {
	plist := launchdPlistPath()
	if plist == "" {
		return fmt.Errorf("cannot resolve the user's home directory")
	}
	if !enable {
		// bootout is a no-op when the job is not loaded; ignore the error so a
		// stale plist can still be cleaned up.
		_ = exec.Command("launchctl", "bootout", launchdServiceTarget()).Run()
		_ = exec.Command("launchctl", "bootout", launchdServiceTargetFor(legacyLaunchdLabel)).Run()
		for _, p := range []string{plist, legacyLaunchdPlistPath()} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}

	// A pre-rename job would otherwise keep launching the old binary after
	// login alongside the new one.
	_ = exec.Command("launchctl", "bootout", launchdServiceTargetFor(legacyLaunchdLabel)).Run()
	_ = os.Remove(legacyLaunchdPlistPath())

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>ProcessType</key>
    <string>Interactive</string>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, launchdLabel, plistXMLEscape(exe), logPath("loimo-launchd.out"), logPath("loimo-launchd.err"))
	tmp := plist + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, plist); err != nil {
		return err
	}

	// Re-bootstrap so a toggle takes effect without the next login. bootout
	// first: bootstrap refuses a domain that already holds the label.
	_ = exec.Command("launchctl", "bootout", launchdServiceTarget()).Run()
	if out, err := exec.Command("launchctl", "bootstrap", launchdDomain(), plist).CombinedOutput(); err != nil {
		// Older macOS (and any launchd hiccup) still has the plist on disk,
		// which is what AutostartEnabled reports — surface the log anyway.
		return fmt.Errorf("launchctl bootstrap: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func plistXMLEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// logPath points launchd's stdout/stderr at the loimo data dir so a
// launch-at-login instance is debuggable without a terminal.
func logPath(name string) string { return filepath.Join(store.Home(), name) }
