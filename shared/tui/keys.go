package tui

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// KeyKind names one decoded keystroke. Hand-decoded and deliberately small: every entry is a key some plugin binds.
type KeyKind int

const (
	KeyNone KeyKind = iota
	KeyRune
	KeyUp
	KeyDown
	KeyPageUp
	KeyPageDown
	KeyEnter
	KeyBackspace
	KeyEsc
	// KeyCtrlC is ctrl-c and ctrl-d: the cancel that works everywhere, so no shortcut can take it.
	KeyCtrlC
	// KeyCtrl is any other ctrl-letter, with Rune the lower-case letter; KeyAlt is alt with any printable ASCII rune.
	// These are what the configurable shortcuts are made of.
	KeyCtrl
	KeyAlt
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyDelete
	KeyTab
	KeyShiftTab
	KeyMouse
)

// Mod is the modifiers held with a cursor key, decoded from the xterm "1;<n>" parameter (n = 1 + the bits below).
type Mod int

const (
	ModShift Mod = 1 << iota
	ModAlt
	ModCtrl
)

// Key is one keystroke. Rune is set for KeyRune, KeyCtrl and KeyAlt and Mouse for KeyMouse; both are zero otherwise.
// Mod is only ever set on arrows, Home, End, Delete and the page keys.
type Key struct {
	Kind  KeyKind
	Rune  rune
	Mod   Mod
	Mouse Mouse
}

// Ctrl is the ctrl-letter key.
func Ctrl(letter rune) Key { return Key{Kind: KeyCtrl, Rune: letter} }

// Alt is the alt-character key.
func Alt(r rune) Key { return Key{Kind: KeyAlt, Rune: r} }

// Is reports whether k is the keystroke other names; a Key with modifiers or a mouse report never is.
func (k Key) Is(other Key) bool { return k.Kind == other.Kind && k.Rune == other.Rune && k.Mod == 0 }

// String spells a key the way the footer hints and the config do: "ctrl-r", "alt-x", "enter".
func (k Key) String() string {
	switch k.Kind {
	case KeyCtrl:
		return "ctrl-" + string(k.Rune)
	case KeyAlt:
		return "alt-" + string(k.Rune)
	case KeyCtrlC:
		return "ctrl-c"
	case KeyEnter:
		return "enter"
	case KeyEsc:
		return "esc"
	case KeyTab:
		return "tab"
	case KeyRune:
		if k.Rune == ' ' {
			return "space"
		}

		return string(k.Rune)
	}

	return "?"
}

// reservedCtrl are the ctrl-letters that mean something else at the terminal or in every field here, so a shortcut
// cannot be put on them. The value says what the user would be taking away.
var reservedCtrl = map[rune]string{
	'a': "home",
	'e': "end",
	'c': "cancel",
	'd': "cancel",
	'h': "backspace",
	'i': "tab",
	'j': "enter",
	'm': "enter",
	'n': "down",
	'p': "up",
}

// ParseKey reads a shortcut from the config: "ctrl-<letter>" or "alt-<character>", case-insensitive, "+" allowed for
// "-". Letters the terminal or the line editor already own are refused with what they do.
func ParseKey(text string) (Key, error) {
	spelling := strings.ToLower(strings.TrimSpace(text))
	modifier, rest, ok := strings.Cut(strings.NewReplacer("+", "-").Replace(spelling), "-")
	if !ok || utf8.RuneCountInString(rest) != 1 {
		return Key{}, fmt.Errorf("%q is not a shortcut such as \"ctrl-r\" or \"alt-x\"", text)
	}
	r, _ := utf8.DecodeRuneInString(rest)

	switch modifier {
	case "ctrl":
		if r < 'a' || r > 'z' {
			return Key{}, fmt.Errorf("%q: ctrl works with a letter", text)
		}
		if taken, reserved := reservedCtrl[r]; reserved {
			return Key{}, fmt.Errorf("%q is %s and cannot be a shortcut", text, taken)
		}

		return Ctrl(r), nil
	case "alt":
		if r <= ' ' || r > '~' {
			return Key{}, fmt.Errorf("%q: alt works with a printable ASCII character", text)
		}

		return Alt(r), nil
	}

	return Key{}, fmt.Errorf("%q is not a shortcut such as \"ctrl-r\" or \"alt-x\"", text)
}

// Mouse is one mouse report. Coordinates are 1-based terminal cells, as the terminal sends them.
type Mouse struct {
	Button int
	Col    int
	Row    int
	// Release says the button came back up rather than went down.
	Release bool
	// Shift is the one modifier kept: Shift+mouse has to be left alone rather than acted on.
	Shift bool
}

