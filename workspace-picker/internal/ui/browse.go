package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grongor/herdr-plugins/shared/paths"
	"github.com/grongor/herdr-plugins/shared/search"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
)

// gitMarker is the detail column of a directory that is a checkout of its own.
const gitMarker = "git"

// pathQuery splits a query naming a directory — one starting with "/" or "~/", or a lone "~" — into the directory typed
// so far, as typed and ending in a slash, and the partial name after it. Repository and branch names never start that
// way, so nothing a filter would find is lost.
func pathQuery(query string) (base, leaf string, ok bool) {
	if query == "~" {
		return "~/", "", true
	}
	if !strings.HasPrefix(query, "/") && !strings.HasPrefix(query, "~/") {
		return "", "", false
	}

	i := strings.LastIndex(query, "/")

	return query[:i+1], query[i+1:], true
}

// browse lists what a path query names. With nothing typed after the last slash, the directory itself comes first: as
// the repository it is the root of, with every checkout of it, or as a plain directory. Its subdirectories follow,
// those matching the partial name best first; hidden ones only once the name starts with a dot.
func browse(query string, roots []string) []Row {
	base, leaf, _ := pathQuery(query)
	dir, err := paths.ExpandHome(base)
	if err != nil {
		return nil
	}
	dir = filepath.Clean(dir)

	var rows []Row
	if leaf == "" {
		if repo, ok := discover.Load(dir, roots); ok {
			rows = BuildRows([]discover.Repo{repo})
		} else if isDir(dir) {
			rows = append(rows, Row{Dir: dir, Label: base, Match: base})
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return rows
	}

	type candidate struct {
		row   Row
		score int
	}
	candidates := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(leaf, ".") {
			continue
		}

		score := 0
		if leaf != "" {
			result, ok := search.Match(name, leaf)
			if !ok {
				continue
			}
			score = result.Score
		}

		sub := filepath.Join(dir, name)
		// Stat rather than entry.IsDir so symlinked directories are listed.
		if !isDir(sub) {
			continue
		}

		row := Row{Dir: sub, Label: name + "/", Match: name}
		if commonDir, ok := discover.CommonDir(sub); ok {
			row.CommonDir, row.Detail = commonDir, gitMarker
		}
		candidates = append(candidates, candidate{row, score})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}

		return candidates[i].row.Label < candidates[j].row.Label
	})

	// Groups only keep a repository's rows together, so each directory is its own.
	group := len(rows)
	for _, c := range candidates {
		c.row.group = group
		group++
		rows = append(rows, c.row)
	}

	return rows
}

// browsed is the visible list for a path query: browse already filtered, so only the toggles and the highlights apply.
func browsed(rows []Row, leaf string, show toggles) []visibleRow {
	visible := make([]visibleRow, 0, len(rows))
	for i, row := range rows {
		if row.hidden(show) {
			continue
		}
		v := visibleRow{index: i}
		if row.Dir != "" && leaf != "" {
			if result, ok := search.Match(row.Label, leaf); ok {
				v.highlights = result.Positions
			}
		}
		visible = append(visible, v)
	}

	return retree(rows, visible)
}

// completion is the query tab turns a subdirectory row into: its path as typed so far, descended into.
func completion(query string, row Row) (string, bool) {
	base, _, ok := pathQuery(query)
	if !ok || row.Dir == "" || row.Label == base {
		return "", false
	}

	return base + row.Label, true
}

func isDir(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}
