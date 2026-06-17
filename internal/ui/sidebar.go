package ui

import (
	"fmt"
	"strings"

	"github.com/bjarneo/ku/internal/k8s"
)

// crdState tracks the CRD discovery button: not yet run, running, or done.
type crdState int

const (
	crdNone crdState = iota
	crdLoading
	crdReady
)

type navCatItem struct{ label, query string }

type navCatGroup struct {
	section string
	items   []navCatItem
}

// defaultNavCatalog is the curated, lazygit-style quick list used when the user
// has no config file. Entries that the cluster does not expose are dropped, and
// empty sections are hidden. Resources beyond these core ones (CRDs, autoscalers,
// etc.) are opt-in via the config file; see config.go and `ku config init`.
func defaultNavCatalog() []navCatGroup {
	return []navCatGroup{
		{"Workloads", []navCatItem{
			{"Pods", "pods"},
			{"Deployments", "deployments"},
			{"StatefulSets", "statefulsets"},
			{"DaemonSets", "daemonsets"},
			{"ReplicaSets", "replicasets"},
			{"Jobs", "jobs"},
			{"CronJobs", "cronjobs"},
		}},
		{"Network", []navCatItem{
			{"Services", "services"},
			{"Ingresses", "ingresses"},
			{"Endpoints", "endpoints"},
		}},
		{"Config", []navCatItem{
			{"ConfigMaps", "configmaps"},
			{"Secrets", "secrets"},
			{"ServiceAccounts", "serviceaccounts"},
		}},
		{"Storage", []navCatItem{
			{"PVCs", "persistentvolumeclaims"},
			{"PVs", "persistentvolumes"},
			{"StorageClasses", "storageclasses"},
		}},
		{"Cluster", []navCatItem{
			{"Nodes", "nodes"},
			{"Namespaces", "namespaces"},
			{"Events", "events"},
		}},
	}
}

// devHiddenResource reports whether a resource is cluster-level infrastructure
// that developer mode keeps out of the nav. Developers manage their own app, not
// the cluster, so nodes, cluster-scoped storage, namespaces, and events are
// dropped. Matched on the resolved resource so config aliases are covered too.
func devHiddenResource(ri k8s.ResourceInfo) bool {
	switch {
	case ri.IsNodes():
		return true
	case ri.Group == "" && ri.Resource == "persistentvolumes":
		return true
	case ri.Group == "" && ri.Resource == "namespaces":
		return true
	case ri.Resource == "events" && (ri.Group == "" || ri.Group == "events.k8s.io"):
		return true
	case ri.Group == "storage.k8s.io" && ri.Resource == "storageclasses":
		return true
	}
	return false
}

// overviewKey marks the sidebar entry that opens the cockpit dashboard;
// discoverKey marks the CRD discovery button.
const (
	overviewKey = "~overview"
	discoverKey = "~discover"
)

type navEntry struct {
	header   bool
	overview bool
	discover bool // the CRD discovery button at the bottom
	label    string
	res      k8s.ResourceInfo
	key      string
}

// crdLabel is the sidebar label for a custom resource: its Kind when known
// (e.g. "ScaledObject"), else the plural resource name.
func crdLabel(ri k8s.ResourceInfo) string {
	if ri.Kind != "" {
		return ri.Kind
	}
	return ri.Resource
}

// crdButtonLabel is the discovery button's text for the current state.
func crdButtonLabel(state crdState, n int) string {
	switch state {
	case crdLoading:
		return "Discovering CRDs…"
	case crdReady:
		return fmt.Sprintf("Refresh CRDs (%d)", n)
	default:
		return "Discover CRDs"
	}
}

// sidebar is the left navigation pane listing common resource kinds.
type sidebar struct {
	th         Theme
	entries    []navEntry
	selectable []int // indices into entries that are real items
	cursor     int   // index into selectable
	width      int
	height     int
}

func newSidebar(th Theme, reg *k8s.Registry, catalog []navCatGroup, crds []k8s.ResourceInfo, state crdState, dev bool) sidebar {
	s := sidebar{th: th}

	// The cockpit overview is the first, always-present entry.
	s.add(navEntry{overview: true, label: "Overview", key: overviewKey})

	for _, sec := range catalog {
		var items []navEntry
		for _, it := range sec.items {
			if ri, ok := reg.Resolve(it.query); ok {
				// Developer mode keeps cluster admin resources out of the nav.
				if dev && devHiddenResource(ri) {
					continue
				}
				items = append(items, navEntry{label: it.label, res: ri, key: ri.Key()})
			}
		}
		s.addSection(sec.section, items)
	}

	// Discovered CRDs, populated on demand by the discovery button.
	if len(crds) > 0 {
		items := make([]navEntry, 0, len(crds))
		for _, ri := range crds {
			items = append(items, navEntry{label: crdLabel(ri), res: ri, key: ri.Key()})
		}
		s.addSection("CRDs", items)
	}

	// The discovery button sits at the bottom of the nav. Developer mode does not
	// surface CRD discovery, so it is omitted there.
	if !dev {
		s.add(navEntry{discover: true, label: crdButtonLabel(state, len(crds)), key: discoverKey})
	}
	return s
}

