// Package ui draws the "delete worktree checkout?" dialog with its delete-the-branch checkbox.
package ui

import (
	"errors"
	"strings"

	"github.com/grongor/herdr-plugins/shared/tui"
)

const (
	title      = "Delete worktree checkout?"
	markerOn   = "❯ "
	markerOff  = "  "
	checkedBox = "[x] "
	emptyBox   = "[ ] "

	branchLabel = "Delete the branch too"
	noBranch    = "(detached — no branch)"

	// gap separates the checkbox label from the branch it names.
	gap = "   "

	// errorLines caps how much of a failure is shown; git's messages repeat themselves, the first lines carry the reason.
	errorLines = 3
)

// consequences are in the order Herdr's own dialog lists them. The branch is left out: that is the checkbox's business.
var consequences = []string{
	"The checkout folder is deleted.",
	"The Herdr workspace will close.",
}

// dirtyConsequence is dimmed prose like the others until the checkout is known dirty; then it turns red and is never
// dropped for room.
const dirtyConsequence = "Dirty or untracked files will be permanently deleted."

// dirtyWarning replaces Herdr's refusal message, which talks about --force rather than what enter costs.
const dirtyWarning = "This checkout has uncommitted or untracked files. Deleting it loses them."

var hints = []tui.Hint{
	{Text: "enter delete", Key: tui.Key{Kind: tui.KeyEnter}},
	{Text: "space toggle", Key: tui.Key{Kind: tui.KeyRune, Rune: ' '}},
	{Text: "esc cancel", Key: tui.Key{Kind: tui.KeyEsc}},
}

// forceHints replace hints once Herdr has refused a dirty checkout.
var forceHints = []tui.Hint{
	{Text: "enter delete anyway", Key: tui.Key{Kind: tui.KeyEnter}},
	{Text: "space toggle", Key: tui.Key{Kind: tui.KeyRune, Rune: ' '}},
	{Text: "esc cancel", Key: tui.Key{Kind: tui.KeyEsc}},
}

var doneHints = []tui.Hint{
	{Text: "esc close", Key: tui.Key{Kind: tui.KeyEsc}},
}

// zoneCheckbox is the click zone kind for the checkbox; zero is the shared layer's "just the keystroke".
const zoneCheckbox = 1

// Target is the checkout the dialog is about.
type Target struct {
	Repo string
	// Branch is empty for a detached checkout.
	Branch   string
	Checkout string
	// Dirty says the checkout is known to have uncommitted or untracked files.
	Dirty bool
}

// Request is what the dialog commits to on enter.
type Request struct {
	DeleteBranch bool
	// Force asks Herdr to remove a checkout it has already refused once as dirty.
	Force bool
}

// ErrNeedsForce says Herdr refused a dirty checkout; the dialog turns it into a second, louder confirmation.
var ErrNeedsForce = errors.New("the checkout has uncommitted or untracked files")

// ErrRemoved says the checkout was deleted but the branch was not; the dialog shows the message and waits to be closed.
var ErrRemoved = errors.New("the checkout was removed")

// Actions are what the dialog calls when the user commits. An error is shown and the dialog stays open.
type Actions struct {
	Remove func(Request) error
}

// stage is how far the dialog has got, which decides what the footer and enter mean.
type stage int

const (
	stageConfirm stage = iota
	// stageForce is after Herdr refused a dirty checkout: enter deletes it anyway.
	stageForce
	// stageDone is after the checkout is gone.
	stageDone
)

// Run shows the dialog and blocks until the checkout is removed or the user cancels. Cancelling is not an error.
func Run(target Target, deleteBranch bool, actions Actions) error {
	return tui.Run(&dialog{target: target, deleteBranch: deleteBranch, actions: actions})
}

type dialog struct {
	target       Target
	actions      Actions
	deleteBranch bool
	stage        stage
	err          string

	// forceDrawn says the "delete anyway" frame has been rendered. tui.Run hands every key of one input chunk to Handle
	// without redrawing in between, so a held enter would otherwise force the removal on a frame never seen.
	forceDrawn bool

	zones tui.Zones
	// checkboxRow is the 1-based screen row of the checkbox, where the terminal cursor is parked.
	checkboxRow int
}

