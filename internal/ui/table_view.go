package ui

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bjarneo/kli/internal/k8s"
)

const (
	maxColWidth = 60
	colFloor    = 4
	nameFloor   = 16
)

// tableView renders a resource list with live filtering, column sorting, and
// per-cell color. It owns its own cursor/scroll so it can color cells (which
// the bubbles table cannot do safely).
type tableView struct {
	th Theme

	cols    []k8s.Column // all columns from the server
	allRows []k8s.Row    // unfiltered
	rows    []k8s.Row    // currently displayed (filtered + sorted)
	vis     []int        // visible column indices (parallel to widths)
	widths  []int        // natural (capped/floored) widths for visible columns

	// Horizontal scroll, used when the columns are wider than the viewport. The
	// first column is frozen so rows stay identifiable; hoff is how many of the
	// remaining columns are scrolled off the left edge.
	overflow bool
	hoff     int
	maxHoff  int
	frozenW  int // rendered width of the frozen first column when overflowing

	filtering bool
	filter    textinput.Model
	showWide  bool

	sortCol  int // index into cols, -1 for default (server) order
	sortDesc bool

	cursor int
	offset int // first visible row
	width  int
	height int
}

func newTableView(th Theme) tableView {
	return tableView{th: th, filter: newFilterInput("filter"), sortCol: -1}
}

func (v *tableView) setSize(w, h int) {
	v.width = w
	if h < 1 {
		h = 1
	}
	v.height = h
	if fw := w - 8; fw > 4 {
		v.filter.SetWidth(fw) // bound the filter input so it can't overflow
	}
	v.rebuild()
}

func (v *tableView) setData(t *k8s.Table) {
	if t == nil {
		v.cols = nil
		v.allRows = nil
	} else {
		v.cols = t.Columns
		v.allRows = t.Rows
	}
	v.rebuild()
}

func (v *tableView) toggleWide() {
	v.showWide = !v.showWide
	v.hoff = 0 // the column set changed; start from the left
	v.rebuild()
}

// resetHScroll returns the horizontal scroll to the leftmost column. Used when
// switching resources, whose column sets differ.
func (v *tableView) resetHScroll() { v.hoff = 0 }

// scrollLeft/scrollRight move the horizontal column window by one column. They
// report whether anything moved, so the caller can fall back to other behavior
// (e.g. focusing the sidebar) when already at the edge.
func (v *tableView) scrollLeft() bool {
	if !v.overflow || v.hoff <= 0 {
		return false
	}
	v.hoff--
	return true
}

func (v *tableView) scrollRight() bool {
	if !v.overflow || v.hoff >= v.maxHoff {
		return false
	}
	v.hoff++
	return true
}

func (v *tableView) startFilter() {
	v.filtering = true
	v.filter.Focus()
}

// stopFilter exits filter mode. If clear is true the filter text is dropped and
// the full list restored.
func (v *tableView) stopFilter(clear bool) {
	v.filtering = false
	v.filter.Blur()
	if clear {
		v.filter.SetValue("")
		v.rebuild()
	}
}

func (v *tableView) filterActive() bool  { return v.filter.Value() != "" }
func (v *tableView) filterValue() string { return v.filter.Value() }

// setSort sorts by the given column index. Re-selecting the active column flips
// the direction; idx < 0 restores the server's default order. Numeric columns
// default to descending (most interesting first).
func (v *tableView) setSort(idx int) {
	switch {
	case idx < 0:
		v.sortCol = -1
	case idx == v.sortCol:
		v.sortDesc = !v.sortDesc
	default:
		v.sortCol = idx
		v.sortDesc = sortDescByDefault(v.colName(idx))
	}
	v.rebuild()
}

func (v *tableView) resetSort() {
	v.sortCol = -1
	v.sortDesc = false
}

func (v *tableView) colName(i int) string {
	if i >= 0 && i < len(v.cols) {
		return v.cols[i].Name
	}
	return ""
}

func sortDescByDefault(name string) bool {
	n := strings.ToLower(name)
	switch {
	case n == "age", n == "restarts", strings.Contains(n, "cpu"), strings.Contains(n, "mem"), strings.HasSuffix(n, "%"):
		return true
	}
	return false
}

func (v *tableView) selected() (k8s.Row, bool) {
	if v.cursor < 0 || v.cursor >= len(v.rows) {
		return k8s.Row{}, false
	}
	return v.rows[v.cursor], true
}

func (v *tableView) count() int { return len(v.rows) }

