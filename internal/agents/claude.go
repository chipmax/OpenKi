package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// claude swaps the env block in ~/.claude/settings.json (ANTHROPIC_BASE_URL,
// ANTHROPIC_AUTH_TOKEN and the model selectors) to point Claude Code at the
// local gateway. The whole file is backed up before the first write and the
// pre-injection values are restored verbatim on disable.

type claude struct {
	home string // Windows profile when injected through /mnt/c; "" = host home
	id   string
	name string
}

func newClaude() *claude { return newClaudeAt("", "claude", "Claude Code") }

func newClaudeAt(home, id, name string) *claude {
	return &claude{home: home, id: id, name: name}
}

func (c *claude) p(parts ...string) string {
	if c.home == "" {
		return homePath(parts...)
	}
	return filepath.Join(append([]string{c.home}, parts...)...)
}

func (c *claude) Meta() (string, string, string) {
	return c.id, c.name, c.p(".claude", "settings.json") + ": chuyển env sang cổng cục bộ (giá trị gốc được sao lưu và khôi phục tự động)"
}

func (c *claude) settingsPath() string { return c.p(".claude", "settings.json") }

func (c *claude) Detect() (bool, string, string) {
	if _, err := os.Stat(c.settingsPath()); err == nil {
		return true, "", "Đã phát hiện settings.json"
	}
	if _, err := os.Stat(c.p(".claude")); err == nil {
		return true, "", "Đã phát hiện " + c.p(".claude") + " (chưa tạo settings.json)"
	}
	return false, "", "Chưa phát hiện Claude Code"
}

func (c *claude) IsEnabled() (bool, string, error) {
	doc, err := c.read()
	if err != nil {
		return false, "", nil
	}
	env, _ := doc["env"].(map[string]any)
	base, _ := env["ANTHROPIC_BASE_URL"].(string)
	return strings.Contains(base, "127.0.0.1"), "", nil
}

func (c *claude) read() (map[string]any, error) {
	data, err := os.ReadFile(c.settingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	doc := map[string]any{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("Không phân tích được settings.json (%w); loimo từ chối ghi", err)
	}
	return doc, nil
}

func (c *claude) Enable(o Options) error {
	path := c.settingsPath()
	if data, err := os.ReadFile(path); err == nil {
		// back up the pristine file once per enable
		_, _ = backupFile(c.id, path, data)
	}
	doc, err := c.read()
	if err != nil {
		return err
	}
	env, _ := doc["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	// The Anthropic SDK appends /v1/messages itself, so advertise the origin.
	root := strings.TrimSuffix(o.BaseURL, "/v1")
	env["ANTHROPIC_BASE_URL"] = root
	env["ANTHROPIC_AUTH_TOKEN"] = o.APIKey
	env["ANTHROPIC_MODEL"] = o.DefaultModel
	env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = o.DefaultModel
	env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = o.DefaultModel
	env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = o.DefaultModel
	doc["env"] = env
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	// A fresh install has no ~/.claude yet; Detect only reports "detected"
	// from the directory, so Enable is routinely the first writer.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func (c *claude) Disable() error {
	path := c.settingsPath()
	if b := latestBackup(c.id, "settings.json"); b != "" {
		data, err := os.ReadFile(b)
		if err == nil {
			return atomicWrite(path, data)
		}
	}
	// No backup: strip our own values only.
	doc, err := c.read()
	if err != nil {
		return nil
	}
	if env, ok := doc["env"].(map[string]any); ok {
		if base, _ := env["ANTHROPIC_BASE_URL"].(string); strings.Contains(base, "127.0.0.1") {
			delete(env, "ANTHROPIC_BASE_URL")
			delete(env, "ANTHROPIC_AUTH_TOKEN")
			delete(env, "ANTHROPIC_MODEL")
		}
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}
