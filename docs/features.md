# Features

## Cockpit overview

ku opens on a cockpit: a single-screen overview of the cluster. It shows the
server version, node readiness and any node pressure (DiskPressure,
MemoryPressure, NotReady, ...), live CPU and memory gauges (from the metrics
API), pod counts by phase plus how many are not-ready or crashlooping,
deployment readiness, and recent warning events (deduplicated, with a recurrence
count). It refreshes every few seconds. Reach it any time from the Overview
entry at the top of the nav, and press Enter on a nav resource (or `:` /
`Ctrl+K`) to drill in.

## Any resource, real columns

ku lists resources by asking the API server for the `Table` representation, the
same one `kubectl get` uses. Columns match what you expect for every kind,
including custom resources. Discovery builds the catalog and alias map (short
names like `po`, `deploy`, `svc`) at startup. If discovery is partial (an
aggregated API is down), ku warns instead of silently hiding kinds.

Jump anywhere with `:` (type a name or alias) or the `Ctrl+K` palette. The left
nav lists common kinds for quick access, and `O` opens upstream Kubernetes docs
for known built-in resources.

## Live tables

The table auto-refreshes every couple of seconds. `/` filters rows (the active
filter shows in the header and clears with `esc`). `w` toggles the wide columns
that `kubectl get -o wide` would show. Listing across all namespaces (`a`) adds a
NAMESPACE column.

Cells are colored by meaning so lists are not flat: status/phase is green when
healthy, yellow when transient (Pending, ContainerCreating), and red when broken
(CrashLoopBackOff, Error, OOMKilled); the READY ratio and restart counts are
colored the same way; CPU%/MEM% turn yellow then red as they climb; and metadata
like age and IP is dimmed so names and problems stand out. The selected row is a
single highlight bar.

`S` sorts by a column: it opens a picker of the current columns, and re-picking
the active column flips direction (the sorted column header shows the direction).
Sorting is type-aware, so AGE sorts by duration, CPU/MEM and percentages sort
numerically, and names sort case-insensitively. Numeric columns default to
descending (largest/oldest first). Sorting composes with the `/` filter and
resets when you switch resources.

`C` shows the closest equivalent `kubectl` command for the current view: table,
cockpit, YAML detail, config summary, or logs.

## Config summary and YAML

`Enter` opens a curated config summary for the selected object. Pods show live
usage when metrics are available, health, requests and limits, and pod spec
details. Workloads, Jobs, and CronJobs show status before overview fields.
Services, Ingresses, ConfigMaps, and Secrets get purpose-built summaries;
unknown kinds fall back to an overview plus spec summary.

`d` or `y` opens the object's YAML in a scrollable view (managed fields
stripped), with theme-aware syntax highlighting. `g` / `G` jump to top and
bottom. Secret `data` is base64-decoded in read-only views for readability;
editing a Secret still fetches raw base64 so saves stay valid.

## Live resource usage

On the nodes view, ku appends live CPU and memory usage with percentages from
the metrics API, the same numbers as `kubectl top nodes`. On the pods view it
appends per-pod CPU and memory (summed across containers), like `kubectl top
pods`. Both are best-effort: if metrics-server is not installed, the columns are
simply omitted.

## Edit, apply after confirm

`e` opens the object in your editor (`$EDITOR`, then `$VISUAL`, then whatever is
installed: nvim, vim, nano, or vi) in an overlay inside the TUI. Save and quit,
then confirm the apply. If you make no changes, nothing is sent. `Ctrl+\`
cancels without applying. ku rejects edits that change the object's kind or
name.

## Logs

`l` on a pod streams logs live in an overlay, starting with the last 1000 lines
(it prompts for the container when there are several). `f` toggles auto-scroll
so you can read back through history; `g` / `G` jump to top and bottom.

## Shell into a pod or node

`s` on a pod opens an interactive shell in an overlay, run inside the TUI using
a virtual terminal. It runs `bash` if present, otherwise `sh`, over the cluster's
exec stream (WebSocket with SPDY fallback, like kubectl). `Ctrl+\` detaches; the
overlay also closes when you `exit`. Paste with `Ctrl+Shift+V`; `Ctrl+V` is sent
to the running shell/program. Mouse selection uses your terminal's native
click-and-drag selection inside shell mode.

`s` on a node opens a node shell the way `kubectl debug node` does: it spawns a
short-lived privileged debug pod pinned to the node, with the host filesystem
mounted at `/host`, and drops you into a `chroot /host` shell. The debug pod is
deleted when you exit or detach. Override the image with `$KU_DEBUG_IMAGE`
(default `busybox`). This needs permission to create privileged pods, so it may
be blocked on clusters with restrictive Pod Security settings.

## Scale, restart, trigger, delete

`s` on a workload (deployment, statefulset, replicaset) prompts for a replica
count. `R` triggers a rolling restart of a deployment, statefulset, or daemonset
(the same restartedAt-annotation mechanism kubectl uses), after a confirm. `t`
on a CronJob creates a one-off Job from its job template, after a confirm. `x`
deletes the selected object after a confirm.

The bottom bar is context-aware: it shows logs and shell for pods, scale and
restart for workloads, and trigger for CronJobs, so the relevant actions are
always in view.

## Namespaces and contexts

`n` picks a namespace, `a` toggles all-namespaces, and `c` switches context.
Switching context rebuilds the client, the resource catalog, and the left nav.
Your last context and namespace are remembered in `~/.config/ku/state.json` for
next launch.
