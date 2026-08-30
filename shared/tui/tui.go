// Package tui is the terminal layer the plugins' popups draw with: raw mode, input decoding, mouse reports and a
// full-screen repaint per frame. Hand-rolled because an empty bubbletea program builds larger than a whole plugin here
// and pulls in thirty-one more modules, on plugins that compile from source on the user's machine.
package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"
)

// Fallback size, used only when the ioctl fails — which inside a Herdr pane means something is badly wrong already.
const (
	fallbackWidth  = 80
	fallbackHeight = 24

	// Below minWidth×minHeight the fallback is more likely to be right than the measurement.
	minWidth  = 20
	minHeight = 4
)

// App is one plugin's user interface. Run owns the terminal and calls these.
type App interface {
	// Render draws one whole frame at the size measured for it.
	Render(*Screen)
	// Handle takes one keystroke and reports whether the app is finished, which ends Run.
	Handle(Key) bool
}

// Waker is an App whose screen can change without a keystroke: Run repaints every time Wake yields. A nil channel means
// the app never repaints on its own.
type Waker interface {
	App
	// Wake yields once per wanted repaint. Sends must not block: give it a buffer of one and drop sends that would block.
	Wake() <-chan struct{}
}

// Run shows app and blocks until it says it is done or its input ends. Everything the terminal is put into is undone
// before returning, including on a signal: a pane closed while a popup is up sends SIGHUP, which runs no deferred
// function and would otherwise leave the host terminal in raw mode with mouse reporting on.
func Run(app App) error {
	tty, closeTTY, err := openTTY()
	if err != nil {
		return err
	}

	fd := int(tty.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		closeTTY()

		return fmt.Errorf("enter raw mode: %w", err)
	}

	// Alternate screen keeps the pane's previous contents intact underneath.
	_, _ = tty.WriteString("\x1b[?1049h")

	// Button reporting in the SGR encoding, so a click past column 223 still reports its column.
	_, _ = tty.WriteString("\x1b[?1000h\x1b[?1006h")

	// Bracketed paste: without it a pasted newline is indistinguishable from Enter.
	_, _ = tty.WriteString("\x1b[?2004h")

	// Undoes all of it in reverse order. Three paths reach it: an ordinary return, a panic and a signal.
	teardown := sync.OnceFunc(func() {
		// A frame may have hidden the cursor; the host pane gets it back whatever the last frame said.
		_, _ = tty.WriteString("\x1b[?25h")
		_, _ = tty.WriteString("\x1b[?2004l")
		_, _ = tty.WriteString("\x1b[?1006l\x1b[?1000l")
		_, _ = tty.WriteString("\x1b[?1049l")
		_ = term.Restore(fd, state)
		closeTTY()
	})
	defer teardown()

	stopSignals := onTerminate(teardown)
	defer stopSignals()

	screen := &Screen{out: tty, tty: tty, colors: colorsEnabled()}

	input := make(chan []byte, 8)
	go readInput(tty, input)

	// One decoder for the run: a paste split across reads is held open on it.
	keys := &Decoder{}

	resized, stopResize := watchResize()
	defer stopResize()

	// A receive on a nil channel blocks forever, so the select is the same whether or not the app repaints on its own.
	var woken <-chan struct{}
	if waker, ok := app.(Waker); ok {
		woken = waker.Wake()
	}

	for {
		screen.measure()
		app.Render(screen)

		select {
		case <-resized:
			continue
		case <-woken:
			continue
		case chunk, ok := <-input:
			if !ok {
				return nil
			}
			for _, k := range keys.Keys(chunk) {
				if app.Handle(k) {
					return nil
				}
			}
		}
	}
}

func openTTY() (*os.File, func(), error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		return tty, func() { _ = tty.Close() }, nil
	}

	// Some sandboxes and Windows terminals present a pane with no controlling terminal device.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, nil, fmt.Errorf("no terminal available: %w", err)
	}

	return os.Stdin, func() {}, nil
}

func readInput(tty *os.File, out chan<- []byte) {
	defer close(out)

	buf := make([]byte, 256)
	for {
		n, err := tty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			out <- chunk
		}
		if err != nil {
			return
		}
	}
}

func colorsEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	return os.Getenv("TERM") != "dumb"
}

// Screen draws to the terminal. Its size is measured once per frame, so no part of a frame's layout disagrees with the
// rest. The zero value draws to nothing with colours off, which is what the rendering tests capture.
type Screen struct {
	// out is where frames go; nil discards them.
	out io.Writer
	// tty is the terminal to measure; nil outside a real run.
	tty    *os.File
	colors bool
	width  int
	height int
}

