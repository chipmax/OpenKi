package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// openCode injects a provider block into opencode.json
// (npm: @ai-sdk/openai-compatible). Both the CLI and the desktop app read it.

type openCode struct {
	home string // Windows profile when injected through /mnt/c; "" = host home
	id   string
	name string
}

func newOpenCode() *openCode { return newOpenCodeAt("", "opencode", "OpenCode") }

func newOpenCodeAt(home, id, name string) *openCode {
	return &openCode{home: home, id: id, name: name}
}

func (o *openCode) p(parts ...string) string {
	if o.home == "" {
		return homePath(parts...)
	}
	return filepath.Join(append([]string{o.home}, parts...)...)
}

func (o *openCode) Meta() (string, string, string) {
	return o.id, o.name, o.p(".config", "opencode", "opencode.json") + ": tiêm provider @ai-sdk/openai-compatible"
}

func (o *openCode) configPath() string {
	if o.home == "" {
		if p := os.Getenv("OPENCODE_CONFIG"); p != "" {
			return p
		}
	}
	return o.p(".config", "opencode", "opencode.json")
}

// defaultJSONC names the commented-config sibling OpenCode also reads. Only
// the default path is at risk: OPENCODE_CONFIG is the user naming the exact
// file to write, so no second config can appear beside it.
func (o *openCode) defaultJSONC() string {
	if o.home == "" && os.Getenv("OPENCODE_CONFIG") != "" {
		return ""
	}
	p := filepath.Join(filepath.Dir(o.configPath()), "opencode.jsonc")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

func (o *openCode) Detect() (bool, string, string) {
	if _, err := os.Stat(o.configPath()); err == nil {
		return true, "", "Đã phát hiện opencode.json"
	}
	if p := o.defaultJSONC(); p != "" {
		return true, "", "Đã phát hiện opencode.jsonc (cấu hình có chú thích); zen-gate sẽ không ghi thêm opencode.json song song, vui lòng kết nối thủ công"
	}
	if _, err := os.Stat(o.p(".config", "opencode")); err == nil {
		return true, "", "Đã phát hiện thư mục cấu hình opencode (chưa tạo opencode.json)"
	}
	return false, "", "Chưa phát hiện OpenCode"
}

func (o *openCode) IsEnabled() (bool, string, error) {
	doc, err := o.read()
	if err != nil {
		return false, "", nil
	}
	prov, _ := doc["provider"].(map[string]any)
	zg, ok := prov["zen-gate"].(map[string]any)
	if !ok {
		return false, "", nil
	}
	enabled := true
	if opts, ok := zg["options"].(map[string]any); ok {
		if d, ok := opts["disabled"].(bool); ok {
			enabled = !d
		}
	}
	return enabled, "", nil
}

func (o *openCode) read() (map[string]any, error) {
	data, err := os.ReadFile(o.configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	doc := map[string]any{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("Không phân tích được opencode.json (%w); nếu là JSONC vui lòng kết nối thủ công", err)
	}
	return doc, nil
}

func (o *openCode) Enable(opts Options) error {
	path := o.configPath()
	if p := o.defaultJSONC(); p != "" {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// OpenCode reads .jsonc too, so a plain .json written here becomes a
			// second live config with no documented winner — and the .jsonc is the
			// file the user maintains. Refuse rather than guess which one wins.
			return fmt.Errorf("Đã phát hiện %s: OpenCode sẽ đọc nó, zen-gate không tạo thêm opencode.json song song nữa; vui lòng tự ghi khối provider vào file .jsonc đó", p)
		}
	}
	doc, err := o.read()
	if err != nil {
		return err
	}
	prov, _ := doc["provider"].(map[string]any)
	if prov == nil {
		prov = map[string]any{}
	}
	models := map[string]any{}
	for _, m := range opts.Models {
		models[m.ID] = map[string]any{"name": m.Name}
	}
	prov["zen-gate"] = map[string]any{
		"name": "Zen Gate",
		"npm":  "@ai-sdk/openai-compatible",
		"options": map[string]any{
			"baseURL": opts.BaseURL,
			"apiKey":  opts.APIKey,
		},
		"models": models,
	}
	doc["provider"] = prov
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	backupBeforeWrite(o.id, path)
	return atomicWrite(path, data)
}

func (o *openCode) Disable() error {
	path := o.configPath()
	doc, err := o.read()
	if err != nil {
		return nil
	}
	prov, _ := doc["provider"].(map[string]any)
	if prov == nil {
		return nil
	}
	if zg, ok := prov["zen-gate"].(map[string]any); ok {
		if name, _ := zg["name"].(string); name != "Zen Gate" {
			return nil // not ours — leave it alone
		}
		delete(prov, "zen-gate")
		if len(prov) == 0 {
			delete(doc, "provider")
		}
	} else {
		return nil
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	backupBeforeWrite(o.id, path)
	return atomicWrite(path, data)
}