// hasBranch is false for a detached checkout; the box is then shown cleared and dimmed rather than hidden, so the
// dialog keeps its shape.
func (d *dialog) hasBranch() bool { return d.target.Branch != "" }

// canToggle is false once the checkout is gone; the box still shows what was asked, which the failure beneath is about.
func (d *dialog) canToggle() bool { return d.hasBranch() && d.stage != stageDone }

// Handle takes one keystroke and reports whether the dialog is finished.
func (d *dialog) Handle(k tui.Key) bool {
	if k.Kind == tui.KeyMouse {
		synthesised, ok := d.click(k.Mouse)
		if !ok {
			return false
		}
		k = synthesised
	}

	switch k.Kind {
	case tui.KeyEsc, tui.KeyCtrlC:
		return true
	case tui.KeyEnter:
		return d.commit()
	case tui.KeyRune:
		if k.Rune == ' ' || k.Rune == 'x' || k.Rune == 'X' {
			d.toggle()
		}
	}

	return false
}

func (d *dialog) toggle() {
	if !d.canToggle() {
		return
	}

	d.deleteBranch = !d.deleteBranch
	// The error has been read. The dirty warning is not in err and stays.
	d.err = ""
}

// commit runs the removal and reports whether the dialog is finished.
func (d *dialog) commit() bool {
	if d.stage == stageDone {
		return true
	}

	// An enter from the same input chunk as the refusal answers a question not yet on screen; drop it.
	if d.stage == stageForce && !d.forceDrawn {
		return false
	}

	err := d.actions.Remove(Request{DeleteBranch: d.deleteBranch, Force: d.stage == stageForce})
	switch {
	case err == nil:
		return true
	case errors.Is(err, ErrNeedsForce):
		d.stage, d.err = stageForce, ""
	case errors.Is(err, ErrRemoved):
		d.stage, d.err = stageDone, err.Error()
	default:
		d.err = err.Error()
	}

	return false
}

// click resolves a mouse report against the last frame; anything standing for a keystroke comes back as that keystroke.
func (d *dialog) click(m tui.Mouse) (tui.Key, bool) {
	// Shift+mouse is the terminal's, for selecting text; Herdr's own mouse bindings leave it alone too.
	if m.Shift || m.Release || m.Button != tui.MouseLeft {
		return tui.Key{}, false
	}

	target, ok := d.zones.At(m)
	if !ok {
		return tui.Key{}, false
	}
	if target.Kind == zoneCheckbox {
		d.toggle()

		return tui.Key{}, false
	}

	return target.Key, true
}

// Drop priorities for a pane too short for every line (Herdr sizes a popup off the terminal). Dropped in descending
// order: spacing, then chrome, then prose; keep is never dropped.
const (
	keep = iota
	dropConsequences
	dropSubtitle
	dropDivider
	dropBlank
)

type line struct {
	text string
	drop int
}

