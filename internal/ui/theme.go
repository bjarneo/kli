package ui

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// Palette is the small set of semantic colors every style is derived from.
type Palette struct {
	Accent   color.Color // primary accent: logo, titles, active keys
	Accent2  color.Color // secondary accent
	Fg       color.Color // body text ("" / NoColor = terminal default)
	Muted    color.Color // hints, dim text
	Border   color.Color // box and rule borders
	Good     color.Color // success / running
	Warn     color.Color // warning / pending
	Bad      color.Color // error / failed
	SelFg    color.Color // selected row foreground
	SelBg    color.Color // selected row background
	HeaderBg color.Color // logo chip background
	LogoFg   color.Color // logo chip text

	// ReverseSel highlights the selected row with reverse video instead of an
	// explicit background, so it tracks the terminal's own palette. Used by the
	// ANSI theme.
	ReverseSel bool
}

// ansiPalette uses the terminal's own 16-color ANSI palette so ku looks
// consistent with whatever color scheme the user already runs. The accent and
// status colors use the bright variants (8-15) on dark backgrounds; muted text
// avoids bright black because many dark themes render it too low-contrast.
func ansiPalette(dark bool) Palette {
	if dark {
		return Palette{
			Accent:     lipgloss.Color("12"), // bright blue
			Accent2:    lipgloss.Color("13"), // bright magenta
			Fg:         lipgloss.NoColor{},   // terminal default
			Muted:      lipgloss.Color("7"),  // readable gray on dark backgrounds
			Border:     lipgloss.Color("8"),
			Good:       lipgloss.Color("10"), // bright green
			Warn:       lipgloss.Color("11"), // bright yellow
			Bad:        lipgloss.Color("9"),  // bright red
			SelFg:      lipgloss.NoColor{},
			SelBg:      lipgloss.NoColor{},
			HeaderBg:   lipgloss.Color("12"),
			LogoFg:     lipgloss.Color("0"), // dark text on a bright chip
			ReverseSel: true,
		}
	}
	return Palette{
		Accent:     lipgloss.Color("4"), // blue
		Accent2:    lipgloss.Color("5"), // magenta
		Fg:         lipgloss.NoColor{},  // terminal default
		Muted:      lipgloss.Color("8"), // gray
		Border:     lipgloss.Color("8"),
		Good:       lipgloss.Color("2"), // green
		Warn:       lipgloss.Color("3"), // yellow
		Bad:        lipgloss.Color("1"), // red
		SelFg:      lipgloss.NoColor{},
		SelBg:      lipgloss.NoColor{},
		HeaderBg:   lipgloss.Color("4"),
		LogoFg:     lipgloss.Color("15"), // light text on a darker chip
		ReverseSel: true,
	}
}

// Theme bundles a palette with the precomputed styles the views render with.
type Theme struct {
	Name string
	P    Palette
	Dark bool // terminal background detected at startup, for rebuilding the ANSI palette

	Logo      lipgloss.Style
	HeaderKey lipgloss.Style
	HeaderVal lipgloss.Style
	Rule      lipgloss.Style

	TableHeader   lipgloss.Style
	TableCell     lipgloss.Style
	TableSelected lipgloss.Style
	Cell          lipgloss.Style // base cell text (terminal default fg on ANSI)

	FooterKey  lipgloss.Style
	FooterDesc lipgloss.Style
	StatusOK   lipgloss.Style
	StatusErr  lipgloss.Style
	Spinner    lipgloss.Style

	PaneActive   lipgloss.Style
	PaneInactive lipgloss.Style
	NavSection   lipgloss.Style

	ModalBorder lipgloss.Style
	ModalTitle  lipgloss.Style
	Prompt      lipgloss.Style
	SelItem     lipgloss.Style
	SelItemSel  lipgloss.Style
	SelDesc     lipgloss.Style

	Good lipgloss.Style
	Warn lipgloss.Style
	Bad  lipgloss.Style
	Dim  lipgloss.Style
}

