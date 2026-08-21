package tui

import "testing"

// Holding a lone ESC back for a sequence that never completes would leave Escape dead until the next keystroke.
func TestLoneEscapeIsTheEscapeKey(t *testing.T) {
	var dec Decoder

	keys := dec.Keys([]byte("\x1b"))

	if len(keys) != 1 || keys[0].Kind != KeyEsc {
		t.Fatalf("decoded %+v, want the escape key", keys)
	}
	if len(dec.pending) != 0 {
		t.Fatalf("pending = %q, want the escape key not held back", dec.pending)
	}
}

// A sequence cut in half by a chunk boundary used to decode as Escape plus the rest typed into the query.
func TestSplitEscapeSequencesAreFinished(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  KeyKind
	}{
		{"arrow", []string{"\x1b[", "A"}, KeyUp},
		{"arrow, introducer alone", []string{"\x1b[", "B"}, KeyDown},
		{"page", []string{"\x1b[5", "~"}, KeyPageUp},
		{"application cursor", []string{"\x1bO", "A"}, KeyUp},
		{"mouse report", []string{"\x1b[<0;12", ";5M"}, KeyMouse},
		{"mouse report, byte by byte past the introducer", []string{
			"\x1b[", "<", "0", ";", "1", "2", ";", "5", "M",
		}, KeyMouse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				dec  Decoder
				keys []Key
			)
			for _, part := range test.parts {
				keys = append(keys, dec.Keys([]byte(part))...)
			}

			if len(keys) != 1 || keys[0].Kind != test.want {
				t.Fatalf("decoded %+v, want one key of kind %d", keys, test.want)
			}
			if len(dec.pending) != 0 {
				t.Fatalf("pending = %q, want it consumed", dec.pending)
			}
		})
	}
}

// A carry-over that turns out not to be a known sequence is dropped, not held, so it cannot swallow later keystrokes.
func TestCarriedOverEscapeDoesNotStickAround(t *testing.T) {
	var dec Decoder

	keys := append(dec.Keys([]byte("\x1b[")), dec.Keys([]byte("qab"))...)

	if got := runes(keys); got != "ab" {
		t.Fatalf("decoded %q, want the unknown sequence dropped and ab left", got)
	}
	if len(dec.pending) != 0 {
		t.Fatalf("pending = %q, want it cleared", dec.pending)
	}
}

func TestCursorKeysCarryTheirModifiers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Key
	}{
		{"ctrl-left", "\x1b[1;5D", Key{Kind: KeyLeft, Mod: ModCtrl}},
		{"alt-right", "\x1b[1;3C", Key{Kind: KeyRight, Mod: ModAlt}},
		{"shift-up", "\x1b[1;2A", Key{Kind: KeyUp, Mod: ModShift}},
		{"ctrl-shift-down", "\x1b[1;6B", Key{Kind: KeyDown, Mod: ModCtrl | ModShift}},
		{"esc b is alt-left", "\x1bb", Key{Kind: KeyLeft, Mod: ModAlt}},
		{"esc f is alt-right", "\x1bf", Key{Kind: KeyRight, Mod: ModAlt}},
		{"home", "\x1b[H", Key{Kind: KeyHome}},
		{"home, application mode", "\x1bOH", Key{Kind: KeyHome}},
		{"home, vt", "\x1b[1~", Key{Kind: KeyHome}},
		{"home, rxvt", "\x1b[7~", Key{Kind: KeyHome}},
		{"ctrl-home", "\x1b[1;5H", Key{Kind: KeyHome, Mod: ModCtrl}},
		{"end", "\x1b[F", Key{Kind: KeyEnd}},
		{"end, application mode", "\x1bOF", Key{Kind: KeyEnd}},
		{"end, vt", "\x1b[4~", Key{Kind: KeyEnd}},
		{"end, rxvt", "\x1b[8~", Key{Kind: KeyEnd}},
		{"delete", "\x1b[3~", Key{Kind: KeyDelete}},
		{"ctrl-delete", "\x1b[3;5~", Key{Kind: KeyDelete, Mod: ModCtrl}},
		{"shift-page down", "\x1b[6;2~", Key{Kind: KeyPageDown, Mod: ModShift}},
		{"application cursor left", "\x1bOD", Key{Kind: KeyLeft}},
		{"ctrl-a is home", "\x01", Key{Kind: KeyHome}},
		{"ctrl-e is end", "\x05", Key{Kind: KeyEnd}},
		{"plain arrows carry no modifier", "\x1b[D", Key{Kind: KeyLeft}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var dec Decoder

			keys := dec.Keys([]byte(test.input))

			if len(keys) != 1 || keys[0] != test.want {
				t.Fatalf("decoded %+v, want %+v", keys, test.want)
			}
		})
	}
}

// A modified arrow split across reads must finish as the modified arrow, not as Escape and a typed 5D.
func TestSplitModifiedArrowIsFinished(t *testing.T) {
	var dec Decoder

	keys := append(dec.Keys([]byte("\x1b[1;")), dec.Keys([]byte("5D"))...)

	if len(keys) != 1 || keys[0] != (Key{Kind: KeyLeft, Mod: ModCtrl}) {
		t.Fatalf("decoded %+v, want ctrl-left", keys)
	}
}
