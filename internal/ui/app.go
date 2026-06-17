package ui

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"github.com/bjarneo/ku/internal/k8s"
)

type screen int

const (
	screenTable screen = iota
	screenConfig
	screenDetail
	screenLogs
	screenCockpit
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlaySelector
	overlayHelp
	overlayConfirm
	overlayTerm
	overlayCommand
)

type focusKind int

const (
	focusMain focusKind = iota
	focusSidebar
)

const (
	headerHeight   = 1
	footerHeight   = 1
	minSidebar     = 60 // hide the sidebar below this terminal width
	mouseWheelRows = 3
)

// target identifies a single object an action operates on.
type target struct {
	res  k8s.ResourceInfo
	ns   string
	name string
}

// App is the root Bubble Tea model.
type App struct {
	client *k8s.Client
	theme  Theme
	keys   keyMap
	navCat []navCatGroup // sidebar catalog (from config or built-in defaults)

	crds     []k8s.ResourceInfo // CRDs surfaced by the sidebar discovery button
	crdState crdState

	// Startup splash: the program shows the logo + spinner while the cluster
	// connection and config load run in the background (startupCmd). opts/saved
	// are kept so the load result can be applied; startErr surfaces a fatal
	// connection error to Run after the program exits.
	splash   bool
	opts     Options
	saved    savedState
	hasSaved bool
	startErr error

	width, height int // usable area, inside the outer gutter
	gutter        int // equal padding (cells) on every side

	res       k8s.ResourceInfo
	namespace string // "" means all namespaces
	lastNS    string // remembered specific namespace for the all-ns toggle

	screen  screen
	focus   focusKind
	sidebar sidebar
	cockpit cockpitView
	table   tableView
	config  configView
	detail  detailView
	logs    logView

	cockpitAt time.Time // last cockpit refresh, for throttling

	overlay overlayKind
	sel     selector
	help    helpView
	confirm confirmView
	term    termView
	command commandView

	termSession int

	// pending action context
	scaleTarget  target
	configTarget target
	detailTarget target
	logTarget    target
	execTarget   target

	logSession int
	loadSeq    int

	spin      spinner.Model
	loading   bool
	status    string
	statusErr bool
}

func newSpinner(th Theme) spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = th.Spinner
	return sp
}

// NewApp builds the root model for a connected client.
func NewApp(cl *k8s.Client, th Theme, navCat []navCatGroup) App {
	a := App{theme: th, keys: defaultKeys()}
	a.spin = newSpinner(th)
	a.connect(cl, navCat)
	return a
}

// connect wires up the views and initial state for a freshly connected client.
// It is used both by NewApp and by the splash screen once startup finishes; the
// spinner is left untouched so its tick loop carries over.
func (a *App) connect(cl *k8s.Client, navCat []navCatGroup) {
	a.client = cl
	a.navCat = navCat
	a.sidebar = newSidebar(a.theme, cl.Registry(), navCat, a.crds, a.crdState)
	a.cockpit = newCockpitView(a.theme)
	a.table = newTableView(a.theme)
	a.config = newConfigView(a.theme)
	a.detail = newDetailView(a.theme)
	a.logs = newLogView(a.theme)
	a.sel = newSelector(a.theme)
	a.help = newHelpView(a.theme, a.keys)
	a.term = newTermView(a.theme)
	a.command = newCommandView(a.theme)

	if ri, ok := cl.Registry().Resolve("pods"); ok {
		a.res = ri
	} else if all := cl.Registry().All(); len(all) > 0 {
		a.res = all[0]
	}
	// The cluster overview (cockpit) is the default landing screen.
	a.screen = screenCockpit
	a.sidebar.syncTo(overviewKey)
	a.namespace = cl.Namespace
	a.lastNS = cl.Namespace
	if a.lastNS == "" {
		a.lastNS = "default"
	}
	if cl.DiscoveryWarning != "" {
		a.setStatus(cl.DiscoveryWarning, true)
	}
}

// adoptStartup applies the background startup result, leaving the splash for the
// cockpit. A fatal connection error is surfaced to Run via startErr and quits.
func (a App) adoptStartup(m startupReadyMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		a.startErr = m.err
		return a, tea.Quit
	}
	a.splash = false
	a.connect(m.client, m.catalog)
	if m.cfgErr != nil {
		a.setStatus("config: "+m.cfgErr.Error()+"; using defaults", true)
	}
	switch {
	case a.opts.Namespace != "":
		a.namespace = a.opts.Namespace
		a.lastNS = a.opts.Namespace
	case a.hasSaved && a.saved.Context == m.client.ContextName:
		// Only restore the namespace if we connected to the saved context.
		a.namespace = a.saved.Namespace
		if a.saved.Namespace != "" {
			a.lastNS = a.saved.Namespace
		}
	}
	if a.opts.Resource != "" {
		if ri, ok := m.client.Registry().Resolve(a.opts.Resource); ok {
			a.useResource(ri)
		}
	}
	a.relayout()
	a.loading = true // keep the spinner running through the first load
	return a, tea.Batch(tickCmd(), a.loadCmd())
}

func (a App) Init() tea.Cmd {
	if a.splash {
		// Animate the spinner while the cluster connection and config load run in
		// the background.
		return tea.Batch(a.spin.Tick, startupCmd(a.opts, a.saved, a.hasSaved))
	}
	return tea.Batch(a.spin.Tick, tickCmd(), a.loadCmd())
}

// loadCmd fetches the data for the current screen: the cockpit overview, or the
// active resource's table.
func (a App) loadCmd() tea.Cmd {
	if a.screen == screenCockpit {
		return loadCockpitCmd(a.client, a.loadSeq)
	}
	return loadResourceCmd(a.client, a.loadSeq, a.res, a.namespace)
}

func (a App) bodyH() int {
	h := a.height - headerHeight - footerHeight
	if h < 1 {
		return 1
	}
	return h
}

func (a App) sidebarVisible() bool {
	return a.width >= minSidebar && len(a.sidebar.selectable) > 0
}

func (a App) sidebarWidth() int {
	return clamp(a.width/5, 18, 28)
}

func (a *App) relayout() {
	bh := a.bodyH()
	if a.sidebarVisible() {
		sw := a.sidebarWidth()
		a.sidebar.setSize(paneContentWidth(sw), paneContentHeight(bh))
		mainW := a.width - sw - paneGap
		a.table.setSize(paneContentWidth(mainW), paneContentHeight(bh))
	} else {
		a.table.setSize(paneContentWidth(a.width), paneContentHeight(bh))
	}
	a.config.setSize(paneContentWidth(a.width), paneContentHeight(bh))
	a.detail.setSize(paneContentWidth(a.width), paneContentHeight(bh))
	a.logs.setSize(paneContentWidth(a.width), paneContentHeight(bh))
}

func (a *App) setStatus(text string, isErr bool) {
	a.status = text
	a.statusErr = isErr
}

func (a *App) clearStatus() {
	a.status = ""
	a.statusErr = false
}

