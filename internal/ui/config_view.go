package ui

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/bjarneo/kli/internal/k8s"
)

const configKeyWidth = 14

// configView renders a curated, read-only configuration summary for common
// Kubernetes objects. Raw YAML remains available through the detail view.
type configView struct {
	th    Theme
	vp    viewport.Model
	title string
	label string
}

func newConfigView(th Theme) configView {
	return configView{th: th, vp: viewport.New(0, 0), label: "config"}
}

func (c *configView) setSize(w, h int) {
	if h < 1 {
		h = 1
	}
	c.vp.Width = w
	c.vp.Height = h - 1
}

func (c *configView) setMessage(title, body string) {
	c.title = title
	c.label = "config"
	c.vp.SetContent(body)
	c.vp.GotoTop()
}

func (c *configView) setObject(res k8s.ResourceInfo, title string, obj map[string]interface{}) {
	c.title = title
	c.label = strings.ToLower(res.Kind) + " config"
	c.vp.SetContent(renderConfig(c.th, res, obj, c.vp.Width))
	c.vp.GotoTop()
}

func (c configView) Update(msg tea.Msg) (configView, tea.Cmd) {
	var cmd tea.Cmd
	c.vp, cmd = c.vp.Update(msg)
	return c, cmd
}

func (c configView) View() string {
	title := c.th.ModalTitle.Render(c.title)
	right := c.th.Dim.Render(c.label + " · " + scrollPercent(c.vp.ScrollPercent()))
	return spread(title, right, c.vp.Width) + "\n" + c.vp.View()
}

type configRow struct{ key, value string }

func renderConfig(th Theme, res k8s.ResourceInfo, obj map[string]interface{}, width int) string {
	var lines []string
	add := func(title string, rows []configRow) {
		if len(rows) == 0 {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, ansi.Truncate(th.ModalTitle.Render(title), width, ""))
		for _, r := range rows {
			lines = append(lines, ansi.Truncate(configKV(th, r.key, r.value), width, ""))
		}
	}

	add("Overview", overviewRows(res, obj))
	switch strings.ToLower(res.Resource) {
	case "deployments", "statefulsets", "daemonsets", "replicasets", "replicationcontrollers":
		add("Workload", workloadRows(obj))
		addPodSpecSections(th, obj, []string{"spec", "template", "spec"}, add)
	case "jobs":
		add("Job", jobRows(obj))
		addPodSpecSections(th, obj, []string{"spec", "template", "spec"}, add)
	case "cronjobs":
		add("Schedule", cronJobRows(obj))
		addPodSpecSections(th, obj, []string{"spec", "jobTemplate", "spec", "template", "spec"}, add)
	case "pods":
		add("Pod", podRows(obj))
		addPodSpecSections(th, obj, []string{"spec"}, add)
	case "configmaps":
		add("ConfigMap", configMapSummaryRows(obj))
		add("Data", configMapDataRows(obj, width))
		add("Binary Data", dataKeyRows(obj, []string{"binaryData"}, "encoded"))
	case "secrets":
		add("Secret", secretRows(obj))
		add("Decoded Data", secretDataRows(th, obj, width))
	case "services":
		add("Service", serviceRows(obj))
	case "ingresses":
		add("Ingress", ingressRows(obj))
	default:
		add("Spec", genericSpecRows(obj))
	}
	if len(lines) == 0 {
		return th.Dim.Render("no config fields found")
	}
	return strings.Join(lines, "\n")
}

func configKV(th Theme, key, value string) string {
	if strings.TrimSpace(value) == "" {
		value = th.Dim.Render("-")
	}
	return th.HeaderKey.Render(fmt.Sprintf("  %-*s", configKeyWidth, key)) + value
}

func overviewRows(res k8s.ResourceInfo, obj map[string]interface{}) []configRow {
	rows := []configRow{{"kind", res.Kind}}
	if ns, ok := stringAt(obj, "metadata", "namespace"); ok && ns != "" {
		rows = append(rows, configRow{"namespace", ns})
	}
	if created, ok := stringAt(obj, "metadata", "creationTimestamp"); ok {
		rows = append(rows, configRow{"created", created})
	}
	if labels, ok := mapAt(obj, "metadata", "labels"); ok {
		rows = append(rows, configRow{"labels", mapSummary(labels, 4)})
	}
	if ann, ok := mapAt(obj, "metadata", "annotations"); ok {
		rows = append(rows, configRow{"annotations", countSummary(len(ann), "annotation")})
	}
	if owners, ok := sliceAt(obj, "metadata", "ownerReferences"); ok {
		rows = append(rows, configRow{"owners", countSummary(len(owners), "owner")})
	}
	return rows
}

