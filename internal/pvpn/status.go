// Package pvpn reads the local pVPN helper's connection status.
//
// The helper owns the Proton account, credentials and tunnel lifecycle; OpenKi
// only asks its CLI for the same status a user sees in a terminal. Nothing in
// this package can connect, disconnect or log in, and credentials are never
// read. Every command is a fixed argv (no shell), so a status value cannot
// become an argument.
package pvpn

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DefaultPath is where the Debian helper installed on this machine lives. The
// binary is root-owned and executable by the gateway's unprivileged user.
const DefaultPath = "/opt/pvpn/pvpnctl"

// statusTTL bounds how often the dashboard can fork the helper. Admin state is
// polled every ten seconds; a shorter TTL would spawn a process per poll for a
// value that only changes when the user acts.
const statusTTL = 15 * time.Second

// errorTTL is deliberately shorter: a missing/broken helper should recover
// as soon as the user installs or fixes it.
const errorTTL = 5 * time.Second

// Status is the sanitized output of `pvpnctl status`.
type Status struct {
	Available bool   `json:"available"`
	Connected bool   `json:"connected"`
	Server    string `json:"server,omitempty"`
	Country   string `json:"country,omitempty"`
	IP        string `json:"ip,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Upload    string `json:"upload,omitempty"`
	Download  string `json:"download,omitempty"`
	// CheckedAt is when this sample was read, unix milliseconds. The dashboard
	// shows it so a green "Connected" is never mistaken for a live probe.
	CheckedAt int64  `json:"checkedAt,omitempty"`
	Error     string `json:"error,omitempty"`
	// ControlEnabled stays false: switching the tunnel needs root and is
	// deliberately outside the gateway's privilege boundary.
	ControlEnabled bool `json:"controlEnabled"`
}

var cache struct {
	sync.Mutex
	at time.Time
	st Status
}

// Locate returns the configured helper path. OPENKI_PVPNCTL exists for
// non-Debian layouts and tests; an empty result means the helper is absent.
func Locate() string {
	if p := strings.TrimSpace(os.Getenv("OPENKI_PVPNCTL")); p != "" {
		if isExecutable(p) {
			return p
		}
		return ""
	}
	if isExecutable(DefaultPath) {
		return DefaultPath
	}
	if p, err := exec.LookPath("pvpnctl"); err == nil {
		return p
	}
	return ""
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// Read returns a cached status when possible. It never blocks longer than the
// context deadline; a hung helper degrades to an unavailable status instead of
// stalling the whole admin state response.
func Read(ctx context.Context) Status {
	cache.Lock()
	defer cache.Unlock()
	ttl := statusTTL
	if !cache.st.Available || cache.st.Error != "" {
		ttl = errorTTL
	}
	if !cache.at.IsZero() && time.Since(cache.at) < ttl {
		return cache.st
	}
	st := readUncached(ctx)
	cache.st = st
	cache.at = time.Now()
	return st
}

// ReadFresh bypasses the cache. It is exported for tests and for a future
// explicit refresh control; the dashboard's periodic poll uses Read.
func ReadFresh(ctx context.Context) Status {
	cache.Lock()
	defer cache.Unlock()
	st := readUncached(ctx)
	cache.st = st
	cache.at = time.Now()
	return st
}

func readUncached(ctx context.Context) Status {
	now := time.Now().UnixMilli()
	path := Locate()
	if path == "" {
		return Status{CheckedAt: now, Error: "không tìm thấy pvpnctl"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, path, "status")
	// The helper prints English labels. Pin the locale so a future translated
	// build cannot silently make every field look empty.
	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(err.Error())
		if runCtx.Err() == context.DeadlineExceeded {
			msg = "pvpnctl quá thời gian chờ"
		}
		return Status{Available: true, CheckedAt: now, Error: msg}
	}
	st := ParseStatus(string(out))
	st.Available = true
	st.CheckedAt = now
	return st
}

// ParseStatus parses `Key: value` lines. Unknown keys are ignored so a helper
// upgrade cannot break the panel; a missing Status line is an error rather
// than a guessed "connected".
func ParseStatus(out string) Status {
	st := Status{}
	seenStatus := false
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "status":
			seenStatus = true
			// "disconnected" contains "connected"; compare the whole token.
			switch strings.ToLower(value) {
			case "connected", "connecting", "reconnecting":
				st.Connected = strings.EqualFold(value, "connected")
			default:
				st.Connected = false
			}
		case "server":
			st.Server = value
		case "country":
			st.Country = value
		case "ip":
			st.IP = value
		case "protocol":
			st.Protocol = value
		case "duration":
			st.Duration = value
		case "upload":
			st.Upload = value
		case "download":
			st.Download = value
		}
	}
	if !seenStatus {
		st.Error = "đầu ra pvpnctl không có trường Status"
	}
	return st
}

// DefaultBinaryName is used by docs/tests to describe the expected helper.
const DefaultBinaryName = "pvpnctl"
