package newworktree

import (
	"testing"

	"github.com/grongor/herdr-plugins/shared/tui"
)

// valueColumn is the 1-based screen column of the offset-th column of a field's value, as the rendered rows have it.
func valueColumn(offset int) int { return tui.Cells(markerOn) + labelWidth + offset + 1 }

// fieldRow is the 1-based screen row a field is drawn on.
func fieldRow(field int) int { return fieldsTop + field + 1 }

// rendered lays out the state a click is resolved against, the way the real frame does.
func rendered(f *Form, width, height int) { f.Render(&tui.Screen{}, width, height) }

// TestClickingBaseTwiceOpensTheBranchList: the first click only moves to the field, as it only moves the cursor in a
// list, so a stray click cannot put a modal list on screen; the second opens it, which is what space does.
func TestClickingBaseTwiceOpensTheBranchList(t *testing.T) {
	f := open(t, Options{}, branchList())
	rendered(f, 80, 24)

	press := tui.Mouse{Button: tui.MouseLeft, Col: valueColumn(2), Row: fieldRow(fieldBase)}

	if k, ok := f.Click(press); ok {
		t.Fatalf("the first click on Base stood for %+v", k)
	}
	if f.field != fieldBase {
		t.Fatalf("field = %d, want the click to have moved to Base", f.field)
	}
	if f.picker != nil {
		t.Fatal("the first click on Base opened the branch list")
	}

	if k, ok := f.Click(press); ok {
		t.Fatalf("the second click on Base stood for %+v", k)
	}
	if f.picker == nil {
		t.Fatal("the second click on Base did not open the branch list")
	}
}

// TestClickingTheOfferedBaseStillSwaps pins that the dimmed "←/→ <base>" keeps swapping the base: it names a key, and
// the list is the rest of the row.
func TestClickingTheOfferedBaseStillSwaps(t *testing.T) {
	f := open(t, Options{}, branchList())
	rendered(f, 80, 24)

	before := f.Request().Base

	var swap tui.Zone
	for _, zone := range f.zones {
		if zone.Kind == zoneBase {
			swap = zone
		}
	}
	if swap.Kind != zoneBase {
		t.Fatal("the row has no zone for the offered base")
	}

	if _, ok := f.Click(tui.Mouse{Button: tui.MouseLeft, Col: swap.From + 2, Row: swap.Line + 1}); ok {
		t.Fatal("clicking the offered base stood for a keystroke")
	}
	if f.picker != nil {
		t.Fatal("clicking the offered base opened the branch list")
	}
	if got := f.Request().Base; got == before {
		t.Fatalf("base = %q, want the click to have swapped it", got)
	}
}

// TestClickingATextFieldPlacesTheCaret: a click inside the focused field puts the caret where the text was drawn, so
// the next keystroke lands there. An unfocused field is only moved to, as Base is.
func TestClickingATextFieldPlacesTheCaret(t *testing.T) {
	f := open(t, Options{}, branchList())
	f.branch.Set("feat/dashboard")
	f.proposePath()
	rendered(f, 80, 24)

	if _, ok := f.Click(tui.Mouse{Button: tui.MouseLeft, Col: valueColumn(4), Row: fieldRow(fieldBranch)}); ok {
		t.Fatal("a click in the branch field stood for a keystroke")
	}
	if f.branch.Cursor != 4 {
		t.Fatalf("caret = %d, want it on the clicked rune", f.branch.Cursor)
	}

	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: 'X'})
	if got := f.Request().Branch; got != "featX/dashboard" {
		t.Fatalf("branch = %q, want the rune typed where the click put the caret", got)
	}

	end := len(f.path.Text)
	if _, ok := f.Click(tui.Mouse{Button: tui.MouseLeft, Col: valueColumn(3), Row: fieldRow(fieldPath)}); ok {
		t.Fatal("a click on the path field stood for a keystroke")
	}
	if f.field != fieldPath {
		t.Fatalf("field = %d, want the click to have moved to Path", f.field)
	}
	if f.path.Cursor != end {
		t.Fatalf("caret = %d, want the first click to leave it at the end (%d)", f.path.Cursor, end)
	}

	rendered(f, 80, 24)
	if _, ok := f.Click(tui.Mouse{Button: tui.MouseLeft, Col: valueColumn(3), Row: fieldRow(fieldPath)}); ok {
		t.Fatal("a second click on the path field stood for a keystroke")
	}
	if f.path.Cursor != 3 {
		t.Fatalf("caret = %d, want the second click to place it", f.path.Cursor)
	}
}

// TestClickingTheFilterPlacesTheCaret pins the branch list's query line: clicking it moves the caret without
// refiltering, so the cursor stays on the branch it was on.
func TestClickingTheFilterPlacesTheCaret(t *testing.T) {
	f := open(t, Options{}, branchList())
	f.openPicker()
	f.showRemote = true
	f.refreshPicker()
	for _, r := range "main" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	f.Handle(tui.Key{Kind: tui.KeyDown})
	rendered(f, 80, 24)

	if got := len(f.picker.matches); got != 2 {
		t.Fatalf("matches = %q, want main and origin/main", names(f))
	}

	if _, ok := f.Click(tui.Mouse{Button: tui.MouseLeft, Col: tui.Cells(promptText) + 2, Row: 1}); ok {
		t.Fatal("a click on the filter stood for a keystroke")
	}
	if f.picker.query.Cursor != 1 {
		t.Fatalf("caret = %d, want it on the clicked rune", f.picker.query.Cursor)
	}
	if f.picker.cursor != 1 {
		t.Fatalf("list cursor = %d, want the click on the filter to leave it alone", f.picker.cursor)
	}

	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: 'X'})
	if got := f.picker.query.String(); got != "mXain" {
		t.Fatalf("filter = %q, want the rune typed where the click put the caret", got)
	}
}