// --- Update -----------------------------------------------------------------

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve an equal gutter on every side, then let all layout work against
		// the reduced area; View pads it back. Skipped on small terminals where
		// the space is too precious.
		a.gutter = 1
		if m.Width < 24 || m.Height < 10 {
			a.gutter = 0
		}
		a.width = m.Width - 2*a.gutter
		a.height = m.Height - 2*a.gutter
		if a.splash {
			return a, nil // no panes to lay out yet
		}
		if !a.sidebarVisible() {
			a.focus = focusMain
		}
		a.relayout()
		if a.overlay == overlayTerm {
			a.term.setSize(a.width, a.bodyH())
		}
		return a, nil

	case tea.KeyMsg:
		// Ignore input until the cluster is connected, except ctrl+c, which
		// handleKey handles uniformly.
		if a.splash && m.String() != "ctrl+c" {
			return a, nil
		}
		return a.handleKey(m)

	case tea.MouseMsg:
		return a.handleMouse(m)

	case startupReadyMsg:
		return a.adoptStartup(m)

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spin, cmd = a.spin.Update(m)
		// Keep the tick loop alive during the startup splash and any data load.
		if a.splash || a.loading {
			return a, cmd
		}
		return a, nil

	case tickMsg:
		cmds := []tea.Cmd{tickCmd()}
		switch {
		case a.screen == screenTable && a.overlay == overlayNone && !a.loading:
			a.loading = true
			a.loadSeq++
			cmds = append(cmds, loadResourceCmd(a.client, a.loadSeq, a.res, a.namespace), a.spin.Tick)
		case a.screen == screenCockpit && a.overlay == overlayNone && !a.loading &&
			time.Time(m).Sub(a.cockpitAt) >= 5*time.Second:
			// The cockpit aggregates many lists, so refresh it less often.
			a.loading = true
			a.cockpitAt = time.Time(m)
			a.loadSeq++
			cmds = append(cmds, loadCockpitCmd(a.client, a.loadSeq), a.spin.Tick)
		}
		return a, tea.Batch(cmds...)

	case cockpitLoadedMsg:
		if m.client != a.client || m.seq != a.loadSeq {
			return a, nil
		}
		a.loading = false
		if m.err != nil {
			a.setStatus(trimErr(m.err), true)
		} else {
			a.cockpit.setData(m.overview)
			a.clearStatus()
		}
		return a, nil

	case resourcesLoadedMsg:
		if m.client == a.client && m.seq == a.loadSeq && m.res.Key() == a.res.Key() && m.ns == a.namespace {
			a.loading = false
			if m.err != nil {
				a.setStatus(trimErr(m.err), true)
			} else {
				a.table.setData(m.tbl)
				a.clearStatus()
			}
		}
		return a, nil

	case detailLoadedMsg:
		// Ignore a stale fetch: only apply if it matches the object currently
		// being viewed (the user may have navigated away and back to another).
		if m.client == a.client && m.seq == a.loadSeq && a.screen == screenDetail &&
			m.res.Key() == a.detailTarget.res.Key() &&
			m.ns == a.detailTarget.ns && m.name == a.detailTarget.name {
			if m.err != nil {
				a.detail.setMessage(m.title, "Error: "+m.err.Error())
			} else {
				a.detail.setYAML(m.title, m.yaml)
			}
		}
		return a, nil

	case configLoadedMsg:
		// Ignore stale fetches if the user moved to another object before this
		// response arrived.
		if m.client == a.client && m.seq == a.loadSeq && a.screen == screenConfig &&
			m.res.Key() == a.configTarget.res.Key() &&
			m.ns == a.configTarget.ns && m.name == a.configTarget.name {
			if m.err != nil {
				a.config.setMessage(m.title, "Error: "+m.err.Error())
			} else {
				a.config.setObject(m.res, m.title, m.obj, m.usage)
			}
		}
		return a, nil

	case namespacesMsg:
		if a.overlay == overlaySelector && a.sel.kind == selNamespace {
			if m.err != nil {
				a.setStatus(trimErr(m.err), true)
				a.overlay = overlayNone
				return a, nil
			}
			items := []selItem{{title: "all namespaces", desc: "*", id: "*"}}
			for _, n := range m.names {
				items = append(items, selItem{title: n, id: n})
			}
			a.sel.setItems(items)
		}
		return a, nil

	case containersMsg:
		return a.handleContainers(m)

	case nodeDebugReadyMsg:
		if m.client != a.client {
			if m.pod != "" {
				a.setStatus("node shell cancelled after context switch", false)
				return a, deletePodCmd(m.client, m.ns, m.pod)
			}
			return a, nil
		}
		if m.err != nil {
			a.setStatus("node shell: "+trimErr(m.err), true)
			return a, nil
		}
		return a.startNodeExec(m)

	case actionDoneMsg:
		if m.err != nil {
			a.setStatus(trimErr(m.err), true)
			return a, nil
		}
		a.setStatus(m.text, false)
		if m.reload {
			return a.reload()
		}
		return a, nil

	case clientReadyMsg:
		if m.err != nil {
			a.setStatus(trimErr(m.err), true)
			return a, nil
		}
		return a.adoptClient(m.client)

	case crdsDiscoveredMsg:
		if m.client != a.client {
			return a, nil // a context switch landed first; ignore the stale result
		}
		a.client.Registry().Merge(m.crds) // make them searchable via the : jump
		a.crds = m.crds
		a.crdState = crdReady
		a.rebuildSidebar()
		if len(m.crds) == 0 {
			a.setStatus("no CRDs found", false)
		} else {
			a.setStatus("discovered "+itoa(len(m.crds))+" CRDs", false)
		}
		return a, nil

	case editReadyMsg:
		if m.client != a.client {
			if m.path != "" {
				os.Remove(m.path)
			}
			return a, nil
		}
		if m.err != nil {
			a.setStatus("edit: "+trimErr(m.err), true)
			return a, nil
		}
		return a.startEdit(m)

	case termTickMsg:
		if a.overlay == overlayTerm && m.session == a.termSession && !a.term.finished {
			return a, termTick(m.session)
		}
		return a, nil

	case termDoneMsg:
		return a.handleTermDone(m)

	case logEvent:
		if m.session != a.logSession || a.screen != screenLogs {
			return a, nil
		}
		// Apply this event plus any others already buffered, so a burst (notably
		// the initial tail) costs one viewport sync and render instead of one per
		// line.
		a.applyLogEvent(m)
	drain:
		for n := 0; n < logBatchMax; n++ {
			select {
			case ev := <-a.logs.ch:
				if ev.session == a.logSession {
					a.applyLogEvent(ev)
				}
			default:
				break drain
			}
		}
		a.logs.syncViewport()
		if a.logs.streams <= 0 {
			return a, nil // every stream ended; stop waiting
		}
		return a, waitForLog(a.logs.ch)

	case deploymentLogsMsg:
		return a.handleDeploymentLogs(m)

	case statusMsg:
		a.setStatus(m.text, m.err)
		return a, nil
	}

	return a.routeAux(msg)
}

// --- mouse routing ----------------------------------------------------------

func (a App) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if a.overlay != overlayNone {
		return a, nil
	}
	mouse := msg.Mouse()
	x, bodyY, ok := a.bodyMousePos(mouse)
	if !ok {
		return a, nil
	}

	if _, ok := msg.(tea.MouseWheelMsg); ok && (mouse.Button == tea.MouseWheelUp || mouse.Button == tea.MouseWheelDown) {
		return a.handleMouseWheel(x, bodyY, mouse.Button)
	}
	if _, ok := msg.(tea.MouseClickMsg); !ok || mouse.Button != tea.MouseLeft {
		return a, nil
	}

	if a.sidebarVisible() {
		if _, y, ok := a.sidebarMousePos(x, bodyY); ok {
			a.focus = focusSidebar
			if e, ok := a.sidebar.selectAt(y); ok {
				return a.openNavEntry(e)
			}
			return a, nil
		}
	}

	switch a.screen {
	case screenTable:
		return a.handleTableClick(x, bodyY)
	case screenConfig, screenDetail, screenLogs:
		if _, _, ok := a.fullPaneMousePos(x, bodyY); ok {
			a.focus = focusMain
		}
	}
	return a, nil
}

func (a App) bodyMousePos(msg tea.Mouse) (int, int, bool) {
	x := msg.X - a.gutter
	y := msg.Y - a.gutter
	if x < 0 || y < headerHeight || x >= a.width || y >= a.height-footerHeight {
		return 0, 0, false
	}
	return x, y - headerHeight, true
}

func (a App) paneMousePos(outerX, outerW, x, bodyY int) (int, int, bool) {
	if outerW < 5 || a.bodyH() < 3 {
		return 0, 0, false
	}
	cx := x - (outerX + 1 + panePaddingX)
	cy := bodyY - (1 + panePaddingY)
	if cx < 0 || cy < 0 || cx >= paneContentWidth(outerW) || cy >= paneContentHeight(a.bodyH()) {
		return 0, 0, false
	}
	return cx, cy, true
}

func (a App) sidebarMousePos(x, bodyY int) (int, int, bool) {
	return a.paneMousePos(0, a.sidebarWidth(), x, bodyY)
}

func (a App) tableMousePos(x, bodyY int) (int, int, bool) {
	outerX, outerW := 0, a.width
	if a.sidebarVisible() {
		outerX = a.sidebarWidth() + paneGap
		outerW = a.width - outerX
	}
	return a.paneMousePos(outerX, outerW, x, bodyY)
}

func (a App) fullPaneMousePos(x, bodyY int) (int, int, bool) {
	return a.paneMousePos(0, a.width, x, bodyY)
}

func (a App) handleTableClick(x, bodyY int) (tea.Model, tea.Cmd) {
	if a.table.filtering {
		return a, nil
	}
	cx, cy, ok := a.tableMousePos(x, bodyY)
	if !ok {
		return a, nil
	}
	a.focus = focusMain
	if cy == 0 {
		if ci, ok := a.table.colAt(cx); ok {
			a.table.setSort(ci)
		}
		return a, nil
	}
	if row, ok := a.table.rowAt(cy); ok {
		a.table.setCursor(row)
	}
	return a, nil
}

func (a App) handleMouseWheel(x, bodyY int, button tea.MouseButton) (tea.Model, tea.Cmd) {
	delta := mouseWheelRows
	if button == tea.MouseWheelUp {
		delta = -delta
	}
	if a.sidebarVisible() {
		if _, _, ok := a.sidebarMousePos(x, bodyY); ok {
			a.focus = focusSidebar
			a.sidebar.move(delta)
			return a, nil
		}
	}
	switch a.screen {
	case screenTable:
		if _, _, ok := a.tableMousePos(x, bodyY); ok && !a.table.filtering {
			a.focus = focusMain
			a.table.moveCursor(delta)
		}
	case screenConfig:
		if _, _, ok := a.fullPaneMousePos(x, bodyY); ok {
			scrollViewport(&a.config.vp, delta)
		}
	case screenDetail:
		if _, _, ok := a.fullPaneMousePos(x, bodyY); ok {
			scrollViewport(&a.detail.vp, delta)
		}
	case screenLogs:
		if _, _, ok := a.fullPaneMousePos(x, bodyY); ok {
			a.logs.follow = false
			scrollViewport(&a.logs.vp, delta)
		}
	}
	return a, nil
}

func scrollViewport(vp interface {
	ScrollUp(int)
	ScrollDown(int)
}, delta int) {
	if delta < 0 {
		vp.ScrollUp(-delta)
		return
	}
	vp.ScrollDown(delta)
}

