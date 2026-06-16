package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// Options configures a kli session.
type Options struct {
	Context    string
	Namespace  string
	Resource   string
	Theme      string
	Kubeconfig string // explicit kubeconfig path ("" = default lookup)
}

// Run starts the interactive TUI. The cluster connection and config load run in
// the background behind a splash screen (see startupCmd / adoptStartup); flags
// take precedence over the remembered context/namespace from the last session.
func Run(opts Options) error {
	th := PickTheme(opts.Theme)
	saved, hasSaved := loadState()

	app := App{theme: th, keys: defaultKeys(), splash: true, opts: opts, saved: saved, hasSaved: hasSaved}
	app.spin = newSpinner(th)

	m, err := tea.NewProgram(app).Run()
	if err != nil {
		return err
	}
	// A fatal connection error is reported here rather than from a goroutine.
	if fin, ok := m.(App); ok && fin.startErr != nil {
		return fin.startErr
	}
	// A clean farewell on the normal screen once the alt-screen is torn down.
	fmt.Printf("\n  %s\n\n", goodbye(th))
	return nil
}

// goodbye is the farewell line printed after a clean quit.
func goodbye(th Theme) string {
	return th.HeaderVal.Render("kli") + th.Dim.Render(" · see you next time · "+creatorHandle)
}
