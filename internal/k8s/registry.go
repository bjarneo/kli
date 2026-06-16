package k8s

import (
	"slices"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceInfo describes a single API resource kind discovered on the server.
type ResourceInfo struct {
	Group      string
	Version    string
	Resource   string // plural, e.g. "pods"
	Kind       string // e.g. "Pod"
	Singular   string // e.g. "pod"
	ShortNames []string
	Namespaced bool
}

// GVR returns the GroupVersionResource used by the dynamic client.
func (r ResourceInfo) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: r.Group, Version: r.Version, Resource: r.Resource}
}

// Key is the canonical lookup key (the plural resource name, group-qualified
// when not in the core group) used to identify a resource unambiguously.
func (r ResourceInfo) Key() string {
	return resourceKey(r.Resource, r.Group)
}

// resourceKey formats the canonical lookup key: the plural resource name,
// group-qualified when not in the core group.
func resourceKey(resource, group string) string {
	if group == "" {
		return resource
	}
	return resource + "." + group
}

// Title is a short human label for the resource shown in the header.
func (r ResourceInfo) Title() string {
	return r.Resource
}

// IsPod reports whether this resource is the core Pod type.
func (r ResourceInfo) IsPod() bool {
	return r.Group == "" && r.Resource == "pods"
}

// IsDeployment reports whether this resource is an apps Deployment list.
func (r ResourceInfo) IsDeployment() bool {
	return r.Resource == "deployments" && (r.Group == "" || r.Group == "apps")
}

// Scalable reports whether the resource exposes spec.replicas.
func (r ResourceInfo) Scalable() bool {
	switch r.Resource {
	case "deployments", "statefulsets", "replicasets", "replicationcontrollers":
		return true
	}
	return false
}

// Restartable reports whether the resource supports a rolling restart.
func (r ResourceInfo) Restartable() bool {
	switch r.Resource {
	case "deployments", "statefulsets", "daemonsets":
		return true
	}
	return false
}

// IsCronJob reports whether this is a batch CronJob list.
func (r ResourceInfo) IsCronJob() bool {
	return r.Group == "batch" && r.Resource == "cronjobs"
}

// IsNodes reports whether this is the core Node list.
func (r ResourceInfo) IsNodes() bool {
	return r.Group == "" && r.Resource == "nodes"
}

// Registry is an in-memory catalog of discovered resources with alias lookup.
type Registry struct {
	all   []ResourceInfo
	byKey map[string]ResourceInfo
}

// NewRegistry builds a registry from a set of resources, indexed and sorted.
func NewRegistry(resources []ResourceInfo) *Registry {
	reg := &Registry{byKey: make(map[string]ResourceInfo, len(resources))}
	reg.Merge(resources)
	return reg
}

// Merge folds resources into the registry, skipping any whose key is already
// known, then re-sorts. Used to add on-demand discovery (CRDs) to the catalog
// so they become searchable alongside the rest.
func (reg *Registry) Merge(resources []ResourceInfo) {
	if reg == nil {
		return
	}
	added := false
	for _, ri := range resources {
		if _, exists := reg.byKey[ri.Key()]; exists {
			continue
		}
		reg.add(ri)
		added = true
	}
	if added {
		sort.Slice(reg.all, func(i, j int) bool { return resourceLess(reg.all[i], reg.all[j]) })
	}
}

// resourceLess orders resources by group, then resource name.
func resourceLess(a, b ResourceInfo) bool {
	if a.Group != b.Group {
		return a.Group < b.Group
	}
	return a.Resource < b.Resource
}

// loadRegistry queries discovery for the server's preferred resources and
// builds the catalog. It tolerates partial discovery failures.
func (c *Client) loadRegistry() error {
	lists, err := c.disco.ServerPreferredResources()
	var infos []ResourceInfo

	for _, list := range lists {
		if list == nil {
			continue
		}
		gv, perr := schema.ParseGroupVersion(list.GroupVersion)
		if perr != nil {
			continue
		}
		for _, ar := range list.APIResources {
			// Skip subresources like pods/log, deployments/scale.
			if strings.Contains(ar.Name, "/") {
				continue
			}
			if !canList(ar.Verbs) {
				continue
			}
			infos = append(infos, ResourceInfo{
				Group:      gv.Group,
				Version:    gv.Version,
				Resource:   ar.Name,
				Kind:       ar.Kind,
				Singular:   ar.SingularName,
				ShortNames: ar.ShortNames,
				Namespaced: ar.Namespaced,
			})
		}
	}

	reg := NewRegistry(infos)
	if err != nil && len(reg.all) == 0 {
		return err
	}

	c.registry = reg
	return err
}

func canList(verbs []string) bool {
	return slices.Contains(verbs, "list")
}

func (reg *Registry) add(ri ResourceInfo) {
	reg.all = append(reg.all, ri)
	// The first resource to claim a key wins. ServerPreferredResources returns
	// the preferred version first, so this keeps the canonical mapping.
	keys := []string{
		strings.ToLower(ri.Resource),
		strings.ToLower(ri.Singular),
		strings.ToLower(ri.Kind),
		ri.Key(),
	}
	for _, sn := range ri.ShortNames {
		keys = append(keys, strings.ToLower(sn))
	}
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, exists := reg.byKey[k]; !exists {
			reg.byKey[k] = ri
		}
	}
}

// Resolve maps a user query (plural, singular, kind, short name, or
// group-qualified key) to a resource. Returns false if unknown.
func (reg *Registry) Resolve(query string) (ResourceInfo, bool) {
	if reg == nil {
		return ResourceInfo{}, false
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if ri, ok := reg.byKey[q]; ok {
		return ri, true
	}
	return ResourceInfo{}, false
}

// All returns the full catalog sorted by group then resource.
func (reg *Registry) All() []ResourceInfo {
	if reg == nil {
		return nil
	}
	return reg.all
}
