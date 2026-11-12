package agents

import (
	"runtime"
	"testing"
)

func TestAgentPlatformKeepsWindowsVariantsDistinct(t *testing.T) {
	if got := agentPlatform("opencode-windows"); got != "windows" {
		t.Fatalf("agentPlatform(opencode-windows) = %q, want windows", got)
	}
	host := runtime.GOOS
	if isWSL() {
		host = "wsl"
	}
	if got := agentPlatform("opencode"); got != host {
		t.Fatalf("agentPlatform(opencode) = %q, want %q", got, host)
	}
}
