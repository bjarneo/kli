package ui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/kli/internal/k8s"
)

const (
	opTimeout       = 15 * time.Second
	refreshInterval = 2 * time.Second
	logTailLines    = 1000 // initial server-side tail, so we never fetch the whole log
	logBatchMax     = 2048 // lines drained per update, so a burst renders at once
)

// --- messages ---------------------------------------------------------------

type statusMsg struct {
	text string
	err  bool
}

type resourcesLoadedMsg struct {
	client *k8s.Client
	seq    int
	res    k8s.ResourceInfo
	ns     string
	tbl    *k8s.Table
	err    error
}

type detailLoadedMsg struct {
	client   *k8s.Client
	seq      int
	res      k8s.ResourceInfo
	ns, name string
	title    string
	yaml     string
	err      error
}

type configLoadedMsg struct {
	client   *k8s.Client
	seq      int
	res      k8s.ResourceInfo
	ns, name string
	title    string
	obj      map[string]interface{}
	usage    *k8s.PodUsage
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

type deploymentLogsMsg struct {
	ns, name string
	targets  []k8s.LogTarget
	err      error
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
	client   *k8s.Client
	seq      int
	overview *k8s.ClusterOverview
	err      error
}

type editReadyMsg struct {
	client   *k8s.Client
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

// crdsDiscoveredMsg carries the result of the sidebar's CRD discovery button.
// client guards against a context switch landing a stale result.
type crdsDiscoveredMsg struct {
	client *k8s.Client
	crds   []k8s.ResourceInfo
}

// --- commands ---------------------------------------------------------------

func opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}

func discoverCRDsCmd(cl *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		return crdsDiscoveredMsg{client: cl, crds: cl.DiscoverCRDs(ctx)}
	}
}

func loadResourceCmd(cl *k8s.Client, seq int, res k8s.ResourceInfo, ns string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		tbl, err := cl.ListTable(ctx, res, ns)
		if err == nil {
			// Best-effort live usage columns; skipped if metrics are absent.
			switch {
			case res.IsNodes():
				_ = cl.AppendNodeStats(ctx, tbl)
			case res.IsPod():
				_ = cl.AppendPodStats(ctx, tbl, ns)
			}
		}
		return resourcesLoadedMsg{client: cl, seq: seq, res: res, ns: ns, tbl: tbl, err: err}
	}
}

func loadDetailCmd(cl *k8s.Client, seq int, res k8s.ResourceInfo, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		y, err := cl.GetYAML(ctx, res, ns, name, true) // decode secrets for viewing
		title := res.Resource + "/" + name
		if ns != "" {
			title = ns + "/" + name
		}
		return detailLoadedMsg{client: cl, seq: seq, res: res, ns: ns, name: name, title: title, yaml: y, err: err}
	}
}

func loadConfigCmd(cl *k8s.Client, seq int, res k8s.ResourceInfo, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		obj, err := cl.GetObject(ctx, res, ns, name)
		var usage *k8s.PodUsage
		if err == nil && res.IsPod() && ns != "" {
			if u, uerr := cl.PodUsage(ctx, ns, name); uerr == nil {
				usage = &u
			}
		}
		title := res.Resource + "/" + name
		if ns != "" {
			title = ns + "/" + name
		}
		return configLoadedMsg{client: cl, seq: seq, res: res, ns: ns, name: name, title: title, obj: obj, usage: usage, err: err}
	}
}

type nodeDebugReadyMsg struct {
	client    *k8s.Client
	ns        string
	pod       string
	container string
	node      string
	err       error
}

// createNodeDebugCmd spawns a debug pod on the node and waits for it to run.
// It uses a longer timeout than opCtx because the image may need pulling.
func createNodeDebugCmd(cl *k8s.Client, ns, node string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		pod, container, err := cl.CreateNodeDebugPod(ctx, ns, node)
		return nodeDebugReadyMsg{client: cl, ns: ns, pod: pod, container: container, node: node, err: err}
	}
}

// deletePodCmd removes a pod without surfacing a status message (used to clean
// up node debug pods).
func deletePodCmd(cl *k8s.Client, ns, pod string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		_ = cl.DeletePod(ctx, ns, pod)
		return nil
	}
}

func loadCockpitCmd(cl *k8s.Client, seq int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		o, err := cl.ClusterStats(ctx)
		return cockpitLoadedMsg{client: cl, seq: seq, overview: o, err: err}
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

func deploymentLogsCmd(cl *k8s.Client, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		targets, err := cl.DeploymentLogTargets(ctx, ns, name)
		return deploymentLogsMsg{ns: ns, name: name, targets: targets, err: err}
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

func triggerJobCmd(cl *k8s.Client, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		job, err := cl.TriggerCronJob(ctx, ns, name)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{text: "triggered job " + job}
	}
}

func switchContextCmd(name, kubeconfig string) tea.Cmd {
	return func() tea.Msg {
		cl, err := k8s.NewClient(name, kubeconfig)
		return clientReadyMsg{client: cl, err: err}
	}
}

func prepareEditCmd(cl *k8s.Client, res k8s.ResourceInfo, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := opCtx()
		defer cancel()
		y, err := cl.GetYAML(ctx, res, ns, name, false) // raw base64 for round-trip edits
		if err != nil {
			return editReadyMsg{client: cl, err: err}
		}
		safe := strings.NewReplacer("/", "_", " ", "_").Replace(name)
		f, err := os.CreateTemp("", "kli-"+safe+"-*.yaml")
		if err != nil {
			return editReadyMsg{client: cl, err: err}
		}
		if _, err := f.WriteString(y); err != nil {
			f.Close()
			return editReadyMsg{client: cl, err: err}
		}
		f.Close()
		return editReadyMsg{client: cl, path: f.Name(), original: y, res: res, ns: ns, name: name}
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

func cancelEditCmd(path string) tea.Cmd {
	return func() tea.Msg {
		os.Remove(path)
		return statusMsg{text: "edit cancelled", err: false}
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
