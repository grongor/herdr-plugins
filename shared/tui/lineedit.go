package tui

import "unicode"

// LineEdit is one line of text with a cursor: the text input every field is built on. Cursor is a rune index, 0 to
// len(Text) inclusive.
type LineEdit struct {
	Text   []rune
	Cursor int
}

// Set replaces the text and puts the cursor at its end.
func (e *LineEdit) Set(s string) {
	e.Text = []rune(s)
	e.Cursor = len(e.Text)
}

func (e *LineEdit) String() string { return string(e.Text) }

// Handle applies an editing key and reports whether k was one, whether or not it changed anything. Up, Down, Enter,
// Escape and Tab are not editing keys: the caller owns those.
func (e *LineEdit) Handle(k Key) bool {
	e.clamp()

	switch k.Kind {
	case KeyRune:
		e.Text = append(e.Text[:e.Cursor], append([]rune{k.Rune}, e.Text[e.Cursor:]...)...)
		e.Cursor++
	case KeyBackspace:
		if e.Cursor > 0 {
			e.remove(e.Cursor-1, e.Cursor)
		}
	case KeyDelete:
		if e.Cursor < len(e.Text) {
			e.remove(e.Cursor, e.Cursor+1)
		}
	case KeyLeft:
		if k.Mod&(ModCtrl|ModAlt) != 0 {
			e.Cursor = e.wordStart()
		} else if e.Cursor > 0 {
			e.Cursor--
		}
	case KeyRight:
		if k.Mod&(ModCtrl|ModAlt) != 0 {
			e.Cursor = e.wordEnd()
		} else if e.Cursor < len(e.Text) {
			e.Cursor++
		}
	case KeyHome:
		e.Cursor = 0
	case KeyEnd:
		e.Cursor = len(e.Text)
	case KeyCtrl:
		// ctrl-w and ctrl-u are the line editor's; any other ctrl-letter is a shortcut the caller checks first.
		switch k.Rune {
		case 'w':
			e.remove(e.wordStart(), e.Cursor)
		case 'u':
			e.remove(0, e.Cursor)
		default:
			return false
		}
	default:
		return false
	}

	return true
}

// View returns the runes that fit in width columns with the cursor among them, scrolled so it always shows, and the
// caret's column within them.
func (e *LineEdit) View(width int) (string, int) {
	from, to, caret := e.window(width)

	return string(e.Text[from:to]), caret
}

// Locate is the rune index a click on column col of the View drawn for width means, so a caret placed by mouse lands
// where the text was drawn rather than where the text begins. A click past the end of the drawn text means its end.
func (e *LineEdit) Locate(width, col int) int {
	from, to, _ := e.window(width)

	used := 0
	for i := from; i < to; i++ {
		cells := RuneCells(e.Text[i])
		// The caret goes before the rune the column is on, as it does in every text field: clicking a character puts
		// the caret in front of it.
		if col < used+cells {
			return i
		}
		used += cells
	}

	return to
}

// window is the slice of the text one View shows: from..to with the caret column of Cursor within it.
func (e *LineEdit) window(width int) (from, to, caret int) {
	e.clamp()
	if width <= 0 {
		return 0, 0, 0
	}

	// The cursor sits on the rune after it, or on a column of its own past the last one.
	under := 1
	if e.Cursor < len(e.Text) {
		under = RuneCells(e.Text[e.Cursor])
	}

	from, caret = 0, Cells(string(e.Text[:e.Cursor]))
	for from < e.Cursor && caret+under > width {
		caret -= RuneCells(e.Text[from])
		from++
	}

	to = e.Cursor
	for used := caret; to < len(e.Text) && used+RuneCells(e.Text[to]) <= width; to++ {
		used += RuneCells(e.Text[to])
	}

	return from, to, caret
}

func (e *LineEdit) remove(from, to int) {
	e.Text = append(e.Text[:from], e.Text[to:]...)
	e.Cursor = from
}

// clamp keeps a cursor set by hand inside the text.
func (e *LineEdit) clamp() { e.Cursor = min(max(e.Cursor, 0), len(e.Text)) }

// A word is a run of letters and digits, as readline has it, so a branch name is several of them.
func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// wordStart is where the word before the cursor begins: the separators are crossed first, then the word.
func (e *LineEdit) wordStart() int {
	i := e.Cursor
	for i > 0 && !wordRune(e.Text[i-1]) {
		i--
	}
	for i > 0 && wordRune(e.Text[i-1]) {
		i--
	}

	return i
}

// wordEnd is where the word after the cursor ends.
func (e *LineEdit) wordEnd() int {
	i := e.Cursor
	for i < len(e.Text) && !wordRune(e.Text[i]) {
		i++
	}
	for i < len(e.Text) && wordRune(e.Text[i]) {
		i++
	}

	return i
}
