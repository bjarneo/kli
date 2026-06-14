package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bjarneo/kli/internal/k8s"
)

// commandView shows the kubectl command that most closely reproduces the
// current screen.
type commandView struct {
	th      Theme
	command string
}

func newCommandView(th Theme) commandView {
	return commandView{th: th}
}

func (c *commandView) setCommand(command string) {
	c.command = command
}

func (c commandView) View(width, height int) string {
	boxW := width * 3 / 4
	if boxW < 44 {
		boxW = 44
	}
	if boxW > 96 {
		boxW = 96
	}
	if boxW > width-4 {
		boxW = width - 4
	}
	if boxW < 8 {
		boxW = 8
	}

	innerW := boxW - 6 // modal border plus horizontal padding
	if innerW < 1 {
		innerW = 1
	}

	lines := wrapPlain("$ "+c.command, innerW)
	visible := height - 7 // border/padding/title/spacers/hint
	if visible < 1 {
		visible = 1
	}
	if len(lines) > visible {
		lines = lines[:visible]
	}
	for i, line := range lines {
		if strings.HasPrefix(line, "$ ") {
			lines[i] = c.th.FooterKey.Render("$ ") + c.th.HeaderVal.Render(strings.TrimPrefix(line, "$ "))
		} else {
			lines[i] = c.th.HeaderVal.Render(line)
		}
	}

	body := c.th.ModalTitle.Render("kubectl command") + " " + c.th.Dim.Render("esc to close") +
		"\n\n" + strings.Join(lines, "\n")
	box := c.th.ModalBorder.Width(boxW).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (a App) kubectlCommand() string {
	switch a.screen {
	case screenCockpit:
		return a.kubectlCockpitCommand()
	case screenConfig:
		if a.configTarget.name != "" {
			return a.kubectlGetObjectCommand(a.configTarget)
		}
	case screenDetail:
		if a.detailTarget.name != "" {
			return a.kubectlGetObjectCommand(a.detailTarget)
		}
	case screenLogs:
		if a.logs.pod != "" {
			return a.kubectlLogsCommand()
		}
	}
	return a.kubectlGetTableCommand()
}

func (a App) kubectlBaseArgs() []string {
	args := []string{"kubectl"}
	if a.client == nil {
		return args
	}
	if kubeconfig := a.client.Kubeconfig(); kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	if contextName := a.client.ContextName; contextName != "" {
		args = append(args, "--context", contextName)
	}
	return args
}

func (a App) kubectlGetTableCommand() string {
	args := append(a.kubectlBaseArgs(), "get", kubectlResource(a.res))
	args = append(args, kubectlNamespaceArgs(a.res, a.namespace)...)
	if a.table.showWide {
		args = append(args, "-o", "wide")
	}
	return shellJoin(args)
}

func (a App) kubectlGetObjectCommand(t target) string {
	args := append(a.kubectlBaseArgs(), "get", kubectlResource(t.res), t.name)
	args = append(args, kubectlObjectNamespaceArgs(t.res, t.ns, a.namespace)...)
	args = append(args, "-o", "yaml")
	return shellJoin(args)
}

func (a App) kubectlLogsCommand() string {
	args := append(a.kubectlBaseArgs(), "logs")
	if a.logs.ns != "" {
		args = append(args, "-n", a.logs.ns)
	}
	args = append(args, a.logs.pod)
	if a.logs.cont != "" {
		args = append(args, "-c", a.logs.cont)
	}
	args = append(args, "--tail", strconv.FormatInt(logTailLines, 10), "-f")
	return shellJoin(args)
}

func (a App) kubectlCockpitCommand() string {
	base := a.kubectlBaseArgs()
	cmd := func(args ...string) []string {
		out := make([]string, 0, len(base)+len(args))
		out = append(out, base...)
		return append(out, args...)
	}
	commands := [][]string{
		cmd("get", "nodes"),
		cmd("get", "pods", "--all-namespaces"),
		cmd("get", "deployments", "--all-namespaces"),
		cmd("get", "events", "--all-namespaces", "--field-selector", "type=Warning"),
	}
	parts := make([]string, 0, len(commands))
	for _, cmd := range commands {
		parts = append(parts, shellJoin(cmd))
	}
	return strings.Join(parts, " && ")
}

func kubectlResource(res k8s.ResourceInfo) string {
	if res.Resource == "" {
		return "pods"
	}
	return res.Key()
}

func kubectlNamespaceArgs(res k8s.ResourceInfo, namespace string) []string {
	if !res.Namespaced {
		return nil
	}
	if namespace == "" {
		return []string{"--all-namespaces"}
	}
	return []string{"-n", namespace}
}

func kubectlObjectNamespaceArgs(res k8s.ResourceInfo, namespace, fallback string) []string {
	if !res.Namespaced {
		return nil
	}
	if namespace == "" {
		namespace = fallback
	}
	if namespace == "" {
		return nil
	}
	return []string{"-n", namespace}
}

func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = shellArg(arg)
	}
	return strings.Join(parts, " ")
}

func shellArg(s string) string {
	if s == "" {
		return "''"
	}
	if isSafeShellArg(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func isSafeShellArg(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("@%_+=:,./-", r):
		default:
			return false
		}
	}
	return true
}

func wrapPlain(s string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		for len(word) > width {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			lines = append(lines, word[:width])
			word = word[width:]
		}
		if line == "" {
			line = word
			continue
		}
		if len(line)+1+len(word) <= width {
			line += " " + word
			continue
		}
		lines = append(lines, line)
		line = word
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}
