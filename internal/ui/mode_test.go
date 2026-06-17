package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/ku/internal/k8s"
)

func modeTestRegistry() *k8s.Registry {
	return k8s.NewRegistry([]k8s.ResourceInfo{
		{Resource: "pods", Kind: "Pod", Singular: "pod", Namespaced: true},
		{Resource: "deployments", Group: "apps", Kind: "Deployment", Singular: "deployment", Namespaced: true},
		{Resource: "persistentvolumeclaims", Kind: "PersistentVolumeClaim", Singular: "persistentvolumeclaim", Namespaced: true},
		{Resource: "nodes", Kind: "Node", Singular: "node"},
		{Resource: "namespaces", Kind: "Namespace", Singular: "namespace"},
		{Resource: "events", Kind: "Event", Singular: "event", Namespaced: true},
		{Resource: "persistentvolumes", Kind: "PersistentVolume", Singular: "persistentvolume"},
		{Resource: "storageclasses", Group: "storage.k8s.io", Kind: "StorageClass", Singular: "storageclass"},
	})
}

func TestDevHiddenResource(t *testing.T) {
	hidden := []k8s.ResourceInfo{
		{Resource: "nodes"},
		{Resource: "persistentvolumes"},
		{Resource: "namespaces"},
		{Resource: "events"},
		{Resource: "events", Group: "events.k8s.io"},
		{Resource: "storageclasses", Group: "storage.k8s.io"},
	}
	for _, ri := range hidden {
		if !devHiddenResource(ri) {
			t.Errorf("expected %s to be hidden in developer mode", ri.Key())
		}
	}
	kept := []k8s.ResourceInfo{
		{Resource: "pods"},
		{Resource: "deployments", Group: "apps"},
		{Resource: "persistentvolumeclaims"},
		{Resource: "services"},
		{Resource: "configmaps"},
	}
	for _, ri := range kept {
		if devHiddenResource(ri) {
			t.Errorf("expected %s to stay visible in developer mode", ri.Key())
		}
	}
}

func sidebarKeySet(s sidebar) map[string]bool {
	keys := make(map[string]bool)
	for _, e := range s.entries {
		if e.header || e.overview || e.discover {
			continue
		}
		keys[e.key] = true
	}
	return keys
}

func TestSidebarDevModeFiltersClusterAdmin(t *testing.T) {
	reg := modeTestRegistry()
	catalog := []navCatGroup{
		{"Workloads", []navCatItem{{"Pods", "pods"}, {"Deployments", "deployments"}}},
		{"Storage", []navCatItem{{"PVCs", "persistentvolumeclaims"}, {"PVs", "persistentvolumes"}, {"StorageClasses", "storageclasses"}}},
		{"Cluster", []navCatItem{{"Nodes", "nodes"}, {"Namespaces", "namespaces"}, {"Events", "events"}}},
	}

	full := sidebarKeySet(newSidebar(PickTheme("ansi"), reg, catalog, nil, crdNone, false))
	for _, k := range []string{"nodes", "namespaces", "events", "persistentvolumes", "storageclasses.storage.k8s.io"} {
		if !full[k] {
			t.Errorf("full mode should list %q", k)
		}
	}

	dev := sidebarKeySet(newSidebar(PickTheme("ansi"), reg, catalog, nil, crdNone, true))
	for _, k := range []string{"pods", "deployments.apps", "persistentvolumeclaims"} {
		if !dev[k] {
			t.Errorf("developer mode should keep app resource %q", k)
		}
	}
	for _, k := range []string{"nodes", "namespaces", "events", "persistentvolumes", "storageclasses.storage.k8s.io"} {
		if dev[k] {
			t.Errorf("developer mode should hide cluster admin resource %q", k)
		}
	}
	// The Cluster section is entirely admin resources, so it disappears.
	for _, e := range newSidebar(PickTheme("ansi"), reg, catalog, nil, crdNone, true).entries {
		if e.header && e.label == "Cluster" {
			t.Error("developer mode should drop the empty Cluster section")
		}
	}
}

// runHandler invokes a handler and returns the resulting App for assertions.
func runHandler(m tea.Model) App { return m.(App) }

