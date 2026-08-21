package tui

import "strings"

const HintSep = " · "

// Hint is one footer entry. It carries the key it names so clicking and typing cannot drift apart. The text names
// the key ("enter open", not "open") so the footer reads as documentation and matches Herdr's own dimmed hint rows;
// there is no button chrome for the same reason.
type Hint struct {
	Text string
	Key  Key
}

// Zone is a region of the last rendered frame that a click acts on. Only regions whose extent the rendered text decides
// need one; anything at a known line is worked out arithmetically.
type Zone struct {
	// Key is the keystroke this zone stands for; with KeyNone the app tells it apart by Kind.
	Key Key
	// Kind and Index are for the app. Zero means "nothing but the key".
	Kind  int
	Index int
	// Line is the 0-based screen row; From and To are 0-based columns, To exclusive.
	Line int
	From int
	To   int
}

// Zones is what one rendered frame put where. The surface on screen owns the whole slice and replacing it must clear
// them: one chunk of input can hold two events, and the first may have replaced what the second was aimed at.
type Zones []Zone

// At resolves a mouse report against the frame.
func (z Zones) At(m Mouse) (Zone, bool) {
	for _, target := range z {
		if target.Line == m.Row-1 && m.Col-1 >= target.From && m.Col-1 < target.To {
			return target, true
		}
	}

	return Zone{}, false
}

// HintLine draws the footer entries on a 0-based screen line and reports where each landed.
func (sc *Screen) HintLine(items []Hint, line, width int) (string, Zones) {
	var (
		b     strings.Builder
		zones = make(Zones, 0, len(items))
		col   int
	)

	for i, item := range items {
		// The separator is charged for before its entry and written only once the entry is known to fit; otherwise a
		// dangling " · " past the right edge wraps the footer onto a row a popup has not got.
		separator := ""
		if i > 0 {
			separator = HintSep
		}

		text := Truncate(item.Text, width-col-Cells(separator))
		if text == "" {
			break
		}

		b.WriteString(separator)
		col += Cells(separator)

		b.WriteString(text)
		zones = append(zones, Zone{Key: item.Key, Line: line, From: col, To: col + Cells(text)})
		col += Cells(text)
	}

	return sc.Dim(b.String()), zones
}

// Prompt is one drawn filter line: what to print, where the caret goes, and the columns the query itself occupies,
// which is what a click on the line is resolved against.
type Prompt struct {
	Text  string
	Caret int
	// From and To are 0-based columns, To exclusive. Locate(To-From, col-From) is the rune a click on col means.
	From int
	To   int
}

// PromptLine draws a filter: the prompt, the query scrolled to its caret, and the counter in the right corner. The
// counter gives way when the two would not fit side by side.
func (sc *Screen) PromptLine(prompt string, query *LineEdit, counter string, width int) Prompt {
	from := Cells(prompt)

	budget := width - from - Cells(counter) - 1
	if budget < 1 {
		budget = max(width-from, 0)
		text, caret := query.View(budget)

		return Prompt{Text: sc.Accent(prompt) + text, Caret: caret, From: from, To: from + budget}
	}

	text, caret := query.View(budget)
	pad := width - from - Cells(text) - Cells(counter)

	return Prompt{
		Text:  sc.Accent(prompt) + text + strings.Repeat(" ", pad) + sc.Dim(counter),
		Caret: caret,
		From:  from,
		To:    from + budget,
	}
}

// Footer shows failure when there is one and the key hints otherwise, on one line either way: a popup has no spare row.
func (sc *Screen) Footer(failure string, items []Hint, line, width int) (string, Zones) {
	if failure == "" {
		return sc.HintLine(items, line, width)
	}

	return sc.Alert(Wrap(failure, width, 1)[0]), nil
}
