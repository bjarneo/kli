package k8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Cordon marks a node unschedulable so the scheduler places no new pods on it.
// Uncordon reverses it. Both are merge patches on spec.unschedulable, the same
// mechanism as kubectl cordon/uncordon.
func (c *Client) Cordon(ctx context.Context, name string) error {
	return c.setUnschedulable(ctx, name, true)
}

func (c *Client) Uncordon(ctx context.Context, name string) error {
	return c.setUnschedulable(ctx, name, false)
}

func (c *Client) setUnschedulable(ctx context.Context, name string, v bool) error {
	patch := []byte(fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, v))
	_, err := c.clientset.CoreV1().Nodes().Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

// NodeCordoned reports whether a node is currently unschedulable, so the UI can
// present cordon vs uncordon.
func (c *Client) NodeCordoned(ctx context.Context, name string) (bool, error) {
	n, err := c.clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return n.Spec.Unschedulable, nil
}

// DrainResult summarizes a drain: how many pods were evicted, which were left in
// place and why, and which could not be evicted.
type DrainResult struct {
	Evicted int
	Skipped []string // "ns/name (reason)" left running by design
	Failed  []string // "ns/name: reason" that could not be evicted
}

// evictRetry is how long to wait between eviction attempts that a
// PodDisruptionBudget rejects, before trying again.
const evictRetry = 2 * time.Second

// Drain cordons a node and evicts its pods, mirroring `kubectl drain
// --ignore-daemonsets`. DaemonSet-managed and mirror (static) pods are left in
// place since they cannot be meaningfully evicted; already-finished pods are
// skipped. Eviction goes through the Eviction API so PodDisruptionBudgets are
// honored: a budget-blocked pod is retried until ctx is done. The context
// deadline bounds the whole operation, so callers should pass a generous one.
func (c *Client) Drain(ctx context.Context, name string) (DrainResult, error) {
	var res DrainResult
	if err := c.Cordon(ctx, name); err != nil {
		return res, fmt.Errorf("cordon: %w", err)
	}
	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + name,
	})
	if err != nil {
		return res, fmt.Errorf("list pods: %w", err)
	}
	for i := range pods.Items {
		p := &pods.Items[i]
		if reason, skip := drainSkipReason(p); skip {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s/%s (%s)", p.Namespace, p.Name, reason))
			continue
		}
		if err := c.evictPod(ctx, p.Namespace, p.Name); err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s/%s: %v", p.Namespace, p.Name, err))
			continue
		}
		res.Evicted++
	}
	if len(res.Failed) > 0 {
		return res, fmt.Errorf("%d pod(s) could not be evicted", len(res.Failed))
	}
	return res, nil
}

// drainSkipReason reports whether a pod should be left in place during a drain,
// and why. Mirrors kubectl's default --ignore-daemonsets behavior.
func drainSkipReason(p *corev1.Pod) (string, bool) {
	if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
		return "completed", true
	}
	if _, ok := p.Annotations[corev1.MirrorPodAnnotationKey]; ok {
		return "mirror pod", true
	}
	for _, o := range p.OwnerReferences {
		if o.Kind == "DaemonSet" {
			return "daemonset", true
		}
	}
	return "", false
}

// evictPod requests one pod eviction, retrying while a PodDisruptionBudget
// rejects it (HTTP 429) until ctx is done. A pod already gone counts as success.
func (c *Client) evictPod(ctx context.Context, namespace, name string) error {
	for {
		err := c.clientset.PolicyV1().Evictions(namespace).Evict(ctx, &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		})
		switch {
		case err == nil, apierrors.IsNotFound(err):
			return nil
		case apierrors.IsTooManyRequests(err):
			// A PodDisruptionBudget would be violated; wait for capacity and retry.
			select {
			case <-ctx.Done():
				return fmt.Errorf("blocked by PodDisruptionBudget")
			case <-time.After(evictRetry):
			}
		default:
			return err
		}
	}
}
