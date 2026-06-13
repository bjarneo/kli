# Keybindings

Press `?` in the app for this list, or `Ctrl+K` for a fuzzy command palette that
can run any of these and jump to any resource.

## Navigation

| Key | Action |
| --- | --- |
| `↑` / `k`, `↓` / `j` | move |
| `g` / `G` | top / bottom |
| `Ctrl+u` / `Ctrl+d` | half page up / down |
| `pgup` / `pgdn` | page up / down |
| `Tab`, `←` / `h`, `→` / `l` | switch pane (nav / table) |

## Row actions (table)

| Key | Action |
| --- | --- |
| `Enter` / `d` | describe (YAML) |
| `y` | YAML |
| `l` | logs (pods); `f` toggles follow |
| `e` | edit in `$EDITOR`, applied on save |
| `s` | shell into a pod, or scale a workload |
| `R` | rollout restart (deployments, statefulsets, daemonsets) |
| `x` | delete (with confirm) |

The bottom bar adapts to the selected resource: pods show logs and shell,
workloads show scale and restart.

## Views and cluster

| Key | Action |
| --- | --- |
| `:` | jump to any resource (`pods`, `deploy`, `svc`, ...) |
| `Ctrl+K` | command palette |
| `/` | filter the current table (`esc` clears) |
| `r` | refresh now |
| `w` | toggle wide columns |
| `n` | switch namespace |
| `a` | toggle all namespaces |
| `c` | switch context |
| `?` | help |
| `q` | quit (`Ctrl+C` anywhere) |

## Overlays

| Context | Keys |
| --- | --- |
| Logs | `f` follow, `g` / `G` top / bottom, `esc` back |
| Detail (YAML) | `↑↓` scroll, `g` / `G` top / bottom, `e` edit, `esc` back |
| Shell / editor | keys go to the program; `Ctrl+\` detaches (cancels an edit) |
| Pickers / palette | `↑↓` move, `enter` select, `esc` cancel |
| Confirm | `y` confirm, `n` / `esc` cancel |
