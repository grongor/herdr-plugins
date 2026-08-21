package ui

import (
	"slices"
	"strings"

	"github.com/grongor/herdr-plugins/shared/tui"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
	"github.com/grongor/herdr-plugins/workspace-picker/internal/worktree"
)

// Create is a worktree request; empty fields hand the decision to Herdr.
type Create struct {
	Repo *discover.Repo
	// Branch empty has Herdr generate a name.
	Branch string
	// Base empty branches from the repository's HEAD.
	Base string
	// Path empty lets Herdr choose the location.
	Path string
}

// Field order is tab order.
const (
	fieldBranch = iota
	fieldBase
	fieldLayout
	fieldPath
	fieldCount
)

var fieldLabels = [fieldCount]string{"Branch", "Base", "Layout", "Path"}

// labelWidth is the column every field value starts at.
const labelWidth = 8

// errorLines caps the failure shown; git's first lines carry the reason.
const errorLines = 3

// Layout has no placeholder: it is never empty.
var placeholders = [fieldCount]string{
	fieldBranch: "(Herdr generates a name)",
	fieldBase:   "(repository HEAD)",
	fieldPath:   "(Herdr chooses the path)",
}

// baseSwap names the key, since a branch name alone would not read as something to press.
const (
	modeSep  = " · "
	baseSwap = "   ←/→ "
	noteGap  = "   "
)

// form is the new-worktree dialog.
type form struct {
	repo *discover.Repo
	// layouts is built once so the layout the checkouts imply keeps its directory across a trip through the field.
	layouts []worktree.Layout
	mode    int
	// proposed is the mode the dialog opened on; only that one gets the note beside the layout names.
	proposed int
	bases    []string

	branch tui.LineEdit
	// base is one of bases, or empty when there are none; free text is not a choice.
	base string
	path tui.LineEdit
	// pathEdited keeps a hand-written path from being overwritten by a later proposal.
	pathEdited bool

	field int
	err   string
}

func newForm(row Row, detected worktree.Layout) *form {
	f := &form{repo: row.Repo, bases: baseChoices(row)}

	repo := layoutRepo(row.Repo)
	f.layouts = make([]worktree.Layout, len(worktree.Modes))
	for i, mode := range worktree.Modes {
		f.layouts[i] = worktree.Switch(repo, mode)
	}
	if i := slices.Index(worktree.Modes, detected.Mode); i >= 0 {
		f.layouts[i] = detected
		f.mode, f.proposed = i, i
	}

	if len(f.bases) > 0 {
		f.base = f.bases[0]
	}
	f.proposePath()

	return f
}

// baseChoices offers the checkout under the cursor first, the repository's HEAD second.
func baseChoices(row Row) []string {
	choices := make([]string, 0, 2)
	add := func(base string) {
		if base != "" && !slices.Contains(choices, base) {
			choices = append(choices, base)
		}
	}

	// The main checkout is the repository's HEAD, added below.
	if w := row.Worktree; w != nil && !w.Main {
		add(w.Branch)
		// A detached checkout has no branch, but its commit is still a base.
		add(w.Head)
	}
	add(row.Repo.Head)

	return choices
}

func (f *form) baseIndex() int { return slices.Index(f.bases, f.base) }

func (f *form) layout() worktree.Layout { return f.layouts[f.mode] }

func layoutRepo(repo *discover.Repo) worktree.Repo {
	linked := repo.Linked()

	checkouts := make([]worktree.Checkout, 0, len(linked))
	for _, checkout := range linked {
		checkouts = append(checkouts, worktree.Checkout{Path: checkout.Path, Branch: checkout.Branch})
	}

	return worktree.Repo{Anchor: repo.Anchor, Checkouts: checkouts}
}

func (f *form) proposePath() {
	if f.pathEdited {
		return
	}

	f.path.Set(f.layout().Path(f.branch.String()))
}

func (f *form) request() Create {
	return Create{
		Repo:   f.repo,
		Branch: strings.TrimSpace(f.branch.String()),
		Base:   f.base,
		Path:   strings.TrimSpace(f.path.String()),
	}
}

