package ui

import (
	"strings"
	"testing"
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
