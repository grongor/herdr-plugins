package ui

import (
	"fmt"
	"strings"

	"github.com/grongor/herdr-plugins/shared/tui"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/worktree"
)

const (
	promptText = "❯ "
	markerOn   = "❯ "
	markerOff  = "  "

	// Prompt, divider and hint.
	chromeHeight = 3
	// firstRow is the screen line the list starts on; clicks on rows are resolved from it by arithmetic.
	firstRow = 2
	// formFieldsTop is the screen line of the first field: title, repository name, divider, blank.
	formFieldsTop = 4
	// Minimum gap between the label and the dimmed right column.
	columnGap = 2
)

var (
	listHints = []tui.Hint{
		{Text: "enter open", Key: tui.Key{Kind: tui.KeyEnter}},
		{Text: "ctrl-g new worktree", Key: tui.Key{Kind: tui.KeyCtrlG}},
		{Text: "ctrl-t detached", Key: tui.Key{Kind: tui.KeyCtrlT}},
		{Text: "ctrl-u clear", Key: tui.Key{Kind: tui.KeyCtrlU}},
		{Text: "esc cancel", Key: tui.Key{Kind: tui.KeyEsc}},
	}
)

// Zone kinds; zero is the shared layer's, a zone standing for the keystroke it carries.
const (
	zoneMode = iota + 1
	zoneBase
	zoneQuery
)

// Options configures one run of the picker.
type Options struct {
	// Status is an extra note shown next to the row counter.
	Status string
	// HerdrWorktreesDir is where Herdr places worktrees itself; used to recognise a Herdr-managed repository.
	HerdrWorktreesDir string
	// Layout is proposed for repositories whose existing checkouts imply nothing.
	Layout worktree.Mode
	// ShowDetached is whether linked detached checkouts start out listed; ctrl-t flips it either way.
	ShowDetached bool
}

// Actions are what the picker calls when the user commits. An error is shown and the surface stays open.
type Actions struct {
	Open func(row Row) error
	// Create creates a new worktree and opens it.
	Create func(req Create) error
}

// Run shows the picker and blocks until an action succeeds or the user cancels. Cancelling is not an error.
func Run(rows []Row, opts Options, actions Actions) error {
	picker := &pickerState{rows: rows, opts: opts, actions: actions, showDetached: opts.ShowDetached}
	picker.refresh()

	return tui.Run(picker)
}

// Handle routes one keystroke to whichever surface is on screen and reports whether the picker is done.
func (s *pickerState) Handle(k tui.Key) bool {
	if k.Kind == tui.KeyMouse {
		synthesised, ok := s.click(k.Mouse)
		if !ok {
			return false
		}
		k = synthesised
	}

	if s.form != nil {
		switch s.form.handle(k) {
		case actionQuit:
			return true
		case actionCancel:
			s.form, s.zones = nil, nil
		case actionSelect:
			if err := s.actions.Create(s.form.request()); err != nil {
				s.form.err = err.Error()

				return false
			}

			return true
		case actionNone:
		}

		return false
	}

	switch s.handle(k) {
	case actionCancel, actionQuit:
		return true
	case actionSelect:
		if err := s.actions.Open(*s.selected()); err != nil {
			s.err = err.Error()

			return false
		}

		return true
	case actionNone:
	}

	return false
}

type pickerState struct {
	rows    []Row
	opts    Options
	actions Actions
	query   tui.LineEdit
	visible []visibleRow
	cursor  int
	offset  int
	err     string
	// showDetached: linked detached checkouts are mostly throwaways left by review tools and agents.
	showDetached bool

	// form is the new-worktree dialog, non-nil while it is on screen.
	form *form
	// zones is what the last frame put where, for resolving clicks. Cleared when the surface changes: one chunk of input
	// can hold two events, and the first may have replaced what the second was aimed at.
	zones tui.Zones
	// listHeight is how many rows the last frame had room for; bounds the lines a click can name a row on.
	listHeight int
}

type action int

const (
	actionNone action = iota
	actionSelect
	// actionCancel leaves the current surface: the dialog goes back to the list, the list closes the picker.
	actionCancel
	// actionQuit closes the picker from wherever it is.
	actionQuit
)

func (s *pickerState) refresh() {
	s.visible = filter(s.rows, s.query.String(), s.showDetached)
	s.cursor = 0
	s.offset = 0
	if len(s.visible) > 0 && !s.rows[s.visible[0].index].Targetable() {
		s.move(1)
	}
}

