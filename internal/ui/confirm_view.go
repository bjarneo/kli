package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmView is a small yes/no modal. action is the command run on confirm,
// so the overlay stays generic over what it confirms.
type confirmView struct {
	th      Theme
	title   string
	message string
	danger  bool
	action  tea.Cmd
}

func (c confirmView) View(width, height int) string {
	titleStyle := c.th.ModalTitle
	border := c.th.ModalBorder
	if c.danger {
		titleStyle = lipgloss.NewStyle().Foreground(c.th.P.Bad).Bold(true)
		border = c.th.ModalBorder.BorderForeground(c.th.P.Bad) // same padding, danger color
	}

	body := titleStyle.Render(c.title) + "\n\n" +
		c.th.SelItem.Render(c.message) + "\n\n" +
		c.th.FooterKey.Render("y") + c.th.FooterDesc.Render(" confirm") + "    " +
		c.th.FooterKey.Render("n") + c.th.FooterDesc.Render("/") +
		c.th.FooterKey.Render("esc") + c.th.FooterDesc.Render(" cancel")

	boxW := lipgloss.Width(c.message) + 6
	if boxW < 40 {
		boxW = 40
	}
	if boxW > width-6 { // border (2) + padding (4)
		boxW = width - 6
	}
	if boxW < 8 {
		boxW = 8
	}
	box := border.Width(boxW).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
