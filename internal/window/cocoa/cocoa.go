//go:build darwin

// Package cocoa compiles the Objective-C window shim (lmwindow.m) into the
// binary. The shim lives in its own directory because the go tool rejects
// Objective-C sources in any package it builds without cgo — an .m file in
// internal/window itself would break the Windows build, which never enables
// cgo. Nothing here is called from Go directly: internal/window's darwin file
// includes lmwindow.h (declaring the same C surface) and links against the
// compiled symbols; the //export hooks live in window_darwin.go.
package cocoa

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include "lmwindow.h"
*/
import "C"

// touch keeps one C symbol referenced from Go so the linker cannot drop the
// translation unit before the internal/window calls are considered.
var _ = C.lm_noop_string(nil)
