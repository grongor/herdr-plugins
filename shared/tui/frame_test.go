package tui

import (
	"strings"
	"testing"
)

// A frame says whether the cursor shows: a choice field must not be drawn with one, and the next frame must bring it
// back without remembering the last.
func TestFrameStatesCursorVisibility(t *testing.T) {
	sc := &Screen{}

	shown := sc.Frame()
	shown.Cursor(2, 5)
	if got := shown.b.String(); !strings.Contains(got, "\x1b[?25h\x1b[2;5H") {
		t.Fatalf("frame with a cursor = %q, want it shown and parked", got)
	}

	hidden := sc.Frame()
	hidden.HideCursor()
	if got := hidden.b.String(); !strings.Contains(got, "\x1b[?25l") || strings.Contains(got, "?25h") {
		t.Fatalf("frame without a cursor = %q, want it hidden", got)
	}
}
