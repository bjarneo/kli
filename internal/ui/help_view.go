package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// helpView renders all keybindings grouped into columns.
type helpView struct {
	th     Theme
	keys   keyMap
	offset int
	note   string // optional one-line mode summary shown under the title
}

func newHelpView(th Theme, keys keyMap) helpView {
	return helpView{th: th, keys: keys}
}

func (h *helpView) reset() {
	h.offset = 0
}

func (h helpView) Update(msg tea.KeyMsg) helpView {
	switch msg.String() {
	case "up", "k", "ctrl+p":
		if h.offset > 0 {
			h.offset--
		}
	case "down", "j", "ctrl+n":
		h.offset++
	case "pgup", "ctrl+u":
		h.offset -= 5
		if h.offset < 0 {
			h.offset = 0
		}
	case "pgdown", "ctrl+d":
		h.offset += 5
	case "g", "home":
		h.offset = 0
	}
	return h
}

func (h helpView) View(width, height int) string {
	if height < 4 {
		return clampBlock(h.th.ModalTitle.Render("Keybindings"), width, height)
	}
	if h.offset < 0 {
		h.offset = 0
	}
	groups := h.keys.groups()
	colW := 26
	if w := width - 6; w < colW {
		colW = w
	}
	if colW < 12 {
		colW = 12
	}

	blocks := make([]string, len(groups))
	for i, g := range groups {
		var b strings.Builder
		b.WriteString(h.th.HeaderVal.Render(g.title))
		b.WriteString("\n")
		for _, k := range g.keys {
			hk := k.Help()
			b.WriteString(h.th.FooterKey.Render(fmt.Sprintf("%-8s", hk.Key)))
			b.WriteString(" ")
			b.WriteString(h.th.FooterDesc.Render(hk.Desc))
			b.WriteString("\n")
		}
		blocks[i] = lipgloss.NewStyle().Width(colW).Render(strings.TrimRight(b.String(), "\n"))
	}

	perRow := 3
	switch {
	case width < 62:
		perRow = 1
	case width < 92:
		perRow = 2
	}

	var rows []string
	for i := 0; i < len(blocks); i += perRow {
		end := i + perRow
		if end > len(blocks) {
			end = len(blocks)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, blocks[i:end]...))
	}
	grid := lipgloss.JoinVertical(lipgloss.Left, rows...)
	gridLines := strings.Split(grid, "\n")

	border := h.th.ModalBorder
	spacer := "\n\n"
	frameRows := 6 // border/padding plus title and blank spacer
	if height < 7 {
		border = border.Padding(0, 1)
		spacer = "\n"
		frameRows = 3 // border plus title line
	}
	if h.note != "" {
		frameRows++ // the mode summary takes one extra line
	}
	visible := height - frameRows
	if visible < 1 {
		visible = 1
	}
	maxOffset := len(gridLines) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if h.offset > maxOffset {
		h.offset = maxOffset
	}
	end := h.offset + visible
	if end > len(gridLines) {
		end = len(gridLines)
	}
	grid = strings.Join(gridLines[h.offset:end], "\n")

	title := h.th.ModalTitle.Render("Keybindings")
	hint := h.th.Dim.Render("  ? or esc to close")
	if maxOffset > 0 {
		hint = h.th.Dim.Render(fmt.Sprintf("  ↑↓ scroll %d/%d · esc close", h.offset+1, maxOffset+1))
	}
	header := title + hint
	if h.note != "" {
		header += "\n" + h.th.Warn.Render(h.note)
	}
	content := header + spacer + grid

	box := border.Render(content)
	return box
}
