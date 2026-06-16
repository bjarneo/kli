package ui

import (
	"strings"
	"testing"

	"github.com/bjarneo/kli/internal/k8s"
)

func TestTableSelectionFollowsRowAcrossRefresh(t *testing.T) {
	v := newTableView(PickTheme("ansi"))
	v.setData(scrollTable())
	v.setSize(400, 20)
	v.setCursor(1)
	if r, _ := v.selected(); r.Name != "beta" {
		t.Fatalf("setup: expected beta selected, got %q", r.Name)
	}

	// A live refresh delivers the same rows reordered; the selection should
	// follow beta rather than stay on index 1.
	base := scrollTable()
	reordered := &k8s.Table{Columns: base.Columns, Rows: []k8s.Row{base.Rows[1], base.Rows[0]}}
	v.setData(reordered)
	if r, ok := v.selected(); !ok || r.Name != "beta" {
		t.Fatalf("selection should follow beta after refresh, got %q", r.Name)
	}
}

func TestTableSelectionKeepsNamespaceAcrossRefresh(t *testing.T) {
	// In all-namespaces mode a name can repeat across namespaces. The selection
	// must follow the exact namespace+name, not the first name match (which sits
	// higher in the list and would pull the cursor toward the top).
	cols := []k8s.Column{{Name: "NAMESPACE"}, {Name: "NAME"}}
	rows := []k8s.Row{
		{Namespace: "team-a", Name: "redis", Cells: []string{"team-a", "redis"}},
		{Namespace: "team-b", Name: "redis", Cells: []string{"team-b", "redis"}},
		{Namespace: "team-c", Name: "redis", Cells: []string{"team-c", "redis"}},
	}
	v := newTableView(PickTheme("ansi"))
	v.setSize(400, 20)
	v.setData(&k8s.Table{Columns: cols, Rows: rows})
	v.setCursor(2) // team-c/redis
	if r, _ := v.selected(); r.Namespace != "team-c" {
		t.Fatalf("setup: expected team-c selected, got %q", r.Namespace)
	}

	// Same set delivered by a live refresh: the cursor must stay on team-c.
	v.setData(&k8s.Table{Columns: cols, Rows: append([]k8s.Row(nil), rows...)})
	if r, _ := v.selected(); r.Namespace != "team-c" {
		t.Fatalf("selection jumped to %q; namespace identity not preserved", r.Namespace)
	}
}

func TestTableSwitchResetsSelectionToTop(t *testing.T) {
	v := newTableView(PickTheme("ansi"))
	v.setData(scrollTable())
	v.setSize(400, 20)
	v.setCursor(1)

	// A resource switch clears the data first; selection should land at the top.
	v.setData(nil)
	v.setData(scrollTable())
	if r, _ := v.selected(); r.Name != "alpha" {
		t.Fatalf("switch should reset selection to the top, got %q", r.Name)
	}
}

func scrollTable() *k8s.Table {
	return &k8s.Table{
		Columns: []k8s.Column{
			{Name: "NAME"}, {Name: "READY"}, {Name: "STATUS"},
			{Name: "RESTARTS"}, {Name: "AGE"}, {Name: "CPU"}, {Name: "MEM"},
		},
		Rows: []k8s.Row{
			{Name: "alpha", Cells: []string{"alpha-pod-with-a-fairly-long-name", "1/1", "Running", "0", "5d", "12m", "40Mi"}},
			{Name: "beta", Cells: []string{"beta-pod", "1/1", "Running", "3", "2h", "5m", "10Mi"}},
		},
	}
}

// planCols returns the set of underlying column indices the render plan covers.
func planCols(v *tableView) map[int]bool {
	got := map[int]bool{}
	for _, cs := range v.renderPlan() {
		got[v.vis[cs.n]] = true
	}
	return got
}

func TestTableNoOverflowShowsAllColumns(t *testing.T) {
	v := newTableView(PickTheme("ansi"))
	v.setData(scrollTable())
	v.setSize(400, 20) // plenty of room

	if v.overflow {
		t.Fatalf("wide viewport should not overflow")
	}
	if got := planCols(&v); len(got) != len(v.cols) {
		t.Fatalf("expected all %d columns rendered, got %d", len(v.cols), len(got))
	}
}

func TestTableOverflowFreezesFirstAndScrollsToLast(t *testing.T) {
	v := newTableView(PickTheme("ansi"))
	v.setData(scrollTable())
	v.setSize(40, 20) // narrow: columns cannot all fit

	if !v.overflow {
		t.Fatalf("narrow viewport should overflow")
	}
	if v.maxHoff <= 0 {
		t.Fatalf("expected a positive maxHoff when overflowing, got %d", v.maxHoff)
	}
	if !planCols(&v)[0] {
		t.Fatalf("frozen first column missing at hoff=0")
	}

	// Scrolling right must eventually reveal the last column, then stop there.
	last := len(v.cols) - 1
	for i := 0; i < len(v.cols)+2; i++ {
		if planCols(&v)[last] {
			break
		}
		if !v.scrollRight() && !planCols(&v)[last] {
			t.Fatalf("hit the right edge but the last column never appeared")
		}
	}
	if !planCols(&v)[0] {
		t.Fatalf("first column should stay frozen after scrolling")
	}
	if !planCols(&v)[last] {
		t.Fatalf("last column unreachable by scrolling")
	}
	if v.scrollRight() {
		t.Fatalf("scrollRight past maxHoff should report no movement")
	}
	if !v.scrollLeft() {
		t.Fatalf("scrollLeft from the right edge should move")
	}
}

func TestTableScrollLeftStopsAtEdgeAndHints(t *testing.T) {
	v := newTableView(PickTheme("ansi"))
	v.setData(scrollTable())
	v.setSize(40, 20)

	for v.scrollLeft() { // unwind to the leftmost column
	}
	if v.hoff != 0 {
		t.Fatalf("expected hoff 0 at left edge, got %d", v.hoff)
	}
	if v.scrollLeft() {
		t.Fatalf("scrollLeft at the left edge should report no movement")
	}
	if !strings.Contains(v.headerLine(), "›") {
		t.Fatalf("expected a right-scroll hint in the header:\n%s", v.headerLine())
	}
}

func TestTableScrollNoopWithoutOverflow(t *testing.T) {
	v := newTableView(PickTheme("ansi"))
	v.setData(scrollTable())
	v.setSize(400, 20)
	if v.scrollLeft() || v.scrollRight() {
		t.Fatalf("scrolling should be inert when the table fits")
	}
}