func workloadRows(obj map[string]interface{}) []configRow {
	rows := []configRow{
		{"replicas", replicaSummary(obj)},
		{"selector", selectorSummary(obj, "spec", "selector")},
	}
	if v, ok := stringAt(obj, "spec", "strategy", "type"); ok {
		rows = append(rows, configRow{"strategy", v})
	} else if v, ok := stringAt(obj, "spec", "updateStrategy", "type"); ok {
		rows = append(rows, configRow{"strategy", v})
	}
	if v, ok := scalarAt(obj, "spec", "revisionHistoryLimit"); ok {
		rows = append(rows, configRow{"history", v})
	}
	return rows
}

func jobRows(obj map[string]interface{}) []configRow {
	return []configRow{
		{"completions", scalarOrDash(obj, "spec", "completions")},
		{"parallelism", scalarOrDash(obj, "spec", "parallelism")},
		{"backoff", scalarOrDash(obj, "spec", "backoffLimit")},
		{"status", jobStatus(obj)},
	}
}

func cronJobRows(obj map[string]interface{}) []configRow {
	rows := []configRow{
		{"schedule", scalarOrDash(obj, "spec", "schedule")},
		{"suspend", scalarOrDash(obj, "spec", "suspend")},
		{"concurrency", scalarOrDash(obj, "spec", "concurrencyPolicy")},
		{"successful", scalarOrDash(obj, "spec", "successfulJobsHistoryLimit")},
		{"failed", scalarOrDash(obj, "spec", "failedJobsHistoryLimit")},
	}
	if active, ok := sliceAt(obj, "status", "active"); ok {
		rows = append(rows, configRow{"active jobs", countSummary(len(active), "job")})
	}
	if v, ok := stringAt(obj, "status", "lastScheduleTime"); ok {
		rows = append(rows, configRow{"last run", v})
	}
	return rows
}

func podRows(obj map[string]interface{}) []configRow {
	rows := []configRow{
		{"phase", scalarOrDash(obj, "status", "phase")},
		{"node", scalarOrDash(obj, "spec", "nodeName")},
		{"restart", scalarOrDash(obj, "spec", "restartPolicy")},
		{"service acct", scalarOrDash(obj, "spec", "serviceAccountName")},
	}
	if v, ok := scalarAt(obj, "status", "podIP"); ok {
		rows = append(rows, configRow{"pod ip", v})
	}
	return rows
}

func configMapSummaryRows(obj map[string]interface{}) []configRow {
	rows := []configRow{{"immutable", scalarOrDash(obj, "immutable")}}
	if data, ok := mapAt(obj, "data"); ok {
		rows = append(rows, configRow{"data", countSummary(len(data), "key")})
	}
	if data, ok := mapAt(obj, "binaryData"); ok {
		rows = append(rows, configRow{"binary data", countSummary(len(data), "key")})
	}
	return rows
}

func configMapDataRows(obj map[string]interface{}, width int) []configRow {
	data, ok := mapAt(obj, "data")
	if !ok {
		return nil
	}
	keys := sortedKeys(data)
	rows := make([]configRow, 0, len(keys))
	previewW := width - configKeyWidth - 20
	if previewW < 16 {
		previewW = 16
	}
	for _, k := range keys {
		s, _ := data[k].(string)
		preview := firstLine(s)
		if preview != "" {
			preview = " · " + ansi.Truncate(preview, previewW, "…")
		}
		rows = append(rows, configRow{k, byteSize(len(s)) + preview})
	}
	return rows
}

func secretRows(obj map[string]interface{}) []configRow {
	rows := []configRow{{"type", scalarOrDash(obj, "type")}, {"immutable", scalarOrDash(obj, "immutable")}}
	if data, ok := mapAt(obj, "data"); ok {
		rows = append(rows, configRow{"data", countSummary(len(data), "key")})
	}
	return rows
}

func secretDataRows(th Theme, obj map[string]interface{}, width int) []configRow {
	data, ok := mapAt(obj, "data")
	if !ok {
		return nil
	}
	keys := sortedKeys(data)
	rows := make([]configRow, 0, len(keys))
	previewW := width - configKeyWidth - 20
	if previewW < 16 {
		previewW = 16
	}
	for _, k := range keys {
		s, _ := data[k].(string)
		dec, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			rows = append(rows, configRow{k, th.Bad.Render("decode failed") + " " + th.Dim.Render(byteSize(len(s))+" encoded")})
			continue
		}
		if !utf8.Valid(dec) {
			rows = append(rows, configRow{k, th.Warn.Render("binary") + " " + th.Dim.Render(byteSize(len(dec))+" decoded")})
			continue
		}
		preview := firstLine(string(dec))
		if preview == "" {
			preview = "<empty>"
		}
		value := th.Good.Render(ansi.Truncate(preview, previewW, "…")) + " " + th.Dim.Render(byteSize(len(dec))+" decoded")
		rows = append(rows, configRow{k, value})
	}
	return rows
}