func (sc *Screen) Width() int  { return sc.width }
func (sc *Screen) Height() int { return sc.height }

// measure reads the terminal size. term.GetSize on our own tty is the only source: a popup pane has no pane id, so
// Herdr's pane.layout cannot be asked, and the manifest's declared size counts the border.
func (sc *Screen) measure() {
	if sc.tty == nil {
		return
	}

	width, height, err := term.GetSize(int(sc.tty.Fd()))
	if err != nil || width < minWidth || height < minHeight {
		width, height = fallbackWidth, fallbackHeight
	}

	sc.width, sc.height = width, height
}

// Frame accumulates a whole repaint and writes it in one call to the terminal.
type Frame struct {
	sc *Screen
	b  strings.Builder
}

// Frame starts a repaint with the cursor homed.
func (sc *Screen) Frame() *Frame {
	f := &Frame{sc: sc}
	f.b.WriteString("\x1b[H")

	return f
}

// Line writes one line and moves to the next. The line is cleared before writing, not after: a line that fills the
// terminal exactly leaves the cursor in the pending-wrap state, where a trailing erase-to-end-of-line would wipe the
// last character. Rewriting whole lines behind an erase is also why no spacer cell of a double-width character has to
// be blanked: no half of one survives a frame, and truncation never emits one straddling the right edge.
func (f *Frame) Line(content string) {
	f.b.WriteString("\x1b[K")
	f.b.WriteString(content)
	f.b.WriteString("\r\n")
}

// Bottom writes the last row, which must not end in a newline: one on the bottom row scrolls the screen.
func (f *Frame) Bottom(content string) {
	f.b.WriteString("\x1b[K")
	f.b.WriteString(content)
}

// Cursor shows the terminal cursor and parks it at a 1-based row and column. Every frame ends in Cursor or HideCursor,
// so the state never carries over from the previous surface.
func (f *Frame) Cursor(row, col int) {
	fmt.Fprintf(&f.b, "\x1b[?25h\x1b[%d;%dH", row, col)
}

// HideCursor is for a surface with nothing to type into, where a parked cursor would read as an input.
func (f *Frame) HideCursor() {
	f.b.WriteString("\x1b[?25l")
}

func (f *Frame) Flush() {
	if f.sc.out == nil {
		return
	}

	_, _ = io.WriteString(f.sc.out, f.b.String())
}

// Colours come from the terminal's own 16, never the 256-colour cube or RGB: Herdr exposes no way to read the theme's
// palette, so naming the slots the theme defines is the only way to sit inside it.

// Code emits a raw SGR sequence, or nothing when colours are off.
func (sc *Screen) Code(seq string) string {
	if !sc.colors {
		return ""
	}

	return seq
}

// Dim renders secondary text faint.
func (sc *Screen) Dim(text string) string {
	if text == "" || !sc.colors {
		return text
	}

	return "\x1b[2m" + text + "\x1b[22m"
}

// DimIf leaves the text alone where faint would work against it: a terminal may render faint by blending towards the
// background, which on a reverse-video row is the text colour — far enough to be unreadable.
func (sc *Screen) DimIf(text string, dim bool) string {
	if !dim {
		return text
	}

	return sc.Dim(text)
}

// Accent marks the one thing on screen the eye should land on first.
func (sc *Screen) Accent(text string) string {
	if !sc.colors {
		return text
	}

	return "\x1b[36m" + text + "\x1b[39m"
}

// AccentIf keeps a marker readable on a highlighted row, where the accent colour would fight the background.
func (sc *Screen) AccentIf(text string, accent bool) string {
	if !accent {
		return text
	}

	return sc.Accent(text)
}

// Alert marks a failure.
func (sc *Screen) Alert(text string) string {
	if !sc.colors {
		return text
	}

	return "\x1b[31m" + text + "\x1b[39m"
}

// Bold is the only emphasis available: a colour would be picked without knowing what the run is drawn against, which on
// a reversed row is the terminal's highlight colour.
func (sc *Screen) Bold(text string) string {
	return sc.Code("\x1b[1m") + text + sc.Code("\x1b[22m")
}

// Reverse draws text in the terminal's own highlight, so a cursor row follows the theme rather than a grey that suits
// half of them.
func (sc *Screen) Reverse(text string) string {
	return sc.Code("\x1b[7m") + text + sc.Code("\x1b[0m")
}
