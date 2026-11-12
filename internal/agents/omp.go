package agents

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// omp (the omp coding agent) reads ~/.omp/agent/models.yml: a providers map
// keyed by provider id, each carrying baseUrl/api/apiKey and a model list.
// loimo injects one provider named "loimo" and leaves every other
// provider untouched.
//
// Two installs can coexist: the gateway may run inside WSL while omp is also
// installed on Windows. Hence a second adapter ("omp-windows") that resolves
// the Windows profile through /mnt/c, so the Windows-side models.yml is a
// separate, explicitly toggleable entry rather than being silently mixed with
// the Linux one.

const ompProviderID = "loimo"

// legacyOMPProviderID is the provider key written by releases before the
// project was renamed. Enable replaces it; Disable also removes it so a
// migration cannot leave a second, stale provider in the picker.
const legacyOMPProviderID = "zen-gate"

type omp struct {
	root string
	id   string
	name string
}

func newOMP(root, id, name string) *omp {
	return &omp{root: root, id: id, name: name}
}

// ompAdapters returns the omp adapters that exist on this host: the host's own
// ~/.omp, plus — when running inside WSL — the first Windows profile that has
// one.
func ompAdapters() []Agent {
	out := []Agent{newOMP(homePath(".omp"), "omp", hostLabel("omp"))}
	if isWSL() {
		if root := windowsProfileHolding(".omp", "agent", "models.yml"); root != "" {
			out = append(out, newOMP(filepath.Join(root, ".omp"), "omp-windows", "omp (Windows)"))
		}
	}
	return out
}

// isWSL reports whether this Linux build runs under the Windows Subsystem for
// Linux. WSL_DISTRO_NAME is set by WSL itself; /proc/version is the fallback
// for stripped environments.
func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	b, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func (o *omp) Meta() (string, string, string) {
	return o.id, o.name, "models.yml: tiêm nhà cung cấp loimo (OpenAI-compatible)"
}

func (o *omp) modelsPath() string {
	return filepath.Join(o.root, "agent", "models.yml")
}

func (o *omp) Detect() (bool, string, string) {
	if fileExists(o.modelsPath()) {
		return true, "", "Đã phát hiện " + o.modelsPath()
	}
	return false, "", "Chưa phát hiện omp (" + o.root + ")"
}

func (o *omp) IsEnabled() (bool, string, error) {
	doc, err := readOMPDoc(o.modelsPath())
	if err != nil || doc == nil {
		return false, "", nil
	}
	providers, err := ompProviders(doc, false)
	if err != nil || providers == nil {
		return false, "", nil
	}
	return ompProviderPair(providers) >= 0, "", nil
}

func (o *omp) LegacyEnabled() bool {
	doc, err := readOMPDoc(o.modelsPath())
	if err != nil || doc == nil {
		return false
	}
	providers, err := ompProviders(doc, false)
	if err != nil || providers == nil {
		return false
	}
	return ompProviderPairKey(providers, legacyOMPProviderID) >= 0
}

func (o *omp) Enable(opts Options) error {
	path := o.modelsPath()
	doc, err := readOMPDoc(path)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	providers, err := ompProviders(doc, true)
	if err != nil {
		return err
	}
	value, err := ompProviderValue(opts)
	if err != nil {
		return err
	}
	if pair := ompProviderPairKey(providers, legacyOMPProviderID); pair >= 0 {
		providers.Content = append(providers.Content[:pair], providers.Content[pair+2:]...)
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Value: ompProviderID}
	if pair := ompProviderPair(providers); pair >= 0 {
		providers.Content[pair] = key
		providers.Content[pair+1] = value
	} else {
		providers.Content = append(providers.Content, key, value)
	}
	data, err := encodeOMPDoc(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	backupBeforeWrite(o.id, path)
	return atomicWrite(path, data)
}

func (o *omp) Disable() error {
	path := o.modelsPath()
	doc, err := readOMPDoc(path)
	if err != nil || doc == nil {
		return nil
	}
	providers, err := ompProviders(doc, false)
	if err != nil || providers == nil {
		return nil
	}
	removed := false
	for _, id := range []string{ompProviderID, legacyOMPProviderID} {
		if pair := ompProviderPairKey(providers, id); pair >= 0 {
			providers.Content = append(providers.Content[:pair], providers.Content[pair+2:]...)
			removed = true
		}
	}
	if !removed {
		return nil
	}
	data, err := encodeOMPDoc(doc)
	if err != nil {
		return err
	}
	backupBeforeWrite(o.id, path)
	return atomicWrite(path, data)
}

// --- YAML plumbing -----------------------------------------------------------

type ompModel struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	Reasoning     bool     `yaml:"reasoning"`
	Input         []string `yaml:"input"`
	ContextWindow int      `yaml:"contextWindow,omitempty"`
	MaxTokens     int      `yaml:"maxTokens,omitempty"`
}

type ompProvider struct {
	BaseURL string     `yaml:"baseUrl"`
	API     string     `yaml:"api"`
	APIKey  string     `yaml:"apiKey"`
	Models  []ompModel `yaml:"models"`
}

func ompProviderValue(opts Options) (*yaml.Node, error) {
	models := make([]ompModel, 0, len(opts.Models))
	for _, m := range opts.Models {
		if m.ID == "" {
			continue
		}
		name := m.Name
		if name == "" {
			name = m.ID
		}
		input := []string{"text"}
		if m.Vision {
			input = append(input, "image")
		}
		models = append(models, ompModel{
			ID:            m.ID,
			Name:          name,
			Reasoning:     m.Reasoning,
			Input:         input,
			ContextWindow: m.ContextWindow,
			MaxTokens:     m.MaxOutput,
		})
	}
	payload, err := yaml.Marshal(ompProvider{
		BaseURL: opts.BaseURL,
		API:     "openai-completions",
		APIKey:  opts.APIKey,
		Models:  models,
	})
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(payload, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("không dựng được khối provider omp")
	}
	return doc.Content[0], nil
}

func readOMPDoc(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("Không phân tích được %s (%w); từ chối ghi để tránh phá vỡ cấu hình omp", filepath.Base(path), err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s không phải YAML mapping; từ chối ghi", filepath.Base(path))
	}
	return &doc, nil
}

func ompProviders(doc *yaml.Node, create bool) (*yaml.Node, error) {
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "providers" {
			continue
		}
		value := root.Content[i+1]
		if value.Kind == yaml.MappingNode {
			return value, nil
		}
		if create && value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
			value.Kind, value.Tag, value.Value = yaml.MappingNode, "!!map", ""
			return value, nil
		}
		return nil, fmt.Errorf("providers trong %s không phải mapping; từ chối ghi", "models.yml")
	}
	if !create {
		return nil, nil
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Value: "providers"}
	value := &yaml.Node{Kind: yaml.MappingNode}
	root.Content = append(root.Content, key, value)
	return value, nil
}

func ompProviderPair(providers *yaml.Node) int {
	return ompProviderPairKey(providers, ompProviderID)
}

func ompProviderPairKey(providers *yaml.Node, id string) int {
	for i := 0; i+1 < len(providers.Content); i += 2 {
		if providers.Content[i].Value == id {
			return i
		}
	}
	return -1
}

func encodeOMPDoc(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
