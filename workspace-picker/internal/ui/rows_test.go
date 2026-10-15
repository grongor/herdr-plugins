package ui

import (
	"testing"

	"github.com/grongor/herdr-plugins/shared/tui"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
)

// TestTargetable pins that a bare repository with no worktrees is reachable by the cursor, for ctrl-g.
func TestTargetable(t *testing.T) {
	plain := discover.Repo{
		Name:      "org/plain",
		Anchor:    "/p/plain",
		Worktrees: []discover.Worktree{{Path: "/p/plain", Branch: "main", Main: true}},
	}
	bared := discover.Repo{
		Name:      "org/bared",
		Anchor:    "/p/bared.git",
		Bare:      true,
		Worktrees: []discover.Worktree{{Path: "/p/bared-worktrees/topic", Branch: "topic"}},
	}
	lonely := discover.Repo{Name: "org/lonely", Anchor: "/p/lonely.git", Bare: true}

	rows := BuildRows([]discover.Repo{plain, bared, lonely})

	want := []struct {
		label      string
		selectable bool
		targetable bool
	}{
		{"org/plain", true, true},
		{"org/bared", false, false}, // header of a bare repo that has checkouts
		{"topic", true, true},       // ... which is where the cursor lands instead
		{"org/lonely", false, true}, // nothing to open, but somewhere to create
	}

	if len(rows) != len(want) {
		t.Fatalf("built %d rows, want %d", len(rows), len(want))
	}
	for i, expected := range want {
		if rows[i].Label != expected.label {
			t.Fatalf("row %d label = %q, want %q", i, rows[i].Label, expected.label)
		}
		if got := rows[i].Selectable(); got != expected.selectable {
			t.Errorf("%s: Selectable = %v, want %v", expected.label, got, expected.selectable)
		}
		if got := rows[i].Targetable(); got != expected.targetable {
			t.Errorf("%s: Targetable = %v, want %v", expected.label, got, expected.targetable)
		}
	}
}

// TestDetachedCheckoutsHideBehindTheToggle pins that ctrl-t hides linked detached checkouts only, and that the
// remaining sibling closes the tree.
func TestDetachedCheckoutsHideBehindTheToggle(t *testing.T) {
	repo := discover.Repo{
		Name:   "org/repo",
		Anchor: "/p/repo",
		Head:   "main",
		Worktrees: []discover.Worktree{
			{Path: "/p/repo", Head: "abc1234", Detached: true, Main: true},
			{Path: "/p/repo-topic", Branch: "topic"},
			{Path: "/tmp/scratch/src-pr", Head: "def5678", Detached: true},
		},
	}

	picker := &pickerState{rows: BuildRows([]discover.Repo{repo})}
	picker.refresh()

	labels := func() []string {
		visible := make([]string, 0, len(picker.visible))
		for _, v := range picker.visible {
			visible = append(visible, picker.rows[v.index].Label)
		}

		return visible
	}

	want := []string{"org/repo", "topic"}
	if got := labels(); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("hidden view = %v, want %v", got, want)
	}
	if got := picker.visible[1].prefix(picker.rows); got != prefixLast {
		t.Errorf("the remaining child draws %q, want %q", got, prefixLast)
	}
	// The counter keeps counting hidden rows: the one on-screen sign there is anything to show.
	if got := picker.countRows(); got != 3 {
		t.Errorf("countRows = %d, want 3", got)
	}
	if got := picker.countVisible(); got != 2 {
		t.Errorf("countVisible = %d, want 2", got)
	}

	picker.handle(tui.Ctrl('t'))
	if got := labels(); len(got) != 3 {
		t.Fatalf("shown view = %v, want all three rows", got)
	}
	if got := picker.visible[1].prefix(picker.rows); got != prefixChild {
		t.Errorf("the first child draws %q again, want %q", got, prefixChild)
	}

	picker.handle(tui.Ctrl('t'))
	if got := labels(); len(got) != 2 {
		t.Fatalf("re-hidden view = %v, want two rows", got)
	}
}

// TestQueryDoesNotResurfaceHiddenCheckouts pins that a query matching a hidden checkout keeps it hidden.
func TestQueryDoesNotResurfaceHiddenCheckouts(t *testing.T) {
	repo := discover.Repo{
		Name:   "org/repo",
		Anchor: "/p/repo",
		Worktrees: []discover.Worktree{
			{Path: "/p/repo", Branch: "main", Main: true},
			{Path: "/tmp/scratch/src-pr", Head: "def5678", Detached: true},
		},
	}

	picker := &pickerState{rows: BuildRows([]discover.Repo{repo})}
	picker.query.Set("def5678")
	picker.refresh()

	for _, v := range picker.visible {
		if row := picker.rows[v.index]; row.Worktree != nil && row.Worktree.Detached {
			t.Fatalf("query surfaced hidden checkout %q", row.Label)
		}
	}
}

