package ui

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/kli/internal/k8s"
)

const maxLogLines = 5000

// logView streams a pod container's logs into a viewport. The stream always
// follows server-side; the follow flag controls whether new lines auto-scroll
// to the bottom (so the user can scroll up to read history).
type logView struct {
	th     Theme
	vp     viewport.Model
	title  string
	ns     string
	pod    string
	cont   string
	deploy string
	follow bool

	session int
	streams int
	cancel  context.CancelFunc
	ch      chan logEvent
	lines   []string
	content string // joined view content (filtered), maintained incrementally

	// Filtering. The filter is a regular expression matched against each line;
	// an empty filter shows everything and an invalid pattern shows everything
	// (re stays nil) so results update as the user types.
	filtering bool
	filter    textinput.Model
	re        *regexp.Regexp
	matched   int // lines currently shown
	height    int // pane content height, retained so chrome changes can relayout
}

func newLogView(th Theme) logView {
	return logView{th: th, vp: viewport.New(), follow: true, filter: newFilterInput("filter (regex)")}
}

func (l *logView) setSize(w, h int) {
	if h < 1 {
		h = 1
	}
	l.height = h
	l.vp.SetWidth(w)
	if fw := w - 8; fw > 4 {
		l.filter.SetWidth(fw) // bound the filter input so it can't overflow
	}
	l.relayout()
}

// chromeLines counts the non-viewport rows the pane draws: the title, plus the
// filter row when filtering or a filter is applied.
func (l *logView) chromeLines() int {
	if l.filtering || l.filterActive() {
		return 2
	}
	return 1
}

// relayout sizes the viewport to the height left after the chrome, keeping the
// tail in view when following.
func (l *logView) relayout() {
	h := l.height - l.chromeLines()
	if h < 1 {
		h = 1
	}
	l.vp.SetHeight(h)
	if l.follow {
		l.vp.GotoBottom()
	}
}

func (l *logView) appendLine(s string) {
	l.lines = append(l.lines, s)
	switch {
	case len(l.lines) > maxLogLines:
		l.lines = l.lines[len(l.lines)-maxLogLines:]
		l.rebuildContent() // rebuild only when trimming the front
	case l.re != nil && !l.re.MatchString(s):
		return // filtered out: nothing changed on screen
	default:
		l.matched++
		if l.content == "" {
			l.content = s
		} else {
			l.content += "\n" + s
		}
	}
	l.syncViewport()
}

// syncViewport pushes the current content into the viewport, sticking to the
// tail while following.
func (l *logView) syncViewport() {
	l.vp.SetContent(l.content)
	if l.follow {
		l.vp.GotoBottom()
	}
}

// rebuildContent recomputes the joined view from scratch, applying the active
// filter. Used when the line set or the filter changes.
func (l *logView) rebuildContent() {
	if l.re == nil {
		l.content = strings.Join(l.lines, "\n")
		l.matched = len(l.lines)
		return
	}
	var b strings.Builder
	n := 0
	for _, ln := range l.lines {
		if !l.re.MatchString(ln) {
			continue
		}
		if n > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ln)
		n++
	}
	l.content = b.String()
	l.matched = n
}

func (l *logView) filterActive() bool { return l.filter.Value() != "" }

func (l *logView) startFilter() {
	l.filtering = true
	l.filter.Focus()
	l.relayout()
}

// stopFilter exits filter mode. If clear is true the pattern is dropped and the
// full stream restored.
func (l *logView) stopFilter(clear bool) {
	l.filtering = false
	l.filter.Blur()
	if clear {
		l.filter.SetValue("")
		l.applyFilter()
	}
	l.relayout()
}

// applyFilter compiles the current pattern and rebuilds the view. An empty or
// invalid pattern leaves re nil, so everything shows while the user keeps
// typing; filterLine flags the invalid case.
func (l *logView) applyFilter() {
	l.re = nil
	if val := l.filter.Value(); val != "" {
		if re, err := regexp.Compile(val); err == nil {
			l.re = re
		}
	}
	l.rebuildContent()
	l.syncViewport()
}

func (l *logView) stop() {
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
}

func (l logView) Update(msg tea.Msg) (logView, tea.Cmd) {
	if l.filtering {
		prev := l.filter.Value()
		var cmd tea.Cmd
		l.filter, cmd = l.filter.Update(msg)
		if l.filter.Value() != prev {
			l.applyFilter()
		}
		return l, cmd
	}
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	return l, cmd
}

func (l logView) View() string {
	state := "following"
	style := l.th.Good
	if !l.follow {
		state = "paused"
		style = l.th.Warn
	}
	right := style.Render("● " + state)
	title := l.th.ModalTitle.Render(l.title)
	header := spread(title, right, l.vp.Width())
	if l.filtering || l.filterActive() {
		return header + "\n" + l.filterLine() + "\n" + l.vp.View()
	}
	return header + "\n" + l.vp.View()
}

// filterLine renders the filter input with a match count, or an error marker
// when the pattern doesn't compile.
func (l logView) filterLine() string {
	var meta string
	switch {
	case l.re != nil:
		meta = l.th.Dim.Render(fmt.Sprintf("%d/%d", l.matched, len(l.lines)))
	case l.filterActive(): // text present but it didn't compile
		meta = l.th.Warn.Render("invalid regex")
	}
	return spread(l.filter.View(), meta, l.vp.Width())
}

// streamLogs opens the log stream and feeds lines onto ch until the context is
// canceled or the stream ends. It sends a done event unless cancellation already
// made that event irrelevant.
func streamLogs(ctx context.Context, cl *k8s.Client, ns, pod, cont, prefix string, session int, ch chan logEvent) {
	defer func() {
		sendLogEvent(ch, logEvent{session: session, done: true})
	}()

	rc, err := cl.LogStream(ctx, ns, pod, cont, logTailLines, true)
	if err != nil {
		sendLogEvent(ch, logEvent{session: session, err: err})
		return
	}
	defer rc.Close()

	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if prefix != "" {
			line = prefix + " | " + line
		}
		select {
		case <-ctx.Done():
			return
		case ch <- logEvent{session: session, line: line}:
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		sendLogEvent(ch, logEvent{session: session, err: err})
	}
}

func sendLogEvent(ch chan<- logEvent, ev logEvent) {
	select {
	case ch <- ev:
	default:
	}
}

func waitForLog(ch chan logEvent) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}
