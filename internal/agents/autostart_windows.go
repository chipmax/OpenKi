//go:build windows

package agents

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	autostartValueName       = "Lối Mở"
	legacyAutostartValueName = "Zen Gate"
)

// AutostartEnabled reports whether the HKCU Run entry exists.
func AutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	for _, name := range []string{autostartValueName, legacyAutostartValueName} {
		if v, _, err := k.GetStringValue(name); err == nil && v != "" {
			return true
		}
	}
	return false
}

// AutostartSet writes or removes the HKCU Run entry pointing at the current
// executable. The pre-rename value is removed in both directions so an
// upgrade cannot leave two launch-at-login entries behind.
func AutostartSet(enable bool) error {
	if enable {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		k, err := registry.OpenKey(registry.CURRENT_USER,
			`Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		_ = k.DeleteValue(legacyAutostartValueName)
		return k.SetStringValue(autostartValueName, `"`+exe+`"`)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	_ = k.DeleteValue(autostartValueName)
	if err := k.DeleteValue(legacyAutostartValueName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}