func (f *form) handle(k tui.Key) action {
	// Any keystroke means the error has been read.
	f.err = ""

	switch k.Kind {
	case tui.KeyEnter:
		return actionSelect
	case tui.KeyCtrlC:
		return actionQuit
	case tui.KeyEsc:
		return actionCancel
	case tui.KeyTab, tui.KeyDown:
		f.field = (f.field + 1) % fieldCount
	case tui.KeyShiftTab, tui.KeyUp:
		f.field = (f.field + fieldCount - 1) % fieldCount
	case tui.KeyLeft, tui.KeyRight:
		// In a text field the arrows move the caret; in a choice field they choose. No field is both.
		if !f.choice() {
			f.edit(k)

			break
		}
		if k.Kind == tui.KeyLeft {
			f.cycle(-1)
		} else {
			f.cycle(1)
		}
	case tui.KeyRune:
		// A choice field has nothing to type into, so space steps it.
		if f.choice() && k.Rune == ' ' {
			f.cycle(1)

			break
		}
		f.edit(k)
	default:
		f.edit(k)
	}

	return actionNone
}

// choice reports whether the focused field is chosen from rather than typed into.
func (f *form) choice() bool { return f.field == fieldBase || f.field == fieldLayout }

func (f *form) cycle(delta int) {
	switch f.field {
	case fieldLayout:
		f.setMode((f.mode + delta + len(worktree.Modes)) % len(worktree.Modes))
	case fieldBase:
		f.cycleBase(delta)
	}
}

func (f *form) setMode(mode int) {
	f.mode = mode
	// A different layout means its path, even after a hand edit.
	f.pathEdited = false
	f.proposePath()
}

func (f *form) cycleBase(delta int) {
	if len(f.bases) == 0 {
		return
	}

	index := (f.baseIndex() + delta + len(f.bases)) % len(f.bases)
	f.base = f.bases[index]
}

// editor is nil for the choice fields.
func (f *form) editor(field int) *tui.LineEdit {
	switch field {
	case fieldBranch:
		return &f.branch
	case fieldPath:
		return &f.path
	}

	return nil
}

// edit hands an editing key to the focused text field; other keys, and keys on a choice field, do nothing.
func (f *form) edit(k tui.Key) {
	e := f.editor(f.field)
	if e == nil || !e.Handle(k) {
		return
	}

	switch f.field {
	case fieldBranch:
		f.proposePath()
	case fieldPath:
		f.pathEdited = true
	}
}

// value reports whether the text is a placeholder for an empty field.
func (f *form) value(field int) (string, bool) {
	text := ""
	switch field {
	case fieldBase:
		text = f.base
	case fieldLayout:
		return "", false
	default:
		text = f.editor(field).String()
	}
	if text == "" {
		return placeholders[field], true
	}

	return text, false
}

// renderForm replaces the list rather than overlaying it: the pane is a popup already.
func (s *pickerState) renderForm(sc *tui.Screen, width, height int) {
	f := s.form
	s.zones = s.zones[:0]

	// An array so that adding a header line fails to compile unless formFieldsTop moves with it.
	header := [formFieldsTop]string{
		sc.Accent(tui.Truncate("New worktree", width)),
		sc.Dim(tui.Truncate(f.repo.Name, width)),
		sc.Dim(strings.Repeat("─", width)),
		"",
	}

	lines := slices.Clone(header[:])
	for field := range fieldCount {
		line, zones := fieldLine(sc, f, field, formFieldsTop+field, width)
		lines = append(lines, line)
		s.zones = append(s.zones, zones...)
	}

	if f.err != "" {
		lines = append(lines, "")
		for _, line := range tui.Wrap(f.err, width, errorLines) {
			lines = append(lines, sc.Alert(line))
		}
	}

	frame := sc.Frame()

	for i := range height - 1 {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		frame.Line(line)
	}

	hint, zones := sc.HintLine(f.hints(), height-1, width)
	frame.Bottom(hint)
	s.zones = append(s.zones, zones...)

	// A choice field has nothing to type into, so no cursor is drawn there.
	if f.choice() {
		frame.HideCursor()
	} else {
		frame.Cursor(formFieldsTop+f.field+1, min(f.caret(width), width))
	}
	frame.Flush()
}

// hints names ←/→ only where it chooses; in a text field it moves the caret, which needs no telling.
func (f *form) hints() []tui.Hint {
	hints := []tui.Hint{
		{Text: "enter create", Key: tui.Key{Kind: tui.KeyEnter}},
		{Text: "tab field", Key: tui.Key{Kind: tui.KeyTab}},
	}
	if f.choice() {
		hints = append(hints, tui.Hint{Text: "←/→ choose", Key: tui.Key{Kind: tui.KeyRight}})
	}

	return append(hints, tui.Hint{Text: "esc back", Key: tui.Key{Kind: tui.KeyEsc}})
}

