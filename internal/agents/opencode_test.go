package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openCodeHome(t *testing.T) string {
	t.Helper()
	home := fakeAgentHome(t)
	dir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// OpenCode reads either opencode.json or opencode.jsonc, and its user's file is
// often the .jsonc one because it tolerates comments. Writing a second, plain
// .json next to it puts two live configs in one directory with no documented
// winner — the adapter would quietly shadow the file the user actually
// maintains. So openki refuses instead of guessing.
func TestOpenCodeRefusesToShadowAJSONCConfig(t *testing.T) {
	dir := openCodeHome(t)
	jsonc := filepath.Join(dir, "opencode.jsonc")
	if err := os.WriteFile(jsonc, []byte("{ // my providers\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := newOpenCode()
	err := o.Enable(testOptions())
	if err == nil {
		t.Fatalf("Enable wrote a shadowing opencode.json next to the user's opencode.jsonc")
	}
	if !strings.Contains(err.Error(), "jsonc") {
		t.Errorf("error = %q, want it to name the file in the way", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "opencode.json")); statErr == nil {
		t.Errorf("opencode.json was created anyway")
	}
}

// An explicit OPENCODE_CONFIG is the user telling openki where to write, so
// the sibling check does not apply to it.
func TestOpenCodeHonoursExplicitConfigPath(t *testing.T) {
	dir := openCodeHome(t)
	if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "elsewhere.json")
	t.Setenv("OPENCODE_CONFIG", target)
	if err := newOpenCode().Enable(testOptions()); err != nil {
		t.Fatalf("Enable with an explicit path: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("the explicit path was not written: %v", err)
	}
}

// Both files already coexisting is the user's own arrangement: the .json is
// already a live config, so updating it shadows nothing new. Guards against an
// over-broad refusal.
func TestOpenCodeUpdatesAnExistingJSONBesideJSONC(t *testing.T) {
	dir := openCodeHome(t)
	if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := newOpenCode().Enable(testOptions()); err != nil {
		t.Fatalf("Enable on an existing opencode.json: %v", err)
	}
	doc := readJSON(t, path)
	prov, _ := doc["provider"].(map[string]any)
	if _, ok := prov["openki"].(map[string]any); !ok {
		t.Errorf("provider.openki missing: %v", doc)
	}
}

func TestOpenCodeDetectsWithoutJSONC(t *testing.T) {
	dir := openCodeHome(t)
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := newOpenCode().Enable(testOptions()); err != nil {
		t.Fatalf("Enable on a plain opencode.json: %v", err)
	}
}

func TestOpenCodeEnableMigratesLegacyProvider(t *testing.T) {
	dir := openCodeHome(t)
	path := filepath.Join(dir, "opencode.json")
	body := `{
  "provider": {
    "zen-gate": {"name": "Zen Gate", "npm": "@ai-sdk/openai-compatible"},
    "custom": {"name": "Custom", "npm": "@ai-sdk/openai-compatible"}
  }
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	o := newOpenCode()
	if !o.LegacyEnabled() {
		t.Fatal("LegacyEnabled missed the pre-rename provider")
	}
	if err := o.Enable(testOptions()); err != nil {
		t.Fatal(err)
	}
	doc := readJSON(t, path)
	prov, _ := doc["provider"].(map[string]any)
	if _, ok := prov["zen-gate"]; ok {
		t.Fatalf("legacy provider survived migration: %v", prov)
	}
	if _, ok := prov["openki"]; !ok {
		t.Fatalf("current provider missing after migration: %v", prov)
	}
	if _, ok := prov["custom"]; !ok {
		t.Fatalf("unrelated provider lost: %v", prov)
	}
	if err := o.Disable(); err != nil {
		t.Fatal(err)
	}
	doc = readJSON(t, path)
	prov, _ = doc["provider"].(map[string]any)
	if _, ok := prov["openki"]; ok {
		t.Fatalf("Disable left the current provider behind: %v", prov)
	}
	if _, ok := prov["custom"]; !ok {
		t.Fatalf("Disable removed the user's provider: %v", prov)
	}
}
