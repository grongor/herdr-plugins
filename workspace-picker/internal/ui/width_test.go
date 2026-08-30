package ui

import (
	"testing"

	"github.com/grongor/herdr-plugins/shared/tui"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
)

// TestRowLineFitsTheWidth pins that double-width names never push a line past the pane, which would wrap it.
func TestRowLineFitsTheWidth(t *testing.T) {
	repos := []discover.Repo{
		{
			Name:   "org/設定リポジトリ",
			Anchor: "/p/設定",
			Head:   "main",
			Worktrees: []discover.Worktree{
				{Path: "/p/設定", Branch: "main", Main: true},
				{Path: "/p/設定-機能ブランチ", Branch: "機能/ブランチ"},
				{Path: "/p/設定-🚀", Branch: "🚀"},
			},
		},
	}

	picker := &pickerState{rows: BuildRows(repos)}
	picker.refresh()

	// Colours off, so the measured line has no escape sequences.
	sc := &tui.Screen{}

	for _, width := range []int{20, 21, 40, 80} {
		for i, v := range picker.visible {
			for _, selected := range []bool{false, true} {
				line := picker.rowLine(sc, v, selected, width)
				if got := tui.Cells(line); got > width {
					t.Errorf("row %d (selected=%v) is %d columns at width %d: %q",
						i, selected, got, width, line)
				}
			}
		}
	}
}
