package ui

import (
	"testing"

	"github.com/grongor/herdr-plugins/shared/tui"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
)

// TestShiftedClicksAreIgnored pins that Shift+mouse, every button, is left to the terminal for selecting text.
func TestShiftedClicksAreIgnored(t *testing.T) {
	picker, height := clickable(t, []discover.Repo{
		{Name: "org/a", Anchor: "/p/a", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/a", Branch: "main", Main: true},
			{Path: "/p/a-topic", Branch: "topic"},
		}},
	})

	for _, test := range []struct {
		name string
		m    tui.Mouse
	}{
		{"left on a row", tui.Mouse{Button: tui.MouseLeft, Col: 5, Row: 4, Shift: true}},
		{"right on a row", tui.Mouse{Button: tui.MouseRight, Col: 5, Row: 3, Shift: true}},
		{"wheel", tui.Mouse{Button: tui.MouseWheelDown, Col: 1, Row: 1, Shift: true}},
		{"a footer hint", tui.Mouse{Button: tui.MouseLeft, Col: 2, Row: height, Shift: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if k, ok := picker.click(test.m); ok {
				t.Fatalf("a shifted click stood for %+v", k)
			}
			if picker.cursor != 0 {
				t.Fatalf("cursor moved to %d, want it left on 0", picker.cursor)
			}
			if picker.form != nil {
				t.Fatal("a shifted click opened the dialog")
			}
		})
	}
}

// clickable builds a picker and lays out the state a click is resolved against, the way render does.
func clickable(t *testing.T, repos []discover.Repo) (*pickerState, int) {
	t.Helper()

	const height = 24

	picker := &pickerState{rows: BuildRows(repos)}
	picker.refresh()
	picker.listHeight = height - chromeHeight

	_, hints := (&tui.Screen{}).HintLine(hints(defaultKeys, false, false), height-1, 80)
	picker.zones = hints

	return picker, height
}

func TestClickSelectsThenOpens(t *testing.T) {
	picker, _ := clickable(t, []discover.Repo{
		{Name: "org/a", Anchor: "/p/a", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/a", Branch: "main", Main: true},
			{Path: "/p/a-topic", Branch: "topic"},
		}},
	})

	// The topic checkout, drawn on screen line 4.
	press := tui.Mouse{Button: tui.MouseLeft, Col: 5, Row: 4}

	if _, ok := picker.click(press); ok {
		t.Fatal("the first click opened a workspace, want it only to select")
	}
	if picker.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", picker.cursor)
	}

	k, ok := picker.click(press)
	if !ok || k.Kind != tui.KeyEnter {
		t.Fatalf("second click = %+v (%v), want enter", k, ok)
	}
}

// TestRightClickOpensTheDialog pins right click as ctrl-g, including on a header the cursor cannot reach.
func TestRightClickOpensTheDialog(t *testing.T) {
	picker, _ := clickable(t, []discover.Repo{
		{Name: "org/bared", Anchor: "/p/bared.git", Bare: true, Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/bared-worktrees/topic", Branch: "topic"},
		}},
	})

	if _, ok := picker.click(tui.Mouse{Button: tui.MouseRight, Col: 3, Row: 3}); ok {
		t.Fatal("a right click should open the dialog, not stand for a key")
	}
	if picker.form == nil {
		t.Fatal("right-clicking the header did not open the dialog")
	}
	if picker.form.Request().Repo.Name != "org/bared" {
		t.Fatalf("dialog repo = %q, want org/bared", picker.form.Request().Repo.Name)
	}
}

// TestClickingTheHintRunsIt pins that clicking a footer entry does what the key it names does.
func TestClickingTheHintRunsIt(t *testing.T) {
	picker, height := clickable(t, []discover.Repo{
		{Name: "org/a", Anchor: "/p/a", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/a", Branch: "main", Main: true},
		}},
	})

	// "enter open · ctrl-g new worktree" — land inside the second entry.
	col := tui.Cells(hints(defaultKeys, false, false)[0].Text) + tui.Cells(tui.HintSep) + 2

	k, ok := picker.click(tui.Mouse{Button: tui.MouseLeft, Col: col, Row: height})
	if !ok || !k.Is(tui.Ctrl('g')) {
		t.Fatalf("hint click = %+v (%v), want ctrl-g", k, ok)
	}
}

