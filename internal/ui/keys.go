package ui

import "charm.land/bubbles/v2/key"

// keyMap holds every binding in the app. Navigation keys are kept disjoint
// from action keys so the table's own movement keys never shadow an action.
type keyMap struct {
	// movement (handled by the table)
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	HalfUp   key.Binding
	HalfDown key.Binding
	Top      key.Binding
	Bottom   key.Binding
	HScroll  key.Binding

	// row actions
	Enter      key.Binding
	Describe   key.Binding
	YAML       key.Binding
	Logs       key.Binding
	Edit       key.Binding
	Shell      key.Binding
	Restart    key.Binding
	Trigger    key.Binding
	Delete     key.Binding
	Docs       key.Binding
	DeployLogs key.Binding

	// views / navigation
	Focus     key.Binding
	Filter    key.Binding
	Sort      key.Binding
	Refresh   key.Binding
	Jump      key.Binding
	Palette   key.Binding
	Namespace key.Binding
	Context   key.Binding
	Command   key.Binding
	AllNS     key.Binding
	Wide      key.Binding

	// logs
	Follow key.Binding

	// global
	Help key.Binding
	Back key.Binding
	Quit key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "½ page up")),
		HalfDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "½ page down")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		HScroll:  key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "scroll columns")),

		Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "config")),
		Describe:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "describe")),
		YAML:       key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yaml")),
		Logs:       key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
		Edit:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit (nvim)")),
		Shell:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "shell / scale")),
		Restart:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "rollout restart")),
		Trigger:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "trigger job")),
		Delete:     key.NewBinding(key.WithKeys("x", "delete"), key.WithHelp("x", "delete")),
		Docs:       key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "open docs")),
		DeployLogs: key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "deployment logs")),

		Focus:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab ←→", "switch pane")),
		Filter:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Sort:      key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "sort by column")),
		Refresh:   key.NewBinding(key.WithKeys("r", "ctrl+r"), key.WithHelp("r", "refresh")),
		Jump:      key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "jump to resource")),
		Palette:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "command palette")),
		Namespace: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "namespace")),
		Context:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "context")),
		Command:   key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "kubectl command")),
		AllNS:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "all namespaces")),
		Wide:      key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "wide columns")),

		Follow: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "follow")),

		Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// helpGroup is a labeled column of bindings in the full-screen help.
type helpGroup struct {
	title string
	keys  []key.Binding
}

func (k keyMap) groups() []helpGroup {
	return []helpGroup{
		{"Navigation", []key.Binding{k.Up, k.Down, k.HalfUp, k.HalfDown, k.PageUp, k.PageDown, k.Top, k.Bottom, k.HScroll}},
		{"Actions", []key.Binding{k.Enter, k.Describe, k.YAML, k.Logs, k.DeployLogs, k.Edit, k.Shell, k.Restart, k.Trigger, k.Delete, k.Docs}},
		{"Views", []key.Binding{k.Focus, k.Jump, k.Palette, k.Filter, k.Sort, k.Refresh, k.Wide, k.Command}},
		{"Cluster", []key.Binding{k.Namespace, k.AllNS, k.Context}},
		{"Logs", []key.Binding{k.Follow, k.Filter}},
		{"General", []key.Binding{k.Help, k.Back, k.Quit}},
	}
}
