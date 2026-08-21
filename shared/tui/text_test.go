package tui

import (
	"slices"
	"strings"
	"testing"
)

// A terminal gives a CJK ideograph or an emoji two cells; these tests measure what a terminal would.

func TestCells(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"ascii", "main", 4},
		{"czech diacritics stay single-width", "větev", 5},
		{"cjk", "設定", 4},
		{"emoji", "🚀", 2},
		{"mixed", "org/設定", 8},
		// Ambiguous-width ornaments are laid out as one column whatever the locale says.
		{"an ornament", "❯ ", 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Cells(test.text); got != test.want {
				t.Fatalf("Cells(%q) = %d, want %d", test.text, got, test.want)
			}
		})
	}
}

func TestTruncateMeasuresColumns(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		budget int
		want   string
	}{
		{"fits", "main", 8, "main"},
		{"exactly fits", "main", 4, "main"},
		{"ascii", "abcdefgh", 4, "abc…"},
		// Four ideographs are eight columns; in six, two fit alongside the ellipsis.
		{"cjk", "設定設定", 6, "設定…"},
		// Double-width characters cannot fill an odd budget, so the line comes up one column short rather than one over.
		{"cjk on an odd budget", "設定設定", 5, "設定…"},
		{"emoji", "🚀🚀🚀", 4, "🚀…"},
		{"mixed", "org/設定リポ", 9, "org/設定…"},
		{"no room", "設定", 0, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Truncate(test.text, test.budget)
			if got != test.want {
				t.Fatalf("Truncate(%q, %d) = %q, want %q", test.text, test.budget, got, test.want)
			}
			if Cells(got) > test.budget {
				t.Fatalf("%q is %d columns, over the budget of %d", got, Cells(got), test.budget)
			}
		})
	}
}

func TestTruncateLeftMeasuresColumns(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		budget int
		want   string
	}{
		{"fits", "/p/lorem", 8, "/p/lorem"},
		{"ascii", "/p/lorem/dev", 6, "…m/dev"},
		{"cjk", "設定設定", 6, "…設定"},
		{"cjk on an odd budget", "設定設定", 5, "…設定"},
		{"mixed", "/p/設定/dev", 8, "…定/dev"},
		{"no room", "設定", 0, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := TruncateLeft(test.text, test.budget)
			if got != test.want {
				t.Fatalf("TruncateLeft(%q, %d) = %q, want %q", test.text, test.budget, got, test.want)
			}
			if Cells(got) > test.budget {
				t.Fatalf("%q is %d columns, over the budget of %d", got, Cells(got), test.budget)
			}
		})
	}
}

// The break used to be located as a byte offset and applied to a rune slice, which panicked on non-ASCII messages.
func TestWrap(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		limit int
		want  []string
	}{
		{"short enough is left alone", "fatal: nope", 20, 3, []string{"fatal: nope"}},
		{"breaks on a space", "fatal: a branch named wip exists", 20, 3, []string{"fatal: a branch", "named wip exists"}},
		{
			// An early space is refused: breaking there would leave most of the line empty and still not fit the long word.
			"breaks mid-word when the only space is early",
			"fatal: abcdefghijklmnopqrstuvwxyz",
			20,
			2,
			[]string{"fatal: abcdefghijklm", "nopqrstuvwxyz"},
		},
		{
			"multi-byte text breaks in runes, not bytes",
			"fatální: větev pojmenovaná wip už existuje",
			20,
			3,
			[]string{"fatální: větev", "pojmenovaná wip už", "existuje"},
		},
		{"one line truncates the rest away", "fatální: větev pojmenovaná wip", 20, 1, []string{"fatální: větev pojm…"}},
		{"no room at all", "fatal: nope", 0, 3, []string{""}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Wrap(test.text, test.width, test.limit)

			if !slices.Equal(got, test.want) {
				t.Fatalf("Wrap = %q, want %q", got, test.want)
			}
			for i, line := range got {
				if Cells(line) > test.width {
					t.Errorf("line %d is %d columns wide, want at most %d", i, Cells(line), test.width)
				}
			}
		})
	}
}

// A character wider than the whole line has to overflow it, and the cut must still advance or Wrap would spin.
func TestWrapWiderThanTheLine(t *testing.T) {
	want := []string{"設", "定", "設", "…"}

	if got := Wrap("設定設定", 1, 4); !slices.Equal(got, want) {
		t.Fatalf("Wrap = %q, want %q", got, want)
	}
}

// The Czech cases are multi-byte but single-width, so they never exercised double-width input.
func TestWrapDoubleWidth(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		limit int
	}{
		{"cjk with no spaces to break on", "設定設定設定設定設定設定", 10, 3},
		{"cjk with spaces", "設定 リポジトリ が 見つかりません", 12, 3},
		{"emoji", "🚀🚀🚀🚀🚀🚀🚀🚀", 7, 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines := Wrap(test.text, test.width, test.limit)

			if len(lines) > test.limit {
				t.Fatalf("Wrap produced %d lines, over the limit of %d", len(lines), test.limit)
			}
			for i, line := range lines {
				if got := Cells(line); got > test.width {
					t.Errorf("line %d is %d columns, want at most %d: %q", i, got, test.width, line)
				}
			}

			// Nothing may be dropped except by the final truncation.
			joined := strings.ReplaceAll(strings.Join(lines, ""), "…", "")
			if !strings.HasPrefix(strings.ReplaceAll(test.text, " ", ""), strings.ReplaceAll(joined, " ", "")) {
				t.Errorf("Wrap lost or reordered text: %q from %q", lines, test.text)
			}
		})
	}
}

// The two columns together must fit in what was available, whatever the two want.
func TestColumnsNeverOverspend(t *testing.T) {
	for _, available := range []int{0, 1, 10, minDetailWidth, 40, 200} {
		for _, label := range []int{0, 3, 30, 300} {
			for _, detail := range []int{0, 3, 30, 300} {
				gotLabel, gotDetail := Columns(available, label, detail)
				if gotLabel+gotDetail > max(available, 0) {
					t.Fatalf("Columns(%d, %d, %d) = %d + %d, over budget",
						available, label, detail, gotLabel, gotDetail)
				}
				if gotLabel > label || gotDetail > detail {
					t.Fatalf("Columns(%d, %d, %d) = %d, %d, more than was asked for",
						available, label, detail, gotLabel, gotDetail)
				}
			}
		}
	}
}
