# Keybindings

Press `?` in the app for this list, or `Ctrl+K` for a fuzzy command palette that
can run any of these and jump to any resource.

## Navigation

| Key | Action |
| --- | --- |
| `Up` / `k`, `Down` / `j` | move |
| `g` / `G` | top / bottom |
| `Ctrl+u` / `Ctrl+d` | half page up / down |
| `PgUp` / `PgDn` | page up / down |
| `Tab` | switch pane (nav / table) |
| `Left` / `h` | focus the nav from the table |
| `Right` / `l` | open the selected nav entry while the nav is focused |

## Row actions (table)

| Key | Action |
| --- | --- |
| `Enter` | config summary |
| `d` / `y` | YAML |
| `l` | logs (pods); `f` toggles follow |
| `e` | edit in `$EDITOR`, then confirm apply |
| `s` | shell into a pod, node shell on a node, or scale a workload |
| `R` | rollout restart (deployments, statefulsets, daemonsets) |
| `t` | trigger a CronJob once (with confirm) |
| `K` | cordon / uncordon a node (with confirm) |
| `D` | drain a node: cordon and evict its pods (with confirm) |
| `x` / `Delete` | delete (with confirm) |
| `O` | open Kubernetes docs for the current resource, when known |

The bottom bar adapts to the selected resource: pods show logs and shell, nodes
show node shell, cordon, and drain, workloads show scale and restart, and
CronJobs show trigger.

Draining cordons the node, then evicts its pods through the eviction API so
PodDisruptionBudgets are honored. DaemonSet and static (mirror) pods are left in
place, the same as `kubectl drain --ignore-daemonsets`.

## Views and cluster

| Key | Action |
| --- | --- |
| `:` | jump to any resource (`pods`, `deploy`, `svc`, ...) |
| `Ctrl+K` | command palette |
| `Shift+E` | toggle edit mode / read-only mode |
| `/` | filter the current table (`esc` clears) |
| `S` | sort by a column (re-pick to flip direction) |
| `r` / `Ctrl+r` | refresh now |
| `w` | toggle wide columns |
| `n` | switch namespace |
| `a` | toggle all namespaces |
| `c` | switch context |
| `C` | show the equivalent `kubectl` command |
| `O` | open Kubernetes docs for the current resource, when known |
| `?` | help |
| `q` | quit from the table or cockpit; back from config, YAML, and logs |
| `Ctrl+C` | quit outside the embedded shell/editor |

`ku` starts read-only unless launched with `--edit`. Mutating actions are hidden
until edit mode is enabled with `Shift+E` or the command palette.

## Overlays

| Context | Keys |
| --- | --- |
| Logs | `f` follow, `/` filter (regex), `w` (or `Ctrl+w` while filtering) wrap / truncate lines, `c` copy all, `Ctrl+l` clear, `v` select lines, `g` / `G` top / bottom, `esc` back |
| Logs (selecting) | `↑` / `↓` (and `g` / `G`, page keys) move the cursor, `m` mark, `y` / `Enter` copy, `esc` cancel |
| Config summary | scroll, `d` / `y` YAML, `e` edit, `t` trigger CronJob, `esc` back |
| Detail (YAML) | scroll, `Enter` config, `e` edit, `t` trigger CronJob, `esc` back |
| Shell / editor | keys go to the program; `Ctrl+\` detaches (cancels an edit) |
| Command overlay | `C`, `q`, or `esc` closes |
| Pickers / palette | move, type to filter, `Enter` select, `esc` cancel |
| Confirm | `y` / `Enter` confirm, `n` / `esc` cancel |

## Selecting and copying logs

Two ways to mark and copy log lines:

- Keyboard: press `v` to enter selection mode, move the cursor with `↑` / `↓`
  (or `g` / `G` and the page keys), press `m` to mark the start, then move to
  extend the range and `y` or `Enter` to copy it to the clipboard (via OSC 52,
  so it works over SSH too). Without marking, `y` copies the cursor line. `esc`
  cancels.
- Mouse: the logs view releases the mouse, so your terminal's own click-and-drag
  selection and copy work directly. (Keyboard scrolling still applies; the mouse
  wheel does not scroll here.)

To grab the whole buffer at once, press `c` to copy every buffered line to the
clipboard (the raw lines, so an active filter never hides anything). Press
`Ctrl+l` to clear the on-screen buffer; the stream keeps running, so new lines
flow back in right away.