func (s *pickerState) selected() *Row {
	if len(s.visible) == 0 {
		return nil
	}
	row := s.rows[s.visible[s.cursor].index]
	if !row.Selectable() {
		return nil
	}

	return &row
}

// move steps the cursor by delta, skipping untargetable rows.
func (s *pickerState) move(delta int) {
	if len(s.visible) == 0 {
		return
	}

	for i := s.cursor + delta; i >= 0 && i < len(s.visible); i += sign(delta) {
		if s.rows[s.visible[i].index].Targetable() {
			s.cursor = i

			return
		}
	}

	// Nothing that way; if parked on a header (only possible right after a refresh), search the other way.
	if !s.rows[s.visible[s.cursor].index].Targetable() {
		for i := s.cursor - sign(delta); i >= 0 && i < len(s.visible); i -= sign(delta) {
			if s.rows[s.visible[i].index].Targetable() {
				s.cursor = i

				return
			}
		}
	}
}

func (s *pickerState) handle(k tui.Key) action {
	s.err = ""

	switch k.Kind {
	case tui.KeyEnter:
		if s.selected() != nil {
			return actionSelect
		}
	case tui.KeyEsc, tui.KeyCtrlC:
		return actionCancel
	case tui.KeyCtrlG:
		s.openForm()
	case tui.KeyCtrlT:
		s.showDetached = !s.showDetached
		s.refresh()
	case tui.KeyUp:
		s.move(-1)
	case tui.KeyDown:
		s.move(1)
	case tui.KeyPageUp:
		s.move(-10)
	case tui.KeyPageDown:
		s.move(10)
	default:
		// Only a changed query refilters: moving the caret must not send the cursor back to the top.
		before := s.query.String()
		if s.query.Handle(k) && s.query.String() != before {
			s.refresh()
		}
	}

	return actionNone
}

func (s *pickerState) openForm() { s.openFormAt(s.cursor) }

// openFormAt starts the dialog for one visible row; the checkout it names is the default base.
func (s *pickerState) openFormAt(index int) {
	if index < 0 || index >= len(s.visible) {
		return
	}

	row := s.rows[s.visible[index].index]
	layout := worktree.For(layoutRepo(row.Repo), s.opts.HerdrWorktreesDir, s.opts.Layout)
	s.form, s.zones = newForm(row, layout), nil
}

// click resolves a mouse report against the last frame. Cursor moves and field focus happen here; anything standing
// for a keystroke comes back as that keystroke.
func (s *pickerState) click(m tui.Mouse) (tui.Key, bool) {
	// Shift+mouse is the terminal's, for selecting text; Herdr's own mouse bindings leave it alone too.
	if m.Shift || m.Release || m.Button == tui.MouseMiddle {
		return tui.Key{}, false
	}

	switch m.Button {
	case tui.MouseWheelUp:
		return tui.Key{Kind: tui.KeyUp}, true
	case tui.MouseWheelDown:
		return tui.Key{Kind: tui.KeyDown}, true
	}

	if target, ok := s.zones.At(m); ok {
		switch target.Kind {
		case zoneQuery:
			s.query.Cursor = s.query.Locate(target.To-target.From, m.Col-1-target.From)
		case zoneMode:
			s.form.field = fieldLayout
			s.form.setMode(target.Index)
		case zoneBase:
			s.form.field = fieldBase
			s.form.cycleBase(1)
		default:
			return target.Key, true
		}

		return tui.Key{}, false
	}

	line := m.Row - 1

	if s.form != nil {
		if field := line - formFieldsTop; field >= 0 && field < fieldCount {
			s.form.field = field
		}

		return tui.Key{}, false
	}

	if row := line - firstRow; row >= 0 && row < s.listHeight {
		return s.clickRow(s.offset+row, m.Button)
	}

	return tui.Key{}, false
}

func (s *pickerState) clickRow(index, button int) (tui.Key, bool) {
	if index >= len(s.visible) {
		return tui.Key{}, false
	}

	targetable := s.rows[s.visible[index].index].Targetable()
	onCursor := targetable && index == s.cursor
	if targetable {
		s.cursor = index
	}

	// Right-click is the mouse's ctrl-g; it works on a repository header too, where the cursor cannot rest.
	if button == tui.MouseRight {
		s.openFormAt(index)

		return tui.Key{}, false
	}

	// Only a click on the row already under the cursor opens, so a stray click does not open a workspace.
	if onCursor && s.selected() != nil {
		return tui.Key{Kind: tui.KeyEnter}, true
	}

	return tui.Key{}, false
}

func sign(n int) int {
	if n < 0 {
		return -1
	}

	return 1
}

