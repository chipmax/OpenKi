//go:build darwin

package gateway

import (
	"runtime"

	"golang.org/x/sys/unix"
)

func fillPlatform(r *SystemReport) {
	r.Platform = "darwin"
	r.OS = "macOS"
	r.OSVersion, _ = unix.Sysctl("kern.osproductversion")
	r.Kernel, _ = unix.Sysctl("kern.osrelease")
	r.CPU, _ = unix.Sysctl("machdep.cpu.brand_string")
	r.CPUThreads = runtime.NumCPU()
	if total, err := unix.SysctlUint64("hw.memsize"); err == nil {
		r.MemoryTotal = total
	}
	if pageSize, err := unix.SysctlUint64("hw.pagesize"); err == nil {
		if freePages, err := unix.SysctlUint64("vm.page_free_count"); err == nil {
			r.MemoryAvailable = pageSize * freePages
		}
	}
}