// Render draws the dialog over the whole pane.
func (d *dialog) Render(sc *tui.Screen) {
	width, height := sc.Width(), sc.Height()
	d.zones = d.zones[:0]

	lines := []line{
		{sc.Accent(tui.Truncate(title, width)), keep},
		{sc.Dim(tui.Truncate(d.subtitle(), width)), dropSubtitle},
		{sc.Dim(strings.Repeat("─", width)), dropDivider},
		{"", dropBlank},
		// From the left: the end of the path is what tells two checkouts apart.
		{tui.TruncateLeft(d.target.Checkout, width), keep},
		{"", dropBlank},
	}
	for _, text := range consequences {
		indented := markerOff + tui.Truncate(text, max(width-tui.Cells(markerOff), 0))
		lines = append(lines, line{sc.Dim(indented), dropConsequences})
	}
	dirtyLine := markerOff + tui.Truncate(dirtyConsequence, max(width-tui.Cells(markerOff), 0))
	if d.target.Dirty {
		lines = append(lines, line{sc.Alert(dirtyLine), keep})
	} else {
		lines = append(lines, line{sc.Dim(dirtyLine), dropConsequences})
	}
	lines = append(lines, line{"", dropBlank})

	// Filled in after fit, once its row is known; clicks and the terminal cursor resolve against that row.
	checkboxAt := len(lines)
	lines = append(lines, line{"", keep})

	if warning := d.warning(); warning != "" {
		lines = append(lines, line{"", dropBlank})
		for _, text := range tui.Wrap(warning, width, errorLines) {
			lines = append(lines, line{sc.Alert(text), keep})
		}
	}

	body := height - 1
	drawn, checkboxAt := fit(lines, checkboxAt, body)

	checkbox, zone := d.checkboxLine(sc, checkboxAt, width)
	d.checkboxRow = checkboxAt + 1
	drawn[checkboxAt] = checkbox
	d.zones = append(d.zones, zone)

	frame := sc.Frame()
	for i := range body {
		text := ""
		if i < len(drawn) {
			text = drawn[i]
		}
		frame.Line(text)
	}

	hint, zones := sc.HintLine(d.hints(), height-1, width)
	frame.Bottom(hint)
	d.zones = append(d.zones, zones...)

	frame.Cursor(d.checkboxRow, tui.Cells(markerOn)+1)
	frame.Flush()

	// Only now is the escalated question on screen, which arms enter.
	d.forceDrawn = d.stage == stageForce
}

// fit drops the least valuable lines until at most rows remain, and reports where the line at anchor ended up. Lines
// marked keep are never dropped, so a pane too short even for those overflows.
func fit(lines []line, anchor, rows int) ([]string, int) {
	dropped := make(map[int]bool)
	for drop := dropBlank; drop > keep && len(lines)-len(dropped) > rows; drop-- {
		for i, l := range lines {
			if l.drop == drop {
				dropped[i] = true
			}
		}
	}

	drawn := make([]string, 0, len(lines))
	moved := anchor
	for i, l := range lines {
		if dropped[i] {
			if i < anchor {
				moved--
			}

			continue
		}
		drawn = append(drawn, l.text)
	}

	return drawn, moved
}

func (d *dialog) subtitle() string {
	if d.target.Branch == "" {
		return d.target.Repo
	}

	return d.target.Repo + " — " + d.target.Branch
}

func (d *dialog) checkboxLine(sc *tui.Screen, line, width int) (string, tui.Zone) {
	box := emptyBox
	if d.deleteBranch && d.hasBranch() {
		box = checkedBox
	}

	marker := markerOn
	if !d.canToggle() {
		marker = markerOff
	}

	detail := d.target.Branch
	if !d.hasBranch() {
		detail = noBranch
	}

	start := tui.Cells(marker)
	labelWidth, detailWidth := tui.Columns(
		max(width-start-tui.Cells(box)-tui.Cells(gap), 0),
		tui.Cells(branchLabel),
		tui.Cells(detail),
	)

	label := tui.Truncate(branchLabel, labelWidth)
	text := box + label
	// A branch name truncates from the left, where its tail tells two apart; the stand-in is prose.
	shown := tui.Truncate(detail, detailWidth)
	if d.hasBranch() {
		shown = tui.TruncateLeft(detail, detailWidth)
	}
	if shown != "" {
		text += gap + shown
	}

	zone := tui.Zone{Kind: zoneCheckbox, Line: line, From: start, To: start + tui.Cells(text)}

	if !d.canToggle() {
		return markerOff + sc.Dim(text), zone
	}

	return sc.Accent(marker) + text, zone
}

// warning is the red text: the second confirmation, or a failure.
func (d *dialog) warning() string {
	if d.stage == stageForce {
		return dirtyWarning
	}

	return d.err
}

func (d *dialog) hints() []tui.Hint {
	switch d.stage {
	case stageForce:
		return forceHints
	case stageDone:
		return doneHints
	default:
		return hints
	}
}
