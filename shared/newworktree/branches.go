package newworktree

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grongor/herdr-plugins/shared/search"
	"github.com/grongor/herdr-plugins/shared/tui"
)

// Zero is reserved by the shared layer for footer zones that carry only a keystroke.
const (
	zoneMode = iota + 1
	zoneBase
	zoneField
	zoneBranch
	zoneQuery
)

// pickerChrome is the query line, divider and footer; listTop is the screen row of the first branch.
const (
	pickerChrome = 3
	listTop      = 2
)

const remoteMarker = "remote"

// branchPicker is the filtered list behind the Base field; a cycling field is unusable with hundreds of branches.
type branchPicker struct {
	query   tui.LineEdit
	matches []branchMatch
	cursor  int
	offset  int
	// height is the rows the last frame had room for, which bounds the lines a click can name a branch on.
	height int
}

type branchMatch struct {
	name      string
	remote    bool
	positions []int
}

func (f *Form) openPicker() {
	f.picker = &branchPicker{}
	f.zones = nil
	f.refreshPicker()
}

func (f *Form) closePicker() {
	f.picker = nil
	f.zones = nil
}

func (f *Form) refreshPicker() {
	p := f.picker
	query := p.query.String()

	p.matches = p.matches[:0]
	for _, source := range []struct {
		names  []string
		remote bool
	}{{f.local, false}, {f.remote, true}} {
		if source.remote && !f.showRemote {
			continue
		}

		for _, name := range source.names {
			result, ok := search.Match(name, query)
			if !ok {
				continue
			}
			p.matches = append(p.matches, branchMatch{name: name, remote: source.remote, positions: result.Positions})
		}
	}

	// With no query the lists keep git's order: most recently committed first.
	if query != "" {
		scores := make(map[string]int, len(p.matches))
		for _, match := range p.matches {
			result, _ := search.Match(match.name, query)
			scores[match.name] = result.Score
		}
		sort.SliceStable(p.matches, func(i, j int) bool {
			return scores[p.matches[i].name] > scores[p.matches[j].name]
		})
	}

	p.cursor, p.offset = 0, 0
}

func (f *Form) handlePicker(k tui.Key) Action {
	p := f.picker

	if k.Is(f.opts.Config.RemoteKey()) {
		f.showRemote = !f.showRemote
		f.refreshPicker()

		return ActionNone
	}

	switch k.Kind {
	case tui.KeyEnter:
		// A filter nothing matches is taken as the ref itself, so a tag, a remote ref or a commit works; an empty one
		// is no answer and keeps the list up.
		base := p.selected()
		if base == "" {
			base = strings.TrimSpace(p.query.String())
		}
		if base == "" {
			return ActionNone
		}
		f.setBase(base)
		f.closePicker()
	case tui.KeyCtrlC:
		return ActionQuit
	case tui.KeyEsc:
		f.closePicker()
	case tui.KeyUp:
		p.move(-1)
	case tui.KeyDown:
		p.move(1)
	case tui.KeyPageUp:
		p.move(-10)
	case tui.KeyPageDown:
		p.move(10)
	default:
		// Only a changed filter refilters: moving the caret must not send the cursor back to the top.
		before := p.query.String()
		if p.query.Handle(k) && p.query.String() != before {
			f.refreshPicker()
		}
	}

	return ActionNone
}

func (p *branchPicker) selected() string {
	if p.cursor < 0 || p.cursor >= len(p.matches) {
		return ""
	}

	return p.matches[p.cursor].name
}

func (p *branchPicker) move(delta int) {
	if len(p.matches) == 0 {
		return
	}

	p.cursor = min(max(p.cursor+delta, 0), len(p.matches)-1)
}

