package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type nodeUsage struct {
	cpuMilli int64
	memBytes int64
}

// PodUsage is a live CPU and memory sample for a single pod.
type PodUsage struct {
	CPUUsedMilli int64
	MemUsedBytes int64
}

// AppendNodeStats augments a nodes table with live CPU and memory usage from
// the metrics API (metrics.k8s.io). It is best-effort: if metrics are
// unavailable it returns an error and leaves the table unchanged.
func (c *Client) AppendNodeStats(ctx context.Context, t *Table) error {
	usage, err := c.nodeUsage(ctx)
	if err != nil {
		return err
	}
	if len(usage) == 0 {
		return fmt.Errorf("no node metrics")
	}
	alloc := c.nodeAllocatable(ctx) // best-effort, for percentages

	t.Columns = append(t.Columns,
		Column{Name: "CPU"}, Column{Name: "CPU%"},
		Column{Name: "MEM"}, Column{Name: "MEM%"})

	for i := range t.Rows {
		u, ok := usage[t.Rows[i].Name]
		if !ok {
			t.Rows[i].Cells = append(t.Rows[i].Cells, "-", "-", "-", "-")
			continue
		}
		cpuPct, memPct := "-", "-"
		if a, ok := alloc[t.Rows[i].Name]; ok {
			if a.cpuMilli > 0 {
				cpuPct = fmt.Sprintf("%d%%", u.cpuMilli*100/a.cpuMilli)
			}
			if a.memBytes > 0 {
				memPct = fmt.Sprintf("%d%%", u.memBytes*100/a.memBytes)
			}
		}
		t.Rows[i].Cells = append(t.Rows[i].Cells,
			fmt.Sprintf("%dm", u.cpuMilli),
			cpuPct,
			fmt.Sprintf("%dMi", u.memBytes/(1024*1024)),
			memPct)
	}
	return nil
}

// AppendPodStats augments a pods table with live CPU and memory usage (summed
// across containers) from the metrics API. Best-effort: returns an error and
// leaves the table unchanged when metrics are unavailable. namespace is "" to
// match an all-namespaces listing.
func (c *Client) AppendPodStats(ctx context.Context, t *Table, namespace string) error {
	usage, err := c.podUsage(ctx, namespace)
	if err != nil {
		return err
	}
	if len(usage) == 0 {
		return fmt.Errorf("no pod metrics")
	}
	t.Columns = append(t.Columns, Column{Name: "CPU"}, Column{Name: "MEM"})
	for i := range t.Rows {
		u, ok := usage[t.Rows[i].Namespace+"/"+t.Rows[i].Name]
		if !ok {
			t.Rows[i].Cells = append(t.Rows[i].Cells, "-", "-")
			continue
		}
		t.Rows[i].Cells = append(t.Rows[i].Cells,
			fmt.Sprintf("%dm", u.cpuMilli),
			fmt.Sprintf("%dMi", u.memBytes/(1024*1024)))
	}
	return nil
}

// PodUsage fetches live CPU and memory usage for one pod from metrics.k8s.io.
func (c *Client) PodUsage(ctx context.Context, namespace, pod string) (PodUsage, error) {
	usage, err := c.podUsage(ctx, namespace)
	if err != nil {
		return PodUsage{}, err
	}
	u, ok := usage[namespace+"/"+pod]
	if !ok {
		return PodUsage{}, fmt.Errorf("pod metrics not found")
	}
	return PodUsage{CPUUsedMilli: u.cpuMilli, MemUsedBytes: u.memBytes}, nil
}

func (c *Client) podUsage(ctx context.Context, namespace string) (map[string]nodeUsage, error) {
	gv := schema.GroupVersion{Group: "metrics.k8s.io", Version: "v1beta1"}
	rc, err := c.restClientFor(gv)
	if err != nil {
		return nil, err
	}
	req := rc.Get().Resource("pods")
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	raw, err := req.Do(ctx).Raw()
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Containers []struct {
				Usage struct {
					CPU    string `json:"cpu"`
					Memory string `json:"memory"`
				} `json:"usage"`
			} `json:"containers"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make(map[string]nodeUsage, len(list.Items))
	for _, it := range list.Items {
		var u nodeUsage
		for _, ct := range it.Containers {
			if q, err := resource.ParseQuantity(ct.Usage.CPU); err == nil {
				u.cpuMilli += q.MilliValue()
			}
			if q, err := resource.ParseQuantity(ct.Usage.Memory); err == nil {
				u.memBytes += q.Value()
			}
		}
		out[it.Metadata.Namespace+"/"+it.Metadata.Name] = u
	}
	return out, nil
}

func (c *Client) nodeUsage(ctx context.Context) (map[string]nodeUsage, error) {
	gv := schema.GroupVersion{Group: "metrics.k8s.io", Version: "v1beta1"}
	rc, err := c.restClientFor(gv)
	if err != nil {
		return nil, err
	}
	raw, err := rc.Get().Resource("nodes").Do(ctx).Raw()
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Usage struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"usage"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make(map[string]nodeUsage, len(list.Items))
	for _, it := range list.Items {
		var u nodeUsage
		if q, err := resource.ParseQuantity(it.Usage.CPU); err == nil {
			u.cpuMilli = q.MilliValue()
		}
		if q, err := resource.ParseQuantity(it.Usage.Memory); err == nil {
			u.memBytes = q.Value()
		}
		out[it.Metadata.Name] = u
	}
	return out, nil
}

func (c *Client) nodeAllocatable(ctx context.Context) map[string]nodeUsage {
	out := map[string]nodeUsage{}
	list, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return out
	}
	for i := range list.Items {
		n := &list.Items[i]
		var u nodeUsage
		if q, ok := n.Status.Allocatable[corev1.ResourceCPU]; ok {
			u.cpuMilli = q.MilliValue()
		}
		if q, ok := n.Status.Allocatable[corev1.ResourceMemory]; ok {
			u.memBytes = q.Value()
		}
		out[n.Name] = u
	}
	return out
}
