// Package discover finds git repositories and their worktrees by reading git metadata off disk, never shelling out.
package discover

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grongor/herdr-plugins/shared/git"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/config"
)

// Worktree is a single checkout belonging to a repository.
type Worktree struct {
	Path string
	// Branch is empty when detached.
	Branch string
	// Head is the abbreviated commit, set only when Detached.
	Head     string
	Detached bool
	// Main is the primary (non-linked) worktree; a bare repository has none.
	Main bool
}

// Label is the branch name, or a readable stand-in for a detached HEAD.
func (w Worktree) Label() string {
	if w.Branch != "" {
		return w.Branch
	}
	if w.Head != "" {
		return "detached@" + w.Head
	}

	return filepath.Base(w.Path)
}

// Repo is one git repository with every checkout that belongs to it.
type Repo struct {
	// Name is the root-relative display name, e.g. "lorem/dolor-service".
	Name string
	// CommonDir is "<repo>/.git", or the bare repository itself. Herdr's worktree API takes it as --cwd.
	CommonDir string
	// Anchor is the main checkout, or CommonDir for a bare repository. Worktree paths are shown relative to its parent.
	Anchor string
	Bare   bool
	// Head is the branch the repository's own HEAD points at; the default base for a new worktree.
	Head string
	// Worktrees is the main worktree (if any) first, then linked worktrees by branch.
	Worktrees []Worktree
}

// Linked returns the linked checkouts without the main one.
func (r Repo) Linked() []Worktree {
	if len(r.Worktrees) > 0 && r.Worktrees[0].Main {
		return r.Worktrees[1:]
	}

	return r.Worktrees
}

// Scan walks the configured roots and returns every repository found, sorted by display name.
func Scan(cfg *config.Config) []Repo {
	// commonDir -> root it was discovered under.
	found := make(map[string]string)
	visited := make(map[string]bool)

	for _, root := range cfg.Roots {
		walk(root, root, 0, cfg, found, visited)
	}

	repos := make([]Repo, 0, len(found))
	for commonDir := range found {
		if repo, ok := load(commonDir, cfg.Roots); ok {
			repos = append(repos, repo)
		}
	}

	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })

	return repos
}

func walk(dir, root string, depth int, cfg *config.Config, found map[string]string, visited map[string]bool) {
	// Symlinked directories can form cycles; key on the resolved path.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return
	}
	if visited[real] {
		return
	}
	visited[real] = true

	if commonDir, stop := commonDirAt(dir); stop {
		if commonDir != "" {
			found[commonDir] = root
		}

		return
	}

	if isGitDir(dir) {
		found[filepath.Clean(dir)] = root

		return
	}

	if depth >= cfg.MaxDepth {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || cfg.IsIgnored(name) {
			continue
		}
		sub := filepath.Join(dir, name)
		// Stat rather than entry.IsDir so symlinked project directories are followed.
		if info, err := os.Stat(sub); err != nil || !info.IsDir() {
			continue
		}
		walk(sub, root, depth+1, cfg, found, visited)
	}
}

// commonDirAt inspects dir/.git. The bool reports whether the walk should stop descending: true for anything git-owned,
// including submodules, which are not listed as projects.
func commonDirAt(dir string) (string, bool) {
	gitPath := filepath.Join(dir, ".git")

	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return gitPath, true
	}

	// A linked worktree's .git file points at <common>/worktrees/<name>; a submodule's at <super>/.git/modules/<name>.
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", true
	}
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
	if target == "" {
		return "", true
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	target = filepath.Clean(target)

	if filepath.Base(filepath.Dir(target)) != "worktrees" {
		return "", true
	}

	return filepath.Dir(filepath.Dir(target)), true
}

// isGitDir reports whether dir holds a repository's metadata: a bare repository, or a checkout's ".git" directory.
func isGitDir(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, "HEAD")); err != nil || info.IsDir() {
		return false
	}
	for _, sub := range []string{"objects", "refs"} {
		if info, err := os.Stat(filepath.Join(dir, sub)); err != nil || !info.IsDir() {
			return false
		}
	}

	return true
}

