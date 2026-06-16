package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
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
	vp := viewport.New()
	vp.SoftWrap = true // wrap long lines so the full line is visible, not truncated
	return logView{th: th, vp: vp, follow: true, filter: newFilterInput("filter (regex)")}
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
	l.stickToBottom()
}

// stickToBottom keeps the newest lines in view while following.
func (l *logView) stickToBottom() {
	if l.follow {
		l.vp.GotoBottom()
	}
}

func (l *logView) appendLine(s string) {
	l.storeLine(s)
	l.syncViewport()
}

// storeLine adds a line to the buffer and the filtered content without touching
// the viewport, so a burst of lines can be stored and then synced once.
func (l *logView) storeLine(s string) {
	s = expandTabs(s) // tabs measure as zero width and would spill past the pane
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
}

// syncViewport pushes the current content into the viewport, sticking to the
// tail while following.
func (l *logView) syncViewport() {
	l.vp.SetContent(l.content)
	l.stickToBottom()
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

// toggleWrap switches between wrapping long lines (full line visible) and
// truncating them (one row per entry, scrollable left/right). The viewport
// re-wraps from the stored lines on the next render, so only the flag flips.
func (l *logView) toggleWrap() {
	l.vp.SoftWrap = !l.vp.SoftWrap
	l.stickToBottom()
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
	mode := "wrap"
	if !l.vp.SoftWrap {
		mode = "nowrap"
	}
	right := l.th.Dim.Render(mode) + "  " + style.Render("● "+state)
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

	// A bufio.Reader (not Scanner) keeps lines intact at any length: ReadString
	// grows past the 64KB read buffer until it hits the newline, where a Scanner
	// would error out and drop the line once it crossed its token cap.
	br := bufio.NewReaderSize(rc, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
			if prefix != "" {
				line = prefix + " | " + line
			}
			select {
			case <-ctx.Done():
				return
			case ch <- logEvent{session: session, line: line}:
			}
		}
		if err != nil {
			if err != io.EOF && ctx.Err() == nil {
				sendLogEvent(ch, logEvent{session: session, err: err})
			}
			return
		}
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
