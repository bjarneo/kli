package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/bjarneo/kli/internal/k8s"
)

const cockpitPanelGap = 1

// cockpitView renders the cluster overview dashboard shown on launch.
type cockpitView struct {
	th    Theme
	data  *k8s.ClusterOverview
	ready bool
}

func newCockpitView(th Theme) cockpitView { return cockpitView{th: th} }

func (c *cockpitView) setData(o *k8s.ClusterOverview) {
	c.data = o
	c.ready = true
}

// View renders the dashboard to exactly width x height, clamping so it can
// never break the surrounding layout.
func (c cockpitView) View(width, height int) string {
	if !c.ready || c.data == nil {
		return clampBlock(c.th.Dim.Render("  loading cluster overview…"), width, height)
	}
	o := c.data
	th := c.th

	cluster := []string{
		c.kv("version", valOrDash(o.Version)),
		c.kv("nodes", fmt.Sprintf("%d/%d ready", o.NodesReady, o.Nodes)),
		c.kv("namespaces", fmt.Sprintf("%d", o.Namespaces)),
	}
	if len(o.NodeIssues) > 0 {
		msg := o.NodeIssues[0]
		if extra := len(o.NodeIssues) - 1; extra > 0 {
			msg += fmt.Sprintf(" +%d", extra)
		}
		cluster = append(cluster, th.Bad.Render("⚠ "+msg))
	}

	var resources []string
	if o.HasMetrics {
		cpuPct := pct(o.CPUUsedMilli, o.CPUAllocMilli)
		memPct := pct(o.MemUsedBytes, o.MemAllocBytes)
		resources = []string{
			c.gauge("CPU", cpuPct, gaugeWidth(width)),
			c.gauge("MEM", memPct, gaugeWidth(width)),
			th.Dim.Render(fmt.Sprintf("%s / %s cores · %s / %s",
				cores(o.CPUUsedMilli), cores(o.CPUAllocMilli),
				gib(o.MemUsedBytes), gib(o.MemAllocBytes))),
		}
	} else {
		resources = []string{th.Dim.Render("metrics unavailable"), th.Dim.Render("(install metrics-server)")}
	}

	workloads := []string{
		c.kv("pods", fmt.Sprintf("%d", o.Pods)),
		"  " + th.Good.Render(fmt.Sprintf("%d running", o.PodRunning)),
		"  " + th.Warn.Render(fmt.Sprintf("%d pending", o.PodPending)),
		"  " + th.Bad.Render(fmt.Sprintf("%d failed", o.PodFailed)),
	}
	if o.PodNotReady > 0 {
		workloads = append(workloads, "  "+th.Warn.Render(fmt.Sprintf("%d not ready", o.PodNotReady)))
	}
	if o.PodCrashLoop > 0 {
		workloads = append(workloads, "  "+th.Bad.Render(fmt.Sprintf("%d crashloop", o.PodCrashLoop)))
	}
	workloads = append(workloads, c.kv("deploys", fmt.Sprintf("%d/%d ready", o.DeploymentsReady, o.Deployments)))

	// Top region: three columns when wide, stacked otherwise.
	var top string
	topH := clamp(height/2, 5, 9)
	if width >= 72 {
		w1 := width / 3
		w2 := width / 3
		w3 := width - w1 - w2
		top = lipgloss.JoinHorizontal(lipgloss.Top,
			c.panel("Cluster", cluster, w1, topH),
			c.panel("Resources", resources, w2, topH),
			c.panel("Workloads", workloads, w3, topH),
		)
	} else {
		top = lipgloss.JoinVertical(lipgloss.Left,
			c.panel("Cluster", cluster, width, 5),
			c.panel("Resources", resources, width, 5),
			c.panel("Workloads", workloads, width, 7),
		)
		topH = lipgloss.Height(top)
	}

	warnH := height - topH - cockpitPanelGap
	if warnH < 3 {
		warnH = 3
	}
	warnings := c.panel("Recent warnings", c.warningLines(paneContentWidth(width)), width, warnH)

	return clampBlock(lipgloss.JoinVertical(lipgloss.Left, top, "", warnings), width, height)
}

func (c cockpitView) warningLines(w int) []string {
	o := c.data
	th := c.th
	if len(o.Warnings) == 0 {
		return []string{th.Good.Render("no recent warnings")}
	}
	lines := make([]string, 0, len(o.Warnings))
	for _, e := range o.Warnings {
		loc := e.Object
		if e.Namespace != "" {
			loc = e.Namespace + "/" + e.Object
		}
		reason := e.Reason
		if e.Count > 1 {
			reason += fmt.Sprintf(" ×%d", e.Count)
		}
		left := fmt.Sprintf("%-4s ", e.Age) + th.HeaderVal.Render(truncate(loc, 36)) + " " + th.Warn.Render(reason)
		msg := th.Dim.Render(e.Message)
		line := left + "  " + msg
		lines = append(lines, ansi.Truncate(line, w, "…"))
	}
	return lines
}

func (c cockpitView) kv(k, v string) string {
	return c.th.HeaderKey.Render(fmt.Sprintf("%-11s", k)) + v
}

// gauge renders "LABEL [████░░░░] 67%" colored by utilization.
func (c cockpitView) gauge(label string, p, width int) string {
	th := c.th
	// layout: "LABEL " + bar + " 100%"  => label + 1 + barW + 1 + 4
	barW := width - len(label) - 6
	if barW < 4 {
		barW = 4
	}
	filled := clamp(barW*p/100, 0, barW)
	style := th.Good
	switch {
	case p >= 85:
		style = th.Bad
	case p >= 70:
		style = th.Warn
	}
	bar := style.Render(strings.Repeat("█", filled)) + th.Dim.Render(strings.Repeat("░", barW-filled))
	return fmt.Sprintf("%s %s %3d%%", label, bar, p)
}

// panel draws a titled, bordered box of exactly outerW x outerH, clipping its
// content (which may be styled) to fit.
func (c cockpitView) panel(title string, lines []string, outerW, outerH int) string {
	innerW := paneContentWidth(outerW)
	innerH := paneContentHeight(outerH)
	content := []string{c.th.ModalTitle.Render(truncate(title, innerW))}
	for _, ln := range lines {
		if len(content) >= innerH {
			break
		}
		content = append(content, ansi.Truncate(ln, innerW, "…"))
	}
	return c.th.PaneInactive.Width(paneStyleWidth(outerW)).Height(paneStyleHeight(outerH)).Render(strings.Join(content, "\n"))
}

// --- helpers ---------------------------------------------------------------

func gaugeWidth(width int) int {
	return clamp(width/3-4, 12, 40)
}

func pct(used, total int64) int {
	if total <= 0 {
		return 0
	}
	return clamp(int(used*100/total), 0, 100)
}

func cores(milli int64) string { return fmt.Sprintf("%.1f", float64(milli)/1000) }

func gib(b int64) string { return fmt.Sprintf("%.1fGi", float64(b)/(1024*1024*1024)) }

func valOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// clampBlock forces a rendered block to exactly width x height lines.
func clampBlock(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