// Buttons, after the modifier bits have been masked off.
const (
	MouseLeft      = 0
	MouseMiddle    = 1
	MouseRight     = 2
	MouseWheelUp   = 64
	MouseWheelDown = 65

	// mouseModifiers are the shift, meta and control bits; mouseShift is the shift bit within them.
	mouseModifiers = 0x1c
	mouseShift     = 0x04
)

// The delimiters a terminal wraps pasted text in once bracketed paste is on.
const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// Decoder turns terminal input into keystrokes. A bracketed paste routinely outruns a single read, so where it ends has
// to be remembered across chunks.
type Decoder struct {
	pasting bool
	paste   []byte
	// pending holds an escape sequence the end of a chunk cut in half. A lone ESC is never held: it means the Escape key
	// and has to act now.
	pending []byte
}

// Keys decodes a chunk of terminal input. A terminal may split a write anywhere, so a sequence the chunk ended part-way
// through is carried over and finished when the rest arrives; a lone ESC byte means the Escape key.
func (d *Decoder) Keys(buf []byte) []Key {
	keys := make([]Key, 0, len(buf))

	if len(d.pending) > 0 {
		buf, d.pending = append(d.pending, buf...), nil
	}

	for len(buf) > 0 {
		if incompleteEscape(buf) {
			d.pending = append(d.pending, buf...)

			return keys
		}

		if d.pasting {
			rest, closed := d.collect(buf)
			if !closed {
				return keys
			}
			keys = append(keys, d.pasted()...)
			buf = rest

			continue
		}

		// Checked ahead of the escape branch, which would otherwise consume the introducer as an unrecognised sequence
		// and hand the pasted text to the ordinary key path, where a trailing newline reads as Enter.
		if rest, ok := bytes.CutPrefix(buf, []byte(pasteStart)); ok {
			d.pasting = true
			buf = rest

			continue
		}

		b := buf[0]

		if b == 0x1b {
			k, size := parseEscape(buf)
			if k.Kind != KeyNone {
				keys = append(keys, k)
			}
			buf = buf[size:]

			continue
		}

		if k, ok := parseControl(b); ok {
			keys = append(keys, k)
			buf = buf[1:]

			continue
		}

		r, size := utf8.DecodeRune(buf)
		if r == utf8.RuneError && size <= 1 {
			buf = buf[1:]

			continue
		}
		if r >= 0x20 && r != 0x7f {
			keys = append(keys, Key{Kind: KeyRune, Rune: r})
		}
		buf = buf[size:]
	}

	return keys
}

// collect appends a chunk to the open paste and reports whether the paste ended within it, returning what followed
// the closing delimiter. The search runs over everything accumulated because the delimiter itself can be split across
// reads.
func (d *Decoder) collect(buf []byte) ([]byte, bool) {
	d.paste = append(d.paste, buf...)

	end := bytes.Index(d.paste, []byte(pasteEnd))
	if end < 0 {
		return nil, false
	}

	rest := d.paste[end+len(pasteEnd):]
	d.paste = d.paste[:end]

	return rest, true
}

// pasted turns a completed paste into keystrokes and clears the state. Newlines and tabs are dropped rather than
// becoming Enter or Tab: a pasted branch name with a trailing newline must fill the field, not submit the dialog. The
// text is decoded as UTF-8 in one pass because a multi-byte character may have been split across reads.
func (d *Decoder) pasted() []Key {
	text := string(d.paste)
	d.paste, d.pasting = nil, false

	keys := make([]Key, 0, len(text))
	for _, r := range text {
		if r == utf8.RuneError || r < 0x20 || r == 0x7f {
			continue
		}
		keys = append(keys, Key{Kind: KeyRune, Rune: r})
	}

	return keys
}

// incompleteEscape reports whether the chunk ends part-way through a CSI or SS3 sequence (every byte after the
// introducer is still a parameter byte). A chunk holding nothing but ESC is not incomplete: that is the Escape key.
func incompleteEscape(buf []byte) bool {
	if len(buf) < 2 || buf[0] != 0x1b || (buf[1] != '[' && buf[1] != 'O') {
		return false
	}

	for _, b := range buf[2:] {
		if b < 0x30 || b > 0x3f {
			return false
		}
	}

	return true
}

