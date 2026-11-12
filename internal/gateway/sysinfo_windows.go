//go:build windows

package gateway

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// fillPlatform resolves the native Windows host. The Windows build is the
// only one that can replace itself in place, so showing the exact build and
// process identity here is also useful when diagnosing an update.
func fillPlatform(r *SystemReport) {
	r.Platform = "windows"
	v := windows.RtlGetVersion()
	build := uint64(v.BuildNumber)
	r.Kernel = fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	product, display := windowsProduct()
	if product == "" {
		product = "Windows"
	}
	if build >= 22000 {
		product = strings.Replace(product, "Windows 10", "Windows 11", 1)
	}
	r.OS = product
	parts := []string{}
	if display != "" {
		parts = append(parts, display)
	}
	parts = append(parts, "build "+strconv.FormatUint(build, 10))
	r.OSVersion = strings.Join(parts, " · ")
	r.CPU = windowsCPU()
	r.CPUThreads = runtime.NumCPU()
	r.MemoryTotal, r.MemoryAvailable = windowsMemory()
}

func windowsProduct() (product, display string) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	defer key.Close()
	product, _, _ = key.GetStringValue("ProductName")
	display, _, _ = key.GetStringValue("DisplayVersion")
	return strings.TrimSpace(product), strings.TrimSpace(display)
}

func windowsCPU() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return strings.TrimSpace(os.Getenv("PROCESSOR_IDENTIFIER"))
	}
	defer key.Close()
	value, _, err := key.GetStringValue("ProcessorNameString")
	if err != nil {
		return strings.TrimSpace(os.Getenv("PROCESSOR_IDENTIFIER"))
	}
	return strings.TrimSpace(value)
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

func windowsMemory() (total, available uint64) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	ok, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if ok == 0 {
		return 0, 0
	}
	return m.TotalPhys, m.AvailPhys
}
