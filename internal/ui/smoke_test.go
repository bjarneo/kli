package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/vt"

	"github.com/bjarneo/kli/internal/k8s"
)

// fakeTable returns a small server-style table for render exercises.
func fakeTable() *k8s.Table {
	return &k8s.Table{
		Columns: []k8s.Column{
			{Name: "NAMESPACE"},
			{Name: "Name"},
			{Name: "Ready"},
			{Name: "Status"},
			{Name: "Restarts"},
			{Name: "Age"},
			{Name: "IP", Priority: 1},
		},
		Rows: []k8s.Row{
			{Namespace: "default", Name: "api-7d9", Cells: []string{"default", "api-7d9", "1/1", "Running", "0", "4d", "10.0.0.1"}},
			{Namespace: "kube-system", Name: "coredns-abc", Cells: []string{"kube-system", "coredns-abc", "1/1", "Running", "2", "9d", "10.0.0.2"}},
		},
	}
}

func fakeOverview() *k8s.ClusterOverview {
	return &k8s.ClusterOverview{
		Version: "v1.31.0", Nodes: 5, NodesReady: 5, HasMetrics: true,
		CPUUsedMilli: 4200, CPUAllocMilli: 16000,
		MemUsedBytes: 12 << 30, MemAllocBytes: 32 << 30,
		Namespaces: 23, Pods: 200, PodRunning: 190, PodPending: 2, PodFailed: 1,
		PodNotReady: 3, PodCrashLoop: 1,
		Deployments: 42, DeploymentsReady: 40,
		NodeIssues: []string{"ip-10-0-1-2 DiskPressure", "ip-10-0-3-4 NotReady"},
		Warnings: []k8s.EventLine{
			{Age: "2m", Namespace: "default", Reason: "BackOff", Object: "Pod/api-7d9", Message: "Back-off restarting failed container", Count: 12},
		},
	}
}

func mkKey(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "ctrl+k":
		return tea.KeyMsg{Type: tea.KeyCtrlK}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// TestAppSmoke drives the model through every screen and overlay, asserting no
// panic and non-empty output. It needs a reachable cluster only for the client
// catalog; skipped otherwise.
func TestAppSmoke(t *testing.T) {
	cl, err := k8s.NewClient("", "")
	if err != nil {
		t.Skipf("no cluster available: %v", err)
	}

	for _, themeName := range []string{"ansi", "tokyonight"} {
		th := PickTheme(themeName)
		app := NewApp(cl, th, defaultNavCatalog())
		var m tea.Model = app

		// Try several terminal sizes including very small.
		for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 12}, {20, 6}, {12, 8}} {
			m, _ = m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})

			// Cockpit is the default screen; render it populated.
			m, _ = m.Update(cockpitLoadedMsg{overview: fakeOverview()})
			mustRender(t, m, themeName, size)

			// Switch to the table and load synthetic data for the rest.
			a := m.(App)
			a.screen = screenTable
			m = a
			m, _ = m.Update(resourcesLoadedMsg{res: a.res, ns: a.namespace, tbl: fakeTable()})
			mustRender(t, m, themeName, size)

			// Walk through key-driven states.
			for _, seq := range []string{"?", "esc", ":", "po", "esc", "ctrl+k", "esc", "/", "api", "esc", "tab", "j", "k", "left", "right", "h", "l", "j", "k", "g", "G", "w", "S", "esc", "a", "a"} {
				m, _ = m.Update(mkKey(seq))
				mustRender(t, m, themeName, size)
			}

			// Exercise applied sorting (set, flip direction, clear) and the
			// header arrow.
			a = m.(App)
			a.screen = screenTable
			for _, ci := range []int{1, 1, 5, -1} {
				a.table.setSort(ci)
				mustRender(t, a, themeName, size)
			}
			m = a

			// Force the detail and logs screens directly.
			a = m.(App)
			a.screen = screenDetail
			a.detail.setYAML("default/api-7d9", "apiVersion: v1\nkind: Pod\nmetadata:\n  name: api-7d9\n")
			mustRender(t, a, themeName, size)

			a.screen = screenLogs
			a.logs.appendLine("hello world")
			a.logs.appendLine("another line")
			mustRender(t, a, themeName, size)

			// Confirm overlay.
			a.screen = screenTable
			a.overlay = overlayConfirm
			a.confirm = confirmView{th: th, title: "Delete Pod", message: "Delete default/api-7d9 ?", danger: true}
			mustRender(t, a, themeName, size)

			// Embedded terminal overlay: build an emulator with content, no exec.
			a.overlay = overlayTerm
			a.term = newTermView(th)
			cols, rows := termDims(size[0], a.bodyH())
			em := vt.NewSafeEmulator(cols, rows)
			a.term.em = em
			a.term.cols, a.term.rows = cols, rows
			a.term.title = "api-7d9 › app"
			em.Write([]byte("$ echo hello\r\nhello\r\n"))
			mustRender(t, a, themeName, size)
			a.term = newTermView(th) // drop the emulator reference

			// Context switch: the sidebar must rebuild AND be sized so it does
			// not render blank until the next resize.
			a.overlay = overlayNone
			switched, _ := a.Update(clientReadyMsg{client: cl})
			mustRender(t, switched, themeName, size)
			if sa, ok := switched.(App); ok && sa.sidebarVisible() && sa.sidebar.height <= 0 {
				t.Fatalf("sidebar not sized after context switch theme=%s size=%v", themeName, size)
			}

			m = switched
		}
	}
}

func TestPaneRenderingFitsShortBody(t *testing.T) {
	th := PickTheme("ansi")
	app := App{theme: th}

	for _, h := range []int{1, 2, 3, 5} {
		out := app.renderPane(th.PaneActive, strings.Repeat("pod-name-", 12), 20, h)
		if lines := strings.Count(out, "\n") + 1; lines != h {
			t.Fatalf("renderPane height=%d rendered %d lines, want %d:\n%s", h, lines, h, out)
		}
		for _, ln := range strings.Split(out, "\n") {
			if w := lipgloss.Width(ln); w > 20 {
				t.Fatalf("renderPane height=%d line width %d exceeds 20: %q", h, w, ln)
			}
		}
	}
}

func TestSpreadTruncatesLongLeftSide(t *testing.T) {
	left := PickTheme("ansi").ModalTitle.Render("pod/" + strings.Repeat("api-", 20))
	right := "100%"
	out := spread(left, right, 24)
	if w := lipgloss.Width(out); w > 24 {
		t.Fatalf("spread width = %d, want <= 24: %q", w, out)
	}
	if !strings.Contains(out, right) {
		t.Fatalf("spread dropped right side %q: %q", right, out)
	}
}

func mustRender(t *testing.T, m tea.Model, theme string, size [2]int) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic rendering theme=%s size=%v: %v", theme, size, r)
		}
	}()
	out := m.View()
	if out == "" {
		t.Fatalf("empty view theme=%s size=%v", theme, size)
	}
	// The layout invariant: header + body + footer must total exactly the
	// terminal height, so no overlay can push the footer off-screen or wrap.
	if h := size[1]; h >= 3 {
		if lines := strings.Count(out, "\n") + 1; lines != h {
			t.Fatalf("view is %d lines, want %d (theme=%s size=%v)", lines, h, theme, size)
		}
		for _, ln := range strings.Split(out, "\n") {
			if w := lipgloss.Width(ln); w > size[0] {
				t.Fatalf("line width %d exceeds terminal %d (theme=%s size=%v)", w, size[0], theme, size)
			}
		}
	}
}
