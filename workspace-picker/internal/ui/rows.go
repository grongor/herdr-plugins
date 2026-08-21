package ui

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
	"github.com/grongor/herdr-plugins/workspace-picker/internal/search"
)

const (
	prefixChild = "  ├ "
	prefixLast  = "  └ "
)

// pathPenalty divides the score of a term found only in a checkout's path.
const pathPenalty = 2

// Row is one line in the picker: a repository header, or a worktree indented under its repository.
type Row struct {
	Repo *discover.Repo
	// Worktree is nil for the header of a bare repository.
	Worktree *discover.Worktree

	// Prefix is the tree drawing prefix, empty for repository rows.
	Prefix string
	// Label is the left column: repository name, or branch for a worktree.
	Label string
	// Detail is the dimmed right column: branch for a repository row, path for a worktree row.
	Detail string
	// Match is what the query is scored against; it includes the repository name so "store 2478" narrows to one worktree.
	Match string
	// Path is a checkout's path, matched too but ranked below Match: it usually repeats the branch, but need not.
	Path string

	group int
}

// Selectable reports whether Enter does anything on this row.
func (r Row) Selectable() bool { return r.Worktree != nil }

// Targetable reports whether the cursor can rest here: selectable rows, plus the header of a bare repository with no
// checkouts yet, where a new worktree can still be created.
func (r Row) Targetable() bool { return r.Selectable() || len(r.Repo.Worktrees) == 0 }

// BuildRows flattens repositories into the display list.
func BuildRows(repos []discover.Repo) []Row {
	rows := make([]Row, 0, len(repos)*2)

	for i := range repos {
		repo := &repos[i]
		// Relative to the directory holding the repo covers both sibling worktrees and a "<repo>-worktrees" tree.
		base := filepath.Dir(repo.Anchor)

		worktrees := repo.Worktrees
		header := Row{
			Repo:  repo,
			Label: repo.Name,
			Match: repo.Name,
			group: i,
		}

		if len(worktrees) > 0 && worktrees[0].Main {
			header.Worktree = &worktrees[0]
			header.Detail = worktrees[0].Label()
			worktrees = worktrees[1:]
		} else {
			header.Detail = "bare"
		}
		rows = append(rows, header)

		for j := range worktrees {
			prefix := prefixChild
			if j == len(worktrees)-1 {
				prefix = prefixLast
			}
			label := worktrees[j].Label()
			detail := relativePath(base, worktrees[j].Path)
			rows = append(rows, Row{
				Repo:     repo,
				Worktree: &worktrees[j],
				Prefix:   prefix,
				Label:    label,
				Detail:   detail,
				Match:    repo.Name + " " + label,
				Path:     detail,
				group:    i,
			})
		}
	}

	return rows
}

func relativePath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}

	return filepath.ToSlash(rel)
}

// visibleRow is a row that survived filtering, with the highlight positions for its label.
type visibleRow struct {
	index      int
	highlights []int
	score      int
	// prefixOverride replaces Row.Prefix once filtering has changed which child is last in its group.
	prefixOverride string
}

func (v visibleRow) prefix(rows []Row) string {
	if v.prefixOverride != "" {
		return v.prefixOverride
	}

	return rows[v.index].Prefix
}

// hidden reports whether the toggle keeps this row off screen. Only linked detached checkouts hide: hiding a main
// checkout on a detached HEAD would hide the repository with it.
func (r Row) hidden(showDetached bool) bool {
	return !showDetached && r.Worktree != nil && r.Worktree.Detached && !r.Worktree.Main
}

// filter narrows and ranks rows keeping the hierarchy intact: groups rank by their best-matching row, and a header
// stays visible whenever any of its worktrees matched.
func filter(rows []Row, query string, showDetached bool) []visibleRow {
	if strings.TrimSpace(query) == "" {
		visible := make([]visibleRow, 0, len(rows))
		for i := range rows {
			if rows[i].hidden(showDetached) {
				continue
			}
			visible = append(visible, visibleRow{index: i})
		}

		return retree(rows, visible)
	}

	type group struct {
		header  int
		matched []visibleRow
		score   int
	}

	groups := make([]*group, 0, len(rows))
	byGroup := make(map[int]*group, len(rows))

	for i, row := range rows {
		current, ok := byGroup[row.group]
		if !ok {
			current = &group{header: -1}
			byGroup[row.group] = current
			groups = append(groups, current)
		}

		if row.hidden(showDetached) {
			continue
		}

		score, matched := row.matches(query)
		if !matched {
			continue
		}

		if row.Prefix == "" {
			current.header = i
		} else {
			current.matched = append(current.matched, visibleRow{
				index:      i,
				highlights: highlight(row.Label, query),
				score:      score,
			})
		}
		current.score = max(current.score, score)
	}

	ordered := make([]*group, 0, len(groups))
	for _, current := range groups {
		if current.header >= 0 || len(current.matched) > 0 {
			ordered = append(ordered, current)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].score > ordered[j].score })

	visible := make([]visibleRow, 0, len(rows))
	for _, current := range ordered {
		headerIndex := current.header
		if headerIndex < 0 {
			headerIndex = groupHeader(rows, current.matched[0].index)
		}
		visible = append(visible, visibleRow{
			index:      headerIndex,
			highlights: highlight(rows[headerIndex].Label, query),
		})

		// Best match first, then by name.
		sort.SliceStable(current.matched, func(i, j int) bool {
			if current.matched[i].score != current.matched[j].score {
				return current.matched[i].score > current.matched[j].score
			}

			return rows[current.matched[i].index].Label < rows[current.matched[j].index].Label
		})
		visible = append(visible, current.matched...)
	}

	return retree(rows, visible)
}

// matches scores the query against the row a term at a time. A term found only in the path counts for less, so
// "2811" lists the branch named after it before a checkout that merely sits in a directory mentioning it.
func (r Row) matches(query string) (int, bool) {
	score := 0
	for _, term := range strings.Fields(query) {
		result, ok := search.Match(r.Match, term)
		path, inPath := search.Match(r.Path, term)
		if !ok && !inPath {
			return 0, false
		}
		score += max(result.Score, path.Score/pathPenalty)
	}

	return score, true
}

// groupHeader walks back to the repository row owning index.
func groupHeader(rows []Row, index int) int {
	for i := index; i >= 0; i-- {
		if rows[i].group == rows[index].group && rows[i].Prefix == "" {
			return i
		}
	}

	return index
}

// retree fixes the tree characters so the last visible child of a group draws as the last one after filtering.
func retree(rows []Row, visible []visibleRow) []visibleRow {
	for i := range visible {
		if rows[visible[i].index].Prefix == "" {
			continue
		}
		last := i == len(visible)-1 || rows[visible[i+1].index].Prefix == ""
		prefix := prefixChild
		if last {
			prefix = prefixLast
		}
		visible[i].prefixOverride = prefix
	}

	return visible
}

// highlight marks each term found in the visible label; a term that matched the repository name or the path has
// nothing to mark there.
func highlight(label, query string) []int {
	var positions []int
	for _, term := range strings.Fields(query) {
		if result, ok := search.Match(label, term); ok {
			positions = append(positions, result.Positions...)
		}
	}

	return positions
}
