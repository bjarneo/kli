package k8s

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventLine is a single recent event for the cockpit. Count is how many times
// the same (namespace, object, reason) recurred.
type EventLine struct {
	Age       string
	Namespace string
	Type      string
	Reason    string
	Object    string
	Message   string
	Count     int
}

// ClusterOverview is the cockpit's snapshot of cluster health and usage.
type ClusterOverview struct {
	Version    string
	Nodes      int
	NodesReady int

	HasMetrics    bool
	CPUUsedMilli  int64
	CPUAllocMilli int64
	MemUsedBytes  int64
	MemAllocBytes int64

	Namespaces int

	Pods         int
	PodRunning   int
	PodPending   int
	PodFailed    int
	PodSucceeded int
	PodNotReady  int // Running but a container is not Ready
	PodCrashLoop int // CrashLoopBackOff / image pull errors

	Deployments      int
	DeploymentsReady int

	NodeIssues []string // e.g. "node-x NotReady", "node-y DiskPressure"

	Warnings []EventLine
}

// ClusterStats gathers a cluster overview. The sections are independent reads
// run concurrently; each is best-effort, so a failure in one (e.g. no metrics)
// leaves its fields zero rather than failing the whole snapshot. Each section
// writes a disjoint set of fields, so no locking is needed.
func (c *Client) ClusterStats(ctx context.Context) (*ClusterOverview, error) {
	o := &ClusterOverview{}

	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}

	run(func() {
		if v, err := c.disco.ServerVersion(); err == nil {
			o.Version = v.GitVersion
		}
	})

	run(func() {
		nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return
		}
		o.Nodes = len(nodes.Items)
		for i := range nodes.Items {
			n := &nodes.Items[i]
			if nodeReady(n) {
				o.NodesReady++
			}
			o.NodeIssues = append(o.NodeIssues, nodeIssues(n)...)
			if q, ok := n.Status.Allocatable[corev1.ResourceCPU]; ok {
				o.CPUAllocMilli += q.MilliValue()
			}
			if q, ok := n.Status.Allocatable[corev1.ResourceMemory]; ok {
				o.MemAllocBytes += q.Value()
			}
		}
	})

	run(func() {
		if usage, err := c.nodeUsage(ctx); err == nil && len(usage) > 0 {
			o.HasMetrics = true
			for _, u := range usage {
				o.CPUUsedMilli += u.cpuMilli
				o.MemUsedBytes += u.memBytes
			}
		}
	})

	run(func() {
		if nss, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err == nil {
			o.Namespaces = len(nss.Items)
		}
	})

	run(func() {
		pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return
		}
		o.Pods = len(pods.Items)
		for i := range pods.Items {
			p := &pods.Items[i]
			switch p.Status.Phase {
			case corev1.PodRunning:
				o.PodRunning++
			case corev1.PodPending:
				o.PodPending++
			case corev1.PodFailed:
				o.PodFailed++
			case corev1.PodSucceeded:
				o.PodSucceeded++
			}
			// A crashlooping or image-pull-failing pod still reports Running, so
			// look at container statuses to find what is actually broken.
			for _, cs := range p.Status.ContainerStatuses {
				if w := cs.State.Waiting; w != nil {
					switch w.Reason {
					case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull":
						o.PodCrashLoop++
					}
				}
			}
			if p.Status.Phase == corev1.PodRunning && !podReady(p) {
				o.PodNotReady++
			}
		}
	})

	run(func() {
		deps, err := c.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return
		}
		o.Deployments = len(deps.Items)
		for i := range deps.Items {
			d := &deps.Items[i]
			// Ready when fully available, or scaled to zero (healthy).
			if d.Status.Replicas == 0 || d.Status.ReadyReplicas >= d.Status.Replicas {
				o.DeploymentsReady++
			}
		}
	})

	run(func() { o.Warnings = c.recentWarnings(ctx) })

	wg.Wait()
	return o, nil
}

func nodeReady(n *corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// nodeIssues returns human-readable problems for a node (not-ready, or a
// pressure/network condition that is True).
func nodeIssues(n *corev1.Node) []string {
	var out []string
	for _, c := range n.Status.Conditions {
		switch c.Type {
		case corev1.NodeReady:
			if c.Status != corev1.ConditionTrue {
				out = append(out, n.Name+" NotReady")
			}
		case corev1.NodeDiskPressure, corev1.NodeMemoryPressure, corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
			if c.Status == corev1.ConditionTrue {
				out = append(out, n.Name+" "+string(c.Type))
			}
		}
	}
	return out
}

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (c *Client) recentWarnings(ctx context.Context) []EventLine {
	list, err := c.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
		Limit:         300,
	})
	if err != nil {
		return nil
	}
	items := list.Items
	sort.Slice(items, func(i, j int) bool {
		return eventTime(&items[i]).After(eventTime(&items[j]))
	})

	// Dedupe by (namespace, object, reason) so one flapping object cannot crowd
	// out other distinct problems; keep the newest and count the rest.
	seen := map[string]int{} // key -> index in out
	var out []EventLine
	for i := range items {
		e := &items[i]
		obj := e.InvolvedObject.Kind
		if e.InvolvedObject.Name != "" {
			obj += "/" + e.InvolvedObject.Name
		}
		key := e.Namespace + "|" + obj + "|" + e.Reason
		if idx, ok := seen[key]; ok {
			out[idx].Count++
			continue
		}
		seen[key] = len(out)
		out = append(out, EventLine{
			Age:       ageString(eventTime(e)),
			Namespace: e.Namespace,
			Type:      e.Type,
			Reason:    e.Reason,
			Object:    obj,
			Message:   e.Message,
			Count:     1,
		})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func eventTime(e *corev1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	return e.CreationTimestamp.Time
}

// ageString renders a compact age like 2m, 3h, or 4d.
func ageString(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