// routeAux forwards auxiliary messages (e.g. cursor blink) to the focused input.
func (a App) routeAux(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.overlay == overlaySelector {
		var cmd tea.Cmd
		a.sel, _, cmd = a.sel.Update(msg)
		return a, cmd
	}
	if a.screen == screenTable && a.table.filtering {
		var cmd tea.Cmd
		a.table, cmd = a.table.Update(msg)
		return a, cmd
	}
	if a.screen == screenLogs && a.logs.filtering {
		var cmd tea.Cmd
		a.logs, cmd = a.logs.Update(msg)
		return a, cmd
	}
	return a, nil
}

// filterInput reports whether a text filter is capturing keystrokes, so global
// single-key actions (command, docs) don't fire while the user is typing.
func (a App) filterInput() bool {
	return (a.screen == screenTable && a.table.filtering) ||
		(a.screen == screenLogs && (a.logs.filtering || a.logs.selecting))
}

// --- key routing ------------------------------------------------------------

func (a App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The embedded terminal captures every key (incl. ctrl+c) except detach.
	if a.overlay == overlayTerm {
		return a.updateTerm(msg)
	}

	if msg.String() == "ctrl+c" {
		a.logs.stop()
		return a, tea.Quit
	}

	switch a.overlay {
	case overlaySelector:
		return a.updateSelector(msg)
	case overlayHelp:
		if key.Matches(msg, a.keys.Help, a.keys.Back) || msg.String() == "q" {
			a.overlay = overlayNone
			return a, nil
		}
		a.help = a.help.Update(msg)
		return a, nil
	case overlayConfirm:
		return a.updateConfirm(msg)
	case overlayCommand:
		if key.Matches(msg, a.keys.Back) || key.Matches(msg, a.keys.Command) || msg.String() == "q" {
			a.overlay = overlayNone
		}
		return a, nil
	}

	// The logs screen rebinds C to "clear", so don't let the global kubectl
	// command shortcut shadow it there.
	if !a.filterInput() && a.screen != screenLogs && key.Matches(msg, a.keys.Command) {
		return a.openCommand()
	}
	if !a.filterInput() && key.Matches(msg, a.keys.Docs) {
		return a.openDocs()
	}

	switch a.screen {
	case screenConfig:
		return a.updateConfig(msg)
	case screenDetail:
		return a.updateDetail(msg)
	case screenLogs:
		return a.updateLogs(msg)
	case screenCockpit:
		return a.updateCockpit(msg)
	default:
		return a.updateTable(msg)
	}
}

func (a App) updateCockpit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Quit):
		a.logs.stop()
		return a, tea.Quit
	case key.Matches(msg, a.keys.Help):
		a.help.reset()
		a.overlay = overlayHelp
		return a, nil
	case key.Matches(msg, a.keys.Palette):
		return a.openPalette()
	case key.Matches(msg, a.keys.Jump):
		return a.openResourceJump()
	case key.Matches(msg, a.keys.Namespace):
		return a.openNamespacePicker()
	case key.Matches(msg, a.keys.Context):
		return a.openContextPicker()
	case key.Matches(msg, a.keys.Refresh):
		return a.reloadCockpit()
	case key.Matches(msg, a.keys.Focus):
		if a.sidebarVisible() {
			a.focus = toggleFocus(a.focus)
		}
		return a, nil
	}
	if a.focus == focusSidebar {
		return a.updateSidebarKeys(msg)
	}
	return a, nil
}

func (a App) updateTable(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Filtering captures all typing until it ends.
	if a.table.filtering {
		switch {
		case key.Matches(msg, a.keys.Back):
			a.table.stopFilter(true)
			return a, nil
		case msg.String() == "enter":
			a.table.stopFilter(false)
			return a, nil
		default:
			var cmd tea.Cmd
			a.table, cmd = a.table.Update(msg)
			return a, cmd
		}
	}

	// Global keys, available regardless of which pane has focus.
	switch {
	case key.Matches(msg, a.keys.Quit):
		a.logs.stop()
		return a, tea.Quit
	case key.Matches(msg, a.keys.Back):
		// esc clears an applied filter; otherwise it is a no-op on the table.
		if a.table.filterActive() {
			a.table.stopFilter(true)
			a.setStatus("filter cleared", false)
		}
		return a, nil
	case key.Matches(msg, a.keys.Help):
		a.help.reset()
		a.overlay = overlayHelp
		return a, nil
	case key.Matches(msg, a.keys.Palette):
		return a.openPalette()
	case key.Matches(msg, a.keys.Jump):
		return a.openResourceJump()
	case key.Matches(msg, a.keys.Namespace):
		return a.openNamespacePicker()
	case key.Matches(msg, a.keys.Context):
		return a.openContextPicker()
	case key.Matches(msg, a.keys.AllNS):
		return a.toggleAllNS()
	case key.Matches(msg, a.keys.Refresh):
		return a.reload()
	case key.Matches(msg, a.keys.Wide):
		// Affects the always-visible table, so it works from either pane.
		a.table.toggleWide()
		return a, nil
	case key.Matches(msg, a.keys.Focus):
		if a.sidebarVisible() {
			a.focus = toggleFocus(a.focus)
		}
		return a, nil
	case key.Matches(msg, a.keys.Filter):
		a.focus = focusMain
		a.table.startFilter()
		return a, nil
	}

	if a.focus == focusSidebar {
		return a.updateSidebarKeys(msg)
	}
	return a.updateMainKeys(msg)
}

func (a App) updateSidebarKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Up):
		a.sidebar.moveUp()
		return a, nil
	case key.Matches(msg, a.keys.Down):
		a.sidebar.moveDown()
		return a, nil
	case key.Matches(msg, a.keys.Top):
		a.sidebar.moveTop()
		return a, nil
	case key.Matches(msg, a.keys.Bottom):
		a.sidebar.moveBottom()
		return a, nil
	case key.Matches(msg, a.keys.PageUp, a.keys.HalfUp):
		a.sidebar.move(-5)
		return a, nil
	case key.Matches(msg, a.keys.PageDown, a.keys.HalfDown):
		a.sidebar.move(5)
		return a, nil
	case msg.String() == "enter" || msg.String() == "right" || msg.String() == "l":
		if e, ok := a.sidebar.current(); ok {
			return a.openNavEntry(e)
		}
		return a, nil
	}
	return a, nil
}

func (a App) updateMainKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "left":
		// Scroll the table left first; only jump to the sidebar once it is already
		// at its leftmost column, so wide tables stay reachable on small screens.
		if a.table.scrollLeft() {
			return a, nil
		}
		if a.sidebarVisible() {
			a.focus = focusSidebar
		}
		return a, nil
	case msg.String() == "h":
		// h keeps its original meaning (focus the sidebar); only the arrows scroll.
		if a.sidebarVisible() {
			a.focus = focusSidebar
		}
		return a, nil
	case msg.String() == "right":
		// `l` is the Logs shortcut, so only the right arrow scrolls columns.
		a.table.scrollRight()
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		return a.openConfig()
	case key.Matches(msg, a.keys.Describe, a.keys.YAML):
		return a.openDetail()
	case key.Matches(msg, a.keys.Logs):
		return a.openLogs()
	case key.Matches(msg, a.keys.DeployLogs):
		return a.openDeploymentLogs()
	case key.Matches(msg, a.keys.Edit):
		return a.openEdit()
	case key.Matches(msg, a.keys.Shell):
		return a.openShellOrScale()
	case key.Matches(msg, a.keys.Sort):
		return a.openSort()
	case key.Matches(msg, a.keys.Restart):
		return a.openRestart()
	case key.Matches(msg, a.keys.Trigger):
		return a.openTriggerJob()
	case key.Matches(msg, a.keys.Delete):
		return a.openDelete()
	default:
		var cmd tea.Cmd
		a.table, cmd = a.table.Update(msg)
		return a, cmd
	}
}

func (a App) updateConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Back) || msg.String() == "q":
		a.screen = screenTable
		return a, nil
	case key.Matches(msg, a.keys.Describe, a.keys.YAML):
		return a.openDetailTarget(a.configTarget)
	case key.Matches(msg, a.keys.Edit):
		return a.editTarget(a.configTarget)
	case key.Matches(msg, a.keys.DeployLogs):
		return a.openDeploymentLogsTarget(a.configTarget)
	case key.Matches(msg, a.keys.Trigger):
		return a.openTriggerJobTarget(a.configTarget)
	case key.Matches(msg, a.keys.Top):
		a.config.vp.GotoTop()
		return a, nil
	case key.Matches(msg, a.keys.Bottom):
		a.config.vp.GotoBottom()
		return a, nil
	default:
		var cmd tea.Cmd
		a.config, cmd = a.config.Update(msg)
		return a, cmd
	}
}

func (a App) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Back) || msg.String() == "q":
		a.screen = screenTable
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		return a.openConfigTarget(a.detailTarget)
	case key.Matches(msg, a.keys.Edit):
		return a.editTarget(a.detailTarget)
	case key.Matches(msg, a.keys.DeployLogs):
		return a.openDeploymentLogsTarget(a.detailTarget)
	case key.Matches(msg, a.keys.Trigger):
		return a.openTriggerJobTarget(a.detailTarget)
	case key.Matches(msg, a.keys.Top):
		a.detail.vp.GotoTop()
		return a, nil
	case key.Matches(msg, a.keys.Bottom):
		a.detail.vp.GotoBottom()
		return a, nil
	default:
		var cmd tea.Cmd
		a.detail, cmd = a.detail.Update(msg)
		return a, cmd
	}
}

