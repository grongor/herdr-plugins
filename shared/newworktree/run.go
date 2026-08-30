package newworktree

import (
	"github.com/grongor/herdr-plugins/shared/issue"
	"github.com/grongor/herdr-plugins/shared/tui"
)

func referenceIn(text string) (string, bool) { return issue.Reference(text) }

// Run shows the dialog as the whole of a pane and blocks until done; cancelling is not an error. A plugin with a
// surface of its own underneath (the picker) drives Form directly so Escape returns there.
func Run(opts Options) error {
	return tui.Run(&app{form: New(opts)})
}

type app struct{ form *Form }

func (a *app) Render(sc *tui.Screen) { a.form.Render(sc, sc.Width(), sc.Height()) }

func (a *app) Wake() <-chan struct{} { return a.form.Wake() }

func (a *app) Handle(k tui.Key) bool {
	if k.Kind == tui.KeyMouse {
		synthesised, ok := a.form.Click(k.Mouse)
		if !ok {
			return false
		}
		k = synthesised
	}

	switch a.form.Handle(k) {
	case ActionCancel, ActionQuit, ActionDone:
		return true
	case ActionNone:
	}

	return false
}
