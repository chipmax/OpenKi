//go:build headless

// cmd/loimo, headless build: this file only proves the GUI-free build even
// compiles. The behaviour lives in main.go, whose -no-tray / LOIMO_HEADLESS
// branch never touches the window package.
package main

// headlessBuild is read by main's startup log so a container operator can see
// at a glance whether the binary they pulled was the GUI-free one.
const headlessBuild = true
