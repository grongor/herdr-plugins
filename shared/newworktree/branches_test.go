package newworktree

import (
	"testing"

	"github.com/grongor/herdr-plugins/shared/tui"
)

func branchList() BranchList {
	return BranchList{
		Local:   []string{"main", "fix/PROJ-12-kafka-retries", "chore/deps"},
		Remote:  []string{"origin/main", "origin/feat/PROJ-40-dashboard"},
		Default: "origin/main",
	}
}

func names(f *Form) []string {
	shown := make([]string, 0, len(f.picker.matches))
	for _, match := range f.picker.matches {
		shown = append(shown, match.name)
	}

	return shown
}

func TestBranchListHidesRemotesUntilAsked(t *testing.T) {
	f := open(t, Options{}, branchList())

	f.field = fieldBase
	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: ' '})

	if f.picker == nil {
		t.Fatal("space on the base field did not open the branch list")
	}
	if got := len(names(f)); got != 3 {
		t.Fatalf("list = %q, want the three local branches", names(f))
	}

	f.Handle(tui.Ctrl('r'))
	if got := len(names(f)); got != 5 {
		t.Fatalf("list after ctrl-r = %q, want the remote ones too", names(f))
	}

	f.Handle(tui.Ctrl('r'))
	if got := len(names(f)); got != 3 {
		t.Fatalf("list after a second ctrl-r = %q, want them hidden again", names(f))
	}
}

func TestBranchListStartsOnRemotesWhenConfigured(t *testing.T) {
	yes := true
	f := open(t, Options{Config: Config{ShowRemoteBranches: &yes}}, branchList())

	f.openPicker()
	if got := len(names(f)); got != 5 {
		t.Fatalf("list = %q, want the remote branches listed from the start", names(f))
	}
}

func TestBranchListFiltersAndPicks(t *testing.T) {
	f := open(t, Options{}, branchList())

	f.openPicker()
	for _, r := range "kafka" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}

	shown := names(f)
	if len(shown) != 1 || shown[0] != "fix/PROJ-12-kafka-retries" {
		t.Fatalf("list = %q, want the one branch that matches", shown)
	}

	f.Handle(tui.Key{Kind: tui.KeyEnter})

	if f.picker != nil {
		t.Fatal("choosing a branch left the list on screen")
	}
	if got := f.Request().Base; got != "fix/PROJ-12-kafka-retries" {
		t.Fatalf("base = %q, want the chosen branch", got)
	}
}

// TestBranchListEnterWithoutAMatch: an empty filter is no answer, but text nothing matches is the ref itself.
func TestBranchListEnterWithoutAMatch(t *testing.T) {
	empty := open(t, Options{Base: "main"}, BranchList{})
	empty.openPicker()
	empty.Handle(tui.Key{Kind: tui.KeyEnter})
	if empty.picker == nil {
		t.Fatal("enter on an empty filter closed the list")
	}

	f := open(t, Options{Base: "main"}, branchList())
	f.openPicker()
	for _, r := range " origin/v2 " {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	if len(names(f)) != 0 {
		t.Fatalf("list = %q, want nothing to match", names(f))
	}
	f.Handle(tui.Key{Kind: tui.KeyEnter})

	if f.picker != nil {
		t.Fatal("enter on an unmatched filter left the list up")
	}
	if got := f.Request().Base; got != "origin/v2" {
		t.Fatalf("base = %q, want the trimmed filter text", got)
	}
}

// TestBranchListFilterEditsAtTheCaret: the filter has a caret, and moving it alone leaves the selection alone.
func TestBranchListFilterEditsAtTheCaret(t *testing.T) {
	f := open(t, Options{}, branchList())

	f.openPicker()
	for _, r := range "kfka" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	f.Handle(tui.Key{Kind: tui.KeyLeft})
	f.Handle(tui.Key{Kind: tui.KeyLeft})
	f.Handle(tui.Key{Kind: tui.KeyLeft})
	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: 'a'})

	if got := f.picker.query.String(); got != "kafka" {
		t.Fatalf("filter = %q, want the a inserted at the caret", got)
	}
	if shown := names(f); len(shown) != 1 || shown[0] != "fix/PROJ-12-kafka-retries" {
		t.Fatalf("list = %q, want the one branch that matches", shown)
	}

	f.picker.query.Set("")
	f.refreshPicker()
	f.Handle(tui.Key{Kind: tui.KeyDown})
	f.Handle(tui.Key{Kind: tui.KeyHome})
	if f.picker.cursor != 1 {
		t.Fatalf("cursor = %d, want a caret move to leave the selection alone", f.picker.cursor)
	}
}