func parseControl(b byte) (Key, bool) {
	switch b {
	case '\r', '\n':
		return Key{Kind: KeyEnter}, true
	case 0x7f, 0x08:
		return Key{Kind: KeyBackspace}, true
	case 0x03, 0x04: // ctrl-c, ctrl-d
		return Key{Kind: KeyCtrlC}, true
	case 0x0e: // ctrl-n
		return Key{Kind: KeyDown}, true
	case 0x10: // ctrl-p
		return Key{Kind: KeyUp}, true
	case 0x01: // ctrl-a
		return Key{Kind: KeyHome}, true
	case 0x05: // ctrl-e
		return Key{Kind: KeyEnd}, true
	case 0x09: // tab
		return Key{Kind: KeyTab}, true
	}

	// Every other ctrl-letter: 0x01 is ctrl-a. ESC (0x1b) went to parseEscape before this.
	if b >= 0x01 && b <= 0x1a {
		return Ctrl(rune('a' + b - 1)), true
	}

	return Key{}, false
}

// parseEscape decodes one CSI or SS3 sequence and reports how many bytes it consumed. ESC b / ESC f are what some
// terminals send for alt-arrows; ESC and any other printable character in one read is alt with that character.
func parseEscape(buf []byte) (Key, int) {
	if len(buf) >= 2 {
		switch b := buf[1]; {
		case b == 'b':
			return Key{Kind: KeyLeft, Mod: ModAlt}, 2
		case b == 'f':
			return Key{Kind: KeyRight, Mod: ModAlt}, 2
		case b != '[' && b != 'O' && b > ' ' && b <= '~':
			return Alt(rune(b)), 2
		}
	}
	if len(buf) < 3 || (buf[1] != '[' && buf[1] != 'O') {
		return Key{Kind: KeyEsc}, 1
	}

	end := 2
	for end < len(buf) && buf[end] >= 0x30 && buf[end] <= 0x3f {
		end++
	}
	if end >= len(buf) {
		return Key{Kind: KeyEsc}, 1
	}

	params := string(buf[2:end])
	size := end + 1

	if final := buf[end]; final == 'M' || final == 'm' {
		if report, ok := strings.CutPrefix(params, "<"); ok {
			return sgrMouse(report, final == 'm'), size
		}
		// A terminal that ignored the SGR request answers in the older encoding, whose three biased bytes sit past the
		// final byte. Nothing decodes them — every terminal Herdr runs in speaks SGR — but they must be swallowed rather
		// than typed into the query.
		if final == 'M' && params == "" {
			return Key{Kind: KeyNone}, min(size+3, len(buf))
		}

		return Key{Kind: KeyNone}, size
	}

	number, mod := splitParams(params)
	k := Key{Mod: mod}

	switch buf[end] {
	case 'A':
		k.Kind = KeyUp
	case 'B':
		k.Kind = KeyDown
	case 'C':
		k.Kind = KeyRight
	case 'D':
		k.Kind = KeyLeft
	case 'H':
		k.Kind = KeyHome
	case 'F':
		k.Kind = KeyEnd
	case 'Z':
		k.Kind = KeyShiftTab
	case '~':
		switch number {
		case "1", "7":
			k.Kind = KeyHome
		case "4", "8":
			k.Kind = KeyEnd
		case "3":
			k.Kind = KeyDelete
		case "5":
			k.Kind = KeyPageUp
		case "6":
			k.Kind = KeyPageDown
		}
	}

	if k.Kind == KeyNone {
		return Key{Kind: KeyNone}, size
	}

	return k, size
}

// splitParams separates "1;5" into the key number and the modifier it was held with; either may be absent.
func splitParams(params string) (string, Mod) {
	number, modifier, _ := strings.Cut(params, ";")

	n, err := strconv.Atoi(modifier)
	if err != nil || n < 2 {
		return number, 0
	}

	return number, Mod(n - 1)
}

// sgrMouse decodes "<button;col;row" from an SGR report; the final byte says whether the button went down or up.
func sgrMouse(report string, release bool) Key {
	fields := strings.Split(report, ";")
	if len(fields) != 3 {
		return Key{Kind: KeyNone}
	}

	numbers := [3]int{}
	for i, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil {
			return Key{Kind: KeyNone}
		}
		numbers[i] = value
	}

	return Key{Kind: KeyMouse, Mouse: Mouse{
		Button:  numbers[0] &^ mouseModifiers,
		Col:     numbers[1],
		Row:     numbers[2],
		Release: release,
		Shift:   numbers[0]&mouseShift != 0,
	}}
}
