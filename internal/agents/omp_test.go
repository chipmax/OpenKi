package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func ompFixture(t *testing.T, content string) (root, path string) {
	t.Helper()
	t.Setenv("OPENKI_HOME", t.TempDir())
	root = t.TempDir()
	dir := filepath.Join(root, "agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, "models.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func readYAMLMap(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("written file is not valid YAML: %v", err)
	}
	return doc
}

const ompFixtureYAML = `providers:
  other:
    baseUrl: http://example.invalid/v1
    api: openai-completions
    apiKey: other-key
    models:
      - id: keep-me
        name: keep-me
`

func TestOMPEnableAddsProviderAndPreservesOthers(t *testing.T) {
	root, path := ompFixture(t, ompFixtureYAML)
	o := newOMP(root, "omp", "omp")
	if err := o.Enable(testOptions()); err != nil {
		t.Fatal(err)
	}
	enabled, _, err := o.IsEnabled()
	if err != nil || !enabled {
		t.Fatalf("IsEnabled after Enable = %v, %v", enabled, err)
	}
	doc := readYAMLMap(t, path)
	providers, _ := doc["providers"].(map[string]any)
	if _, ok := providers["other"].(map[string]any); !ok {
		t.Fatalf("the pre-existing provider was dropped: %v", providers)
	}
	zg, _ := providers["openki"].(map[string]any)
	if zg == nil {
		t.Fatalf("providers.openki missing: %v", providers)
	}
	if got, _ := zg["baseUrl"].(string); got != "http://127.0.0.1:8787/v1" {
		t.Errorf("baseUrl = %q", got)
	}
	models, _ := zg["models"].([]any)
	if len(models) != 2 {
		t.Fatalf("models = %d, want 2", len(models))
	}
	vision, _ := models[1].(map[string]any)
	input, _ := vision["input"].([]any)
	if len(input) != 2 || input[1] != "image" {
		t.Errorf("vision model input = %v, want text+image", input)
	}
}

func TestOMPDisableRemovesOnlyItsProvider(t *testing.T) {
	root, path := ompFixture(t, ompFixtureYAML)
	o := newOMP(root, "omp", "omp")
	if err := o.Enable(testOptions()); err != nil {
		t.Fatal(err)
	}
	if err := o.Disable(); err != nil {
		t.Fatal(err)
	}
	if enabled, _, _ := o.IsEnabled(); enabled {
		t.Fatal("provider still present after Disable")
	}
	doc := readYAMLMap(t, path)
	providers, _ := doc["providers"].(map[string]any)
	if _, ok := providers["other"].(map[string]any); !ok {
		t.Fatalf("Disable removed the user's own provider: %v", providers)
	}
}

// models.yml is the user's file; an unknown shape must be refused, not
// flattened into a openki-only document.
func TestOMPEnableRefusesAnUnparseableFile(t *testing.T) {
	broken := "providers: [unclosed\n"
	root, path := ompFixture(t, broken)
	if err := newOMP(root, "omp", "omp").Enable(testOptions()); err == nil {
		t.Fatal("Enable reported success on a file it could not parse")
	}
	if got := readFile(t, path); got != broken {
		t.Errorf("the unreadable file was rewritten:\n%s", got)
	}
}

func TestOMPDetectRequiresModelsYAML(t *testing.T) {
	root := t.TempDir()
	o := newOMP(root, "omp", "omp")
	installed, _, _ := o.Detect()
	if installed {
		t.Fatal("Detect reported omp installed without a models.yml")
	}
	if err := os.MkdirAll(filepath.Join(root, "agent"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agent", "models.yml"), []byte("providers: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if installed, _, _ := o.Detect(); !installed {
		t.Fatal("Detect missed an existing models.yml")
	}
}

func TestOMPMetaNamesThePlatformSide(t *testing.T) {
	windows := newOMP("C:/x", "omp-windows", "omp (Windows)")
	id, name, detail := windows.Meta()
	if id != "omp-windows" || name != "omp (Windows)" || !strings.Contains(detail, "models.yml") {
		t.Errorf("Meta = %q, %q, %q", id, name, detail)
	}
}

func TestOMPEnableMigratesLegacyProvider(t *testing.T) {
	root, path := ompFixture(t, `providers:
  zen-gate:
    baseUrl: http://127.0.0.1:8787/v1
    api: openai-completions
    apiKey: old-key
    models:
      - id: old-model
        name: old-model
  other:
    baseUrl: http://example.invalid/v1
    api: openai-completions
    apiKey: other-key
    models:
      - id: keep-me
        name: keep-me
`)
	o := newOMP(root, "omp", "omp")
	if !o.LegacyEnabled() {
		t.Fatal("LegacyEnabled missed the pre-rename provider")
	}
	if err := o.Enable(testOptions()); err != nil {
		t.Fatal(err)
	}
	doc := readYAMLMap(t, path)
	providers, _ := doc["providers"].(map[string]any)
	if _, ok := providers["zen-gate"]; ok {
		t.Fatalf("legacy provider survived migration: %v", providers)
	}
	if _, ok := providers["openki"]; !ok {
		t.Fatalf("current provider missing after migration: %v", providers)
	}
	if _, ok := providers["other"]; !ok {
		t.Fatalf("unrelated provider lost: %v", providers)
	}
}