func serviceRows(obj map[string]interface{}) []configRow {
	rows := []configRow{
		{"type", scalarOrDash(obj, "spec", "type")},
		{"cluster ip", scalarOrDash(obj, "spec", "clusterIP")},
		{"selector", mapAtSummary(obj, []string{"spec", "selector"}, 5)},
		{"ports", servicePorts(obj)},
	}
	if ips, ok := sliceAt(obj, "spec", "externalIPs"); ok {
		rows = append(rows, configRow{"external ips", joinScalars(ips, 4)})
	}
	return rows
}

func ingressRows(obj map[string]interface{}) []configRow {
	rows := []configRow{{"class", scalarOrDash(obj, "spec", "ingressClassName")}}
	if tls, ok := sliceAt(obj, "spec", "tls"); ok {
		rows = append(rows, configRow{"tls", countSummary(len(tls), "entry")})
	}
	if rules, ok := sliceAt(obj, "spec", "rules"); ok {
		for i, r := range rules {
			m, ok := asMap(r)
			if !ok {
				continue
			}
			host, _ := scalarString(m["host"])
			if host == "" {
				host = "*"
			}
			paths := ingressPathCount(m)
			rows = append(rows, configRow{fmt.Sprintf("rule %d", i+1), host + " · " + countSummary(paths, "path")})
		}
	}
	return rows
}

func genericSpecRows(obj map[string]interface{}) []configRow {
	spec, ok := mapAt(obj, "spec")
	if !ok {
		return nil
	}
	keys := sortedKeys(spec)
	if len(keys) > 12 {
		keys = keys[:12]
	}
	rows := make([]configRow, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, configRow{k, compactValue(spec[k])})
	}
	return rows
}

func addPodSpecSections(th Theme, obj map[string]interface{}, podSpecPath []string, add func(string, []configRow)) {
	if spec, ok := mapAt(obj, podSpecPath...); ok {
		add("Pod Template", podSpecRows(spec))
		add("Containers", containerRows(spec, "containers"))
		add("Init Containers", containerRows(spec, "initContainers"))
		add("Volumes", volumeRows(th, spec))
	}
}

func podSpecRows(spec map[string]interface{}) []configRow {
	rows := []configRow{
		{"service acct", scalarInMapOrDash(spec, "serviceAccountName")},
		{"restart", scalarInMapOrDash(spec, "restartPolicy")},
	}
	if node, ok := scalarString(spec["nodeName"]); ok && node != "" {
		rows = append(rows, configRow{"node", node})
	}
	if nodeSelector, ok := asMap(spec["nodeSelector"]); ok {
		rows = append(rows, configRow{"node selector", mapSummary(nodeSelector, 4)})
	}
	if tolerations, ok := asSlice(spec["tolerations"]); ok {
		rows = append(rows, configRow{"tolerations", countSummary(len(tolerations), "rule")})
	}
	return rows
}

func containerRows(spec map[string]interface{}, field string) []configRow {
	containers, ok := asSlice(spec[field])
	if !ok {
		return nil
	}
	rows := make([]configRow, 0, len(containers))
	for _, item := range containers {
		c, ok := asMap(item)
		if !ok {
			continue
		}
		name, _ := scalarString(c["name"])
		if name == "" {
			name = "container"
		}
		image, _ := scalarString(c["image"])
		parts := []string{image}
		if ports := portsSummary(c); ports != "" {
			parts = append(parts, ports)
		}
		if env := envSummary(c); env != "" {
			parts = append(parts, env)
		}
		if res := resourcesSummary(c); res != "" {
			parts = append(parts, res)
		}
		rows = append(rows, configRow{name, strings.Join(nonEmpty(parts), " · ")})
	}
	return rows
}

func volumeRows(th Theme, spec map[string]interface{}) []configRow {
	volumes, ok := asSlice(spec["volumes"])
	if !ok {
		return nil
	}
	rows := make([]configRow, 0, len(volumes))
	for _, item := range volumes {
		v, ok := asMap(item)
		if !ok {
			continue
		}
		name, _ := scalarString(v["name"])
		kind, ref := volumeSource(v)
		if ref == "" {
			ref = th.Dim.Render("-")
		}
		rows = append(rows, configRow{name, kind + " " + ref})
	}
	return rows
}