// visibleCols returns the indices of columns shown given the wide toggle.
func (v *tableView) visibleCols() []int {
	idx := make([]int, 0, len(v.cols))
	for i, c := range v.cols {
		if c.Priority == 0 || v.showWide {
			idx = append(idx, i)
		}
	}
	return idx
}

func cell(cells []string, i int) string {
	if i < 0 || i >= len(cells) {
		return ""
	}
	return cells[i]
}

// rebuild recomputes the filtered+sorted rows and fitted widths, keeping the
// cursor in range and visible.
func (v *tableView) rebuild() {
	v.vis = v.visibleCols()

	// Filter (an empty filter keeps all rows in their original order).
	v.rows = fuzzyRank(v.allRows, v.filter.Value(), func(r k8s.Row) string {
		return strings.Join(r.Cells, " ")
	})

	// Optional explicit column sort, layered on top of the filter.
	if v.sortCol >= 0 && v.sortCol < len(v.cols) {
		name := v.cols[v.sortCol].Name
		less := func(i, j int) bool {
			return cellLess(name, cell(v.rows[i].Cells, v.sortCol), cell(v.rows[j].Cells, v.sortCol))
		}
		if v.sortDesc {
			sort.SliceStable(v.rows, func(i, j int) bool { return less(j, i) })
		} else {
			sort.SliceStable(v.rows, less)
		}
	}

	v.computeLayout()

	if v.cursor >= len(v.rows) {
		v.cursor = len(v.rows) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	v.ensureVisible()
}

// computeLayout sizes each visible column to its natural (content) width and
// decides whether the row is wider than the viewport. When it overflows, the
// first column is frozen and the rest become horizontally scrollable; otherwise
// every column is shown. It clamps the scroll offset to the valid range.
func (v *tableView) computeLayout() {
	vis := v.vis
	v.widths = make([]int, len(vis))
	for n, ci := range vis {
		w := len(v.cols[ci].Name)
		if ci == v.sortCol {
			w += 2 // room for the " ▲" / " ▼" indicator
		}
		for _, r := range v.allRows {
			if l := len(cell(r.Cells, ci)); l > w {
				w = l
			}
		}
		if w > maxColWidth {
			w = maxColWidth
		}
		floor := colFloor
		if strings.EqualFold(v.cols[ci].Name, "name") {
			floor = nameFloor
		}
		if w < floor {
			w = floor
		}
		v.widths[n] = w
	}

	// cellW counts a column's two padding cells alongside its content width.
	cellW := func(n int) int { return v.widths[n] + 2 }

	total := 0
	for n := range vis {
		total += cellW(n)
	}
	v.overflow = len(vis) > 1 && total > v.width
	if !v.overflow {
		v.hoff, v.maxHoff, v.frozenW = 0, 0, 0
		return
	}

	// Freeze the first column, but cap it so at least part of one scrolling
	// column always has room (a very long name shouldn't eat the whole row).
	v.frozenW = v.widths[0]
	if cap := v.width/2 - 2; v.frozenW > cap && cap >= nameFloor {
		v.frozenW = cap
	}
	avail := v.width - (v.frozenW + 2)

	// maxHoff is the smallest offset that still lets the last column fit: walk
	// the scrollable columns from the right, accumulating until they overflow.
	minStart := len(vis)
	used := 0
	for n := len(vis) - 1; n >= 1; n-- {
		used += cellW(n)
		if used > avail {
			break
		}
		minStart = n
	}
	v.maxHoff = minStart - 1
	if v.maxHoff < 0 {
		v.maxHoff = 0
	}
	v.hoff = clamp(v.hoff, 0, v.maxHoff)
}

// colSlot is one column to render: its index into vis and its rendered width.
type colSlot struct {
	n     int
	width int
}

// renderPlan lists the columns to draw left-to-right for the current scroll
// position. Without overflow that is simply every visible column; with overflow
// it is the frozen first column followed by as many scrolling columns as fit.
func (v tableView) renderPlan() []colSlot {
	if len(v.vis) == 0 {
		return nil
	}
	if !v.overflow {
		plan := make([]colSlot, len(v.vis))
		for n := range v.vis {
			plan[n] = colSlot{n: n, width: v.widths[n]}
		}
		return plan
	}
	plan := []colSlot{{n: 0, width: v.frozenW}}
	used := v.frozenW + 2
	for n := 1 + v.hoff; n < len(v.vis); n++ {
		w := v.widths[n] + 2
		if used+w > v.width && len(plan) > 1 {
			break
		}
		plan = append(plan, colSlot{n: n, width: v.widths[n]})
		used += w
	}
	return plan
}

// --- navigation -------------------------------------------------------------

func (v *tableView) visibleRows() int {
	if v.height > 1 {
		return v.height - 1 // one line for the header
	}
	return 0
}

func (v *tableView) moveCursor(d int) { v.setCursor(v.cursor + d) }

func (v *tableView) setCursor(i int) {
	if len(v.rows) == 0 {
		v.cursor, v.offset = 0, 0
		return
	}
	v.cursor = clamp(i, 0, len(v.rows)-1)
	v.ensureVisible()
}

func (v *tableView) ensureVisible() {
	vr := v.visibleRows()
	if vr <= 0 {
		v.offset = 0
		return
	}
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+vr {
		v.offset = v.cursor - vr + 1
	}
	maxOff := len(v.rows) - vr
	if maxOff < 0 {
		maxOff = 0
	}
	v.offset = clamp(v.offset, 0, maxOff)
}

func (v *tableView) colAt(x int) (int, bool) {
	if x < 0 {
		return 0, false
	}
	pos := 0
	for _, cs := range v.renderPlan() {
		w := cs.width + 2 // one leading and one trailing cell of padding
		if x >= pos && x < pos+w {
			return v.vis[cs.n], true
		}
		pos += w
	}
	return 0, false
}

func (v *tableView) rowAt(y int) (int, bool) {
	if y <= 0 { // y=0 is the header line
		return 0, false
	}
	i := v.offset + y - 1
	if i < 0 || i >= len(v.rows) {
		return 0, false
	}
	return i, true
}

// Update routes input to the filter box, or moves the cursor.
func (v tableView) Update(msg tea.Msg) (tableView, tea.Cmd) {
	if v.filtering {
		prev := v.filter.Value()
		var cmd tea.Cmd
		v.filter, cmd = v.filter.Update(msg)
		if v.filter.Value() != prev {
			v.cursor, v.offset = 0, 0
			v.rebuild()
		}
		return v, cmd
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return v, nil
	}
	vr := v.visibleRows()
	switch k.String() {
	case "up", "k":
		v.moveCursor(-1)
	case "down", "j":
		v.moveCursor(1)
	case "pgup":
		v.moveCursor(-vr)
	case "pgdown":
		v.moveCursor(vr)
	case "ctrl+u":
		v.moveCursor(-vr / 2)
	case "ctrl+d":
		v.moveCursor(vr / 2)
	case "g", "home":
		v.setCursor(0)
	case "G", "end":
		v.setCursor(len(v.rows) - 1)
	}
	return v, nil
}

// --- rendering --------------------------------------------------------------

func (v tableView) View() string {
	lines := make([]string, 0, v.height)
	lines = append(lines, v.headerLine())
	vr := v.visibleRows()
	for i := 0; i < vr; i++ {
		idx := v.offset + i
		if idx >= len(v.rows) {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, v.renderRow(v.rows[idx], idx == v.cursor))
	}
	return strings.Join(lines, "\n")
}

func (v tableView) headerLine() string {
	plan := v.renderPlan()
	var b strings.Builder
	for slot, cs := range plan {
		ci := v.vis[cs.n]
		title := strings.ToUpper(v.cols[ci].Name)
		if ci == v.sortCol {
			if v.sortDesc {
				title += " ▼"
			} else {
				title += " ▲"
			}
		}
		// Arrow hints mark that columns are scrolled off either edge.
		if v.overflow {
			if slot == 0 && v.hoff > 0 {
				title = "‹" + title
			}
			if slot == len(plan)-1 && cs.n < len(v.vis)-1 {
				title = strings.TrimRight(cellFit(title, cs.width-1), " ") + "›"
			}
		}
		b.WriteString(" " + v.th.HeaderVal.Render(cellFit(title, cs.width)) + " ")
	}
	return b.String()
}

func (v tableView) renderRow(row k8s.Row, selected bool) string {
	plan := v.renderPlan()
	var b strings.Builder
	for _, cs := range plan {
		ci := v.vis[cs.n]
		val := cell(row.Cells, ci)
		txt := cellFit(val, cs.width)
		if selected {
			b.WriteString(" " + txt + " ") // colored later via the whole-row highlight
		} else {
			b.WriteString(" " + styleCell(v.th, v.cols[ci].Name, val).Render(txt) + " ")
		}
	}
	if selected {
		return v.th.SelItemSel.Render(b.String())
	}
	return b.String()
}

// cellFit truncates s to w display columns (with an ellipsis when cut) and pads
// it on the right to exactly w.
func cellFit(s string, w int) string {
	s = truncate(s, w)
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}
