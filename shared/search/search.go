// Package search implements forgiving term matching: whitespace-separated terms, each a case-insensitive substring,
// in any order. A subsequence would be more forgiving still, but in names full of ticket numbers a few typed
// characters are a subsequence of nearly everything. Scoring prefers terms that start and end on word boundaries.
//
// A slash in a term also matches across a slash in the text: "lorem/dol" finds "lorem/ipsum-dolor" as well as
// "lorem/dolor", each part anywhere in its segment.
package search

import (
	"slices"
	"strings"
	"unicode"
)

const (
	scoreMatch = 16

	bonusBoundary = scoreMatch / 2
	// camelCase and letter-to-digit transitions are weaker boundaries.
	bonusCamel               = bonusBoundary - 1
	bonusFirstCharMultiplier = 2
)

// Result is a successful match.
type Result struct {
	Score int
	// Positions are rune indices into the matched text.
	Positions []int
}

// Match scores query against text. Whitespace splits the query into terms that must all occur, in any order.
func Match(text, query string) (Result, bool) {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return Result{}, true
	}

	runes := []rune(text)
	lowered := make([]rune, len(runes))
	for i, r := range runes {
		lowered[i] = unicode.ToLower(r)
	}

	var result Result
	for _, term := range terms {
		termResult, ok := matchTerm(runes, lowered, []rune(strings.ToLower(term)))
		if !ok {
			return Result{}, false
		}
		result.Score += termResult.Score
		result.Positions = append(result.Positions, termResult.Positions...)
	}

	return result, true
}

// matchTerm scores the best match of term: as typed, or split at its slashes into parts in successive segments.
func matchTerm(text, lowered, term []rune) (Result, bool) {
	best, found := occurrence(text, lowered, term, 0, len(text))
	if segmented, ok := matchSegments(text, lowered, term); ok && (!found || segmented.Score > best.Score) {
		best, found = segmented, true
	}

	return best, found
}

// segment is a run of text between slashes or whitespace; slashed marks one that a slash precedes.
type segment struct {
	start, end int
	slashed    bool
}

// matchSegments places each slash-separated part of term in a segment of text, each part's segment directly after the
// previous one's across a slash, and scores the best such placement. Whitespace ends a segment too, so "org/a" does not
// reach from "org/b" into a branch name after it. Empty parts add nothing: "lorem/" matches as "lorem" would.
func matchSegments(text, lowered, term []rune) (Result, bool) {
	if !slices.Contains(term, '/') {
		return Result{}, false
	}

	var parts [][]rune
	for part := range strings.SplitSeq(string(term), "/") {
		if part != "" {
			parts = append(parts, []rune(part))
		}
	}
	if len(parts) == 0 {
		return Result{}, false
	}

	var segments []segment
	start, slashed := 0, false
	for i, r := range lowered {
		if r == '/' || unicode.IsSpace(r) {
			segments = append(segments, segment{start, i, slashed})
			start, slashed = i+1, r == '/'
		}
	}
	segments = append(segments, segment{start, len(lowered), slashed})

	best, found := Result{}, false
	for first := 0; first+len(parts) <= len(segments); first++ {
		placed, ok := Result{}, true
		for k, part := range parts {
			seg := segments[first+k]
			if k > 0 && !seg.slashed {
				ok = false
				break
			}
			result, inSegment := occurrence(text, lowered, part, seg.start, seg.end)
			if !inSegment {
				ok = false
				break
			}
			placed.Score += result.Score
			placed.Positions = append(placed.Positions, result.Positions...)
		}
		if ok && (!found || placed.Score > best.Score) {
			best, found = placed, true
		}
	}

	return best, found
}

// occurrence scores the best occurrence of term within text[from:to]: "dolor" in "condolores-dolor" is the second one.
func occurrence(text, lowered, term []rune, from, to int) (Result, bool) {
	best, found := Result{}, false
	for start := from; start+len(term) <= to; start++ {
		end := start + len(term)
		if !slices.Equal(lowered[start:end], term) {
			continue
		}

		score := len(term)*scoreMatch + bonusAt(text, start)*bonusFirstCharMultiplier
		if end == len(text) || isDelimiter(text[end]) {
			score += bonusBoundary
		}
		if found && score <= best.Score {
			continue
		}

		best, found = Result{Score: score, Positions: make([]int, 0, len(term))}, true
		for i := start; i < end; i++ {
			best.Positions = append(best.Positions, i)
		}
	}

	return best, found
}

func bonusAt(text []rune, i int) int {
	if i == 0 {
		return bonusBoundary
	}

	prev, cur := text[i-1], text[i]
	switch {
	case isDelimiter(prev) && !isDelimiter(cur):
		return bonusBoundary
	case unicode.IsLower(prev) && unicode.IsUpper(cur):
		return bonusCamel
	case !unicode.IsDigit(prev) && unicode.IsDigit(cur):
		return bonusCamel
	}

	return 0
}

func isDelimiter(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}
