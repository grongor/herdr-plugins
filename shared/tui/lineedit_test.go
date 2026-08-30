package tui

import "testing"

// typed builds an editor by feeding the text through Handle, so the cursor ends up where typing leaves it.
func typed(text string) *LineEdit {
	var e LineEdit
	for _, r := range text {
		e.Handle(Key{Kind: KeyRune, Rune: r})
	}

	return &e
}

func TestLineEditOperations(t *testing.T) {
	const ref = "fix/TICKET-2568-billing-dashboard"

	tests := []struct {
		name   string
		text   string
		cursor int
		keys   []Key
		want   string
		at     int
	}{
		{"typing inserts at the cursor", "ab", 1, []Key{{Kind: KeyRune, Rune: 'x'}}, "axb", 2},
		{"backspace deletes before the cursor", "abc", 2, []Key{{Kind: KeyBackspace}}, "ac", 1},
		{"backspace at the start does nothing", "abc", 0, []Key{{Kind: KeyBackspace}}, "abc", 0},
		{"delete removes under the cursor", "abc", 1, []Key{{Kind: KeyDelete}}, "ac", 1},
		{"delete at the end does nothing", "abc", 3, []Key{{Kind: KeyDelete}}, "abc", 3},
		{"left moves one rune", "abc", 3, []Key{{Kind: KeyLeft}}, "abc", 2},
		{"left stops at the start", "abc", 0, []Key{{Kind: KeyLeft}}, "abc", 0},
		{"right moves one rune", "abc", 1, []Key{{Kind: KeyRight}}, "abc", 2},
		{"right stops at the end", "abc", 3, []Key{{Kind: KeyRight}}, "abc", 3},
		{"home and end", "abc", 1, []Key{{Kind: KeyHome}}, "abc", 0},
		{"end", "abc", 1, []Key{{Kind: KeyEnd}}, "abc", 3},
		{"ctrl-right stops after each word", ref, 0, []Key{{Kind: KeyRight, Mod: ModCtrl}}, ref, 3},
		{"ctrl-right twice", ref, 0, []Key{{Kind: KeyRight, Mod: ModCtrl}, {Kind: KeyRight, Mod: ModCtrl}}, ref, 10},
		{"ctrl-right across the digits", ref, 10, []Key{{Kind: KeyRight, Mod: ModCtrl}}, ref, 15},
		{"ctrl-right at the end stays", ref, len(ref), []Key{{Kind: KeyRight, Mod: ModCtrl}}, ref, len(ref)},
		{"alt-left is ctrl-left", ref, len(ref), []Key{{Kind: KeyLeft, Mod: ModAlt}}, ref, 24},
		{"ctrl-left twice lands before the number", "fix/TICKET-1-x", 14,
			[]Key{{Kind: KeyLeft, Mod: ModCtrl}, {Kind: KeyLeft, Mod: ModCtrl}}, "fix/TICKET-1-x", 11},
		{"ctrl-w drops the word before the cursor", "feature topic", 13, []Key{Ctrl('w')}, "feature ", 8},
		{"ctrl-w crosses trailing spaces", "feature topic   ", 16, []Key{Ctrl('w')}, "feature ", 8},
		{"ctrl-w on the only word", "topic", 5, []Key{Ctrl('w')}, "", 0},
		{"ctrl-w on spaces alone", "   ", 3, []Key{Ctrl('w')}, "", 0},
		{"ctrl-w mid-line keeps the rest", ref, 10, []Key{Ctrl('w')}, "fix/-2568-billing-dashboard", 4},
		{"ctrl-u deletes back to the start", "abc/def", 4, []Key{Ctrl('u')}, "def", 0},
		{"ctrl-u at the end clears", "abc", 3, []Key{Ctrl('u')}, "", 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := &LineEdit{Text: []rune(test.text), Cursor: test.cursor}
			for _, k := range test.keys {
				if !e.Handle(k) {
					t.Fatalf("Handle(%+v) = false, want it consumed", k)
				}
			}

			if got := e.String(); got != test.want {
				t.Fatalf("text = %q, want %q", got, test.want)
			}
			if e.Cursor != test.at {
				t.Fatalf("cursor = %d, want %d", e.Cursor, test.at)
			}
		})
	}
}

