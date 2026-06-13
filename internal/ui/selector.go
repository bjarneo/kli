package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type selKind int

const (
	selPalette selKind = iota
	selResource
	selNamespace
	selContext
	selContainer
	selExecContainer
	selScale
)

const selMaxVisible = 12

// selItem is one selectable entry. id is the stable value passed back to the
// app when chosen; title/desc are shown and matched against.
type selItem struct {
	title string
	desc  string
	id    string
}

// selResult is returned from the selector's Update when the user acts.
type selResult struct {
	accepted bool
	canceled bool
	id       string // chosen item id ("" for a freeform value)
	value    string // raw typed text (for freeform inputs like scale)
}

// selector is a fuzzy-filtered list used for the palette and all pickers.
type selector struct {
	th       Theme
	kind     selKind
	title    string
	input    textinput.Model
	items    []selItem
	match    []int // indices into items, best first
	cursor   int
	offset   int
	freeform bool // accept the typed value even when nothing matches
	loading  bool
}

func newSelector(th Theme) selector {
	ti := textinput.New()
	ti.Prompt = th.Prompt.Render("❯ ")
	ti.Cursor.SetMode(cursor.CursorStatic)
	return selector{th: th, input: ti}
}

func (s *selector) open(kind selKind, title, placeholder string, items []selItem, freeform bool) {
	s.kind = kind
	s.title = title
	s.items = items
	s.freeform = freeform
	s.loading = false
	s.cursor = 0
	s.offset = 0
	s.input.SetValue("")
	s.input.Placeholder = placeholder
	s.input.Focus()
	s.refilter()
}

// openLoading opens an empty selector waiting for items to arrive async.
func (s *selector) openLoading(kind selKind, title, placeholder string) {
	s.open(kind, title, placeholder, nil, false)
	s.loading = true
}

func (s *selector) setItems(items []selItem) {
	s.items = items
	s.loading = false
	s.cursor = 0
	s.offset = 0
	s.refilter()
}

func (s *selector) refilter() {
	q := s.input.Value()
	type scored struct{ idx, score int }
	var ms []scored
	for i, it := range s.items {
		if sc, ok := fuzzyScore(q, it.title+" "+it.desc+" "+it.id); ok {
			ms = append(ms, scored{i, sc})
		}
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].score > ms[j].score })
	s.match = s.match[:0]
	for _, m := range ms {
		s.match = append(s.match, m.idx)
	}
	s.clampCursor()
}

func (s *selector) clampCursor() {
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= len(s.match) {
		s.cursor = len(s.match) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	// Keep the cursor within the visible window.
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+selMaxVisible {
		s.offset = s.cursor - selMaxVisible + 1
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

func (s selector) Update(msg tea.Msg) (selector, selResult, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return s, selResult{canceled: true}, nil
		case "enter":
			if len(s.match) > 0 {
				it := s.items[s.match[s.cursor]]
				return s, selResult{accepted: true, id: it.id, value: s.input.Value()}, nil
			}
			if s.freeform {
				return s, selResult{accepted: true, value: s.input.Value()}, nil
			}
			return s, selResult{}, nil
		case "up", "ctrl+p", "ctrl+k":
			s.cursor--
			s.clampCursor()
			return s, selResult{}, nil
		case "down", "ctrl+n", "ctrl+j", "tab":
			s.cursor++
			s.clampCursor()
			return s, selResult{}, nil
		}
	}
	prev := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() != prev {
		s.cursor = 0
		s.offset = 0
		s.refilter()
	}
	return s, selResult{}, cmd
}

func (s selector) View(width, height int) string {
	boxW := width * 2 / 3
	if boxW > 84 {
		boxW = 84
	}
	if boxW < 40 {
		boxW = 40
	}
	// Never exceed the terminal; clamp so the modal can't wrap and break layout.
	if boxW > width-2 {
		boxW = width - 2
	}
	if boxW < 8 {
		boxW = 8
	}
	inner := boxW - 6 // account for border (2) + padding (4)
	if inner < 1 {
		inner = 1
	}

	var b strings.Builder
	b.WriteString(s.th.ModalTitle.Render(s.title))
	b.WriteString("\n")
	b.WriteString(s.input.View())
	b.WriteString("\n")
	b.WriteString(s.th.Rule.Render(strings.Repeat("─", inner)))
	b.WriteString("\n")

	switch {
	case s.loading:
		b.WriteString(s.th.Dim.Render("  loading…"))
	case len(s.match) == 0:
		if s.freeform && s.input.Value() != "" {
			b.WriteString(s.th.Dim.Render("  press enter to use “" + s.input.Value() + "”"))
		} else {
			b.WriteString(s.th.Dim.Render("  no matches"))
		}
	default:
		end := s.offset + selMaxVisible
		if end > len(s.match) {
			end = len(s.match)
		}
		for i := s.offset; i < end; i++ {
			it := s.items[s.match[i]]
			line := s.renderItem(it, inner, i == s.cursor)
			b.WriteString(line)
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	box := s.th.ModalBorder.Width(boxW).Render(b.String())
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (s selector) renderItem(it selItem, width int, selected bool) string {
	title := it.title
	desc := it.desc
	// Reserve space for the description on the right.
	titleMax := width
	if desc != "" {
		titleMax = width - lipgloss.Width(desc) - 2
	}
	if titleMax < 4 {
		titleMax = 4
	}
	title = truncate(title, titleMax)

	left := "  " + title
	pad := width - lipgloss.Width(left) - lipgloss.Width(desc)
	if pad < 1 {
		pad = 1
	}
	line := left + strings.Repeat(" ", pad) + s.th.SelDesc.Render(desc)
	if selected {
		return s.th.SelItemSel.Width(width).Render(line)
	}
	return s.th.SelItem.Render(line)
}
