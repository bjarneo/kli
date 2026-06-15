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
| `x` / `Delete` | delete (with confirm) |
| `O` | open Kubernetes docs for the current resource, when known |

The bottom bar adapts to the selected resource: pods show logs and shell, nodes
show a node shell, workloads show scale and restart, and CronJobs show trigger.

## Views and cluster

| Key | Action |
| --- | --- |
| `:` | jump to any resource (`pods`, `deploy`, `svc`, ...) |
| `Ctrl+K` | command palette |
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

## Overlays

| Context | Keys |
| --- | --- |
| Logs | `f` follow, `/` filter (regex), `g` / `G` top / bottom, `esc` back |
| Config summary | scroll, `d` / `y` YAML, `e` edit, `t` trigger CronJob, `esc` back |
| Detail (YAML) | scroll, `Enter` config, `e` edit, `t` trigger CronJob, `esc` back |
| Shell / editor | keys go to the program; `Ctrl+\` detaches (cancels an edit) |
| Command overlay | `C`, `q`, or `esc` closes |
| Pickers / palette | move, type to filter, `Enter` select, `esc` cancel |
| Confirm | `y` / `Enter` confirm, `n` / `esc` cancel |
