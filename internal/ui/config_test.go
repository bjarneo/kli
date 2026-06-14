package ui

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// parseConfig mirrors loadConfig's parsing without touching the filesystem.
func parseConfig(t *testing.T, src string) Config {
	t.Helper()
	var cfg Config
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}

func TestDefaultCatalogExcludesOptIns(t *testing.T) {
	for _, g := range defaultNavCatalog() {
		for _, it := range g.items {
			switch it.query {
			case "horizontalpodautoscalers", "scaledobjects", "opentelemetrycollectors":
				t.Fatalf("default catalog must not include opt-in resource %q", it.query)
			}
		}
	}
}

func TestConfigFilesUseHomeConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))

	config, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	state, err := stateFile()
	if err != nil {
		t.Fatalf("stateFile: %v", err)
	}
	dir := filepath.Join(home, ".config", "kli")
	if want := filepath.Join(dir, "config.yaml"); config != want {
		t.Fatalf("configPath() = %q; want %q", config, want)
	}
	if want := filepath.Join(dir, "state.json"); state != want {
		t.Fatalf("stateFile() = %q; want %q", state, want)
	}
}

func TestCatalogConfigRoundTrip(t *testing.T) {
	def := defaultNavCatalog()
	b, err := marshalConfig(catalogConfig(def))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	cfg := parseConfig(t, string(b))
	if got := cfg.sidebarCatalog(); !reflect.DeepEqual(got, def) {
		t.Fatalf("round-trip mismatch:\n got=%+v\nwant=%+v", got, def)
	}
}

func TestSidebarCatalogConvertsSectionsAndItems(t *testing.T) {
	cfg := parseConfig(t, `
sidebar:
  - section: Workloads
    items:
      - { label: Pods, resource: pods }
      - { label: HPAs, resource: horizontalpodautoscalers }
  - section: Network
    items:
      - { label: Services, resource: services }
`)
	got := cfg.sidebarCatalog()
	want := []navCatGroup{
		{section: "Workloads", items: []navCatItem{
			{label: "Pods", query: "pods"},
			{label: "HPAs", query: "horizontalpodautoscalers"},
		}},
		{section: "Network", items: []navCatItem{
			{label: "Services", query: "services"},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("conversion mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestSidebarCatalogEmptyFallsBack(t *testing.T) {
	// No sidebar key -> nil, so the caller uses the built-in defaults.
	if got := (Config{}).sidebarCatalog(); got != nil {
		t.Fatalf("empty config should yield nil catalog, got %+v", got)
	}
}

func TestOptInExamplesAreInertComments(t *testing.T) {
	// The seeded file is "marshaled defaults" + commented examples; parsing the
	// whole thing must yield exactly the defaults (comments are inert).
	b, err := marshalConfig(catalogConfig(defaultNavCatalog()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	full := string(b) + optInExamples
	if !strings.Contains(optInExamples, "horizontalpodautoscalers") {
		t.Fatalf("opt-in examples should mention the opt-in resources")
	}
	cfg := parseConfig(t, full)
	if got := cfg.sidebarCatalog(); !reflect.DeepEqual(got, defaultNavCatalog()) {
		t.Fatalf("commented examples leaked into parsed config:\n%+v", got)
	}
}
