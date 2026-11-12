//go:build !linux && !windows && !darwin

package gateway

import "runtime"

// fillPlatform keeps the dashboard useful on the remaining GOOS values
// (FreeBSD and friends): the standard library still provides the essentials.
func fillPlatform(r *SystemReport) {
	r.Platform = runtime.GOOS
	r.OS = runtime.GOOS
	r.CPUThreads = runtime.NumCPU()
}