func (a App) updateLogs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Visual selection captures movement and copy keys until it ends.
	if a.logs.selecting {
		switch {
		case key.Matches(msg, a.keys.Back):
			a.logs.stopSelect()
		case key.Matches(msg, a.keys.Mark):
			a.logs.mark()
		case key.Matches(msg, a.keys.Copy) || msg.String() == "enter":
			text, n := a.logs.copySelection(), a.logs.selCount()
			a.logs.stopSelect()
			if text == "" {
				return a, nil
			}
			a.setStatus("copied "+itoa(n)+" lines to clipboard", false)
			return a, tea.SetClipboard(text)
		case key.Matches(msg, a.keys.Up):
			a.logs.moveSel(-1)
		case key.Matches(msg, a.keys.Down):
			a.logs.moveSel(1)
		case key.Matches(msg, a.keys.PageUp):
			a.logs.moveSel(-a.logs.vp.Height())
		case key.Matches(msg, a.keys.PageDown):
			a.logs.moveSel(a.logs.vp.Height())
		case key.Matches(msg, a.keys.HalfUp):
			a.logs.moveSel(-a.logs.vp.Height() / 2)
		case key.Matches(msg, a.keys.HalfDown):
			a.logs.moveSel(a.logs.vp.Height() / 2)
		case key.Matches(msg, a.keys.Top):
			a.logs.setSelCursor(0)
		case key.Matches(msg, a.keys.Bottom):
			a.logs.setSelCursor(len(a.logs.selLines) - 1)
		}
		return a, nil
	}
	// ctrl+w toggles wrap in any state, including while filtering where plain w
	// is filter text.
	if msg.String() == "ctrl+w" {
		a.logs.toggleWrap()
		return a, nil
	}
	// Filtering captures all typing until it ends.
	if a.logs.filtering {
		switch {
		case key.Matches(msg, a.keys.Back):
			a.logs.stopFilter(true)
			return a, nil
		case msg.String() == "enter":
			a.logs.stopFilter(false)
			return a, nil
		default:
			var cmd tea.Cmd
			a.logs, cmd = a.logs.Update(msg)
			return a, cmd
		}
	}
	switch {
	case key.Matches(msg, a.keys.Back) || msg.String() == "q":
		// esc clears an applied filter first; otherwise it leaves the logs.
		if key.Matches(msg, a.keys.Back) && a.logs.filterActive() {
			a.logs.stopFilter(true)
			return a, nil
		}
		a.logs.stop()
		a.logSession++
		a.screen = screenTable
		return a, nil
	case key.Matches(msg, a.keys.Filter):
		a.logs.startFilter()
		return a, nil
	case key.Matches(msg, a.keys.Wrap):
		a.logs.toggleWrap()
		return a, nil
	case key.Matches(msg, a.keys.CopyAll):
		text := a.logs.copyAll()
		if text == "" {
			return a, nil
		}
		a.setStatus("copied "+itoa(len(a.logs.lines))+" lines to clipboard", false)
		return a, tea.SetClipboard(text)
	case key.Matches(msg, a.keys.Clear):
		a.logs.clear()
		a.setStatus("cleared logs", false)
		return a, nil
	case key.Matches(msg, a.keys.Select):
		a.logs.startSelect()
		return a, nil
	case key.Matches(msg, a.keys.Follow):
		a.logs.follow = !a.logs.follow
		a.logs.stickToBottom()
		return a, nil
	case key.Matches(msg, a.keys.Top):
		a.logs.follow = false
		a.logs.vp.GotoTop()
		return a, nil
	case key.Matches(msg, a.keys.Bottom):
		a.logs.vp.GotoBottom()
		return a, nil
	default:
		var cmd tea.Cmd
		a.logs, cmd = a.logs.Update(msg)
		return a, cmd
	}
}

func (a App) updateSelector(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var res selResult
	var cmd tea.Cmd
	a.sel, res, cmd = a.sel.Update(msg)
	switch {
	case res.canceled:
		a.overlay = overlayNone
		return a, nil
	case res.accepted:
		a.overlay = overlayNone
		return a.applySelection(res)
	}
	return a, cmd
}

func (a App) updateTerm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// After the shell exits, any key dismisses the panel.
	if a.term.finished {
		a.term.stop()
		a.overlay = overlayNone
		a.screen = screenTable
		return a, nil
	}
	// ctrl+\ detaches/cancels without killing ku; everything else goes to the
	// running program.
	if msg.String() == "ctrl+\\" {
		note := "shell detached"
		if a.term.isEdit {
			os.Remove(a.term.editPath) // cancelled: discard the unsaved edit
			note = "edit cancelled"
		}
		cleanup := a.term.onClose // e.g. delete the node debug pod
		a.term.stop()
		a.termSession++
		a.overlay = overlayNone
		a.screen = screenTable
		a.setStatus(note, false)
		return a, cleanup
	}
	if ti, ok := translateKey(msg); ok && a.term.input != nil {
		select {
		case a.term.input <- ti:
		default: // drop if the input buffer is somehow saturated
		}
	}
	return a, nil
}

func (a App) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		a.overlay = overlayNone
		return a, a.confirm.action
	case "n", "N", "esc":
		a.overlay = overlayNone
		return a, a.confirm.cancel
	}
	return a, nil
}

// --- actions ----------------------------------------------------------------

func (a App) reload() (tea.Model, tea.Cmd) {
	a.loading = true
	a.loadSeq++
	return a, tea.Batch(loadResourceCmd(a.client, a.loadSeq, a.res, a.namespace), a.spin.Tick)
}

func (a App) switchResource(ri k8s.ResourceInfo) (tea.Model, tea.Cmd) {
	a.useResource(ri)
	return a.reload()
}

func (a *App) useResource(ri k8s.ResourceInfo) {
	a.res = ri
	a.screen = screenTable
	a.focus = focusMain
	a.sidebar.syncTo(ri.Key())
	a.table.stopFilter(true)
	a.table.resetSort()    // columns differ per resource
	a.table.resetHScroll() // restart horizontal scroll from the left
	a.table.setData(nil)
}

func (a App) switchToCockpit() (tea.Model, tea.Cmd) {
	a.screen = screenCockpit
	a.focus = focusMain
	a.sidebar.syncTo(overviewKey)
	return a.reloadCockpit()
}

func (a App) reloadCockpit() (tea.Model, tea.Cmd) {
	a.loading = true
	a.loadSeq++
	return a, tea.Batch(loadCockpitCmd(a.client, a.loadSeq), a.spin.Tick)
}

func (a App) openNavEntry(e navEntry) (tea.Model, tea.Cmd) {
	switch {
	case e.overview:
		return a.switchToCockpit()
	case e.discover:
		return a.discoverCRDs()
	default:
		return a.switchResource(e.res)
	}
}

// discoverCRDs kicks off CRD discovery for the current cluster. The button
// shows progress while the list call runs; the result arrives as a
// crdsDiscoveredMsg.
func (a App) discoverCRDs() (tea.Model, tea.Cmd) {
	if a.crdState == crdLoading {
		return a, nil // already in flight
	}
	a.crdState = crdLoading
	a.rebuildSidebar()
	a.setStatus("discovering CRDs…", false)
	return a, discoverCRDsCmd(a.client)
}

// rebuildSidebar rebuilds the left nav from current state, keeping the cursor on
// whatever entry was selected.
func (a *App) rebuildSidebar() {
	key := ""
	if e, ok := a.sidebar.current(); ok {
		key = e.key
	}
	a.sidebar = newSidebar(a.theme, a.client.Registry(), a.navCat, a.crds, a.crdState)
	if key != "" {
		a.sidebar.syncTo(key)
	}
	a.relayout() // a fresh sidebar starts at size 0; re-apply pane sizes
}

func (a App) openDetail() (tea.Model, tea.Cmd) {
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	return a.openDetailTarget(target{res: a.res, ns: row.Namespace, name: row.Name})
}

func (a App) openDetailTarget(t target) (tea.Model, tea.Cmd) {
	if t.name == "" {
		return a, nil
	}
	a.detailTarget = t
	a.screen = screenDetail
	a.detail.setMessage(t.name, "loading…")
	a.loadSeq++
	return a, loadDetailCmd(a.client, a.loadSeq, t.res, t.ns, t.name)
}

func (a App) openConfig() (tea.Model, tea.Cmd) {
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	return a.openConfigTarget(target{res: a.res, ns: row.Namespace, name: row.Name})
}

func (a App) openConfigTarget(t target) (tea.Model, tea.Cmd) {
	if t.name == "" {
		return a, nil
	}
	a.configTarget = t
	a.screen = screenConfig
	a.config.setMessage(t.name, "loading…")
	a.loadSeq++
	return a, loadConfigCmd(a.client, a.loadSeq, t.res, t.ns, t.name)
}

func (a App) openLogs() (tea.Model, tea.Cmd) {
	if !a.res.IsPod() {
		a.setStatus("logs: switch to pods first", true)
		return a, nil
	}
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	a.logTarget = target{ns: row.Namespace, name: row.Name}
	return a, containersCmd(a.client, row.Namespace, row.Name, false)
}

func (a App) openDeploymentLogs() (tea.Model, tea.Cmd) {
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	return a.openDeploymentLogsTarget(target{res: a.res, ns: row.Namespace, name: row.Name})
}