// The acceptance case: from the end of fix/TICKET-1-x, two word jumps back and a keystroke land inside the number.
func TestLineEditWordJumpThenType(t *testing.T) {
	e := typed("fix/TICKET-1-x")

	e.Handle(Key{Kind: KeyLeft, Mod: ModCtrl})
	e.Handle(Key{Kind: KeyLeft, Mod: ModCtrl})
	e.Handle(Key{Kind: KeyRune, Rune: 'y'})

	if got := e.String(); got != "fix/TICKET-y1-x" {
		t.Fatalf("text = %q, want fix/TICKET-y1-x", got)
	}
}

func TestLineEditLeavesTheCallersKeysAlone(t *testing.T) {
	for _, kind := range []KeyKind{KeyUp, KeyDown, KeyEnter, KeyEsc, KeyTab, KeyShiftTab, KeyCtrlC, KeyMouse} {
		e := typed("abc")
		if e.Handle(Key{Kind: kind}) {
			t.Errorf("Handle(kind %d) = true, want it left to the caller", kind)
		}
		if e.String() != "abc" || e.Cursor != 3 {
			t.Errorf("kind %d changed the editor to %q at %d", kind, e.String(), e.Cursor)
		}
	}
}

func TestLineEditSetPutsTheCursorAtTheEnd(t *testing.T) {
	e := typed("abc")
	e.Cursor = 1
	e.Set("fix/x")

	if e.String() != "fix/x" || e.Cursor != 5 {
		t.Fatalf("after Set: %q at %d, want fix/x at 5", e.String(), e.Cursor)
	}
}

func TestLineEditViewScrollsToTheCursor(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		cursor int
		width  int
		want   string
		caret  int
	}{
		{"fits", "main", 4, 8, "main", 4},
		{"just fits with the caret past the end", "main", 4, 5, "main", 4},
		{"the caret past the end needs a column", "main", 4, 4, "ain", 3},
		{"scrolled to the end", "abcdefgh", 8, 4, "fgh", 3},
		{"cursor mid-text scrolls just far enough", "abcdefgh", 4, 4, "bcde", 3},
		{"cursor at the start shows the head", "abcdefgh", 0, 4, "abcd", 0},
		{"cursor near the end", "abcdefgh", 6, 4, "defg", 3},
		// Ideographs are two columns: four of them are eight, and the caret past them needs a ninth.
		{"cjk at the end", "設定設定", 4, 5, "設定", 4},
		{"cjk mid-text", "設定設定", 1, 3, "定", 0},
		{"cjk cursor on a wide rune that does not fit beside the caret", "a設", 1, 2, "設", 0},
		{"no room", "main", 4, 0, "", 0},
		{"empty", "", 0, 4, "", 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := &LineEdit{Text: []rune(test.text), Cursor: test.cursor}
			got, caret := e.View(test.width)

			if got != test.want || caret != test.caret {
				t.Fatalf("View(%d) = %q at %d, want %q at %d", test.width, got, caret, test.want, test.caret)
			}
			if Cells(got) > test.width {
				t.Fatalf("%q is %d columns, over the width of %d", got, Cells(got), test.width)
			}
		})
	}
}

// TestLineEditLocateFindsTheClickedRune: the columns Locate is asked about are the columns View drew, so a scrolled
// field answers with the rune the click was actually over.
func TestLineEditLocateFindsTheClickedRune(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		cursor int
		width  int
		col    int
		want   int
	}{
		{"the first column is the start", "main", 4, 8, 0, 0},
		{"mid-text", "main", 4, 8, 2, 2},
		{"the column past the last rune is the end", "main", 4, 8, 4, 4},
		{"far past the text is still the end", "main", 4, 8, 40, 4},
		// The field shows "fgh" of "abcdefgh": column 0 is 'f', which is rune 5.
		{"scrolled to the end", "abcdefgh", 8, 4, 0, 5},
		{"scrolled, mid-view", "abcdefgh", 8, 4, 2, 7},
		{"scrolled, past the view", "abcdefgh", 8, 4, 3, 8},
		// "設定設定" shows all four with the caret past them; each is two columns wide.
		{"the left column of a wide rune", "設定設定", 4, 9, 4, 2},
		{"the right column of a wide rune", "設定設定", 4, 9, 5, 2},
		{"the column after a wide rune", "設定設定", 4, 9, 6, 3},
		{"a negative column is the start", "main", 4, 8, -3, 0},
		{"no room", "main", 4, 0, 0, 0},
		{"empty", "", 0, 8, 3, 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := &LineEdit{Text: []rune(test.text), Cursor: test.cursor}
			if got := e.Locate(test.width, test.col); got != test.want {
				t.Fatalf("Locate(%d, %d) = %d, want %d", test.width, test.col, got, test.want)
			}
		})
	}
}