func fieldLine(sc *tui.Screen, f *form, field, line, width int) (string, tui.Zones) {
	marker := markerOff
	if field == f.field {
		marker = markerOn
	}

	label := fieldLabels[field] + strings.Repeat(" ", labelWidth-tui.Cells(fieldLabels[field]))
	start := tui.Cells(marker) + labelWidth
	budget := max(width-start, 0)

	var (
		value string
		zones tui.Zones
	)
	switch field {
	case fieldLayout:
		value, zones = layoutValue(sc, f, line, start, budget)
	case fieldBase:
		value, zones = baseValue(sc, f, line, start, budget)
	default:
		value = f.textValue(sc, field, budget)
	}

	return sc.AccentIf(marker, false) + sc.Dim(label) + value, zones
}

// textValue scrolls the focused field to its caret; an unfocused one is cut from the front, keeping the end that
// tells two paths apart.
func (f *form) textValue(sc *tui.Screen, field, budget int) string {
	text, placeholder := f.value(field)
	if placeholder {
		return sc.Dim(tui.TruncateLeft(text, budget))
	}
	if field == f.field {
		text, _ = f.editor(field).View(budget)

		return text
	}

	return tui.TruncateLeft(text, budget)
}

// layoutValue draws the modes as a row, so the alternatives are visible without cycling.
func layoutValue(sc *tui.Screen, f *form, line, start, budget int) (string, tui.Zones) {
	var (
		b     strings.Builder
		zones = make(tui.Zones, 0, len(worktree.Modes))
		col   = start
	)
	// Too narrow for the row: show the selected mode alone rather than wrap and cost a row.
	shown, first := worktree.Modes, 0
	if modesWidth() > budget {
		shown, first = worktree.Modes[f.mode:f.mode+1], f.mode
	}

	for i, mode := range shown {
		if i > 0 {
			b.WriteString(sc.Dim(modeSep))
			col += tui.Cells(modeSep)
		}

		text := tui.Truncate(string(mode), start+budget-col)
		if text == "" {
			break
		}

		if first+i == f.mode {
			b.WriteString(sc.Bold(text))
		} else {
			b.WriteString(sc.Dim(text))
		}
		zones = append(zones, tui.Zone{Kind: zoneMode, Index: first + i, Line: line, From: col, To: col + tui.Cells(text)})
		col += tui.Cells(text)
	}

	note := ""
	if f.mode == f.proposed {
		note = "proposed from config"
		if f.layout().Detected {
			note = "detected from existing worktrees"
		}
	}
	if note != "" && start+budget-col >= tui.Cells(noteGap)+tui.Cells(note) {
		b.WriteString(sc.Dim(noteGap + note))
	}

	return b.String(), zones
}

func modesWidth() int {
	width := tui.Cells(modeSep) * (len(worktree.Modes) - 1)
	for _, mode := range worktree.Modes {
		width += tui.Cells(string(mode))
	}

	return width
}

// baseValue spells out the alternative base rather than leaving it to a hint.
func baseValue(sc *tui.Screen, f *form, line, start, budget int) (string, tui.Zones) {
	text, placeholder := f.value(fieldBase)
	shown := tui.TruncateLeft(text, budget)
	value := shown
	if placeholder {
		value = sc.Dim(value)
	}

	if len(f.bases) < 2 {
		return value, nil
	}

	next := f.bases[0]
	if index := f.baseIndex(); index >= 0 {
		next = f.bases[(index+1)%len(f.bases)]
	}

	swap := baseSwap + next
	// Measured off what was drawn, which may be truncated.
	col := start + tui.Cells(shown)
	if col+tui.Cells(swap) > start+budget {
		return value, nil
	}

	return value + sc.Dim(swap), tui.Zones{{Kind: zoneBase, Line: line, From: col, To: col + tui.Cells(swap)}}
}

// caret is the terminal cursor column of the focused text field.
func (f *form) caret(width int) int {
	start := tui.Cells(markerOn) + labelWidth
	_, col := f.editor(f.field).View(max(width-start, 0))

	return start + col + 1
}
