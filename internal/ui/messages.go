package ui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bjarneo/kli/internal/k8s"
)

const (
	opTimeout       = 15 * time.Second
	refreshInterval = 2 * time.Second
	logTailLines    = 1000
)

// --- messages ---------------------------------------------------------------

type statusMsg struct {
	text string
	err  bool
}

type resourcesLoadedMsg struct {
	res k8s.ResourceInfo
	ns  string
	tbl *k8s.Table
	err error
}

type detailLoadedMsg struct {
	res      k8s.ResourceInfo
	ns, name string
	title    string
	yaml     string
	err      error
}

type namespacesMsg struct {
	names []string
	err   error
}

type containersMsg struct {
	ns, pod string
	names   []string
	forExec bool // true: open a shell; false: stream logs
	err     error
}

type actionDoneMsg struct {
	text   string
	err    error
	reload bool
}

type clientReadyMsg struct {
	client *k8s.Client
	err    error
}

type cockpitLoadedMsg struct {
	overview *k8s.ClusterOverview
	err      error
}

type editReadyMsg struct {
	path     string
	original string
	res      k8s.ResourceInfo
	ns, name string
	err      error
}

type tickMsg time.Time

// termTickMsg drives the embedded terminal's repaint loop.
type termTickMsg struct{ session int }

// termDoneMsg signals that an exec session ended.
type termDoneMsg struct {
	session int
	err     error
}

// logEvent carries one streamed log line, or signals end/error of a stream.
// session distinguishes the active stream from a stale one left behind.
type logEvent struct {
	session int
	line    string
	err     error
	done    bool
}

// --- commands ---------------------------------------------------------------

func opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}

func loadResourceCmd(cl *k8s.Client, res k8s.ResourceInfo, ns string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		tbl, err := cl.ListTable(ctx, res, ns)
		if err == nil && res.IsNodes() {
			_ = cl.AppendNodeStats(ctx, tbl) // best-effort: skip if metrics are absent
		}
		return resourcesLoadedMsg{res: res, ns: ns, tbl: tbl, err: err}
	}
}

func loadDetailCmd(cl *k8s.Client, res k8s.ResourceInfo, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		y, err := cl.GetYAML(ctx, res, ns, name, true) // decode secrets for viewing
		title := res.Resource + "/" + name
		if ns != "" {
			title = ns + "/" + name
		}
		return detailLoadedMsg{res: res, ns: ns, name: name, title: title, yaml: y, err: err}
	}
}

func loadCockpitCmd(cl *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		o, err := cl.ClusterStats(ctx)
		return cockpitLoadedMsg{overview: o, err: err}
	}
}

func namespacesCmd(cl *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		names, err := cl.Namespaces(ctx)
		return namespacesMsg{names: names, err: err}
	}
}

func containersCmd(cl *k8s.Client, ns, pod string, forExec bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		names, err := cl.PodContainers(ctx, ns, pod)
		return containersMsg{ns: ns, pod: pod, names: names, forExec: forExec, err: err}
	}
}

func deleteCmd(cl *k8s.Client, res k8s.ResourceInfo, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		err := cl.Delete(ctx, res, ns, name)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{text: "deleted " + name, reload: true}
	}
}

func scaleCmd(cl *k8s.Client, res k8s.ResourceInfo, ns, name string, n int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		err := cl.Scale(ctx, res, ns, name, n)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{text: "scaled " + name, reload: true}
	}
}

func restartCmd(cl *k8s.Client, res k8s.ResourceInfo, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		if err := cl.RolloutRestart(ctx, res, ns, name); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{text: "restarted " + name, reload: true}
	}
}

func switchContextCmd(name string) tea.Cmd {
	return func() tea.Msg {
		cl, err := k8s.NewClient(name)
		return clientReadyMsg{client: cl, err: err}
	}
}

func prepareEditCmd(cl *k8s.Client, res k8s.ResourceInfo, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		y, err := cl.GetYAML(ctx, res, ns, name, false) // raw base64 for round-trip edits
		if err != nil {
			return editReadyMsg{err: err}
		}
		safe := strings.NewReplacer("/", "_", " ", "_").Replace(name)
		f, err := os.CreateTemp("", "kli-"+safe+"-*.yaml")
		if err != nil {
			return editReadyMsg{err: err}
		}
		if _, err := f.WriteString(y); err != nil {
			f.Close()
			return editReadyMsg{err: err}
		}
		f.Close()
		return editReadyMsg{path: f.Name(), original: y, res: res, ns: ns, name: name}
	}
}

func applyEditCmd(cl *k8s.Client, res k8s.ResourceInfo, ns, name, path string) tea.Cmd {
	return func() tea.Msg {
		defer os.Remove(path)
		data, err := os.ReadFile(path)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		ctx, cancel := opCtx()
		defer cancel()
		if err := cl.Apply(ctx, res, ns, name, data); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{text: "applied " + name, reload: true}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// editorCommand returns the program and args to open path. It honors $EDITOR
// then $VISUAL (which may include flags), then falls back to whatever is
// installed, preferring nvim, then vim, nano, and finally vi.
func editorCommand(path string) (string, []string) {
	ed := strings.TrimSpace(os.Getenv("EDITOR"))
	if ed == "" {
		ed = strings.TrimSpace(os.Getenv("VISUAL"))
	}
	if ed == "" {
		for _, cand := range []string{"nvim", "vim", "nano", "vi"} {
			if _, err := exec.LookPath(cand); err == nil {
				ed = cand
				break
			}
		}
	}
	if ed == "" {
		ed = "vi"
	}
	fields := strings.Fields(ed)
	args := append(fields[1:], path)
	return fields[0], args
}
