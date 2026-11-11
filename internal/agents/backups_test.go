package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zen-gate/internal/lane"
)

// The backups directory is the only undo for an injection into a config
// zen-gate does not own. Two writes in the same clock second used to collide on
// one filename, so the second one overwrote the first restore point.
func TestBackupFileKeepsEveryCopyFromTheSameSecond(t *testing.T) {
	t.Setenv("ZEN_GATE_HOME", t.TempDir())
	first, err := backupFile("probe", filepath.Join("cfg", "agent.json"), []byte(`{"v":1}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := backupFile("probe", filepath.Join("cfg", "agent.json"), []byte(`{"v":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("both backups resolved to %s; the first copy was overwritten", first)
	}
	if got := readFile(t, first); got != `{"v":1}` {
		t.Errorf("first backup holds %q, want the original it was given", got)
	}
	if got := readFile(t, second); got != `{"v":2}` {
		t.Errorf("second backup holds %q", got)
	}
}

func TestLatestBackupPicksTheNewestCopy(t *testing.T) {
	t.Setenv("ZEN_GATE_HOME", t.TempDir())
	if _, err := backupFile("probe", filepath.Join("cfg", "agent.json"), []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	second, err := backupFile("probe", filepath.Join("cfg", "agent.json"), []byte(`{"v":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := latestBackup("probe", "agent.json"); got != second {
		t.Errorf("latestBackup = %q, want %q", got, second)
	}
}

// OpenCode's adapter rewrites a whole foreign document (providers, other
// vendors' entries), so a copy has to exist before the write — a schema
// zen-gate does not recognise should be recoverable.
func TestOpenCodeEnableBacksUpBeforeRewriting(t *testing.T) {
	dir := openCodeHome(t)
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		BaseURL: "http://127.0.0.1:8787/v1",
		APIKey:  "k",
		Models:  []lane.ModelInfo{{ID: "m1", Name: "m1", ContextWindow: 1000, MaxOutput: 100}},
	}
	if err := newOpenCode().Enable(opts); err != nil {
		t.Fatal(err)
	}
	if !hasBackupContaining(t, "opencode", `"theme":"dark"`) {
		t.Errorf("Enable rewrote %s without backing up the previous content", path)
	}
}

func hasBackupContaining(t *testing.T, agentID, substr string) bool {
	t.Helper()
	dir := filepath.Join(os.Getenv("ZEN_GATE_HOME"), "backups", agentID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.Contains(readFile(t, filepath.Join(dir, e.Name())), substr) {
			return true
		}
	}
	return false
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
