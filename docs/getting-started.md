# Getting started

## Install

Install the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/bjarneo/ku/main/install.sh | sh
```

Or install with Go:

```bash
go install github.com/bjarneo/ku@latest
```

Or build from a clone:

```
make install            # builds and installs to a bin dir on PATH
go build -o ku .       # local binary only
```

Building from source requires Go 1.26.3+. Running ku requires access to a
cluster. ku reads your default kubeconfig.

## Run

```
ku                       # current context, remembered namespace
ku --context my-ctx      # a specific context
ku -n kube-system        # start in a namespace
ku --resource deploy     # start on a resource type
ku --theme tokyonight    # use the Tokyo Night theme
ku --check               # read-only connectivity check, no UI
ku --version
```

## Commands

| Command | Description |
| --- | --- |
| `ku` | start the TUI |
| `ku config init` | write a starter config file with the built-in sidebar defaults |
| `ku config init --force` | overwrite the config file with the current defaults |
| `ku config path` | print the config file location |
| `ku upgrade` | download the latest release binary and replace the current binary |

## Flags

| Flag | Description |
| --- | --- |
| `--context` | kubeconfig context to use (default: current-context) |
| `--kubeconfig` | path to the kubeconfig file (default: `$KUBECONFIG`, then `~/.kube/config`) |
| `-n`, `--namespace` | initial namespace; omit to use the remembered or context namespace |
| `--resource` | initial resource, e.g. `pods`, `deploy`, `svc` |
| `--theme` | `ansi` (default) or `tokyonight` |
| `--check` | run a read-only connectivity check and exit |
| `--version` | print version and exit |

`--resource` accepts the same names as the in-app resource picker: plural,
singular, kind, short name, or group-qualified key such as
`scaledobjects.keda.sh`.

## Layout

ku opens on the cockpit, a cluster overview (health, node CPU/memory, pods,
deployments, and recent warnings). From there:

- A left nav lists Overview plus common resource kinds, grouped by category.
- The main area shows the cockpit or, once you pick a resource, its table with
  the same server-rendered columns as `kubectl get`.
- A status bar always shows the keys available right now, with the creator
  handle in the bottom-right.

Press `Tab` to move focus between the nav and the main area, `Enter` on a nav
entry to open it, and the Overview entry to return to the cockpit. The focused
pane has a highlighted border.

## Themes

The default theme uses your terminal's own ANSI palette and adapts to a light or
dark background, so it matches whatever scheme you already run. For a fixed,
high-contrast look, pass `--theme tokyonight` or set `KU_THEME=tokyonight`.

You can also switch live from inside the app: open the command palette
(`Ctrl+K`) and pick **Switch theme**. The choice is remembered for the next
launch, unless `--theme` or `KU_THEME` is set (those still take precedence).

## Configuration

ku reads optional sidebar config from `~/.config/ku/config.yaml`. Use
`ku config init` to seed a starter file, then restart the TUI after edits.

See [Configuration](configuration.md) for file paths, sidebar examples,
resource names, and opt-in resources.

## Session memory

ku remembers the last context, namespace, and theme you used and restores them
on the next launch. Flags override the remembered values. State is written to
`~/.config/ku/state.json`.
