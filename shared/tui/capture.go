package tui

import (
	"regexp"
	"strings"
)

// cursorSequence is what a frame ends with: the cursor shown and positioned, or hidden.
var cursorSequence = regexp.MustCompile(`(?:\x1b\[\?25h\x1b\[\d+;\d+H|\x1b\[\?25l)$`)

// Capture renders one frame of app at width×height and returns its lines, escape sequences and all. Colours are off, so
// a captured line is the visible one with nothing to discount.
func Capture(app App, width, height int) []string {
	var out strings.Builder

	app.Render(&Screen{out: &out, width: width, height: height})

	frame := strings.TrimPrefix(out.String(), "\x1b[H")
	frame = cursorSequence.ReplaceAllString(frame, "")

	lines := strings.Split(frame, "\r\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "\x1b[K")
	}

	return lines
}