func TestWheelMovesTheCursor(t *testing.T) {
	picker, _ := clickable(t, []discover.Repo{
		{Name: "org/a", Anchor: "/p/a", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/a", Branch: "main", Main: true},
			{Path: "/p/a-topic", Branch: "topic"},
		}},
	})

	k, ok := picker.click(tui.Mouse{Button: tui.MouseWheelDown, Col: 1, Row: 1})
	if !ok || k.Kind != tui.KeyDown {
		t.Fatalf("wheel = %+v (%v), want down", k, ok)
	}
}

// TestReleaseIsIgnored pins that a click acts on press only.
func TestReleaseIsIgnored(t *testing.T) {
	picker, _ := clickable(t, []discover.Repo{
		{Name: "org/a", Anchor: "/p/a", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/a", Branch: "main", Main: true},
		}},
	})

	if _, ok := picker.click(tui.Mouse{Button: tui.MouseLeft, Col: 5, Row: 3, Release: true}); ok {
		t.Fatal("the release acted on its own")
	}
}

// TestClicksOffTheListAreIgnored pins that the prompt and footer are bounded out: rows are found by arithmetic, and
// with the list scrolled both would work out to off-screen rows.
func TestClicksOffTheListAreIgnored(t *testing.T) {
	repos := make([]discover.Repo, 0, 40)
	for i := range 40 {
		name := string(rune('a' + i%26))
		repos = append(repos, discover.Repo{
			Name: "org/" + name, Anchor: "/p/" + name, Head: "main",
			Worktrees: []discover.Worktree{{Path: "/p/" + name, Branch: "main", Main: true}},
		})
	}

	picker, height := clickable(t, repos)
	picker.offset = 10
	picker.cursor = 10
	picker.zones = nil // the footer is showing an error, so it has no hint zones

	for _, test := range []struct {
		name string
		row  int
	}{
		{"the prompt line", 1},
		{"the divider", 2},
		{"the footer", height},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := picker.click(tui.Mouse{Button: tui.MouseLeft, Col: 5, Row: test.row}); ok {
				t.Fatal("the click stood for a keystroke")
			}
			if picker.cursor != 10 {
				t.Fatalf("cursor moved to %d, want it left on 10", picker.cursor)
			}
		})
	}
}

// TestOpeningTheDialogDropsStaleZones pins that a click on the list's "esc cancel" hint after ctrl-g does not cancel
// the dialog just opened.
func TestOpeningTheDialogDropsStaleZones(t *testing.T) {
	picker, height := clickable(t, []discover.Repo{
		{Name: "org/a", Anchor: "/p/a", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/a", Branch: "main", Main: true},
		}},
	})

	col := 1
	for _, item := range hints(defaultKeys, false, false) {
		if item.Key.Kind == tui.KeyEsc {
			break
		}
		col += tui.Cells(item.Text) + tui.Cells(tui.HintSep)
	}

	picker.handle(tui.Ctrl('g'))
	if picker.form == nil {
		t.Fatal("ctrl-g did not open the dialog")
	}

	if k, ok := picker.click(tui.Mouse{Button: tui.MouseLeft, Col: col, Row: height}); ok {
		t.Fatalf("a stale hint stood for %+v", k)
	}
	if picker.form == nil {
		t.Fatal("the stale hint cancelled the dialog it was drawn under")
	}
}

// TestClickingTheQueryPlacesTheCaret pins the prompt line: clicking it moves the caret to the rune clicked, so the next
// keystroke lands there, and leaves the list alone.
func TestClickingTheQueryPlacesTheCaret(t *testing.T) {
	picker, _ := clickable(t, []discover.Repo{
		{Name: "org/a", Anchor: "/p/a", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/a", Branch: "main", Main: true},
			{Path: "/p/a-topic", Branch: "topic"},
		}},
	})

	for _, r := range "topic" {
		picker.handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}

	// Rendering is what puts the query's columns where a click is resolved against them.
	tui.Capture(picker, 80, 24)

	if k, ok := picker.click(tui.Mouse{Button: tui.MouseLeft, Col: tui.Cells(promptText) + 3, Row: 1}); ok {
		t.Fatalf("a click on the query stood for %+v", k)
	}
	if picker.query.Cursor != 2 {
		t.Fatalf("caret = %d, want it on the clicked rune", picker.query.Cursor)
	}
	if picker.cursor != 0 {
		t.Fatalf("cursor = %d, want the click on the query to leave the list alone", picker.cursor)
	}

	picker.handle(tui.Key{Kind: tui.KeyRune, Rune: 'X'})
	if got := picker.query.String(); got != "toXpic" {
		t.Fatalf("query = %q, want the rune typed where the click put the caret", got)
	}
}