func (a App) openDeploymentLogsTarget(t target) (tea.Model, tea.Cmd) {
	if t.name == "" {
		return a, nil
	}
	if !t.res.IsDeployment() {
		a.setStatus("logs: switch to deployments first", true)
		return a, nil
	}
	ns := t.ns
	if ns == "" {
		ns = a.namespace
	}
	if ns == "" {
		a.setStatus("logs: deployment namespace unavailable", true)
		return a, nil
	}
	a.logTarget = target{res: t.res, ns: ns, name: t.name}
	a.setStatus("loading logs for "+qualified(ns, t.name), false)
	return a, deploymentLogsCmd(a.client, ns, t.name)
}

func (a App) openShellOrScale() (tea.Model, tea.Cmd) {
	switch {
	case a.res.IsPod():
		return a.openShell()
	case a.res.IsNodes():
		return a.openNodeShell()
	case a.res.Scalable():
		return a.openScale()
	default:
		a.setStatus("s: shell needs a pod or node, scale needs a workload", true)
		return a, nil
	}
}

func (a App) openShell() (tea.Model, tea.Cmd) {
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	a.execTarget = target{ns: row.Namespace, name: row.Name}
	return a, containersCmd(a.client, row.Namespace, row.Name, true)
}

// openNodeShell starts a node shell by spawning a privileged debug pod on the
// node (the host filesystem is mounted at /host), then exec'ing into it.
func (a App) openNodeShell() (tea.Model, tea.Cmd) {
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	ns := a.namespace
	if ns == "" {
		ns = "default"
	}
	a.setStatus("starting node shell on "+row.Name+" (creating debug pod)…", false)
	return a, createNodeDebugCmd(a.client, ns, row.Name)
}

// startNodeExec opens the terminal in the node debug pod and deletes the pod
// when the session ends.
func (a App) startNodeExec(m nodeDebugReadyMsg) (tea.Model, tea.Cmd) {
	cleanup := deletePodCmd(m.client, m.ns, m.pod)
	return a.startExec(m.client, m.ns, m.pod, m.container, "node "+m.node, k8s.NodeShellCommand, cleanup)
}

func (a App) handleContainers(m containersMsg) (tea.Model, tea.Cmd) {
	wantNS, wantPod := a.logTarget.ns, a.logTarget.name
	if m.forExec {
		wantNS, wantPod = a.execTarget.ns, a.execTarget.name
	}
	if m.ns != wantNS || m.pod != wantPod {
		return a, nil
	}
	if m.err != nil {
		a.setStatus(trimErr(m.err), true)
		return a, nil
	}
	if len(m.names) == 0 {
		a.setStatus("no containers found", true)
		return a, nil
	}
	if len(m.names) == 1 {
		if m.forExec {
			return a.startExec(a.client, m.ns, m.pod, m.names[0], "", nil, nil)
		}
		return a.startLogs(m.ns, m.pod, m.names[0])
	}

	items := make([]selItem, len(m.names))
	for i, n := range m.names {
		items[i] = selItem{title: n, id: n}
	}
	kind := selContainer
	title := "Logs — " + m.pod
	if m.forExec {
		kind = selExecContainer
		title = "Shell — " + m.pod
	}
	a.sel.open(kind, title, "container", items, false)
	a.overlay = overlaySelector
	return a, nil
}

func (a App) handleDeploymentLogs(m deploymentLogsMsg) (tea.Model, tea.Cmd) {
	if m.ns != a.logTarget.ns || m.name != a.logTarget.name || !a.logTarget.res.IsDeployment() {
		return a, nil
	}
	if m.err != nil {
		a.setStatus("logs: "+trimErr(m.err), true)
		return a, nil
	}
	if len(m.targets) == 0 {
		a.setStatus("logs: no pods found for deployment "+m.name, true)
		return a, nil
	}
	return a.startDeploymentLogs(m.ns, m.name, m.targets)
}

// applyLogEvent folds one stream event into the log view without syncing the
// viewport, so a batch of events can sync once. A line is stored, a stream error
// is surfaced, and a done event retires one stream.
func (a *App) applyLogEvent(ev logEvent) {
	switch {
	case ev.err != nil:
		a.setStatus("logs: "+trimErr(ev.err), true)
	case ev.done:
		a.logs.streams--
	default:
		a.logs.storeLine(ev.line)
	}
}

func (a App) startLogs(ns, pod, container string) (tea.Model, tea.Cmd) {
	a.logs.stop()
	a.logSession++
	sess := a.logSession

	a.logs = newLogView(a.theme)
	a.logs.session = sess
	a.logs.streams = 1
	a.logs.ns, a.logs.pod, a.logs.cont = ns, pod, container
	a.logs.title = pod + " › " + container
	a.logs.setSize(paneContentWidth(a.width), paneContentHeight(a.bodyH()))

	ch := make(chan logEvent, 256)
	a.logs.ch = ch
	ctx, cancel := context.WithCancel(context.Background())
	a.logs.cancel = cancel

	a.screen = screenLogs
	go streamLogs(ctx, a.client, ns, pod, container, "", sess, ch)
	return a, waitForLog(ch)
}

func (a App) startDeploymentLogs(ns, deployment string, targets []k8s.LogTarget) (tea.Model, tea.Cmd) {
	a.logs.stop()
	a.logSession++
	sess := a.logSession

	a.logs = newLogView(a.theme)
	a.logs.session = sess
	a.logs.streams = len(targets)
	a.logs.ns, a.logs.deploy = ns, deployment
	a.logs.title = "deployment/" + deployment + " › all logs"
	a.logs.setSize(paneContentWidth(a.width), paneContentHeight(a.bodyH()))

	ch := make(chan logEvent, 256)
	a.logs.ch = ch
	ctx, cancel := context.WithCancel(context.Background())
	a.logs.cancel = cancel

	a.screen = screenLogs
	if !a.statusErr {
		a.status = ""
	}
	for _, t := range targets {
		prefix := t.Pod + "/" + t.Container
		go streamLogs(ctx, a.client, t.Namespace, t.Pod, t.Container, prefix, sess, ch)
	}
	return a, waitForLog(ch)
}

// startExec opens the embedded-terminal overlay running command (nil = default
// shell) in a pod container. title labels the panel; onClose, if set, runs when
// the session ends (e.g. to delete a debug pod).
func (a App) startExec(cl *k8s.Client, ns, pod, container, title string, command []string, onClose tea.Cmd) (tea.Model, tea.Cmd) {
	if cl == nil {
		cl = a.client
	}
	a.term.stop()
	a.termSession++
	sess := a.termSession

	cols, rows := termDims(a.width, a.bodyH())
	em := vt.NewSafeEmulator(cols, rows)
	q := k8s.NewResizeQueue()
	q.Set(cols, rows)
	ctx, cancel := context.WithCancel(context.Background())
	result := &termResult{done: make(chan struct{})}
	input := make(chan termInput, 256)

	t := newTermView(a.theme)
	t.em = em
	t.cancel = cancel
	t.closeFn = q.Close
	t.resize = q.Set
	t.onClose = onClose
	t.result = result
	t.input = input
	t.session = sess
	t.cols, t.rows = cols, rows
	if title == "" {
		title = pod + " › " + container
	}
	t.title = title
	a.term = t
	a.overlay = overlayTerm

	go runTermInput(ctx, em, input)
	go func() {
		err := cl.ExecStream(ctx, ns, pod, container, command, em, em, q)
		em.Close() // unblock the stream's stdin reader and the input goroutine
		q.Close()
		result.err = err
		close(result.done)
	}()

	return a, tea.Batch(termTick(sess), waitTermDone(sess, result))
}

func (a App) openEdit() (tea.Model, tea.Cmd) {
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	return a.editTarget(target{res: a.res, ns: row.Namespace, name: row.Name})
}

func (a App) editTarget(t target) (tea.Model, tea.Cmd) {
	if t.name == "" {
		return a, nil
	}
	a.setStatus("opening editor…", false)
	return a, prepareEditCmd(a.client, t.res, t.ns, t.name)
}

// startEdit opens the object's YAML in $EDITOR (nvim) inside an embedded
// terminal overlay. When the editor exits, the file is applied automatically.
func (a App) startEdit(m editReadyMsg) (tea.Model, tea.Cmd) {
	a.term.stop()
	a.termSession++
	sess := a.termSession

	cols, rows := termDims(a.width, a.bodyH())
	em := vt.NewSafeEmulator(cols, rows)
	result := &termResult{done: make(chan struct{})}
	input := make(chan termInput, 256)

	bin, args := editorCommand(m.path)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		cancel()
		os.Remove(m.path)
		a.setStatus("edit: "+trimErr(err), true)
		return a, nil
	}

	t := newTermView(a.theme)
	t.em = em
	t.cancel = cancel
	t.closeFn = func() { _ = ptmx.Close() }
	t.resize = func(c, r int) { _ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(r), Cols: uint16(c)}) }
	t.result = result
	t.input = input
	t.session = sess
	t.cols, t.rows = cols, rows
	t.title = "edit " + m.name
	t.isEdit = true
	t.editPath, t.editOriginal = m.path, m.original
	t.editRes, t.editNs, t.editName = m.res, m.ns, m.name
	t.editCl = m.client // capture: applying targets the cluster we read from
	a.term = t
	a.overlay = overlayTerm

	go runTermInput(ctx, em, input)
	go func() { _, _ = io.Copy(ptmx, em) }() // keystrokes -> editor stdin
	go func() { _, _ = io.Copy(em, ptmx) }() // editor output -> screen
	go func() {
		err := cmd.Wait()
		em.Close()
		_ = ptmx.Close()
		result.err = err
		close(result.done)
	}()

	return a, tea.Batch(termTick(sess), waitTermDone(sess, result))
}

