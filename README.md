# kli

A fast, keyboard-driven Kubernetes TUI. Browse any resource, read and edit
objects, follow logs, and open a shell in a pod, without leaving the terminal.
Inspired by k9s, Lens, and lazygit.

https://github.com/user-attachments/assets/48756c6b-00ae-470d-8fb5-3f93ecbd46df

## Install

Install the latest release with the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/bjarneo/kli/main/install.sh | sh
```

Or with Go:

```bash
go install github.com/bjarneo/kli@latest
```

Or from a clone:

```bash
make install   # builds and installs to ~/.local/bin, /usr/local/bin, or your last $PATH dir
go build -o kli .
```

Requires Go 1.24+ and a reachable cluster.

## Quick start

```
kli                       # current context, remembered namespace
kli -n kube-system        # start in a namespace
kli --resource deploy     # start on a resource type
kli --theme tokyonight    # switch theme
```

Press `?` for help and `Ctrl+K` for the command palette.

## Configuration

`kli` reads an optional config file from `~/.config/kli/config.yaml`. Run
`kli config path` to print it. It is separate from the auto-saved
context/namespace state at `~/.config/kli/state.json`, and is only written by
you or by `kli config init`.

Today it customizes the left sidebar menu. When a `sidebar:` list is present it
replaces the built-in default menu; without a config file the built-in defaults
are used. Resources the cluster doesn't expose are dropped, and empty sections
are hidden.

Seed a starter file with the current defaults (refuses to overwrite without
`--force`):

```
kli config init          # write the default config to populate from
kli config path          # print the config file location
```

The `resource` field accepts anything the resource picker resolves: a plural,
singular, kind, short name, or group-qualified key (e.g. `scaledobjects.keda.sh`).

```yaml
sidebar:
  - section: Workloads
    items:
      - { label: Pods, resource: pods }
      - { label: Deployments, resource: deployments }
      - { label: HPAs, resource: horizontalpodautoscalers }   # opt-in
      - { label: ScaledObjects, resource: scaledobjects }      # opt-in (KEDA)
  - section: Network
    items:
      - { label: Services, resource: services }
```

`HPAs`, `ScaledObjects`, and `OtelCollectors` are not in the default menu; add
them as above (a freshly seeded config lists them as commented examples).

## Highlights

- A cockpit overview on launch: cluster health, node CPU and memory gauges, pod and deployment status, and recent warnings.
- Server-rendered tables for any resource, the same columns as `kubectl get`, including CRDs.
- lazygit-style layout: a left resource nav, `Tab` between panes, and a status bar that always shows the keys that work right now.
- Logs, edit-in-editor (applied on save), shell into a pod, delete, and scale, all in overlays inside the TUI.
- ANSI colors that match your terminal in light or dark mode, with Tokyo Night as a fallback (`--theme tokyonight`).
- A customizable sidebar menu via an optional config file (`kli config init`): add CRDs like HPAs, KEDA ScaledObjects, or OpenTelemetry collectors.
- Remembers your last context and namespace.

## Docs

- [Getting started](docs/getting-started.md)
- [Keybindings](docs/keybindings.md)
- [Features](docs/features.md)

Full index: [docs/](docs/README.md).

## Created by

[x.com/iamdothash](https://x.com/iamdothash)
