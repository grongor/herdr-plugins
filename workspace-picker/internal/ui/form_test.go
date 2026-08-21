package ui

import (
	"testing"

	"github.com/grongor/herdr-plugins/shared/tui"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
	"github.com/grongor/herdr-plugins/workspace-picker/internal/worktree"
)

// repoWithWorktrees: a repository on "main" with one topic checkout and one detached one.
func repoWithWorktrees() discover.Repo {
	return discover.Repo{
		Name:   "org/lorem",
		Anchor: "/p/lorem",
		Head:   "main",
		Worktrees: []discover.Worktree{
			{Path: "/p/lorem", Branch: "main", Main: true},
			{Path: "/p/lorem-topic", Branch: "proposal/di"},
			{Path: "/p/lorem-loose", Head: "abc1234", Detached: true},
		},
	}
}

// TestBaseChoices: the checkout under the cursor first, the repository's HEAD second.
func TestBaseChoices(t *testing.T) {
	repo := repoWithWorktrees()
	rows := BuildRows([]discover.Repo{repo})

	tests := []struct {
		name string
		row  int
		want []string
	}{
		{"a linked checkout branches off itself", 1, []string{"proposal/di", "main"}},
		{"a detached one offers its commit", 2, []string{"abc1234", "main"}},
		{"the main checkout is the repository HEAD, once", 0, []string{"main"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := baseChoices(rows[test.row])

			if len(got) != len(test.want) {
				t.Fatalf("choices = %q, want %q", got, test.want)
			}
			for i, want := range test.want {
				if got[i] != want {
					t.Fatalf("choices = %q, want %q", got, test.want)
				}
			}
		})
	}
}

// TestBaseChoicesWithoutHead: a bare repository that was never given a branch.
func TestBaseChoicesWithoutHead(t *testing.T) {
	rows := BuildRows([]discover.Repo{{Name: "org/lonely", Anchor: "/p/lonely.git", Bare: true}})

	if got := baseChoices(rows[0]); len(got) != 0 {
		t.Fatalf("choices = %q, want none so Herdr decides", got)
	}
}

// TestFormStartsFromSelectedWorktree: the branch under the cursor is the base, the alternative one keystroke away.
func TestFormStartsFromSelectedWorktree(t *testing.T) {
	repo := repoWithWorktrees()
	picker := &pickerState{rows: BuildRows([]discover.Repo{repo})}
	picker.refresh()

	picker.move(1) // onto the "proposal/di" checkout
	picker.handle(tui.Key{Kind: tui.KeyCtrlG})

	f := picker.form
	if f == nil {
		t.Fatal("ctrl-g did not open the dialog")
	}
	if got := f.base; got != "proposal/di" {
		t.Fatalf("base = %q, want the branch the cursor was on", got)
	}

	f.field = fieldBase
	f.handle(tui.Key{Kind: tui.KeyRight})
	if got := f.base; got != "main" {
		t.Fatalf("base after ←/→ = %q, want main", got)
	}

	f.handle(tui.Key{Kind: tui.KeyRight})
	if got := f.base; got != "proposal/di" {
		t.Fatalf("base wrapped to %q, want proposal/di", got)
	}
}

// siblingForm opens the dialog on the "proposal/di" checkout with the sibling layout.
func siblingForm() *form {
	repo := repoWithWorktrees()

	layout := worktree.Switch(worktree.Repo{Anchor: repo.Anchor}, worktree.ModeSibling)

	return newForm(BuildRows([]discover.Repo{repo})[1], layout)
}