func replicaSummary(obj map[string]interface{}) string {
	if desired, ok := scalarAt(obj, "status", "desiredNumberScheduled"); ok {
		ready := scalarOrDash(obj, "status", "numberReady")
		available := scalarOrDash(obj, "status", "numberAvailable")
		updated := scalarOrDash(obj, "status", "updatedNumberScheduled")
		return desired + " desired · " + ready + " ready · " + available + " available · " + updated + " updated"
	}
	desired := scalarOrDash(obj, "spec", "replicas")
	ready := scalarOrDash(obj, "status", "readyReplicas")
	available := scalarOrDash(obj, "status", "availableReplicas")
	updated := scalarOrDash(obj, "status", "updatedReplicas")
	return desired + " desired · " + ready + " ready · " + available + " available · " + updated + " updated"
}

func jobStatus(obj map[string]interface{}) string {
	return scalarOrDash(obj, "status", "succeeded") + " succeeded · " +
		scalarOrDash(obj, "status", "active") + " active · " +
		scalarOrDash(obj, "status", "failed") + " failed"
}

func selectorSummary(obj map[string]interface{}, path ...string) string {
	sel, ok := mapAt(obj, path...)
	if !ok {
		return "-"
	}
	if labels, ok := asMap(sel["matchLabels"]); ok && len(labels) > 0 {
		return mapSummary(labels, 5)
	}
	if len(sel) > 0 {
		return mapSummary(sel, 5)
	}
	return "-"
}

func servicePorts(obj map[string]interface{}) string {
	ports, ok := sliceAt(obj, "spec", "ports")
	if !ok {
		return "-"
	}
	parts := make([]string, 0, len(ports))
	for _, item := range ports {
		p, ok := asMap(item)
		if !ok {
			continue
		}
		port := compactValue(p["port"])
		target := compactValue(p["targetPort"])
		protocol := compactValue(p["protocol"])
		name, _ := scalarString(p["name"])
		label := port
		if target != "" && target != "-" && target != port {
			label += ":" + target
		}
		if protocol != "" && protocol != "-" {
			label += "/" + protocol
		}
		if name != "" {
			label = name + " " + label
		}
		parts = append(parts, label)
	}
	return joinWithMore(parts, 4)
}

func portsSummary(c map[string]interface{}) string {
	ports, ok := asSlice(c["ports"])
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(ports))
	for _, item := range ports {
		p, ok := asMap(item)
		if !ok {
			continue
		}
		port := compactValue(p["containerPort"])
		protocol := compactValue(p["protocol"])
		name, _ := scalarString(p["name"])
		if protocol != "" && protocol != "-" {
			port += "/" + protocol
		}
		if name != "" {
			port = name + ":" + port
		}
		parts = append(parts, port)
	}
	if len(parts) == 0 {
		return ""
	}
	return "ports " + joinWithMore(parts, 3)
}

func envSummary(c map[string]interface{}) string {
	env, _ := asSlice(c["env"])
	envFrom, _ := asSlice(c["envFrom"])
	var parts []string
	if len(env) > 0 {
		parts = append(parts, countSummary(len(env), "env var"))
	}
	if len(envFrom) > 0 {
		parts = append(parts, countSummary(len(envFrom), "env source"))
	}
	return strings.Join(parts, ", ")
}

func resourcesSummary(c map[string]interface{}) string {
	res, ok := asMap(c["resources"])
	if !ok {
		return ""
	}
	var parts []string
	if req, ok := asMap(res["requests"]); ok {
		parts = append(parts, "req "+resourceMap(req))
	}
	if lim, ok := asMap(res["limits"]); ok {
		parts = append(parts, "lim "+resourceMap(lim))
	}
	return strings.Join(parts, " · ")
}

func resourceMap(m map[string]interface{}) string {
	keys := sortedKeys(m)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+compactValue(m[k]))
	}
	return strings.Join(parts, " ")
}

func volumeSource(v map[string]interface{}) (string, string) {
	for _, key := range []string{"configMap", "secret", "persistentVolumeClaim", "projected", "emptyDir", "hostPath", "downwardAPI", "nfs", "csi"} {
		m, ok := asMap(v[key])
		if !ok {
			continue
		}
		if name, ok := scalarString(m["name"]); ok && name != "" {
			return key, name
		}
		if claim, ok := scalarString(m["claimName"]); ok && claim != "" {
			return key, claim
		}
		if path, ok := scalarString(m["path"]); ok && path != "" {
			return key, path
		}
		return key, ""
	}
	return "volume", ""
}

