package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// widths is an explicit condition rather than the package default, which reads the locale at init: a name must
// render the same width whatever LANG says. Ambiguous characters count as one column, as a non-CJK-locale terminal
// does.
var widths = &runewidth.Condition{EastAsianWidth: false}

// Cells is the width of text in terminal columns. Layout budgets in columns, never runes: a CJK character is two.
func Cells(text string) int { return widths.StringWidth(text) }

// RuneCells is Cells for a single rune.
func RuneCells(r rune) int { return widths.RuneWidth(r) }

// Truncate cuts text to a column budget, marking the cut with an ellipsis.
func Truncate(text string, budget int) string {
	if budget <= 0 {
		return ""
	}
	if Cells(text) <= budget {
		return text
	}

	// One column is held back for the ellipsis.
	var b strings.Builder
	used := 0
	for _, r := range text {
		width := RuneCells(r)
		if used+width > budget-1 {
			break
		}
		b.WriteRune(r)
		used += width
	}

	return b.String() + "…"
}

// TruncateLeft cuts from the front, which is what a path wants: the end is what tells two checkouts apart.
func TruncateLeft(text string, budget int) string {
	if budget <= 0 {
		return ""
	}
	if Cells(text) <= budget {
		return text
	}

	runes := []rune(text)
	cut, used := len(runes), 0
	for i := len(runes) - 1; i >= 0; i-- {
		width := RuneCells(runes[i])
		if used+width > budget-1 {
			break
		}
		used += width
		cut = i
	}

	return "…" + string(runes[cut:])
}

// Wrap breaks text into at most limit lines of width columns, on spaces where it can. The last line is truncated rather
// than dropped, so the reader can tell there was more.
func Wrap(text string, width, limit int) []string {
	text = strings.Join(strings.Fields(text), " ")
	if width <= 0 || limit <= 0 {
		return []string{""}
	}

	lines := make([]string, 0, limit)
	for Cells(text) > width && len(lines) < limit-1 {
		runes := []rune(text)

		// Byte offsets are not rune indexes and rune indexes are not columns, so the cut is found by measuring. At least
		// one rune is always taken, or a character wider than the whole line would never advance.
		cut, used := len(runes), 0
		for i, r := range runes {
			if runeWidth := RuneCells(r); used+runeWidth > width {
				cut = max(i, 1)

				break
			} else {
				used += runeWidth
			}
		}

		// Back up to the last space in the right-hand half only; earlier, the break costs more than the long word it avoids.
		for i := cut - 1; i > cut/2; i-- {
			if runes[i] == ' ' {
				cut = i

				break
			}
		}

		lines = append(lines, strings.TrimSpace(string(runes[:cut])))
		text = strings.TrimSpace(string(runes[cut:]))
	}

	return append(lines, Truncate(text, width))
}

// minDetailWidth is the narrowest a dimmed right-hand column stays readable at.
const minDetailWidth = 16

// Columns splits available space between a label and a dimmed detail column. The label is served first; if the detail
// column would be too narrow to read, the two share the space evenly.
func Columns(available, label, detail int) (int, int) {
	if available <= 0 {
		return 0, 0
	}

	labelWidth := min(label, available)
	detailWidth := min(detail, available-labelWidth)

	if detail > 0 && detailWidth < minDetailWidth {
		detailWidth = min(detail, available/2)
		labelWidth = min(labelWidth, available-detailWidth)
	}

	return labelWidth, detailWidth
}