// TestBranchListEscapeGoesBackToTheDialog pins that esc in the list keeps the dialog and its fields.
func TestBranchListEscapeGoesBackToTheDialog(t *testing.T) {
	f := open(t, Options{Base: "main"}, branchList())

	f.openPicker()
	if got := f.Handle(tui.Key{Kind: tui.KeyEsc}); got != ActionNone {
		t.Fatalf("esc in the list = %v, want the dialog kept", got)
	}
	if f.picker != nil {
		t.Fatal("esc left the list on screen")
	}
	if got := f.Request().Base; got != "main" {
		t.Fatalf("base = %q, want it untouched", got)
	}

	if got := f.Handle(tui.Key{Kind: tui.KeyEsc}); got != ActionCancel {
		t.Fatalf("esc in the dialog = %v, want it cancelled", got)
	}
}

// TestBranchLinesFitTheWidth pins narrow widths, where the remote marker is what gets pushed past the edge.
func TestBranchLinesFitTheWidth(t *testing.T) {
	f := open(t, Options{}, BranchList{
		Local:  []string{"機能/とても長いブランチ名", "main"},
		Remote: []string{"origin/機能/とても長いブランチ名"},
	})

	yes := true
	f.opts.Config.ShowRemoteBranches = &yes
	f.showRemote = true
	f.openPicker()

	sc := &tui.Screen{}
	for _, width := range []int{20, 21, 40} {
		for _, match := range f.picker.matches {
			for _, selected := range []bool{false, true} {
				line := branchLine(sc, match, selected, width)
				if got := tui.Cells(line); got > width {
					t.Errorf("%q (selected=%v) is %d columns at width %d", match.name, selected, got, width)
				}
			}
		}
	}
}

// TestRemoteBasePrefillsTheBranch: a remote base is what gets checked out, so its name lands in the branch field without
// the remote's prefix — from the list or from ←/→ — until a name is typed by hand. A local base names nothing.
func TestRemoteBasePrefillsTheBranch(t *testing.T) {
	f := open(t, Options{}, branchList())

	pick := func(query string) {
		f.openPicker()
		f.showRemote = true
		f.refreshPicker()
		for _, r := range query {
			f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
		}
		f.Handle(tui.Key{Kind: tui.KeyEnter})
	}

	pick("dashboard")
	if req := f.Request(); req.Branch != "feat/PROJ-40-dashboard" || req.Base != "origin/feat/PROJ-40-dashboard" {
		t.Fatalf("branch = %q, base = %q; want the remote branch under its own name", req.Branch, req.Base)
	}
	if got := f.Request().Path; got != "/p/lorem-PROJ-40-dashboard" {
		t.Fatalf("path = %q, want it proposed from the prefilled branch", got)
	}

	// origin/main is the remote before it in the choices now; stepping there renames the field too.
	f.field = fieldBase
	f.Handle(tui.Key{Kind: tui.KeyRight})
	if req := f.Request(); req.Base != "origin/main" || req.Branch != "main" {
		t.Fatalf("branch = %q, base = %q; want the field to follow the base", req.Branch, req.Base)
	}

	pick("chore")
	if req := f.Request(); req.Base != "chore/deps" || req.Branch != "main" {
		t.Fatalf("branch = %q, base = %q; want a local base to leave the field alone", req.Branch, req.Base)
	}

	f.branch.Set("mine")
	pick("dashboard")
	if got := f.Request().Branch; got != "mine" {
		t.Fatalf("branch = %q, want a typed name kept", got)
	}
}

// TestEnterOnBaseSubmits: Enter means create on every field, the Base field included, so a dialog arrived at with
// everything prefilled takes one keystroke. The list is space, or the first character of a filter.
func TestEnterOnBaseSubmits(t *testing.T) {
	created := 0
	f := open(t, Options{Create: func(Request) error { created++; return nil }}, branchList())

	f.field = fieldBase
	if got := f.Handle(tui.Key{Kind: tui.KeyEnter}); got != ActionDone {
		t.Fatalf("enter on Base = %v, want it to create", got)
	}
	if f.picker != nil {
		t.Fatal("enter on Base opened the branch list instead of submitting")
	}
	if created != 1 {
		t.Fatalf("created %d worktrees, want one", created)
	}
}
