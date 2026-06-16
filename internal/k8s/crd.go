package k8s

import (
	"context"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// crdGVR is the cluster-scoped CustomResourceDefinition list.
var crdGVR = schema.GroupVersionResource{
	Group:    "apiextensions.k8s.io",
	Version:  "v1",
	Resource: "customresourcedefinitions",
}

// DiscoverCRDs lists the cluster's CustomResourceDefinitions and returns the
// resources they define, sorted by group then resource. A nil result is normal:
// no CRDs installed, no apiextensions API, or no permission to list them.
func (c *Client) DiscoverCRDs(ctx context.Context) []ResourceInfo {
	list, err := c.dynamic.Resource(crdGVR).List(ctx, metav1.ListOptions{})
	if err != nil || list == nil {
		return nil
	}
	seen := make(map[string]bool, len(list.Items))
	out := make([]ResourceInfo, 0, len(list.Items))
	for i := range list.Items {
		ri, ok := c.registry.crdResource(list.Items[i].Object)
		if !ok || seen[ri.Key()] {
			continue
		}
		seen[ri.Key()] = true
		out = append(out, ri)
	}
	sort.Slice(out, func(i, j int) bool { return resourceLess(out[i], out[j]) })
	return out
}

// crdResource resolves a CustomResourceDefinition object to a resource. It
// prefers the version discovery already knows (consistent with the rest of the
// catalog) and falls back to the CRD's served version, so a CRD missed by a
// partial discovery still resolves.
func (reg *Registry) crdResource(obj map[string]any) (ResourceInfo, bool) {
	group, _, _ := unstructured.NestedString(obj, "spec", "group")
	plural, _, _ := unstructured.NestedString(obj, "spec", "names", "plural")
	if group == "" || plural == "" {
		return ResourceInfo{}, false
	}
	if ri, ok := reg.Resolve(resourceKey(plural, group)); ok {
		return ri, true
	}
	version := crdServedVersion(obj)
	if version == "" {
		return ResourceInfo{}, false // nothing served means nothing to list
	}
	kind, _, _ := unstructured.NestedString(obj, "spec", "names", "kind")
	singular, _, _ := unstructured.NestedString(obj, "spec", "names", "singular")
	shortNames, _, _ := unstructured.NestedStringSlice(obj, "spec", "names", "shortNames")
	scope, _, _ := unstructured.NestedString(obj, "spec", "scope")
	return ResourceInfo{
		Group:      group,
		Version:    version,
		Resource:   plural,
		Kind:       kind,
		Singular:   singular,
		ShortNames: shortNames,
		Namespaced: scope == "Namespaced",
	}, true
}

// crdServedVersion returns the CRD's storage version, or the first served
// version, or "" when none are served.
func crdServedVersion(obj map[string]any) string {
	versions, _, _ := unstructured.NestedSlice(obj, "spec", "versions")
	first := ""
	for _, v := range versions {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		served, _, _ := unstructured.NestedBool(m, "served")
		name, _, _ := unstructured.NestedString(m, "name")
		if !served || name == "" {
			continue
		}
		if storage, _, _ := unstructured.NestedBool(m, "storage"); storage {
			return name
		}
		if first == "" {
			first = name
		}
	}
	return first
}