func ingressPathCount(rule map[string]interface{}) int {
	http, ok := asMap(rule["http"])
	if !ok {
		return 0
	}
	paths, ok := asSlice(http["paths"])
	if !ok {
		return 0
	}
	return len(paths)
}

func dataKeyRows(obj map[string]interface{}, path []string, label string) []configRow {
	data, ok := mapAt(obj, path...)
	if !ok {
		return nil
	}
	keys := sortedKeys(data)
	rows := make([]configRow, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, configRow{k, byteSize(len(fmt.Sprint(data[k]))) + " " + label})
	}
	return rows
}

func scalarOrDash(obj map[string]interface{}, path ...string) string {
	if s, ok := scalarAt(obj, path...); ok {
		return s
	}
	return "-"
}

func scalarInMapOrDash(m map[string]interface{}, key string) string {
	if s, ok := scalarString(m[key]); ok {
		return s
	}
	return "-"
}

func scalarAt(obj map[string]interface{}, path ...string) (string, bool) {
	v, ok := valueAt(obj, path...)
	if !ok {
		return "", false
	}
	return scalarString(v)
}

func stringAt(obj map[string]interface{}, path ...string) (string, bool) {
	v, ok := valueAt(obj, path...)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func mapAtSummary(obj map[string]interface{}, path []string, max int) string {
	m, ok := mapAt(obj, path...)
	if !ok {
		return "-"
	}
	return mapSummary(m, max)
}

func mapAt(obj map[string]interface{}, path ...string) (map[string]interface{}, bool) {
	v, ok := valueAt(obj, path...)
	if !ok {
		return nil, false
	}
	return asMap(v)
}

func sliceAt(obj map[string]interface{}, path ...string) ([]interface{}, bool) {
	v, ok := valueAt(obj, path...)
	if !ok {
		return nil, false
	}
	return asSlice(v)
}

func valueAt(obj map[string]interface{}, path ...string) (interface{}, bool) {
	var cur interface{} = obj
	for _, p := range path {
		m, ok := asMap(cur)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func asSlice(v interface{}) ([]interface{}, bool) {
	s, ok := v.([]interface{})
	return s, ok
}

func scalarString(v interface{}) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		return fmt.Sprintf("%t", t), true
	case int:
		return fmt.Sprintf("%d", t), true
	case int64:
		return fmt.Sprintf("%d", t), true
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t)), true
		}
		return fmt.Sprintf("%g", t), true
	}
	return "", false
}

func compactValue(v interface{}) string {
	if s, ok := scalarString(v); ok {
		return s
	}
	if m, ok := asMap(v); ok {
		return countSummary(len(m), "field")
	}
	if s, ok := asSlice(v); ok {
		return countSummary(len(s), "item")
	}
	return "-"
}

func mapSummary(m map[string]interface{}, max int) string {
	keys := sortedKeys(m)
	if len(keys) == 0 {
		return "-"
	}
	shown := keys
	if len(shown) > max {
		shown = shown[:max]
	}
	parts := make([]string, 0, len(shown)+1)
	for _, k := range shown {
		parts = append(parts, k+"="+compactValue(m[k]))
	}
	if extra := len(keys) - len(shown); extra > 0 {
		parts = append(parts, fmt.Sprintf("+%d", extra))
	}
	return strings.Join(parts, ", ")
}

func joinScalars(items []interface{}, max int) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := scalarString(item); ok {
			parts = append(parts, s)
		}
	}
	return joinWithMore(parts, max)
}

func joinWithMore(parts []string, max int) string {
	if len(parts) == 0 {
		return "-"
	}
	shown := parts
	if len(shown) > max {
		shown = shown[:max]
	}
	out := strings.Join(shown, ", ")
	if extra := len(parts) - len(shown); extra > 0 {
		out += fmt.Sprintf(", +%d", extra)
	}
	return out
}

func nonEmpty(parts []string) []string {
	out := parts[:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" && p != "-" {
			out = append(out, p)
		}
	}
	return out
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return strings.ReplaceAll(line, "\t", " ")
		}
	}
	return ""
}

func countSummary(n int, singular string) string {
	if n == 1 {
		return "1 " + singular
	}
	plural := singular + "s"
	if strings.HasSuffix(singular, "y") {
		plural = strings.TrimSuffix(singular, "y") + "ies"
	}
	return fmt.Sprintf("%d %s", n, plural)
}

func byteSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%.1fKiB", float64(n)/1024)
}