func load(commonDir string, roots []string) (Repo, bool) {
	// A linked worktree's ".git" file may name a repository that has since been moved or deleted.
	if !isGitDir(commonDir) {
		return Repo{}, false
	}

	branch, head, detached := git.Head(commonDir)
	repo := Repo{CommonDir: commonDir, Head: branch}

	// "<checkout>/.git" is a normal repository whose main worktree is the parent; anything else is bare.
	if filepath.Base(commonDir) == ".git" {
		parent := filepath.Dir(commonDir)
		if info, err := os.Stat(parent); err == nil && info.IsDir() {
			repo.Anchor = parent
			repo.Worktrees = append(repo.Worktrees, Worktree{
				Path: parent, Branch: branch, Head: head, Detached: detached, Main: true,
			})
		}
	}
	if repo.Anchor == "" {
		repo.Bare = true
		repo.Anchor = commonDir
	}

	repo.Worktrees = append(repo.Worktrees, linkedWorktrees(commonDir)...)
	// A bare repository with no checkouts yet is still listed: it is where the first worktree gets created.
	if len(repo.Worktrees) == 0 && !repo.Bare {
		return Repo{}, false
	}

	repo.Name = displayName(repo.Anchor, roots, repo.Bare)

	linked := repo.Linked()
	sort.Slice(linked, func(i, j int) bool { return linked[i].Label() < linked[j].Label() })

	return repo, true
}

func linkedWorktrees(commonDir string) []Worktree {
	entries, err := os.ReadDir(filepath.Join(commonDir, "worktrees"))
	if err != nil {
		return nil
	}

	worktrees := make([]Worktree, 0, len(entries))
	for _, entry := range entries {
		adminDir := filepath.Join(commonDir, "worktrees", entry.Name())

		// The gitdir file holds "<checkout>/.git".
		data, err := os.ReadFile(filepath.Join(adminDir, "gitdir"))
		if err != nil {
			continue
		}
		checkout := filepath.Dir(filepath.Clean(strings.TrimSpace(string(data))))

		// A prunable worktree points at a directory that no longer exists, or at one that no longer belongs to this
		// repository: like git, require the checkout's ".git" file to point back here.
		if !pointsBack(checkout, adminDir) {
			continue
		}

		branch, head, detached := git.Head(adminDir)
		worktrees = append(worktrees, Worktree{
			Path: checkout, Branch: branch, Head: head, Detached: detached,
		})
	}

	return worktrees
}

// pointsBack reports whether checkout's ".git" file names adminDir as its git directory.
func pointsBack(checkout, adminDir string) bool {
	gitDir, ok := git.Dir(checkout)
	if !ok || gitDir == filepath.Join(checkout, ".git") {
		return false
	}
	if gitDir == filepath.Clean(adminDir) {
		return true
	}

	// Either side may be spelled through a symlink.
	realGitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return false
	}
	realAdminDir, err := filepath.EvalSymlinks(adminDir)

	return err == nil && realGitDir == realAdminDir
}

// displayName is the path relative to the deepest configured root containing it, so nested layouts read as "org/repo".
func displayName(anchor string, roots []string, bare bool) string {
	name := anchor
	best := ""
	for _, root := range roots {
		rel, err := filepath.Rel(root, anchor)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		if best == "" || len(root) > len(best) {
			best, name = root, rel
		}
	}

	if best == "" {
		if home, err := os.UserHomeDir(); err == nil {
			if rel, err := filepath.Rel(home, anchor); err == nil && !strings.HasPrefix(rel, "..") {
				name = filepath.Join("~", rel)
			}
		}
	}

	name = filepath.ToSlash(name)
	if bare {
		name = strings.TrimSuffix(name, ".git")
	}

	return name
}
