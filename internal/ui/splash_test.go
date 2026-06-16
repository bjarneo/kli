package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bjarneo/kli/internal/k8s"
)

func newSplashApp() App {
	a := App{theme: PickTheme("ansi"), keys: defaultKeys(), splash: true}
	a.spin = newSpinner(a.theme)
	return a
}

func TestSplashShowsLogoAndCreditWithinView(t *testing.T) {
	var m tea.Model = newSplashApp()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a := m.(App)

	out := a.View().Content
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "█") {
		t.Fatal("splash should render the logo")
	}
	if !strings.Contains(plain, creatorHandle) {
		t.Fatalf("splash should render the credit %q", creatorHandle)
	}
	if lines := strings.Count(out, "\n") + 1; lines > 24 {
		t.Fatalf("splash is %d lines, exceeds terminal height 24", lines)
	}
}

func TestSplashIgnoresInputUntilConnected(t *testing.T) {
	a := newSplashApp()
	m, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a = m.(App)
	// A normal key must be ignored (no client yet) rather than panic.
	m, _ = a.Update(mkKey("j"))
	if !m.(App).splash {
		t.Fatal("input should not leave the splash before connecting")
	}
}

func TestAdoptStartupTransitionsToCockpit(t *testing.T) {
	a := newSplashApp()
	a.width, a.height = 80, 24

	m, cmd := a.adoptStartup(startupReadyMsg{client: &k8s.Client{}, catalog: defaultNavCatalog()})
	na := m.(App)
	if na.splash {
		t.Fatal("adoptStartup should leave the splash")
	}
	if na.client == nil {
		t.Fatal("client should be set after startup")
	}
	if na.screen != screenCockpit {
		t.Fatalf("expected the cockpit landing screen, got %v", na.screen)
	}
	if cmd == nil {
		t.Fatal("expected initial load commands")
	}
}

func TestGoodbyeMentionsAppAndCredit(t *testing.T) {
	plain := ansi.Strip(goodbye(PickTheme("ansi")))
	if !strings.Contains(plain, "kli") || !strings.Contains(plain, creatorHandle) {
		t.Fatalf("goodbye should mention kli and the credit, got %q", plain)
	}
}

func TestAdoptStartupErrorQuits(t *testing.T) {
	a := newSplashApp()
	m, cmd := a.adoptStartup(startupReadyMsg{err: errors.New("connect failed")})
	if m.(App).startErr == nil {
		t.Fatal("a connection error should be recorded for Run")
	}
	if cmd == nil {
		t.Fatal("a connection error should quit")
	}
}