// TestBaseIsAChoice: the arrows step Base through what is offered and type nothing into it, while the same arrows in
// Branch move the caret.
func TestBaseIsAChoice(t *testing.T) {
	f := siblingForm()

	f.field = fieldBase
	for _, r := range "v1" {
		f.handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	if f.base != "proposal/di" {
		t.Fatalf("base after typing = %q, want it left on the offered choice", f.base)
	}

	f.handle(tui.Key{Kind: tui.KeyLeft})
	if f.base != "main" {
		t.Fatalf("base after ← = %q, want the other choice", f.base)
	}
	f.handle(tui.Key{Kind: tui.KeyRune, Rune: ' '})
	if f.base != "proposal/di" {
		t.Fatalf("base after space = %q, want it stepped", f.base)
	}

	f.field = fieldBranch
	for _, r := range "ab" {
		f.handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	f.handle(tui.Key{Kind: tui.KeyLeft})
	f.handle(tui.Key{Kind: tui.KeyRune, Rune: 'x'})
	if got := f.branch.String(); got != "axb" {
		t.Fatalf("branch = %q, want ← to have moved the caret, not chosen anything", got)
	}
	if f.base != "proposal/di" {
		t.Fatalf("base = %q, want it untouched by an arrow in Branch", f.base)
	}
}

// TestBranchEditsAtTheCaret: word jumps, deletions and home/end all act where the caret is, and the path follows.
func TestBranchEditsAtTheCaret(t *testing.T) {
	f := siblingForm()

	for _, r := range "fix/TICKET-1-x" {
		f.handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}

	f.handle(tui.Key{Kind: tui.KeyLeft, Mod: tui.ModCtrl})
	f.handle(tui.Key{Kind: tui.KeyLeft, Mod: tui.ModCtrl})
	f.handle(tui.Key{Kind: tui.KeyRune, Rune: 'y'})
	if got := f.branch.String(); got != "fix/TICKET-y1-x" {
		t.Fatalf("branch = %q, want fix/TICKET-y1-x", got)
	}
	if got := f.path.String(); got != "/p/lorem-fix-TICKET-y1-x" {
		t.Fatalf("path = %q, want it reproposed from the edited branch", got)
	}

	// The caret is right after the y, so ctrl-w takes the y and the word before it.
	f.handle(tui.Key{Kind: tui.KeyCtrlW})
	f.handle(tui.Key{Kind: tui.KeyCtrlW})
	if got := f.branch.String(); got != "fix/1-x" {
		t.Fatalf("branch after ctrl-w twice = %q, want the two words before the caret gone", got)
	}

	f.handle(tui.Key{Kind: tui.KeyHome})
	f.handle(tui.Key{Kind: tui.KeyDelete})
	f.handle(tui.Key{Kind: tui.KeyEnd})
	f.handle(tui.Key{Kind: tui.KeyBackspace})
	if got := f.branch.String(); got != "ix/1-" {
		t.Fatalf("branch after home, delete, end, backspace = %q, want ix/1-", got)
	}
}

// TestPathEditsAtTheCaret: an insertion in the middle of the path is a hand edit like any other.
func TestPathEditsAtTheCaret(t *testing.T) {
	f := siblingForm()

	f.field = fieldBranch
	f.handle(tui.Key{Kind: tui.KeyRune, Rune: 'a'})
	f.field = fieldPath
	f.handle(tui.Key{Kind: tui.KeyLeft, Mod: tui.ModCtrl})
	f.handle(tui.Key{Kind: tui.KeyRune, Rune: 'x'})
	if got := f.path.String(); got != "/p/lorem-xa" {
		t.Fatalf("path = %q, want x before the last word", got)
	}

	f.field = fieldBranch
	f.handle(tui.Key{Kind: tui.KeyRune, Rune: 'b'})
	if got := f.path.String(); got != "/p/lorem-xa" {
		t.Fatalf("path = %q, want the hand-edited one kept", got)
	}
}

// TestPathFollowsTheBranchUntilItIsTyped: a hand-written path stops tracking the branch, but a layout change
// still reproposes it.
func TestPathFollowsTheBranchUntilItIsTyped(t *testing.T) {
	repo := repoWithWorktrees()
	f := newForm(BuildRows([]discover.Repo{repo})[1], worktree.For(
		worktree.Repo{Anchor: repo.Anchor}, "", worktree.ModeSibling,
	))

	f.field = fieldBranch
	for _, r := range "wip" {
		f.handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	if got := f.path.String(); got != "/p/lorem-wip" {
		t.Fatalf("path = %q, want it proposed from the branch", got)
	}

	f.field = fieldPath
	f.handle(tui.Key{Kind: tui.KeyCtrlU})
	for _, r := range "/elsewhere/x" {
		f.handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}

	f.field = fieldBranch
	f.handle(tui.Key{Kind: tui.KeyRune, Rune: '2'})
	if got := f.path.String(); got != "/elsewhere/x" {
		t.Fatalf("path = %q, want the hand-written one kept", got)
	}

	f.field = fieldLayout
	f.handle(tui.Key{Kind: tui.KeyRight})
	if got := f.path.String(); got != "/p/lorem-worktrees/wip2" {
		t.Fatalf("path = %q, want the newly chosen layout's proposal", got)
	}
}

// TestCycleOnlyTouchesTheFocusedField: ←/→ acts on the focused field only, since both Base and Layout answer to it.
func TestCycleOnlyTouchesTheFocusedField(t *testing.T) {
	f := siblingForm()

	f.field = fieldLayout
	f.handle(tui.Key{Kind: tui.KeyRight})

	if got := f.base; got != "proposal/di" {
		t.Errorf("base = %q, want it left alone while Layout has focus", got)
	}
	if worktree.Modes[f.mode] != worktree.ModeShared {
		t.Errorf("mode = %q, want shared", worktree.Modes[f.mode])
	}
}