func TestReadOnlyBlocksMutations(t *testing.T) {
	nodes, _ := modeTestRegistry().Resolve("nodes")
	base := App{theme: PickTheme("ansi"), readOnly: true, res: nodes}

	cases := []struct {
		name string
		call func(App) (tea.Model, tea.Cmd)
	}{
		{"delete", func(a App) (tea.Model, tea.Cmd) { return a.openDelete() }},
		{"edit", func(a App) (tea.Model, tea.Cmd) { return a.editTarget(target{name: "x"}) }},
		{"scale", func(a App) (tea.Model, tea.Cmd) { return a.openScale() }},
		{"restart", func(a App) (tea.Model, tea.Cmd) { return a.openRestart() }},
		{"trigger", func(a App) (tea.Model, tea.Cmd) { return a.openTriggerJobTarget(target{name: "x"}) }},
		{"shell", func(a App) (tea.Model, tea.Cmd) { return a.openShell() }},
		{"cordon", func(a App) (tea.Model, tea.Cmd) { return a.openCordon() }},
		{"drain", func(a App) (tea.Model, tea.Cmd) { return a.openDrain() }},
		{"node shell", func(a App) (tea.Model, tea.Cmd) { return a.openNodeShell() }},
	}
	for _, tc := range cases {
		na := runHandler(mustModel(tc.call(base)))
		if !na.statusErr {
			t.Errorf("%s should report an error in read-only mode", tc.name)
		}
		if !strings.Contains(na.status, "read-only") {
			t.Errorf("%s status should mention read-only, got %q", tc.name, na.status)
		}
		if na.overlay != overlayNone {
			t.Errorf("%s should not open an overlay in read-only mode", tc.name)
		}
	}
}

func TestDevModeBlocksNodeOps(t *testing.T) {
	nodes, _ := modeTestRegistry().Resolve("nodes")
	base := App{theme: PickTheme("ansi"), dev: true, res: nodes}

	for _, tc := range []struct {
		name string
		call func(App) (tea.Model, tea.Cmd)
	}{
		{"cordon", func(a App) (tea.Model, tea.Cmd) { return a.openCordon() }},
		{"drain", func(a App) (tea.Model, tea.Cmd) { return a.openDrain() }},
		{"node shell", func(a App) (tea.Model, tea.Cmd) { return a.openNodeShell() }},
	} {
		na := runHandler(mustModel(tc.call(base)))
		if !na.statusErr || !strings.Contains(na.status, "developer mode") {
			t.Errorf("%s should be blocked in developer mode, got status %q (err=%v)", tc.name, na.status, na.statusErr)
		}
	}
}

func TestEditModeAllowsWrites(t *testing.T) {
	a := App{theme: PickTheme("ansi")} // dev=false, readOnly=false (edit mode)
	if a.denyReadOnly("edit") {
		t.Error("edit mode must allow writes")
	}
	if a.denyNodeOps("cordon") {
		t.Error("edit mode must allow node ops")
	}
}

func TestEditModeToggle(t *testing.T) {
	// From read-only, entering edit mode prompts for confirmation first.
	read := App{theme: PickTheme("ansi"), readOnly: true}
	m, _ := read.toggleEditMode()
	prompted := m.(App)
	if prompted.overlay != overlayConfirm || prompted.confirm.action == nil {
		t.Fatal("entering edit mode should open a confirm prompt with an action")
	}
	// Running the confirmed action enables edit mode.
	enabled, _ := prompted.Update(prompted.confirm.action())
	if enabled.(App).readOnly {
		t.Error("confirming should turn off read-only")
	}

	// From edit mode, returning to read-only is immediate (no prompt).
	edit := App{theme: PickTheme("ansi"), readOnly: false}
	m2, cmd := edit.toggleEditMode()
	if m2.(App).overlay == overlayConfirm {
		t.Error("leaving edit mode should not prompt")
	}
	if cmd == nil {
		t.Fatal("leaving edit mode should emit a message")
	}
	back, _ := edit.Update(cmd())
	if !back.(App).readOnly {
		t.Error("returning to read-only should re-enable read-only")
	}
}