// Render draws whichever surface is on screen.
func (s *pickerState) Render(sc *tui.Screen) {
	width, height := sc.Width(), sc.Height()

	if s.form != nil {
		s.renderForm(sc, width, height)

		return
	}

	s.listHeight = height - chromeHeight
	s.scrollInto(s.listHeight)
	s.zones = s.zones[:0]

	frame := sc.Frame()
	prompt := sc.PromptLine(promptText, &s.query, s.counter(), width)
	frame.Line(prompt.Text)
	frame.Line(sc.Dim(strings.Repeat("─", width)))
	s.zones = append(s.zones, tui.Zone{Kind: zoneQuery, Line: 0, From: prompt.From, To: prompt.To})

	for i := range s.listHeight {
		index := s.offset + i
		if index >= len(s.visible) {
			frame.Line("")

			continue
		}
		frame.Line(s.rowLine(sc, s.visible[index], index == s.cursor, width))
	}

	footer, zones := sc.Footer(s.err, listHints, height-1, width)
	frame.Bottom(footer)
	s.zones = append(s.zones, zones...)

	frame.Cursor(1, prompt.From+prompt.Caret+1)
	frame.Flush()
}

func (s *pickerState) counter() string {
	counter := fmt.Sprintf("%d/%d", s.countVisible(), s.countRows())
	if s.opts.Status != "" {
		return s.opts.Status + "  " + counter
	}

	return counter
}

// countVisible and countRows count targetable rows, so an empty bare repository is not reported as nothing.
func (s *pickerState) countVisible() int {
	count := 0
	for _, v := range s.visible {
		if s.rows[v.index].Targetable() {
			count++
		}
	}

	return count
}

func (s *pickerState) countRows() int {
	count := 0
	for _, row := range s.rows {
		if row.Targetable() {
			count++
		}
	}

	return count
}

func (s *pickerState) scrollInto(listHeight int) {
	if listHeight <= 0 {
		return
	}
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+listHeight {
		s.offset = s.cursor - listHeight + 1
	}
	// Keep the selected worktree's header on screen when it is one line above the viewport.
	if s.offset > 0 && s.offset == s.cursor {
		s.offset--
	}
	s.offset = max(s.offset, 0)
}

func (s *pickerState) rowLine(sc *tui.Screen, v visibleRow, selected bool, width int) string {
	row := s.rows[v.index]
	prefix := v.prefix(s.rows)

	marker := markerOff
	if selected {
		marker = markerOn
	}

	labelWidth, detailWidth := tui.Columns(
		max(width-tui.Cells(marker)-tui.Cells(prefix)-columnGap, 0),
		tui.Cells(row.Label),
		tui.Cells(row.Detail),
	)

	label, labelUsed := highlighted(sc, row.Label, v.highlights, labelWidth, !row.Selectable() && !selected)
	// From the left: the end of a path is what tells two checkouts apart.
	detail := tui.TruncateLeft(row.Detail, detailWidth)

	// Padded from what was drawn, not the budget: truncating before a double-width character leaves a column unspent.
	pad := max(width-tui.Cells(marker)-tui.Cells(prefix)-labelUsed-tui.Cells(detail), 0)

	var b strings.Builder
	b.WriteString(sc.AccentIf(marker, !selected))
	b.WriteString(sc.DimIf(prefix, !selected))
	b.WriteString(label)
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(sc.DimIf(detail, !selected))

	if selected {
		// Reverse video follows the terminal theme where a fixed grey suits half of them; bold stays free for matches.
		return sc.Reverse(b.String())
	}

	return b.String()
}

// highlighted renders a label with the matched runes emphasised and reports the columns used, which can be one
// short of the budget when it ends before a double-width character.
func highlighted(sc *tui.Screen, label string, positions []int, budget int, dimmed bool) (string, int) {
	if budget <= 0 {
		return "", 0
	}

	marks := make(map[int]bool, len(positions))
	for _, p := range positions {
		marks[p] = true
	}

	// Positions index runes, so the text is walked in runes and only measured in columns.
	truncated := tui.Cells(label) > budget
	limit := budget
	if truncated {
		limit--
	}

	var b strings.Builder
	used := 0
	for i, r := range []rune(label) {
		width := tui.RuneCells(r)
		if used+width > limit {
			break
		}
		used += width

		switch {
		case marks[i]:
			b.WriteString(sc.Bold(string(r)))
		case dimmed:
			b.WriteString(sc.Dim(string(r)))
		default:
			b.WriteRune(r)
		}
	}
	if truncated {
		b.WriteString("…")
		used++
	}

	return b.String(), used
}
