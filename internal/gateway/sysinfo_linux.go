//go:build linux

package gateway

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// fillPlatform resolves Linux and WSL. WSL is reported separately because the
// kernel string alone hides the fact that the process is running on a Windows
// host: the dashboard would otherwise show only "Linux" and the operator
// could not tell which side of the machine the gateway belongs to.
func fillPlatform(r *SystemReport) {
	r.Platform = "linux"
	if wslDistro := os.Getenv("WSL_DISTRO_NAME"); wslDistro != "" || procVersionIsWSL() {
		r.Platform = "wsl"
		r.WSL = true
		r.WSLDistro = wslDistro
	}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		r.OS, r.OSVersion = parseOSRelease(data)
	}
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		r.Kernel = strings.TrimSpace(string(data))
	}
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		r.CPU = parseCPUInfo(data)
	}
	r.CPUThreads = runtime.NumCPU()
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		r.MemoryTotal, r.MemoryAvailable = parseMemInfo(data)
	}
}

func procVersionIsWSL() bool {
	data, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// parseOSRelease returns PRETTY_NAME and VERSION_ID. Quoted values are the
// normal form, but a hand-edited file may leave either side unquoted.
func parseOSRelease(data []byte) (name, version string) {
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[key] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	name = values["PRETTY_NAME"]
	if name == "" {
		name = strings.TrimSpace(values["NAME"] + " " + values["VERSION_ID"])
	}
	return name, values["VERSION_ID"]
}

func parseCPUInfo(data []byte) string {
	var fallback string
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "model name", "Processor", "Hardware":
			if value != "" {
				return value
			}
		case "cpu model":
			if fallback == "" {
				fallback = value
			}
		}
	}
	return fallback
}

// parseMemInfo returns bytes. MemAvailable is the kernel's estimate of what a
// new workload can use; older kernels without it fall back to free+buffers+
// cached, which is the same quantity before that field existed.
func parseMemInfo(data []byte) (total, available uint64) {
	values := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		values[strings.TrimSpace(key)] = n
	}
	total = values["MemTotal"] * 1024
	available = values["MemAvailable"] * 1024
	if available == 0 {
		available = (values["MemFree"] + values["Buffers"] + values["Cached"]) * 1024
	}
	return total, available
}
