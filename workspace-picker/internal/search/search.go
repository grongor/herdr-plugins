// Package search implements forgiving term matching: whitespace-separated terms, each a case-insensitive substring,
// in any order. A subsequence would be more forgiving still, but in names full of ticket numbers a few typed
// characters are a subsequence of nearly everything. Scoring prefers terms that start and end on word boundaries.
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

// matchTerm scores the best occurrence of term: "dolor" in "condolores-dolor" is the second one.
func matchTerm(text, lowered, term []rune) (Result, bool) {
	best, found := Result{}, false
	for start := 0; start+len(term) <= len(lowered); start++ {
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
