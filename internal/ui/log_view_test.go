package ui

import (
	"strings"
	"testing"

	"github.com/bjarneo/kli/internal/k8s"
)

func newTestLogView() logView {
	l := newLogView(PickTheme("ansi"))
	l.setSize(80, 20)
	return l
}

// setFilter mimics typing a pattern into the focused filter box.
func setFilter(l *logView, pattern string) {
	l.startFilter()
	l.filter.SetValue(pattern)
	l.applyFilter()
}

func TestLogFilterNarrowsToMatches(t *testing.T) {
	l := newTestLogView()
	for _, s := range []string{"info: starting", "error: boom", "info: ready", "ERROR: again"} {
		l.appendLine(s)
	}

	setFilter(&l, "error")
	if l.matched != 1 {
		t.Fatalf("expected 1 match for case-sensitive %q, got %d", "error", l.matched)
	}
	if strings.Contains(l.content, "info:") || strings.Contains(l.content, "ERROR:") {
		t.Fatalf("filtered content should only contain the lower-case error line:\n%s", l.content)
	}

	// Case-insensitive flag is honored (RE2 syntax).
	setFilter(&l, "(?i)error")
	if l.matched != 2 {
		t.Fatalf("expected 2 matches for %q, got %d", "(?i)error", l.matched)
	}
}

func TestLogFilterEmptyShowsAll(t *testing.T) {
	l := newTestLogView()
	lines := []string{"one", "two", "three"}
	for _, s := range lines {
		l.appendLine(s)
	}

	setFilter(&l, "two")
	if l.matched != 1 {
		t.Fatalf("expected 1 match, got %d", l.matched)
	}

	setFilter(&l, "")
	if l.matched != len(lines) {
		t.Fatalf("clearing the filter should show all %d lines, got %d", len(lines), l.matched)
	}
	if l.content != strings.Join(lines, "\n") {
		t.Fatalf("expected full content restored, got:\n%s", l.content)
	}
}

func TestLogFilterInvalidRegexIsForgiving(t *testing.T) {
	l := newTestLogView()
	for _, s := range []string{"a", "b", "c"} {
		l.appendLine(s)
	}

	setFilter(&l, "[") // not a valid regex
	if l.re != nil {
		t.Fatalf("invalid pattern should not produce a compiled regex")
	}
	if !l.filterActive() {
		t.Fatalf("an invalid (non-empty) pattern should still register as active")
	}
	if l.matched != 3 {
		t.Fatalf("invalid pattern should keep showing all lines, got %d", l.matched)
	}
}

func TestLogEventDrainsBufferedLinesInOneUpdate(t *testing.T) {
	app := App{client: &k8s.Client{}, theme: PickTheme("ansi"), width: 80, height: 24, screen: screenLogs}
	app.logs = newLogView(app.theme)
	app.logs.setSize(70, 20)
	app.logSession, app.logs.session, app.logs.streams = 7, 7, 1
	ch := make(chan logEvent, 16)
	app.logs.ch = ch
	for i := 0; i < 5; i++ {
		ch <- logEvent{session: 7, line: "buffered " + itoa(i)}
	}

	// One delivered event should pull the 5 already buffered in the same update.
	m, cmd := app.Update(logEvent{session: 7, line: "first"})
	na := m.(App)
	if len(na.logs.lines) != 6 {
		t.Fatalf("expected 6 lines after draining the buffer, got %d", len(na.logs.lines))
	}
	if cmd == nil {
		t.Fatal("expected to keep waiting while a stream is live")
	}
}

func TestLogExpandsTabsToPreventOverflow(t *testing.T) {
	l := newTestLogView()
	l.appendLine("a\tb\tc")

	if strings.Contains(l.lines[0], "\t") {
		t.Fatalf("tabs should be expanded in stored log lines, got %q", l.lines[0])
	}
	// a at col 0, tabs advance to the next 8-column stop.
	if l.lines[0] != "a       b       c" {
		t.Fatalf("unexpected tab expansion: %q", l.lines[0])
	}
}

func TestLogWrapDefaultsOnAndToggles(t *testing.T) {
	l := newTestLogView()
	if !l.vp.SoftWrap {
		t.Fatalf("log view should wrap long lines by default")
	}
	l.toggleWrap()
	if l.vp.SoftWrap {
		t.Fatalf("w should switch to truncate (no wrap)")
	}
	l.toggleWrap()
	if !l.vp.SoftWrap {
		t.Fatalf("w should switch back to wrap")
	}
}

func TestLogFilterAppliesToNewLines(t *testing.T) {
	l := newTestLogView()
	l.appendLine("keep me")
	l.appendLine("drop me")

	setFilter(&l, "keep")
	if l.matched != 1 {
		t.Fatalf("expected 1 match before streaming, got %d", l.matched)
	}

	// New lines arriving while the filter is active are matched too.
	l.appendLine("drop this one")
	l.appendLine("keep this one")
	if l.matched != 2 {
		t.Fatalf("expected 2 matches after streaming, got %d", l.matched)
	}
	if strings.Contains(l.content, "drop") {
		t.Fatalf("streamed non-matching lines must stay hidden:\n%s", l.content)
	}
}