func TestEditModeShortcut(t *testing.T) {
	read := App{theme: PickTheme("ansi"), keys: defaultKeys(), readOnly: true}
	m, _ := read.handleKey(mkKey("E"))
	prompted := m.(App)
	if prompted.overlay != overlayConfirm || prompted.confirm.action == nil {
		t.Fatal("Shift+E from read-only should prompt before entering edit mode")
	}
	enabled, _ := prompted.Update(prompted.confirm.action())
	if enabled.(App).readOnly {
		t.Error("confirming Shift+E should enter edit mode")
	}

	edit := App{theme: PickTheme("ansi"), keys: defaultKeys()}
	m, cmd := edit.handleKey(mkKey("E"))
	if m.(App).overlay == overlayConfirm {
		t.Fatal("Shift+E from edit mode should not prompt")
	}
	if cmd == nil {
		t.Fatal("Shift+E from edit mode should return a mode command")
	}
	back, _ := m.(App).Update(cmd())
	if !back.(App).readOnly {
		t.Error("Shift+E from edit mode should return to read-only")
	}
}

func TestDevModeOmitsDiscoverButton(t *testing.T) {
	reg := modeTestRegistry()
	cat := []navCatGroup{{"Workloads", []navCatItem{{"Pods", "pods"}}}}
	hasDiscover := func(s sidebar) bool {
		for _, e := range s.entries {
			if e.discover {
				return true
			}
		}
		return false
	}
	if !hasDiscover(newSidebar(PickTheme("ansi"), reg, cat, nil, crdNone, false)) {
		t.Error("full mode should show the CRD discovery button")
	}
	if hasDiscover(newSidebar(PickTheme("ansi"), reg, cat, nil, crdNone, true)) {
		t.Error("developer mode should not show the CRD discovery button")
	}
}

func TestDevModeDoesNotDiscoverCRDs(t *testing.T) {
	a := App{theme: PickTheme("ansi"), dev: true, client: &k8s.Client{}}
	m, cmd := a.discoverCRDs()
	if cmd != nil {
		t.Error("developer mode should not start CRD discovery")
	}
	if m.(App).crdState == crdLoading {
		t.Error("developer mode should not enter the loading state")
	}
}

// paletteIDs collects the selector item ids produced by openPalette for a
// resource and mode, with one row selected.
func paletteIDs(res k8s.ResourceInfo, readOnly, dev bool) map[string]bool {
	a := App{client: &k8s.Client{}, theme: PickTheme("ansi"), res: res, readOnly: readOnly, dev: dev}
	a.sel = newSelector(a.theme)
	a.table.rows = []k8s.Row{{Name: "x"}}
	ids := make(map[string]bool)
	for _, it := range runHandler(mustModel(a.openPalette())).sel.items {
		ids[it.id] = true
	}
	return ids
}

func TestPaletteRespectsMode(t *testing.T) {
	reg := modeTestRegistry()
	pods, _ := reg.Resolve("pods")
	nodes, _ := reg.Resolve("nodes")

	full := paletteIDs(pods, false, false)
	for _, id := range []string{"act:edit", "act:delete", "act:shell"} {
		if !full[id] {
			t.Errorf("full mode palette should offer %s", id)
		}
	}

	ro := paletteIDs(pods, true, false)
	for _, id := range []string{"act:edit", "act:delete", "act:shell"} {
		if ro[id] {
			t.Errorf("read-only palette should not offer %s", id)
		}
	}
	if !ro["act:describe"] || !ro["act:logs"] {
		t.Error("read-only palette should still offer read actions")
	}
	// The edit-mode toggle is always available, in either mode.
	if !ro["cmd:editmode"] || !full["cmd:editmode"] {
		t.Error("the palette should always offer the edit-mode toggle")
	}

	devNodes := paletteIDs(nodes, false, true)
	for _, id := range []string{"act:cordon", "act:drain", "act:nodeshell"} {
		if devNodes[id] {
			t.Errorf("developer mode palette should not offer node op %s", id)
		}
	}
	if !paletteIDs(nodes, false, false)["act:cordon"] {
		t.Error("full mode palette on nodes should offer cordon")
	}
}

func TestModeChip(t *testing.T) {
	th := PickTheme("ansi")
	if got := (App{theme: th}).modeChip(); !strings.Contains(got, "EDIT") {
		t.Errorf("read/write chip should say EDIT, got %q", got)
	}
	if got := (App{theme: th, readOnly: true}).modeChip(); !strings.Contains(got, "READ-ONLY") {
		t.Errorf("read-only chip should say READ-ONLY, got %q", got)
	}
}

// mustModel unwraps the (tea.Model, tea.Cmd) handler return for tests.
func mustModel(m tea.Model, _ tea.Cmd) tea.Model { return m }
