# Features

## Cockpit overview

kli opens on a cockpit: a single-screen overview of the cluster. It shows the
server version and node readiness, live CPU and memory gauges (from the metrics
API), pod counts by phase, deployment readiness, and the most recent warning
events. It refreshes every few seconds. Reach it any time from the Overview
entry at the top of the nav, and press Enter on a resource (or `:` / `Ctrl+K`)
to drill in.

## Any resource, real columns

kli lists resources by asking the API server for the `Table` representation, the
same one `kubectl get` uses. Columns match what you expect for every kind,
including custom resources. Discovery builds the catalog and alias map (short
names like `po`, `deploy`, `svc`) at startup. If discovery is partial (an
aggregated API is down), kli warns instead of silently hiding kinds.

Jump anywhere with `:` (type a name or alias) or the `Ctrl+K` palette. The left
nav lists common kinds for quick access.

## Live tables

The table auto-refreshes every couple of seconds. `/` filters rows (the active
filter shows in the header and clears with `esc`). `w` toggles the wide columns
that `kubectl get -o wide` would show. Listing across all namespaces (`a`) adds a
NAMESPACE column.

## Describe and YAML

`Enter` or `d` opens the object's YAML in a scrollable view (managed fields
stripped), with theme-aware syntax highlighting. `g` / `G` jump to top and
bottom. Secret `data` is base64-decoded here for readability; editing a Secret
still fetches raw base64 so saves stay valid.

## Node stats

On the nodes view, kli appends live CPU and memory usage and percentages from
the metrics API, the same numbers as `kubectl top nodes`. This is best-effort:
if metrics-server is not installed, the columns are simply omitted.

## Edit, applied on save

`e` opens the object in your editor (`$EDITOR`, then `$VISUAL`, then whatever is
installed: nvim, vim, nano, or vi) in an overlay inside the TUI. Save and quit to apply the change as an optimistic update; the
editor's cursor is rendered in the panel. If you make no changes, nothing is
sent. `Ctrl+\` cancels without applying. kli rejects edits that change the
object's kind or name.

## Logs

`l` on a pod streams logs live in an overlay (it prompts for the container when
there are several). `f` toggles auto-scroll so you can read back through
history; `g` / `G` jump to top and bottom.

## Shell into a pod

`s` on a pod opens an interactive shell in an overlay, run inside the TUI using
a virtual terminal. It runs `bash` if present, otherwise `sh`, over the cluster's
exec stream (WebSocket with SPDY fallback, like kubectl). `Ctrl+\` detaches; the
overlay also closes when you `exit`.

## Scale, restart, delete

`s` on a workload (deployment, statefulset, replicaset) prompts for a replica
count. `R` triggers a rolling restart of a deployment, statefulset, or daemonset
(the same restartedAt-annotation mechanism kubectl uses), after a confirm. `x`
deletes the selected object after a confirm.

The bottom bar is context-aware: it shows logs and shell for pods, scale and
restart for workloads, so the relevant actions are always in view.

## Namespaces and contexts

`n` picks a namespace, `a` toggles all-namespaces, and `c` switches context.
Switching context rebuilds the client, the resource catalog, and the left nav.
Your last context and namespace are remembered for next launch.