// handleTermDone processes the end of an embedded session. Exec sessions show a
// "press any key" state; edit sessions tear down and apply the file.
func (a App) handleTermDone(m termDoneMsg) (tea.Model, tea.Cmd) {
	if m.session != a.termSession {
		return a, nil
	}
	if a.term.isEdit {
		path, orig := a.term.editPath, a.term.editOriginal
		res, ns, name, cl := a.term.editRes, a.term.editNs, a.term.editName, a.term.editCl
		a.term.stop()
		a.overlay = overlayNone
		a.screen = screenTable
		return a.applyEditedFile(cl, res, ns, name, path, orig)
	}
	a.term.finished = true
	a.term.status = "session ended — press any key"
	if m.err != nil {
		a.term.status = "ended: " + trimErr(m.err)
	}
	// Run any cleanup (e.g. delete the node debug pod) now that the shell exited.
	cleanup := a.term.onClose
	a.term.onClose = nil
	return a, cleanup
}

// applyEditedFile reads the edited temp file and prompts before applying it.
func (a App) applyEditedFile(cl *k8s.Client, res k8s.ResourceInfo, ns, name, path, original string) (tea.Model, tea.Cmd) {
	data, err := os.ReadFile(path)
	if err != nil {
		os.Remove(path)
		a.setStatus(trimErr(err), true)
		return a, nil
	}
	if string(data) == original {
		os.Remove(path)
		a.setStatus("edit cancelled: no changes", false)
		return a, nil
	}
	if cl == nil {
		cl = a.client
	}
	loc := qualified(ns, name)
	a.confirm = confirmView{
		th:      a.theme,
		title:   "Apply edit",
		message: "Apply changes to " + loc + " ?",
		action:  applyEditCmd(cl, res, ns, name, path),
		cancel:  cancelEditCmd(path),
	}
	a.overlay = overlayConfirm
	a.setStatus("review edit confirmation", false)
	return a, nil
}

func (a App) openSort() (tea.Model, tea.Cmd) {
	vis := a.table.visibleCols()
	if len(vis) == 0 {
		a.setStatus("nothing to sort yet", true)
		return a, nil
	}
	items := make([]selItem, 0, len(vis)+1)
	for _, ci := range vis {
		desc := ""
		if ci == a.table.sortCol {
			desc = "active ▲"
			if a.table.sortDesc {
				desc = "active ▼"
			}
		}
		items = append(items, selItem{title: a.table.cols[ci].Name, desc: desc, id: itoa(ci)})
	}
	items = append(items, selItem{title: "Default order", id: "-1"})
	a.sel.open(selSort, "Sort by column", "column (re-pick to flip direction)", items, false)
	a.overlay = overlaySelector
	return a, nil
}

func (a App) openScale() (tea.Model, tea.Cmd) {
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	a.scaleTarget = target{res: a.res, ns: row.Namespace, name: row.Name}
	a.sel.open(selScale, "Scale "+row.Name, "replicas (number)", nil, true)
	a.overlay = overlaySelector
	return a, nil
}

func (a App) openDelete() (tea.Model, tea.Cmd) {
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	loc := qualified(row.Namespace, row.Name)
	return a.confirmAction("Delete "+a.res.Kind, "Delete "+loc+" ?", true,
		deleteCmd(a.client, a.res, row.Namespace, row.Name))
}

func (a App) openRestart() (tea.Model, tea.Cmd) {
	if !a.res.Restartable() {
		a.setStatus("restart: deployments, statefulsets, daemonsets only", true)
		return a, nil
	}
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	loc := qualified(row.Namespace, row.Name)
	return a.confirmAction("Restart "+a.res.Kind, "Rollout restart "+loc+" ?", false,
		restartCmd(a.client, a.res, row.Namespace, row.Name))
}

func (a App) openTriggerJob() (tea.Model, tea.Cmd) {
	row, ok := a.table.selected()
	if !ok {
		return a, nil
	}
	return a.openTriggerJobTarget(target{res: a.res, ns: row.Namespace, name: row.Name})
}

func (a App) openTriggerJobTarget(t target) (tea.Model, tea.Cmd) {
	if !t.res.IsCronJob() {
		a.setStatus("trigger: switch to cronjobs first", true)
		return a, nil
	}
	ns := t.ns
	if ns == "" {
		ns = a.namespace
	}
	if ns == "" {
		a.setStatus("trigger: cronjob namespace unavailable", true)
		return a, nil
	}
	loc := qualified(ns, t.name)
	return a.confirmAction("Trigger Job", "Create one-off Job from "+loc+" ?", false,
		triggerJobCmd(a.client, ns, t.name))
}

// confirmAction opens the confirm overlay for a command to run on yes.
func (a App) confirmAction(title, message string, danger bool, action tea.Cmd) (tea.Model, tea.Cmd) {
	a.confirm = confirmView{th: a.theme, title: title, message: message, danger: danger, action: action}
	a.overlay = overlayConfirm
	return a, nil
}

func (a App) openCommand() (tea.Model, tea.Cmd) {
	a.command.setCommand(a.kubectlCommand())
	a.overlay = overlayCommand
	return a, nil
}

func (a App) toggleAllNS() (tea.Model, tea.Cmd) {
	if a.namespace == "" {
		a.namespace = a.lastNS
	} else {
		a.lastNS = a.namespace
		a.namespace = ""
	}
	a.persist()
	return a.reload()
}

func (a App) adoptClient(cl *k8s.Client) (tea.Model, tea.Cmd) {
	a.logs.stop()
	a.logSession++
	a.client = cl
	// Rebuild the left nav from the new cluster's catalog: available resource
	// kinds differ between clusters, so the previous sidebar (and any CRDs
	// discovered on the old cluster) may be stale.
	a.crds = nil
	a.crdState = crdNone
	a.sidebar = newSidebar(a.theme, cl.Registry(), a.navCat, a.crds, a.crdState)
	a.namespace = cl.Namespace
	a.lastNS = cl.Namespace
	if a.lastNS == "" {
		a.lastNS = "default"
	}
	a.screen = screenTable
	a.focus = focusMain
	a.overlay = overlayNone
	a.table.stopFilter(true)
	a.table.setData(nil)
	if ri, ok := cl.Registry().Resolve(a.res.Resource); ok {
		a.res = ri
	} else if ri, ok := cl.Registry().Resolve("pods"); ok {
		a.res = ri
	}
	a.sidebar.syncTo(a.res.Key())
	// Size the freshly built sidebar/table now; otherwise it stays 0-sized and
	// renders blank until the next window resize.
	a.relayout()
	a.persist()
	a.setStatus("switched to "+shortContext(cl.ContextName), false)
	return a.reload()
}

// --- selector openers -------------------------------------------------------

func (a App) openPalette() (tea.Model, tea.Cmd) {
	var items []selItem

	// Actions on the selected row come first, so the palette is a real
	// discovery surface for what you can do right now.
	if row, ok := a.table.selected(); ok {
		items = append(items,
			selItem{title: "Open config for " + row.Name, desc: "enter", id: "act:config"},
			selItem{title: "Describe " + row.Name, desc: "d", id: "act:describe"},
			selItem{title: "View YAML", desc: "y", id: "act:yaml"},
			selItem{title: "Edit in editor", desc: "e", id: "act:edit"},
			selItem{title: "Delete " + row.Name, desc: "x", id: "act:delete"},
		)
		if a.res.IsPod() {
			items = append(items,
				selItem{title: "Logs", desc: "l", id: "act:logs"},
				selItem{title: "Shell into pod", desc: "s", id: "act:shell"},
			)
		}
		if a.res.IsDeployment() {
			items = append(items, selItem{title: "Follow deployment logs", desc: "L", id: "act:deploylogs"})
		}
		if a.res.IsNodes() {
			items = append(items, selItem{title: "Node shell (debug pod)", desc: "s", id: "act:nodeshell"})
		}
		if a.res.Scalable() {
			items = append(items, selItem{title: "Scale", desc: "s", id: "act:scale"})
		}
		if a.res.Restartable() {
			items = append(items, selItem{title: "Rollout restart", desc: "R", id: "act:restart"})
		}
		if a.res.IsCronJob() {
			items = append(items, selItem{title: "Trigger job now", desc: "t", id: "act:trigger"})
		}
	}

	items = append(items,
		selItem{title: "Open Kubernetes docs", desc: "O", id: "cmd:docs"},
		selItem{title: "Jump to resource", desc: ":", id: "cmd:jump"},
		selItem{title: "Filter list", desc: "/", id: "cmd:filter"},
		selItem{title: "Show kubectl command", desc: "C", id: "cmd:kubectl"},
		selItem{title: "Refresh", desc: "r", id: "cmd:refresh"},
		selItem{title: "Switch namespace", desc: "n", id: "cmd:namespace"},
		selItem{title: "All namespaces", desc: "a", id: "cmd:allns"},
		selItem{title: "Switch context", desc: "c", id: "cmd:context"},
		selItem{title: "Toggle wide columns", desc: "w", id: "cmd:wide"},
		selItem{title: "Help", desc: "?", id: "cmd:help"},
		selItem{title: "Quit", desc: "q", id: "cmd:quit"},
	)
	for _, ri := range a.client.Registry().All() {
		items = append(items, selItem{title: ri.Resource, desc: resourceDesc(ri), id: "res:" + ri.Key()})
	}
	a.sel.open(selPalette, "Command palette", "type a command or resource", items, false)
	a.overlay = overlaySelector
	return a, nil
}

