package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the user-authored configuration file (distinct from the auto-saved
// state.json). It is read once at startup and never written by the running TUI;
// only the user edits it, or `kli config init` seeds it. The first supported key
// is the sidebar menu; the struct leaves room for more keys later.
type Config struct {
	Sidebar []SidebarSection `yaml:"sidebar,omitempty"`
}

// SidebarSection is one labeled group in the left nav (e.g. "Workloads").
type SidebarSection struct {
	Section string        `yaml:"section"`
	Items   []SidebarItem `yaml:"items"`
}

// SidebarItem is one entry in a section. Resource is any string the resource
// registry resolves: plural, singular, kind, short name, or group-qualified key
// (e.g. "scaledobjects.keda.sh").
type SidebarItem struct {
	Label    string `yaml:"label"`
	Resource string `yaml:"resource"`
}

// configPath is <os.UserConfigDir>/kli/config.yaml, the same directory as
// state.json.
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kli", "config.yaml"), nil
}

// ConfigPath exposes the resolved config file path for the `kli config path`
// subcommand.
func ConfigPath() (string, error) { return configPath() }

// loadConfig reads and parses the config file. A missing file is normal and
// returns (zero, false, nil). A read or parse error returns (zero, false, err)
// so the caller can warn and fall back to defaults.
func loadConfig() (Config, bool, error) {
	p, err := configPath()
	if err != nil {
		return Config{}, false, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

// sidebarCatalog converts the config's sidebar into the internal nav catalog.
// It returns nil when no sections are defined, so the caller falls back to the
// built-in defaults.
func (c Config) sidebarCatalog() []navCatGroup {
	if len(c.Sidebar) == 0 {
		return nil
	}
	cat := make([]navCatGroup, 0, len(c.Sidebar))
	for _, sec := range c.Sidebar {
		items := make([]navCatItem, 0, len(sec.Items))
		for _, it := range sec.Items {
			items = append(items, navCatItem{label: it.Label, query: it.Resource})
		}
		cat = append(cat, navCatGroup{section: sec.Section, items: items})
	}
	return cat
}

// catalogConfig builds a Config from a nav catalog, used to serialize the
// defaults for `kli config init`.
func catalogConfig(cat []navCatGroup) Config {
	cfg := Config{Sidebar: make([]SidebarSection, 0, len(cat))}
	for _, g := range cat {
		items := make([]SidebarItem, 0, len(g.items))
		for _, it := range g.items {
			items = append(items, SidebarItem{Label: it.label, Resource: it.query})
		}
		cfg.Sidebar = append(cfg.Sidebar, SidebarSection{Section: g.section, Items: items})
	}
	return cfg
}

// marshalConfig renders a Config as YAML with a compact 2-space indent and
// declaration field order (so "section" precedes "items").
func marshalConfig(cfg Config) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// optInExamples is appended (commented out) to a seeded config so users can
// discover and uncomment resources that are intentionally absent from the
// defaults. As YAML comments it is inert when the file is parsed.
const optInExamples = `
# Opt-in examples — uncomment and add under the relevant section's items:
#   - { label: HPAs, resource: horizontalpodautoscalers }
#   - { label: ScaledObjects, resource: scaledobjects }   # KEDA
#   - { label: OtelCollectors, resource: opentelemetrycollectors }
`

// WriteDefaultConfig seeds the config file with the built-in defaults plus a
// trailing block of commented opt-in examples. It refuses to overwrite an
// existing file unless force is true. It returns the path written.
func WriteDefaultConfig(force bool) (string, error) {
	p, err := configPath()
	if err != nil {
		return "", err
	}
	if !force {
		if _, err := os.Stat(p); err == nil {
			return p, fmt.Errorf("%s already exists; use --force to overwrite", p)
		}
	}
	b, err := marshalConfig(catalogConfig(defaultNavCatalog()))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, append(b, []byte(optInExamples)...), 0o644); err != nil {
		return "", err
	}
	return p, nil
}
