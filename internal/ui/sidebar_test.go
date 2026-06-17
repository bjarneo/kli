package ui

import (
	"strings"
	"testing"

	"github.com/bjarneo/ku/internal/k8s"
)

func TestSidebarHasDiscoverButtonAtBottom(t *testing.T) {
	s := newSidebar(PickTheme("ansi"), nil, nil, nil, crdNone, false)
	last := s.entries[len(s.entries)-1]
	if !last.discover || last.key != discoverKey {
		t.Fatalf("expected the discovery button at the bottom, got %+v", last)
	}
}

func TestSidebarPinsButtonAndStaysFixedHeight(t *testing.T) {
	var crds []k8s.ResourceInfo
	for i := 0; i < 40; i++ {
		crds = append(crds, k8s.ResourceInfo{Group: "g.io", Version: "v1", Resource: "res" + itoa(i), Kind: "Res" + itoa(i)})
	}
	s := newSidebar(PickTheme("ansi"), nil, nil, crds, crdReady, false)
	s.setSize(24, 12)

	out := s.View("", true) // cursor at the top, CRDs overflow below
	lines := strings.Split(out, "\n")
	if len(lines) != 12 {
		t.Fatalf("sidebar must stay at its fixed height (12), got %d lines", len(lines))
	}
	if !strings.Contains(lines[len(lines)-1], "Refresh CRDs") {
		t.Fatalf("discover button must stay pinned to the bottom row, got %q", lines[len(lines)-1])
	}
}

func TestDiscoverButtonStartsDiscovery(t *testing.T) {
	app := App{client: &k8s.Client{}, theme: PickTheme("ansi"), keys: defaultKeys()}
	app.sidebar = newSidebar(app.theme, nil, nil, nil, crdNone, false)

	m, cmd := app.openNavEntry(navEntry{discover: true})
	na := m.(App)
	if na.crdState != crdLoading {
		t.Fatalf("expected crdLoading after pressing discover, got %v", na.crdState)
	}
	if cmd == nil {
		t.Fatal("expected a discovery command to run")
	}
}

func TestCRDsDiscoveredPopulatesSidebar(t *testing.T) {
	cl := &k8s.Client{}
	app := App{client: cl, theme: PickTheme("ansi"), keys: defaultKeys(), width: 120, height: 40, screen: screenCockpit}
	app.sidebar = newSidebar(app.theme, nil, nil, nil, crdNone, false)
	app.relayout()

	crds := []k8s.ResourceInfo{
		{Group: "keda.sh", Version: "v1alpha1", Resource: "scaledobjects", Kind: "ScaledObject", Namespaced: true},
	}
	m, _ := app.Update(crdsDiscoveredMsg{client: cl, crds: crds})
	na := m.(App)

	if na.crdState != crdReady {
		t.Fatalf("expected crdReady after discovery, got %v", na.crdState)
	}
	// The rebuilt sidebar must stay sized, or it renders blank until a resize.
	if na.sidebar.height <= 0 {
		t.Fatalf("sidebar lost its size after discovery (height %d)", na.sidebar.height)
	}
	var hasHeader, hasEntry bool
	for _, e := range na.sidebar.entries {
		if e.header && e.label == "CRDs" {
			hasHeader = true
		}
		if e.key == "scaledobjects.keda.sh" && !e.header {
			hasEntry = true
		}
	}
	if !hasHeader || !hasEntry {
		t.Fatalf("CRDs section not populated: header=%v entry=%v", hasHeader, hasEntry)
	}
}