// TestQueryEditsAtTheCaret: the filter is a line with a cursor, so a correction goes where the caret is and moving
// the caret alone does not refilter.
func TestQueryEditsAtTheCaret(t *testing.T) {
	repo := discover.Repo{
		Name:   "org/repo",
		Anchor: "/p/repo",
		Worktrees: []discover.Worktree{
			{Path: "/p/repo", Branch: "main", Main: true},
			{Path: "/p/repo-topic", Branch: "feature/topic"},
			{Path: "/p/repo-other", Branch: "feature/other"},
		},
	}

	picker := &pickerState{rows: BuildRows([]discover.Repo{repo})}
	picker.refresh()
	for _, r := range "tqopic" {
		picker.handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	if len(picker.visible) != 0 {
		t.Fatalf("visible = %d rows, want none for a misspelt query", len(picker.visible))
	}

	picker.handle(tui.Key{Kind: tui.KeyHome})
	picker.handle(tui.Key{Kind: tui.KeyRight})
	picker.handle(tui.Key{Kind: tui.KeyDelete})

	if got := picker.query.String(); got != "topic" {
		t.Fatalf("query = %q, want the q deleted under the caret", got)
	}
	if len(picker.visible) != 2 {
		t.Fatalf("visible = %d rows, want the repository and its topic checkout", len(picker.visible))
	}

	picker.move(1)
	picker.handle(tui.Key{Kind: tui.KeyHome})
	if picker.cursor != 1 {
		t.Fatalf("cursor = %d, want a caret move to leave the list cursor alone", picker.cursor)
	}
}

// TestCursorReachesEmptyBareRepo pins that the cursor starts on the only row of a lonely bare repository.
func TestCursorReachesEmptyBareRepo(t *testing.T) {
	rows := BuildRows([]discover.Repo{{Name: "org/lonely", Anchor: "/p/lonely.git", Bare: true}})

	picker := &pickerState{rows: rows}
	picker.refresh()

	if len(picker.visible) != 1 {
		t.Fatalf("visible rows = %d, want 1", len(picker.visible))
	}
	if picker.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", picker.cursor)
	}
	if picker.selected() != nil {
		t.Error("a bare repository has no checkout to open")
	}
	if picker.countRows() != 1 {
		t.Errorf("countRows = %d, want 1", picker.countRows())
	}

	picker.handle(tui.Ctrl('g'))
	if picker.form == nil {
		t.Fatal("ctrl-g did not open the dialog")
	}
	if picker.form.Request().Repo.Name != "org/lonely" {
		t.Errorf("dialog repo = %q, want org/lonely", picker.form.Request().Repo.Name)
	}
}

// TestQueryRanksPathHitsLast pins that a term found only in a checkout's path still matches but lists after a branch
// that carries it, that a group lists best match first rather than by name, and that a term found only outside the
// label leaves the other terms highlighted.
func TestQueryRanksPathHitsLast(t *testing.T) {
	repo := discover.Repo{
		Name:   "org/kabal",
		Anchor: "/p/kabal",
		Worktrees: []discover.Worktree{
			{Path: "/p/kabal", Branch: "main", Main: true},
			{Path: "/p/kabal-status", Branch: "fix/PROJ-2811-status"},
			{Path: "/p/scratch-2811", Branch: "chore/other"},
			{Path: "/p/kabal-kafka", Branch: "fix/PROJ-2481-kafka-v1"},
		},
	}
	rows := BuildRows([]discover.Repo{repo})

	visible := filter(rows, "kabal 2811", toggles{})
	got := make([]string, 0, len(visible))
	for _, v := range visible {
		got = append(got, rows[v.index].Label)
	}
	want := []string{"org/kabal", "fix/PROJ-2811-status", "chore/other"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("visible = %q, want %q", got, want)
	}
	if got := visible[1].highlights; len(got) != 4 || got[0] != 9 || got[3] != 12 {
		t.Errorf("highlights = %v, want 2811 marked at 9..12", got)
	}
}

// TestSlashQueryMatchesAcrossTheRepositoryName pins that "lorem/dolor" finds every lorem repository whose own name
// mentions test, not only the one where the two are adjacent.
func TestSlashQueryMatchesAcrossTheRepositoryName(t *testing.T) {
	repos := []discover.Repo{
		{Name: "lorem/ipsum-condolor", Anchor: "/p/ipsum-condolor"},
		{Name: "lorem/dolores", Anchor: "/p/dolores"},
		{Name: "amet/lorem-dolores", Anchor: "/p/lorem-dolores"},
	}
	rows := BuildRows(repos)

	visible := filter(rows, "lorem/dolor", toggles{})
	got := make([]string, 0, len(visible))
	for _, v := range visible {
		got = append(got, rows[v.index].Label)
	}
	want := []string{"lorem/dolores", "lorem/ipsum-condolor"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("visible = %q, want %q", got, want)
	}
}
