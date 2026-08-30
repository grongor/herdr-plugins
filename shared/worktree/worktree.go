// Package worktree infers where a repository puts new checkouts from its existing ones.
package worktree

import (
	"path/filepath"
	"slices"
	"strings"
)

// Mode is one of the layouts a repository's worktrees can follow.
type Mode string

const (
	// ModeSibling: "lorem" and "lorem-fix-issue".
	ModeSibling Mode = "sibling"
	// ModeShared: "lorem-worktrees/fix-issue"; the usual bare-repository layout.
	ModeShared Mode = "shared"
	// ModeHerdr leaves the path to Herdr's configured worktrees directory.
	ModeHerdr Mode = "herdr"
)

// Modes lists every mode in the order the dialog cycles through them.
var Modes = []Mode{ModeSibling, ModeShared, ModeHerdr}

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool { return slices.Contains(Modes, m) }

// sharedSuffix names the shared directory when the repository has no existing one to copy.
const sharedSuffix = "-worktrees"

// Checkout is an existing linked worktree.
type Checkout struct {
	Path string
	// Branch is empty when detached.
	Branch string
}

type Repo struct {
	// Anchor is the main checkout, or the repository directory itself when bare.
	Anchor string
	// Checkouts are the linked worktrees, excluding the main checkout.
	Checkouts []Checkout
}

// Layout is where new checkouts for one repository go.
type Layout struct {
	Mode Mode
	// Dir is the directory new checkouts are created in; empty for ModeHerdr.
	Dir string
	// Prefix is the repository's directory name, which sibling checkouts carry in front of the branch.
	Prefix string
	// Nested means a shared directory spells branches out as directories: "<dir>/proposal/di", not "<dir>/proposal-di".
	Nested bool
	// Detected is false when the mode came from the fallback.
	Detected bool
}

// For picks the layout a repository already follows, or fallback when its checkouts say nothing useful. herdrDir is
// Herdr's worktrees directory, "" when unknown.
func For(repo Repo, herdrDir string, fallback Mode) Layout {
	// Detached checkouts are mostly throwaways in temp directories, which would drag the shared directory towards "/".
	checkouts := make([]Checkout, 0, len(repo.Checkouts))
	for _, checkout := range repo.Checkouts {
		if checkout.Branch != "" {
			checkouts = append(checkouts, checkout)
		}
	}
	if len(checkouts) == 0 {
		return Switch(repo, fallback)
	}

	// Only the mode is voted on; the shared directory itself comes from the checkouts that voted for it.
	votes := make(map[Mode]int, len(Modes))
	var shared []string
	for _, checkout := range checkouts {
		mode := classify(repo, checkout, herdrDir)
		votes[mode]++
		if mode == ModeShared {
			shared = append(shared, filepath.Clean(checkout.Path))
		}
	}

	// Walking Modes rather than the map keeps ties deterministic.
	winner := Modes[0]
	for _, mode := range Modes[1:] {
		if votes[mode] > votes[winner] {
			winner = mode
		}
	}

	layout := Switch(repo, winner)
	layout.Detected = true
	if winner == ModeShared {
		layout.Dir, layout.Nested = sharedLayout(shared)
	}

	return layout
}

// Switch builds the layout for an explicitly chosen mode.
func Switch(repo Repo, mode Mode) Layout {
	if !mode.Valid() {
		mode = ModeSibling
	}

	parent := filepath.Dir(filepath.Clean(repo.Anchor))
	name := base(repo)

	switch mode {
	case ModeHerdr:
		return Layout{Mode: ModeHerdr, Prefix: name}
	case ModeShared:
		return Layout{Mode: ModeShared, Dir: filepath.Join(parent, name+sharedSuffix), Prefix: name}
	default:
		return Layout{Mode: ModeSibling, Dir: parent, Prefix: name}
	}
}

// Path is the checkout path to create for branch; empty for ModeHerdr, which lets Herdr choose. A sibling uses only the
// branch's last segment ("lorem-PROJ-2568", not "lorem-fix-PROJ-2568"), so "feat/x" and "fix/x" collide and git refuses
// the second. A shared directory keeps the whole branch.
func (l Layout) Path(branch string) string {
	if l.Mode == ModeHerdr || Slug(branch) == "" {
		return ""
	}

	if l.Mode == ModeShared {
		if l.Nested {
			return filepath.Join(append([]string{l.Dir}, branchSegments(branch)...)...)
		}

		return filepath.Join(l.Dir, Slug(branch))
	}

	return filepath.Join(l.Dir, l.Prefix+"-"+Leaf(branch))
}

// Leaf is the slug of a branch's last segment.
func Leaf(branch string) string {
	if _, last, found := strings.Cut(branch, "/"); found {
		return Leaf(last)
	}

	return Slug(branch)
}

// classify says which layout one checkout follows.
func classify(repo Repo, checkout Checkout, herdrDir string) Mode {
	if herdrDir != "" && under(herdrDir, checkout.Path) {
		return ModeHerdr
	}

	path := filepath.Clean(checkout.Path)
	if filepath.Dir(path) == filepath.Dir(filepath.Clean(repo.Anchor)) &&
		strings.HasPrefix(filepath.Base(path), base(repo)+"-") {
		return ModeSibling
	}

	return ModeShared
}

// sharedLayout returns the deepest directory every checkout sits in or under, and whether any checkout sits more than
// one level below it (nested). Branch names are deliberately not consulted, so hand-named directories work too.
func sharedLayout(paths []string) (string, bool) {
	dir := filepath.Dir(paths[0])
	for _, path := range paths[1:] {
		dir = commonDir(dir, filepath.Dir(path))
	}

	for _, path := range paths {
		if filepath.Dir(path) != dir {
			return dir, true
		}
	}

	return dir, false
}

// commonDir is the deepest directory both paths sit in or under.
func commonDir(a, b string) string {
	for a != b {
		if len(a) > len(b) {
			a = filepath.Dir(a)
		} else {
			b = filepath.Dir(b)
		}
	}

	return a
}

// branchSegments slugs a branch one path segment at a time.
func branchSegments(branch string) []string {
	segments := make([]string, 0, strings.Count(branch, "/")+1)
	for part := range strings.SplitSeq(branch, "/") {
		if slug := Slug(part); slug != "" {
			segments = append(segments, slug)
		}
	}

	return segments
}

// base is the repository's directory name without a bare repository's ".git".
func base(repo Repo) string {
	return strings.TrimSuffix(filepath.Base(filepath.Clean(repo.Anchor)), ".git")
}

func under(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)

	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Slug turns a branch name into a directory name. Case is kept so ticket ids like PROJ-2512 stay recognisable.
func Slug(branch string) string {
	var b strings.Builder

	dash := false
	for _, r := range branch {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_':
			b.WriteRune(r)
			dash = false
		case dash || b.Len() == 0:
			// Collapse runs, and never start with a separator.
		default:
			b.WriteByte('-')
			dash = true
		}
	}

	return strings.Trim(b.String(), "-._")
}
