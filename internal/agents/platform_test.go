package agents

import (
	"runtime"
	"testing"
)

func TestAgentPlatformKeepsWindowsVariantsDistinct(t *testing.T) {
	if got := agentPlatform("opencode-windows"); got != "windows" {
		t.Fatalf("agentPlatform(opencode-windows) = %q, want windows", got)
	}
	host := "linux"
	if isWSL() {
		host = "wsl"
	}
	if runtime.GOOS == "windows" {
		host = "windows"
	}
	if got := agentPlatform("opencode"); got != host {
		t.Fatalf("agentPlatform(opencode) = %q, want %q", got, host)
	}
}
