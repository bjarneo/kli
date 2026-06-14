package ui

import (
	"strings"

	"github.com/bjarneo/kli/internal/k8s"
)

type navCatItem struct{ label, query string }

type navCatGroup struct {
	section string
	items   []navCatItem
}

// defaultNavCatalog is the curated, lazygit-style quick list used when the user
// has no config file. Entries that the cluster does not expose are dropped, and
// empty sections are hidden. Resources beyond these core ones (CRDs, autoscalers,
// etc.) are opt-in via the config file — see config.go and `kli config init`.
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

// overviewKey marks the sidebar entry that opens the cockpit dashboard.
const overviewKey = "~overview"

type navEntry struct {
	header   bool
	overview bool
	label    string
	res      k8s.ResourceInfo
	key      string
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

func newSidebar(th Theme, reg *k8s.Registry, catalog []navCatGroup) sidebar {
	s := sidebar{th: th}

	// The cockpit overview is the first, always-present entry.
	s.selectable = append(s.selectable, len(s.entries))
	s.entries = append(s.entries, navEntry{overview: true, label: "Overview", key: overviewKey})

	for _, sec := range catalog {
		var items []navEntry
		for _, it := range sec.items {
			ri, ok := reg.Resolve(it.query)
			if !ok {
				continue
			}
			items = append(items, navEntry{label: it.label, res: ri, key: ri.Key()})
		}
		if len(items) == 0 {
			continue
		}
		s.entries = append(s.entries, navEntry{header: true, label: sec.section})
		for _, it := range items {
			s.selectable = append(s.selectable, len(s.entries))
			s.entries = append(s.entries, it)
		}
	}
	return s
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
	th := s.th
	curEntry := -1
	if len(s.selectable) > 0 {
		curEntry = s.selectable[s.cursor]
	}

	// Scroll so the cursor stays visible.
	offset := 0
	if s.height > 0 && curEntry >= s.height {
		offset = curEntry - s.height + 1
	}

	var lines []string
	for i := offset; i < len(s.entries) && len(lines) < s.height; i++ {
		e := s.entries[i]
		if e.header {
			lines = append(lines, th.NavSection.Render(truncate(e.label, s.width)))
			continue
		}
		marker := "  "
		label := e.label
		if e.key == activeKey {
			marker = th.HeaderVal.Render("▸ ")
			label = th.HeaderVal.Render(label)
		} else {
			label = th.SelItem.Render(label)
		}
		line := marker + label
		if focused && i == curEntry {
			line = th.SelItemSel.Width(s.width).Render("  " + truncate(e.label, s.width-2))
		}
		lines = append(lines, line)
	}
	for len(lines) < s.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