func (p *branchPicker) scrollInto(height int) {
	if height <= 0 {
		return
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+height {
		p.offset = p.cursor - height + 1
	}
	p.offset = max(p.offset, 0)
}

// renderPicker replaces the fields rather than overlaying them: a second frame inside a popup costs too much room.
func (f *Form) renderPicker(sc *tui.Screen, width, height int) {
	p := f.picker
	f.zones = f.zones[:0]

	p.height = height - pickerChrome
	p.scrollInto(p.height)

	frame := sc.Frame()
	prompt := f.pickerPrompt(sc, width)
	frame.Line(prompt.Text)
	frame.Line(sc.Dim(strings.Repeat("─", width)))
	f.zones = append(f.zones, tui.Zone{Kind: zoneQuery, Line: 0, From: prompt.From, To: prompt.To})

	for i := range p.height {
		index := p.offset + i
		if index >= len(p.matches) {
			frame.Line(f.emptyListLine(sc, i, width))

			continue
		}
		frame.Line(branchLine(sc, p.matches[index], index == p.cursor, width))
		f.zones = append(f.zones, tui.Zone{
			Kind: zoneBranch, Index: index, Line: listTop + i, From: 0, To: width,
		})
	}

	footer, zones := sc.Footer(f.err, f.pickerHints(), height-1, width)
	frame.Bottom(footer)
	f.zones = append(f.zones, zones...)

	frame.Cursor(1, prompt.From+prompt.Caret+1)
	frame.Flush()
}

func (f *Form) emptyListLine(sc *tui.Screen, row, width int) string {
	if row > 0 || len(f.picker.matches) > 0 {
		return ""
	}

	switch {
	case f.loading:
		return sc.Dim(tui.Truncate("  "+statusFetching, width))
	case len(f.picker.query.Text) > 0:
		return sc.Dim(tui.Truncate("  no branch matches", width))
	case !f.showRemote:
		return sc.Dim(tui.Truncate("  no local branches — "+f.opts.Config.RemoteKey().String()+" lists remote ones", width))
	}

	return sc.Dim(tui.Truncate("  no branches", width))
}

// pickerPrompt counts the matches beside the filter; the drawing is the shared layer's.
func (f *Form) pickerPrompt(sc *tui.Screen, width int) tui.Prompt {
	counter := fmt.Sprintf("%d", len(f.picker.matches))
	if f.showRemote {
		counter += " +remote"
	}

	return sc.PromptLine(promptText, &f.picker.query, counter, width)
}

func (f *Form) pickerHints() []tui.Hint {
	key := f.opts.Config.RemoteKey()
	remote := key.String() + " remote"
	if f.showRemote {
		remote = key.String() + " local only"
	}

	return []tui.Hint{
		{Text: "enter use", Key: tui.Key{Kind: tui.KeyEnter}},
		{Text: remote, Key: key},
		{Text: "esc back", Key: tui.Key{Kind: tui.KeyEsc}},
	}
}

func branchLine(sc *tui.Screen, match branchMatch, selected bool, width int) string {
	marker := markerOff
	if selected {
		marker = markerOn
	}

	tag := ""
	if match.remote {
		tag = remoteMarker
	}

	budget := max(width-tui.Cells(marker)-tui.Cells(tag)-columnGap, 0)
	name, used := highlighted(sc, match.name, match.positions, budget)
	pad := max(width-tui.Cells(marker)-used-tui.Cells(tag), 0)

	var b strings.Builder
	b.WriteString(sc.AccentIf(marker, !selected))
	b.WriteString(name)
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(sc.DimIf(tag, !selected))

	if selected {
		return sc.Reverse(b.String())
	}

	return b.String()
}

// highlighted reports the columns used because a budget ending mid double-width character buys one column fewer.
func highlighted(sc *tui.Screen, text string, positions []int, budget int) (string, int) {
	if budget <= 0 {
		return "", 0
	}

	marks := make(map[int]bool, len(positions))
	for _, p := range positions {
		marks[p] = true
	}

	truncated := tui.Cells(text) > budget
	limit := budget
	if truncated {
		limit--
	}

	var b strings.Builder
	used := 0
	for i, r := range []rune(text) {
		width := tui.RuneCells(r)
		if used+width > limit {
			break
		}
		used += width

		if marks[i] {
			b.WriteString(sc.Bold(string(r)))

			continue
		}
		b.WriteRune(r)
	}
	if truncated {
		b.WriteString("…")
		used++
	}

	return b.String(), used
}
