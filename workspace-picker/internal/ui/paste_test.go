package ui

import (
	"testing"

	"github.com/grongor/herdr-plugins/shared/newworktree"
	"github.com/grongor/herdr-plugins/shared/tui"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
)

// TestPasteReachesTheDialog pins that a bracketed paste with a trailing newline lands in the field as text rather
// than reading as Enter, which would create the worktree.
func TestPasteReachesTheDialog(t *testing.T) {
	picker, _ := clickable(t, []discover.Repo{
		{Name: "org/a", Anchor: "/p/a", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/a", Branch: "main", Main: true},
		}},
	})

	picker.handle(tui.Ctrl('g'))
	if picker.form == nil {
		t.Fatal("ctrl-g did not open the dialog")
	}

	var created bool
	picker.actions = Actions{Create: func(newworktree.Request) error {
		created = true

		return nil
	}}

	for _, k := range (&tui.Decoder{}).Keys([]byte("\x1b[200~feature/PROJ-123\n\x1b[201~")) {
		if picker.Handle(k) {
			t.Fatal("the paste finished the picker")
		}
	}

	if created {
		t.Fatal("the paste created a worktree")
	}
	if picker.form == nil {
		t.Fatal("the paste closed the dialog")
	}
	if got := picker.form.Request().Branch; got != "feature/PROJ-123" {
		t.Fatalf("branch = %q, want the pasted name", got)
	}
}