func (a App) openResourceJump() (tea.Model, tea.Cmd) {
	var items []selItem
	for _, ri := range a.client.Registry().All() {
		items = append(items, selItem{title: ri.Resource, desc: resourceDesc(ri), id: ri.Key()})
	}
	a.sel.open(selResource, "Jump to resource", "pods, deploy, svc…", items, false)
	a.overlay = overlaySelector
	return a, nil
}

func (a App) openNamespacePicker() (tea.Model, tea.Cmd) {
	a.sel.openLoading(selNamespace, "Switch namespace", "namespace")
	a.overlay = overlaySelector
	return a, namespacesCmd(a.client)
}

func (a App) openContextPicker() (tea.Model, tea.Cmd) {
	items := make([]selItem, 0, len(a.client.Contexts()))
	for _, c := range a.client.Contexts() {
		desc := ""
		if c == a.client.ContextName {
			desc = "current"
		}
		items = append(items, selItem{title: c, desc: desc, id: c})
	}
	a.sel.open(selContext, "Switch context", "context", items, false)
	a.overlay = overlaySelector
	return a, nil
}

func (a App) applySelection(res selResult) (tea.Model, tea.Cmd) {
	switch a.sel.kind {
	case selPalette:
		return a.applyPalette(res.id)
	case selResource:
		if ri, ok := a.client.Registry().Resolve(res.id); ok {
			return a.switchResource(ri)
		}
		a.setStatus("unknown resource", true)
		return a, nil
	case selNamespace:
		if res.id == "*" {
			a.namespace = ""
		} else {
			a.namespace = res.id
			a.lastNS = res.id
		}
		a.persist()
		return a.reload()
	case selContext:
		if res.id == a.client.ContextName {
			return a, nil
		}
		a.setStatus("switching context…", false)
		return a, switchContextCmd(res.id, a.client.Kubeconfig())
	case selContainer:
		return a.startLogs(a.logTarget.ns, a.logTarget.name, res.id)
	case selExecContainer:
		return a.startExec(a.client, a.execTarget.ns, a.execTarget.name, res.id, "", nil, nil)
	case selScale:
		n, err := strconv.Atoi(strings.TrimSpace(res.value))
		if err != nil || n < 0 {
			a.setStatus("scale: enter a non-negative number", true)
			return a, nil
		}
		t := a.scaleTarget
		return a, scaleCmd(a.client, t.res, t.ns, t.name, n)
	case selSort:
		idx, _ := strconv.Atoi(res.id)
		a.table.setSort(idx)
		return a, nil
	}
	return a, nil
}

func (a App) applyPalette(id string) (tea.Model, tea.Cmd) {
	if res, ok := strings.CutPrefix(id, "res:"); ok {
		if ri, ok := a.client.Registry().Resolve(res); ok {
			return a.switchResource(ri)
		}
		return a, nil
	}
	switch id {
	case "act:config":
		return a.openConfig()
	case "act:describe", "act:yaml":
		return a.openDetail()
	case "act:edit":
		return a.openEdit()
	case "act:delete":
		return a.openDelete()
	case "act:logs":
		return a.openLogs()
	case "act:deploylogs":
		return a.openDeploymentLogs()
	case "act:shell":
		return a.openShell()
	case "act:nodeshell":
		return a.openNodeShell()
	case "act:scale":
		return a.openScale()
	case "act:restart":
		return a.openRestart()
	case "act:trigger":
		return a.openTriggerJob()
	case "cmd:jump":
		return a.openResourceJump()
	case "cmd:filter":
		a.focus = focusMain
		a.table.startFilter()
		return a, nil
	case "cmd:kubectl":
		return a.openCommand()
	case "cmd:docs":
		return a.openDocs()
	case "cmd:refresh":
		if a.screen == screenCockpit {
			return a.reloadCockpit()
		}
		return a.reload()
	case "cmd:namespace":
		return a.openNamespacePicker()
	case "cmd:allns":
		return a.toggleAllNS()
	case "cmd:context":
		return a.openContextPicker()
	case "cmd:wide":
		a.table.toggleWide()
		return a, nil
	case "cmd:help":
		a.help.reset()
		a.overlay = overlayHelp
		return a, nil
	case "cmd:quit":
		a.logs.stop()
		return a, tea.Quit
	}
	return a, nil
}

// --- View -------------------------------------------------------------------

func (a App) View() tea.View {
	v := tea.NewView(a.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	// Release the mouse in the logs view so the terminal's own click-drag text
	// selection and copy work there (keyboard scrolling still applies).
	if a.screen == screenLogs && a.overlay == overlayNone {
		v.MouseMode = tea.MouseModeNone
	}
	return v
}

func (a App) render() string {
	if a.width == 0 || a.height == 0 {
		return "starting ku…"
	}
	if a.splash {
		return lipgloss.NewStyle().Padding(a.gutter, a.gutter).Render(a.splashView())
	}

	body := a.screenBody()

	// Modal overlays float on top of the current screen rather than replacing it,
	// so the view you came from stays visible behind them. The embedded terminal
	// is the exception: it takes over the body fully.
	switch a.overlay {
	case overlayTerm:
		body = a.term.View(a.width, a.bodyH())
	case overlayHelp:
		body = overlayCenter(body, a.help.View(a.width, a.bodyH()), a.width, a.bodyH())
	case overlaySelector:
		body = overlayCenter(body, a.sel.View(a.width, a.bodyH()), a.width, a.bodyH())
	case overlayConfirm:
		body = overlayCenter(body, a.confirm.View(a.width, a.bodyH()), a.width, a.bodyH())
	case overlayCommand:
		body = overlayCenter(body, a.command.View(a.width, a.bodyH()), a.width, a.bodyH())
	}

	// Guarantee the body is exactly bodyH lines, then width-clamp the whole
	// frame so no line (header, body, or footer) can wrap and break the fixed
	// header/body/footer layout.
	body = lipgloss.NewStyle().MaxHeight(a.bodyH()).Render(body)
	frame := a.headerView() + "\n" + body + "\n" + a.footerView()
	frame = a.renderNotification(frame)
	frame = lipgloss.NewStyle().MaxWidth(a.width).Render(frame)
	// Equal gutter on every side. Padding adds 2*gutter cols and rows back, so
	// the result is exactly the full terminal size.
	return lipgloss.NewStyle().Padding(a.gutter, a.gutter).Render(frame)
}

const kuLogo = `██   ██ ██    ██
██  ██  ██    ██
█████   ██    ██
██  ██  ██    ██
██   ██  ██████  `

// splashView centers the logo, a spinner, and the creator credit while the
// cluster connection and config load in the background.
func (a App) splashView() string {
	logo := a.theme.HeaderVal.Render(kuLogo)
	w := lipgloss.Width(logo)
	spin := lipgloss.PlaceHorizontal(w, lipgloss.Center, a.spin.View())
	credit := lipgloss.PlaceHorizontal(w, lipgloss.Center, a.theme.Dim.Render(creatorHandle))
	block := lipgloss.JoinVertical(lipgloss.Left, logo, "", spin, "", credit)
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, block)
}

// screenBody renders the active screen's body with no overlay. Overlays
// composite on top of this so the current view stays visible behind them.
func (a App) screenBody() string {
	switch a.screen {
	case screenConfig:
		return a.renderPane(a.theme.PaneActive, a.config.View(), a.width, a.bodyH())
	case screenDetail:
		return a.renderPane(a.theme.PaneActive, a.detail.View(), a.width, a.bodyH())
	case screenLogs:
		return a.renderPane(a.theme.PaneActive, a.logs.View(), a.width, a.bodyH())
	case screenCockpit:
		return a.cockpitScreen()
	default:
		return a.tableScreen()
	}
}

func (a App) activeNavKey() string {
	if a.screen == screenCockpit {
		return overviewKey
	}
	return a.res.Key()
}

// renderSidebar renders the left nav pane with a focus-aware border.
func (a App) renderSidebar() string {
	style := a.theme.PaneInactive
	if a.focus == focusSidebar {
		style = a.theme.PaneActive
	}
	return a.renderPane(style, a.sidebar.View(a.activeNavKey(), a.focus == focusSidebar), a.sidebarWidth(), a.bodyH())
}

// paneScreen renders [sidebar gap main], wrapping main in a focus-aware border.
func (a App) paneScreen(main string) string {
	mainStyle := a.theme.PaneActive
	if a.focus == focusSidebar {
		mainStyle = a.theme.PaneInactive
	}
	mainW := a.width - a.sidebarWidth() - paneGap
	box := a.renderPane(mainStyle, main, mainW, a.bodyH())
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		a.renderSidebar(),
		strings.Repeat(" ", paneGap),
		box,
	)
}

func (a App) renderPane(style lipgloss.Style, content string, outerW, outerH int) string {
	if outerW < 5 || outerH < 3 {
		return clampBlock(content, outerW, outerH)
	}
	content = clampBlock(content, paneContentWidth(outerW), paneContentHeight(outerH))
	return style.Width(paneStyleWidth(outerW)).Height(paneStyleHeight(outerH)).MaxHeight(outerH).Render(content)
}