// add appends a selectable entry.
func (s *sidebar) add(e navEntry) {
	s.selectable = append(s.selectable, len(s.entries))
	s.entries = append(s.entries, e)
}

// addSection appends a section header followed by its items. An empty section
// is skipped.
func (s *sidebar) addSection(title string, items []navEntry) {
	if len(items) == 0 {
		return
	}
	s.entries = append(s.entries, navEntry{header: true, label: title})
	for _, it := range items {
		s.add(it)
	}
}

func (s *sidebar) setSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *sidebar) moveUp() {
	if s.cursor > 0 {
		s.cursor--
	}
}

func (s *sidebar) moveDown() {
	if s.cursor < len(s.selectable)-1 {
		s.cursor++
	}
}

func (s *sidebar) move(delta int) {
	s.cursor = clamp(s.cursor+delta, 0, len(s.selectable)-1)
}

func (s *sidebar) moveTop() { s.cursor = 0 }

func (s *sidebar) moveBottom() {
	if len(s.selectable) > 0 {
		s.cursor = len(s.selectable) - 1
	}
}

func (s *sidebar) current() (navEntry, bool) {
	if len(s.selectable) == 0 {
		return navEntry{}, false
	}
	return s.entries[s.selectable[s.cursor]], true
}

// pinnedButton returns the entry index of the bottom discovery button, or -1.
// It is always the last entry when present, so it stays visible while the nav
// entries above it scroll.
func (s sidebar) pinnedButton() int {
	if n := len(s.entries); n > 0 && s.entries[n-1].discover {
		return n - 1
	}
	return -1
}

// navHeight is the rows available for scrollable nav entries; the pinned button
// reserves the last row when present.
func (s sidebar) navHeight() int {
	if s.pinnedButton() >= 0 && s.height > 0 {
		return s.height - 1
	}
	return s.height
}

// navLen is the number of scrollable entries (everything but the pinned button).
func (s sidebar) navLen() int {
	if b := s.pinnedButton(); b >= 0 {
		return b
	}
	return len(s.entries)
}

// navOffset scrolls the nav region to keep the cursor visible, anchoring the
// bottom of the list when the cursor sits on the pinned button.
func (s sidebar) navOffset() int {
	navLen, navH := s.navLen(), s.navHeight()
	if navH <= 0 || navLen <= navH {
		return 0
	}
	target := -1
	if len(s.selectable) > 0 {
		target = s.selectable[s.cursor]
	}
	if target >= navLen { // cursor is on the pinned button
		target = navLen - 1
	}
	if target < navH {
		return 0
	}
	return clamp(target-navH+1, 0, navLen-navH)
}

func (s *sidebar) selectAt(y int) (navEntry, bool) {
	if y < 0 || y >= s.height {
		return navEntry{}, false
	}
	btn := s.pinnedButton()
	var ei int
	if btn >= 0 && y >= s.navHeight() {
		ei = btn // the pinned button occupies the bottom row
	} else {
		ei = s.navOffset() + y
		if ei >= s.navLen() || s.entries[ei].header {
			return navEntry{}, false
		}
	}
	for i, idx := range s.selectable {
		if idx == ei {
			s.cursor = i
			return s.entries[ei], true
		}
	}
	return navEntry{}, false
}

// syncTo moves the cursor to the entry matching key, if present, so the
// highlight follows resource switches made elsewhere (palette, jump).
func (s *sidebar) syncTo(key string) {
	for i, ei := range s.selectable {
		if s.entries[ei].key == key {
			s.cursor = i
			return
		}
	}
}

func (s sidebar) View(activeKey string, focused bool) string {
	if s.height <= 0 {
		return ""
	}
	curEntry := -1
	if len(s.selectable) > 0 {
		curEntry = s.selectable[s.cursor]
	}
	navH := s.navHeight()
	offset := s.navOffset()

	lines := make([]string, 0, s.height)
	for i := offset; i < s.navLen() && len(lines) < navH; i++ {
		lines = append(lines, s.renderEntry(i, curEntry, activeKey, focused))
	}
	for len(lines) < navH {
		lines = append(lines, "")
	}
	// The discovery button is pinned to the bottom row so it stays reachable no
	// matter how many CRDs are listed above it.
	if b := s.pinnedButton(); b >= 0 {
		lines = append(lines, s.renderEntry(b, curEntry, activeKey, focused))
	}
	return strings.Join(lines, "\n")
}

// renderEntry renders one entry: a section header, or a selectable row with the
// active/selected/discover styling.
func (s sidebar) renderEntry(i, curEntry int, activeKey string, focused bool) string {
	th := s.th
	e := s.entries[i]
	if e.header {
		return th.NavSection.Render(truncate(e.label, s.width))
	}
	if focused && i == curEntry {
		return th.SelItemSel.Width(s.width).Render("  " + truncate(e.label, s.width-2))
	}
	marker := "  "
	label := e.label
	switch {
	case e.key == activeKey:
		marker = th.HeaderVal.Render("▸ ")
		label = th.HeaderVal.Render(label)
	case e.discover:
		label = th.FooterKey.Render(label) // accent so the button stands out
	default:
		label = th.SelItem.Render(label)
	}
	return marker + label
}
