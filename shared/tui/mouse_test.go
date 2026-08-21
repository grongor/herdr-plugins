package tui

import "testing"

func TestParseMouse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Mouse
	}{
		{"left press", "\x1b[<0;12;5M", Mouse{Button: MouseLeft, Col: 12, Row: 5}},
		{"left release", "\x1b[<0;12;5m", Mouse{Button: MouseLeft, Col: 12, Row: 5, Release: true}},
		{"right press", "\x1b[<2;1;1M", Mouse{Button: MouseRight, Col: 1, Row: 1}},
		// Ctrl-left is still a left click; 16 is the control bit.
		{"modifiers are dropped", "\x1b[<16;3;4M", Mouse{Button: MouseLeft, Col: 3, Row: 4}},
		{"wheel", "\x1b[<65;9;9M", Mouse{Button: MouseWheelDown, Col: 9, Row: 9}},
		// 4 is the shift bit, 20 is shift and control together.
		{"shift press", "\x1b[<4;3;4M", Mouse{Button: MouseLeft, Col: 3, Row: 4, Shift: true}},
		{"shift right press", "\x1b[<6;3;4M", Mouse{Button: MouseRight, Col: 3, Row: 4, Shift: true}},
		{"shift wheel", "\x1b[<69;9;9M", Mouse{Button: MouseWheelDown, Col: 9, Row: 9, Shift: true}},
		{"shift and control", "\x1b[<20;3;4M", Mouse{Button: MouseLeft, Col: 3, Row: 4, Shift: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			keys := (&Decoder{}).Keys([]byte(test.in))

			if len(keys) != 1 {
				t.Fatalf("parsed %d keys, want 1", len(keys))
			}
			if keys[0].Kind != KeyMouse {
				t.Fatalf("kind = %d, want KeyMouse", keys[0].Kind)
			}
			if keys[0].Mouse != test.want {
				t.Fatalf("mouse = %+v, want %+v", keys[0].Mouse, test.want)
			}
		})
	}
}

// A byte of a report leaking out would be typed into the query, which is why the older encoding is swallowed too.
func TestParseMouseLeavesNoDebris(t *testing.T) {
	keys := (&Decoder{}).Keys([]byte("\x1b[M\x20\x2c\x25a"))

	if len(keys) != 1 || keys[0].Kind != KeyRune || keys[0].Rune != 'a' {
		t.Fatalf("parsed %+v, want the report swallowed and 'a' left", keys)
	}
}

// To is exclusive, so two entries drawn side by side cannot both claim a column.
func TestZonesAt(t *testing.T) {
	zones := Zones{
		{Key: Key{Kind: KeyEnter}, Line: 3, From: 0, To: 5},
		{Key: Key{Kind: KeyEsc}, Line: 3, From: 5, To: 10},
	}

	tests := []struct {
		name string
		m    Mouse
		want KeyKind
	}{
		// Screen coordinates are 1-based and zones are 0-based, hence the offset.
		{"the first entry", Mouse{Col: 1, Row: 4}, KeyEnter},
		{"its last column", Mouse{Col: 5, Row: 4}, KeyEnter},
		{"the second entry starts at the next", Mouse{Col: 6, Row: 4}, KeyEsc},
		{"past the end", Mouse{Col: 11, Row: 4}, KeyNone},
		{"another line", Mouse{Col: 1, Row: 3}, KeyNone},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			zone, ok := zones.At(test.m)
			if !ok {
				zone = Zone{}
			}
			if zone.Key.Kind != test.want {
				t.Fatalf("At(%+v) = %+v, want a key of kind %d", test.m, zone, test.want)
			}
		})
	}
}

// A footer entry's zone must be the columns its text actually occupies.
func TestHintLineIsClickableWhereItWasDrawn(t *testing.T) {
	items := []Hint{
		{"enter delete", Key{Kind: KeyEnter}},
		{"space toggle", Key{Kind: KeyRune, Rune: ' '}},
		{"esc cancel", Key{Kind: KeyEsc}},
	}

	sc := &Screen{}
	line, zones := sc.HintLine(items, 9, 80)

	if len(zones) != len(items) {
		t.Fatalf("drew %d zones for %d entries", len(zones), len(items))
	}

	for i, item := range items {
		if zones[i].Key != item.Key {
			t.Fatalf("zone %d stands for %+v, want %+v", i, zones[i].Key, item.Key)
		}
		if got := line[byteOffset(line, zones[i].From):]; !startsWith(got, item.Text) {
			t.Fatalf("zone %d points at %.20q, want it at %q", i, got, item.Text)
		}
	}
}

// byteOffset converts a column into an index into text; colours are off, so they differ only by multi-byte characters.
func byteOffset(text string, column int) int {
	used := 0
	for i, r := range text {
		if used == column {
			return i
		}
		used += RuneCells(r)
	}

	return len(text)
}

func startsWith(text, prefix string) bool {
	return len(text) >= len(prefix) && text[:len(prefix)] == prefix
}

// The separator used to be written before the entry following it, leaving a dangling " · " past the right edge that
// wrapped the footer out of view.
func TestHintLineFitsTheWidth(t *testing.T) {
	items := []Hint{
		{"enter delete", Key{Kind: KeyEnter}},
		{"space toggle", Key{Kind: KeyRune, Rune: ' '}},
		{"esc cancel", Key{Kind: KeyEsc}},
	}

	sc := &Screen{}
	for width := range 60 {
		line, zones := sc.HintLine(items, 0, width)

		if got := Cells(line); got > width {
			t.Errorf("at width %d the footer is %d columns: %q", width, got, line)
		}
		for i, zone := range zones {
			if zone.To > width {
				t.Errorf("at width %d zone %d reaches column %d", width, i, zone.To)
			}
		}
	}
}
