package newworktree

import (
	"slices"
	"strings"

	"github.com/grongor/herdr-plugins/shared/tui"
	"github.com/grongor/herdr-plugins/shared/worktree"
)

const (
	promptText = "❯ "
	markerOn   = "❯ "
	markerOff  = "  "

	// labelWidth is the column every field value starts at.
	labelWidth = 8
	// columnGap is the minimum gap between a value and the dimmed right column.
	columnGap = 2
	// statusLines caps the failure shown; git's first lines carry the reason.
	statusLines = 3
	// fieldsTop is the screen line of the first field: title, repository name, divider, blank.
	fieldsTop = 4
)

// baseSwap names the key, since a branch name alone would not read as something to press.
const (
	modeSep  = " · "
	baseSwap = "   ←/→ "
	noteGap  = "   "
)

func (f *Form) Render(sc *tui.Screen, width, height int) {
	f.drain()

	if f.picker != nil {
		f.renderPicker(sc, width, height)

		return
	}

	f.zones = f.zones[:0]

	// An array so that adding a header line fails to compile unless fieldsTop moves with it.
	header := [fieldsTop]string{
		sc.Accent(tui.Truncate("New worktree", width)),
		sc.Dim(tui.Truncate(f.opts.Repo.Name, width)),
		sc.Dim(strings.Repeat("─", width)),
		"",
	}

	lines := slices.Clone(header[:])
	for field := range fieldCount {
		line, zones := f.fieldLine(sc, field, fieldsTop+field, width)
		lines = append(lines, line)
		f.zones = append(f.zones, zones...)
	}

	if status, failed := f.status(); status != "" {
		lines = append(lines, "")
		for _, line := range tui.Wrap(status, width, statusLines) {
			if failed {
				line = sc.Alert(line)
			} else {
				line = sc.Dim(line)
			}
			lines = append(lines, line)
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
	f.zones = append(f.zones, zones...)

	// A choice field has nothing to type into, so no cursor is drawn there.
	if f.choice() {
		frame.HideCursor()
	} else {
		frame.Cursor(fieldsTop+f.field+1, min(f.caret(width), width))
	}
	frame.Flush()
}

// hints says whether Enter is about to resolve or create, and names ←/→ only where it chooses: in a text field it moves
// the caret, which needs no telling.
func (f *Form) hints() []tui.Hint {
	enter := "enter create"
	if _, ok := referenceIn(f.branch.String()); ok {
		enter = "enter resolve"
	}

	hints := []tui.Hint{
		{Text: enter, Key: tui.Key{Kind: tui.KeyEnter}},
		{Text: "tab field", Key: tui.Key{Kind: tui.KeyTab}},
	}

	switch f.field {
	case fieldBase:
		hints = append(hints,
			tui.Hint{Text: "←/→ choose", Key: tui.Key{Kind: tui.KeyRight}},
			tui.Hint{Text: "space branches", Key: tui.Key{Kind: tui.KeyRune, Rune: ' '}},
		)
	case fieldLayout:
		hints = append(hints, tui.Hint{Text: "←/→ choose", Key: tui.Key{Kind: tui.KeyRight}})
	}

	return append(hints, tui.Hint{Text: "esc back", Key: tui.Key{Kind: tui.KeyEsc}})
}

func (f *Form) fieldLine(sc *tui.Screen, field, line, width int) (string, tui.Zones) {
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
		value, zones = f.layoutValue(sc, line, start, budget)
	case fieldBase:
		value, zones = f.baseValue(sc, line, start, budget)
	default:
		value = f.textValue(sc, field, budget)
	}

	zones = append(zones, tui.Zone{Kind: zoneField, Index: field, Line: line, From: 0, To: width})

	return sc.AccentIf(marker, false) + sc.Dim(label) + value, zones
}

// textValue scrolls the focused field to its caret; an unfocused one is cut from the front, keeping the end that
// tells two paths apart.
func (f *Form) textValue(sc *tui.Screen, field, budget int) string {
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

func (f *Form) layoutValue(sc *tui.Screen, line, start, budget int) (string, tui.Zones) {
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
func (f *Form) baseValue(sc *tui.Screen, line, start, budget int) (string, tui.Zones) {
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

// seek puts the caret of the focused text field on the column clicked; width is the field row's, since a click is
// resolved against the frame it was aimed at rather than the one being drawn.
func (f *Form) seek(col, width int) {
	e := f.editor(f.field)
	if e == nil {
		return
	}

	start := tui.Cells(markerOn) + labelWidth
	e.Cursor = e.Locate(max(width-start, 0), col-start)
}

// caret is the terminal cursor column of the focused text field.
func (f *Form) caret(width int) int {
	start := tui.Cells(markerOn) + labelWidth
	_, col := f.editor(f.field).View(max(width-start, 0))

	return start + col + 1
}

// Click resolves a mouse report against the last frame; anything standing for a keystroke comes back as that key.
func (f *Form) Click(m tui.Mouse) (tui.Key, bool) {
	f.drain()

	// Shift+mouse is the terminal's, for selecting text.
	if m.Shift || m.Release || m.Button == tui.MouseMiddle {
		return tui.Key{}, false
	}

	switch m.Button {
	case tui.MouseWheelUp:
		return tui.Key{Kind: tui.KeyUp}, true
	case tui.MouseWheelDown:
		return tui.Key{Kind: tui.KeyDown}, true
	}

	target, ok := f.zones.At(m)
	if !ok {
		return tui.Key{}, false
	}

	switch target.Kind {
	case zoneMode:
		f.field = fieldLayout
		f.setMode(target.Index)
	case zoneBase:
		f.field = fieldBase
		f.cycleBase(1)
	case zoneField:
		// The first click only moves to the field, as it only moves the cursor in a list: a stray click must not put
		// the branch list on screen. A second one acts inside the field it is already on.
		if f.field != target.Index {
			f.field = target.Index

			break
		}
		if f.field == fieldBase {
			f.openPicker()

			break
		}
		f.seek(m.Col-1, target.To)
	case zoneQuery:
		if f.picker == nil {
			break
		}
		f.picker.query.Cursor = f.picker.query.Locate(target.To-target.From, m.Col-1-target.From)
	case zoneBranch:
		if f.picker == nil {
			break
		}
		// First click selects, second confirms, so a stray click is not an answer.
		if f.picker.cursor == target.Index {
			return tui.Key{Kind: tui.KeyEnter}, true
		}
		f.picker.cursor = target.Index
	default:
		return target.Key, true
	}

	return tui.Key{}, false
}
