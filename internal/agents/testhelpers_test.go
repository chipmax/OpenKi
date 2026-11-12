package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chipmax/openki/internal/lane"
)

// fakeAgentHome points os.UserHomeDir at a temp tree so an adapter writes into
// the test, not into the user's real agent config.
func fakeAgentHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("OPENKI_HOME", filepath.Join(home, "data"))
	return home
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("written file is not valid JSON: %v", err)
	}
	return doc
}

func testOptions() Options {
	return Options{
		BaseURL: "http://127.0.0.1:8787/v1",
		APIKey:  "k",
		Models: []lane.ModelInfo{
			{ID: "a-free", Name: "A", ContextWindow: 131072, MaxOutput: 65536, Reasoning: true},
			{ID: "vision-free", Name: "Vision", ContextWindow: 262144, MaxOutput: 32768, Vision: true},
		},
	}
}
