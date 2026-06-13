package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette is the small set of semantic colors every style is derived from.
type Palette struct {
	Accent   lipgloss.TerminalColor // primary accent: logo, titles, active keys
	Accent2  lipgloss.TerminalColor // secondary accent
	Fg       lipgloss.TerminalColor // body text ("" / NoColor = terminal default)
	Muted    lipgloss.TerminalColor // hints, dim text
	Border   lipgloss.TerminalColor // box and rule borders
	Good     lipgloss.TerminalColor // success / running
	Warn     lipgloss.TerminalColor // warning / pending
	Bad      lipgloss.TerminalColor // error / failed
	SelFg    lipgloss.TerminalColor // selected row foreground
	SelBg    lipgloss.TerminalColor // selected row background
	HeaderBg lipgloss.TerminalColor // logo chip background
	LogoFg   lipgloss.TerminalColor // logo chip text

	// ReverseSel highlights the selected row with reverse video instead of an
	// explicit background, so it tracks the terminal's own palette. Used by the
	// ANSI theme.
	ReverseSel bool
}

// ansiPalette uses the terminal's own 16-color ANSI palette so kli looks
// consistent with whatever color scheme the user already runs. The bright
// variants (8-15) read well on dark backgrounds and the normal variants (0-7)
// on light ones, so we pick by the detected background and defer the actual
// hues to the terminal's scheme.
func ansiPalette(dark bool) Palette {
	if dark {
		return Palette{
			Accent:     lipgloss.Color("12"), // bright blue
			Accent2:    lipgloss.Color("13"), // bright magenta
			Fg:         lipgloss.NoColor{},   // terminal default
			Muted:      lipgloss.Color("8"),  // gray
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

// tokyoNightPalette is the fallback theme: a fixed, high-contrast palette for
// terminals whose ANSI colors are undefined or unpleasant.
func tokyoNightPalette() Palette {
	return Palette{
		Accent:     lipgloss.Color("#7aa2f7"), // blue
		Accent2:    lipgloss.Color("#bb9af7"), // magenta
		Fg:         lipgloss.Color("#c0caf5"),
		Muted:      lipgloss.Color("#565f89"),
		Border:     lipgloss.Color("#3b4261"),
		Good:       lipgloss.Color("#9ece6a"), // green
		Warn:       lipgloss.Color("#e0af68"), // yellow
		Bad:        lipgloss.Color("#f7768e"), // red
		SelFg:      lipgloss.Color("#c0caf5"),
		SelBg:      lipgloss.Color("#283457"),
		HeaderBg:   lipgloss.Color("#7aa2f7"),
		LogoFg:     lipgloss.Color("#16161e"),
		ReverseSel: false,
	}
}

// Theme bundles a palette with the precomputed styles the views render with.
type Theme struct {
	Name string
	P    Palette

	Logo      lipgloss.Style
	HeaderKey lipgloss.Style
	HeaderVal lipgloss.Style
	Rule      lipgloss.Style

	TableHeader   lipgloss.Style
	TableCell     lipgloss.Style
	TableSelected lipgloss.Style

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

	t.Logo = lipgloss.NewStyle().Bold(true).Foreground(p.LogoFg).Background(p.HeaderBg).Padding(0, 1)
	t.HeaderKey = lipgloss.NewStyle().Foreground(p.Muted)
	t.HeaderVal = lipgloss.NewStyle().Foreground(p.Accent).Bold(true)
	t.Rule = lipgloss.NewStyle().Foreground(p.Border)

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

	t.PaneActive = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.Accent)
	t.PaneInactive = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.Border)
	t.NavSection = lipgloss.NewStyle().Foreground(p.Muted).Bold(true)

	t.ModalBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.Accent).Padding(1, 2)
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

// PickTheme selects the theme. ANSI is the default and adapts to a light or
// dark terminal background; Tokyo Night is the fixed fallback, selectable via
// --theme or $KLI_THEME. Detection runs here (called before the Bubble Tea
// program starts) so it does not race the program's stdin reader.
func PickTheme(name string) Theme {
	if name == "" {
		name = os.Getenv("KLI_THEME")
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "tokyonight", "tokyo-night", "tokyo":
		return NewTheme("tokyonight", tokyoNightPalette())
	default:
		return NewTheme("ansi", ansiPalette(lipgloss.HasDarkBackground()))
	}
}
