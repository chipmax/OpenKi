//go:build linux

package gateway

import "testing"

func TestParseOSRelease(t *testing.T) {
	data := []byte(`NAME="Debian GNU/Linux"
VERSION_ID="13"
PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
`)
	name, version := parseOSRelease(data)
	if name != "Debian GNU/Linux 13 (trixie)" || version != "13" {
		t.Fatalf("parseOSRelease = %q, %q", name, version)
	}
}

func TestParseOSReleaseFallsBackToName(t *testing.T) {
	name, version := parseOSRelease([]byte("NAME=Alpine\nVERSION_ID=3.20\n"))
	if name != "Alpine 3.20" || version != "3.20" {
		t.Fatalf("fallback parseOSRelease = %q, %q", name, version)
	}
}

func TestParseMemInfoPrefersAvailable(t *testing.T) {
	total, available := parseMemInfo([]byte("MemTotal:       16384 kB\nMemFree: 1000 kB\nBuffers: 100 kB\nCached: 2000 kB\nMemAvailable: 8192 kB\n"))
	if total != 16384*1024 {
		t.Fatalf("total = %d", total)
	}
	if available != 8192*1024 {
		t.Fatalf("available = %d", available)
	}
}

func TestParseMemInfoFallsBackWhenAvailableMissing(t *testing.T) {
	_, available := parseMemInfo([]byte("MemTotal: 16384 kB\nMemFree: 1000 kB\nBuffers: 100 kB\nCached: 2000 kB\n"))
	if want := uint64((1000 + 100 + 2000) * 1024); available != want {
		t.Fatalf("available = %d, want %d", available, want)
	}
}