func (a App) tableScreen() string {
	if !a.sidebarVisible() {
		return a.renderPane(a.theme.PaneActive, a.table.View(), a.width, a.bodyH())
	}
	return a.paneScreen(a.table.View())
}

func (a App) cockpitScreen() string {
	if !a.sidebarVisible() {
		return a.cockpit.View(a.width, a.bodyH())
	}
	// The cockpit's own panels already have borders, so render them directly
	// beside the nav rather than inside another bordered pane.
	mainW := a.width - a.sidebarWidth() - paneGap
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		a.renderSidebar(),
		strings.Repeat(" ", paneGap),
		a.cockpit.View(mainW, a.bodyH()),
	)
}

func (a App) headerView() string {
	th := a.theme
	logo := th.Logo.Render("KU")

	chip := func(k, v string) string {
		return th.HeaderKey.Render(k+" ") + th.HeaderVal.Render(v)
	}
	resLabel := a.res.Title()
	if a.screen == screenCockpit {
		resLabel = "overview"
	}
	chips := []string{
		chip("ctx", shortContext(a.client.ContextName)),
		chip("ns", a.nsLabel()),
		chip("res", resLabel),
	}
	// Surface an applied filter so a narrowed list never looks like the whole set.
	if a.table.filterActive() && !a.table.filtering {
		chips = append(chips, th.HeaderKey.Render("filter ")+th.Warn.Render("/"+truncate(a.table.filterValue(), 24)))
	}

	right := ""
	if a.screen != screenCockpit {
		label := itoa(a.table.count()) + " items"
		if a.res.Namespaced && a.namespace == "" {
			label += " · all ns"
		}
		right = th.Dim.Render(label)
	}

	avail := a.width - lipgloss.Width(right) - 2
	left := logo
	for _, c := range chips {
		if lipgloss.Width(left)+2+lipgloss.Width(c) > avail {
			break
		}
		left += "  " + c
	}

	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

type hint struct{ key, desc string }

func (a App) footerView() string {
	th := a.theme

	// The creator credit lives at the far bottom-right; transient status/loading
	// shows just to its left. Dropped on very narrow terminals to keep hints
	// usable.
	credit := ""
	if a.width >= 40 {
		credit = th.Dim.Render(creatorHandle)
	}

	// Cap the status so a long message (e.g. a discovery warning) cannot make
	// the footer wider than the terminal.
	statusMax := a.width - lipgloss.Width(credit) - 4
	if statusMax < 8 {
		statusMax = 8
	}
	showStatus := a.status != "" && !a.notificationVisible()
	statusSeg := ""
	switch {
	case showStatus && a.statusErr:
		statusSeg = th.StatusErr.Render("✘ " + truncate(a.status, statusMax-2))
	case showStatus:
		statusSeg = th.StatusOK.Render(truncate(a.status, statusMax))
	case a.loading:
		statusSeg = th.Spinner.Render(a.spin.View()) + th.Dim.Render(" loading")
	}

	right := credit
	if statusSeg != "" {
		if credit != "" {
			right = statusSeg + "  " + credit
		} else {
			right = statusSeg
		}
	}

	if a.screen == screenTable && a.table.filtering && a.overlay == overlayNone {
		left := th.FooterKey.Render(a.table.filter.View())
		hint := "  esc clear · enter apply"
		if lipgloss.Width(left)+len(hint)+lipgloss.Width(right) <= a.width {
			left += th.FooterDesc.Render(hint)
		}
		return spread(left, right, a.width)
	}

	avail := a.width - lipgloss.Width(right) - 2
	return spread(renderHints(th, a.hints(), avail), right, a.width)
}

// creatorHandle is shown in the footer's bottom-right strip.
const creatorHandle = "x.com/iamdothash"

func (a App) hints() []hint {
	switch a.overlay {
	case overlayTerm:
		return []hint{{"keys", "→ shell"}, {"ctrl+\\", "detach"}}
	case overlaySelector:
		return []hint{{"↑↓", "move"}, {"enter", "select"}, {"esc", "cancel"}}
	case overlayHelp:
		return []hint{{"esc", "close"}}
	case overlayConfirm:
		return []hint{{"y", "confirm"}, {"n", "cancel"}}
	case overlayCommand:
		return []hint{{"esc", "close"}, {"C", "close"}}
	}
	switch a.screen {
	case screenConfig:
		h := []hint{{"↑↓", "scroll"}, {"d", "describe"}}
		if a.configTarget.res.IsDeployment() {
			h = append(h, hint{"L", "all logs"})
		}
		if a.configTarget.res.IsCronJob() {
			h = append(h, hint{"t", "trigger"})
		}
		return append(h, hint{"e", "edit"}, hint{"O", "docs"}, hint{"C", "cmd"}, hint{"esc", "back"})
	case screenDetail:
		h := []hint{{"↑↓", "scroll"}, {"enter", "config"}}
		if a.detailTarget.res.IsDeployment() {
			h = append(h, hint{"L", "all logs"})
		}
		if a.detailTarget.res.IsCronJob() {
			h = append(h, hint{"t", "trigger"})
		}
		return append(h, hint{"e", "edit"}, hint{"O", "docs"}, hint{"C", "cmd"}, hint{"esc", "back"})
	case screenLogs:
		switch {
		case a.logs.selecting && a.logs.marking:
			return []hint{{"↑↓", "extend"}, {"y", "copy"}, {"esc", "cancel"}}
		case a.logs.selecting:
			return []hint{{"↑↓", "move"}, {"m", "mark"}, {"y", "copy"}, {"esc", "cancel"}}
		}
		return []hint{{"↑↓", "scroll"}, {"f", "follow"}, {"/", "filter"}, {"w", "wrap"}, {"v", "select"}, {"c", "copy"}, {"C", "clear"}, {"O", "docs"}, {"esc", "back"}}
	case screenCockpit:
		if a.focus == focusSidebar {
			return []hint{{"↑↓", "pick"}, {"enter", "open"}, {"tab", "table"}, {":", "jump"}, {"C", "cmd"}, {"?", "help"}}
		}
		return []hint{{"tab", "nav"}, {":", "jump"}, {"^k", "palette"}, {"C", "cmd"}, {"r", "refresh"}, {"n", "ns"}, {"c", "ctx"}, {"?", "help"}, {"q", "quit"}}
	}
	if a.focus == focusSidebar {
		return []hint{{"↑↓", "pick"}, {"enter", "open"}, {"tab", "table"}, {":", "jump"}, {"C", "cmd"}, {"?", "help"}}
	}

	// Context-aware: surface the actions that apply to the current resource.
	h := []hint{{"enter", "config"}, {"d", "describe"}}
	switch {
	case a.res.IsPod():
		h = append(h, hint{"l", "logs"}, hint{"s", "shell"})
	case a.res.IsDeployment():
		h = append(h, hint{"L", "all logs"})
	case a.res.IsNodes():
		h = append(h, hint{"s", "node shell"})
	case a.res.Scalable():
		h = append(h, hint{"s", "scale"})
	}
	if a.res.Restartable() {
		h = append(h, hint{"R", "restart"})
	}
	if a.res.IsCronJob() {
		h = append(h, hint{"t", "trigger"})
	}
	h = append(h,
		hint{"e", "edit"}, hint{"x", "del"}, hint{"/", "filter"}, hint{"S", "sort"}, hint{"O", "docs"}, hint{"C", "cmd"},
		hint{"tab", "nav"}, hint{"^k", "palette"}, hint{"?", "help"}, hint{"q", "quit"})
	if a.table.filterActive() {
		h = append([]hint{{"esc", "clear filter"}}, h...)
	}
	return h
}

func renderHints(th Theme, hints []hint, avail int) string {
	var parts []string
	used := 0
	for _, h := range hints {
		add := lipgloss.Width(h.key) + 1 + lipgloss.Width(h.desc)
		if len(parts) > 0 {
			add += 3
		}
		if used+add > avail {
			break
		}
		used += add
		parts = append(parts, th.FooterKey.Render(h.key)+" "+th.FooterDesc.Render(h.desc))
	}
	return strings.Join(parts, th.FooterDesc.Render(" · "))
}

func (a App) nsLabel() string {
	if !a.res.Namespaced {
		return "cluster"
	}
	if a.namespace == "" {
		return "all"
	}
	return a.namespace
}

// --- helpers ----------------------------------------------------------------

func toggleFocus(f focusKind) focusKind {
	if f == focusMain {
		return focusSidebar
	}
	return focusMain
}

// qualified renders "namespace/name", or just name for cluster-scoped objects.
func qualified(ns, name string) string {
	if ns == "" {
		return name
	}
	return ns + "/" + name
}

func resourceDesc(ri k8s.ResourceInfo) string {
	if ri.Group == "" {
		return ri.Kind
	}
	return ri.Kind + " · " + ri.Group
}

func shortContext(name string) string {
	if i := strings.Index(name, "/cluster/"); i >= 0 {
		return name[i+len("/cluster/"):]
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

func trimErr(err error) string {
	s := strings.ReplaceAll(err.Error(), "\n", " ")
	return truncate(s, 120)
}

// persist remembers the current context and namespace for the next launch.
func (a App) persist() {
	saveState(savedState{Context: a.client.ContextName, Namespace: a.namespace})
}