// NewTheme builds all styles from a palette.
func NewTheme(name string, p Palette) Theme {
	t := Theme{Name: name, P: p}
	border := lipgloss.NormalBorder()

	t.Logo = lipgloss.NewStyle().Bold(true).Foreground(p.LogoFg).Background(p.HeaderBg).Padding(0, 1)
	t.HeaderKey = lipgloss.NewStyle().Foreground(p.Muted)
	t.HeaderVal = lipgloss.NewStyle().Foreground(p.Accent).Bold(true)
	t.Rule = lipgloss.NewStyle().Foreground(p.Border)

	t.Cell = lipgloss.NewStyle().Foreground(p.Fg) // NoColor on ANSI = terminal default
	t.TableHeader = lipgloss.NewStyle().Bold(true).Foreground(p.Accent).Padding(0, 1)
	cell := lipgloss.NewStyle().Padding(0, 1)
	if !p.ReverseSel {
		cell = cell.Foreground(p.Fg)
	}
	t.TableCell = cell
	if p.ReverseSel {
		t.TableSelected = lipgloss.NewStyle().Padding(0, 1).Reverse(true).Bold(true)
	} else {
		t.TableSelected = lipgloss.NewStyle().Padding(0, 1).Foreground(p.SelFg).Background(p.SelBg).Bold(true)
	}

	t.FooterKey = lipgloss.NewStyle().Foreground(p.Accent).Bold(true)
	t.FooterDesc = lipgloss.NewStyle().Foreground(p.Muted)
	t.StatusOK = lipgloss.NewStyle().Foreground(p.Good)
	t.StatusErr = lipgloss.NewStyle().Foreground(p.Bad).Bold(true)
	t.Spinner = lipgloss.NewStyle().Foreground(p.Accent)

	t.PaneActive = lipgloss.NewStyle().Border(border).BorderForeground(p.Accent).Padding(panePaddingY, panePaddingX)
	t.PaneInactive = lipgloss.NewStyle().Border(border).BorderForeground(p.Border).Padding(panePaddingY, panePaddingX)
	t.NavSection = lipgloss.NewStyle().Foreground(p.Muted).Bold(true)

	t.ModalBorder = lipgloss.NewStyle().Border(border).BorderForeground(p.Accent).Padding(1, 2)
	t.ModalTitle = lipgloss.NewStyle().Foreground(p.Accent).Bold(true)
	t.Prompt = lipgloss.NewStyle().Foreground(p.Accent2).Bold(true)
	t.SelItem = lipgloss.NewStyle().Foreground(p.Fg)
	if p.ReverseSel {
		t.SelItemSel = lipgloss.NewStyle().Reverse(true).Bold(true)
	} else {
		t.SelItemSel = lipgloss.NewStyle().Foreground(p.SelFg).Background(p.SelBg).Bold(true)
	}
	t.SelDesc = lipgloss.NewStyle().Foreground(p.Muted)

	t.Good = lipgloss.NewStyle().Foreground(p.Good)
	t.Warn = lipgloss.NewStyle().Foreground(p.Warn)
	t.Bad = lipgloss.NewStyle().Foreground(p.Bad)
	t.Dim = lipgloss.NewStyle().Foreground(p.Muted)

	return t
}

// PickTheme resolves a theme name to a Theme at startup, reading the terminal
// background once here (before the Bubble Tea program starts) so detection does
// not race the program's stdin reader. The name is already resolved by the
// caller; theme precedence (--theme, $KU_THEME, saved) lives in Run.
func PickTheme(name string) Theme {
	return buildTheme(name, lipgloss.HasDarkBackground(os.Stdin, os.Stdout))
}

// buildTheme constructs a theme by name without touching the terminal, so it is
// safe to call mid-session. Built-in themes pick their light or dark variant
// from dark; an unknown name (including "ansi") adapts the terminal's own palette.
func buildTheme(name string, dark bool) Theme {
	if oc, ok := lookupOCTheme(name); ok {
		sub := oc.dark
		if !dark {
			sub = oc.light
		}
		t := NewTheme(oc.id, ocToPalette(sub))
		t.Dark = dark
		return t
	}
	t := NewTheme("ansi", ansiPalette(dark))
	t.Dark = dark
	return t
}

// lookupOCTheme finds a built-in theme by name, accepting the tokyonight aliases.
func lookupOCTheme(name string) (ocTheme, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "tokyo" || n == "tokyo-night" {
		n = "tokyonight"
	}
	oc, ok := ocByID[n]
	return oc, ok
}
