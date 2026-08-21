package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/config"
)

func TestScan(t *testing.T) {
	root := t.TempDir()

	// A plain repository, plus a worktree as a sibling directory.
	gitDir := initRepo(t, filepath.Join(root, "org", "plain"), "main")
	addWorktree(t, gitDir, filepath.Join(root, "org", "plain-feature"), "feature/x")

	// A worktree whose checkout is gone but was never pruned.
	addWorktree(t, gitDir, filepath.Join(root, "org", "deleted"), "gone")
	if err := os.RemoveAll(filepath.Join(root, "org", "deleted")); err != nil {
		t.Fatal(err)
	}

	// A worktree living outside every configured root.
	outside := t.TempDir()
	addWorktree(t, gitDir, filepath.Join(outside, "detached-checkout"), "")

	// A bare repository with worktrees and no main checkout, nested deeper than the checkouts themselves.
	bare := initBare(t, filepath.Join(root, "org", "bared.git"))
	addWorktree(t, bare, filepath.Join(root, "org", "bared-worktrees", "team", "topic"), "team/topic")

	// A submodule is part of its superproject, not a project of its own.
	writeFile(t, filepath.Join(root, "org", "plain", "sub", ".git"),
		"gitdir: "+filepath.Join(gitDir, "modules", "sub")+"\n")

	// A stale worktree entry whose checkout directory was recreated by hand: it exists, but is not a checkout.
	addWorktree(t, bare, filepath.Join(root, "org", "bared-worktrees", "team", "hollow"), "team/hollow")
	if err := os.Remove(filepath.Join(root, "org", "bared-worktrees", "team", "hollow", ".git")); err != nil {
		t.Fatal(err)
	}

	// A stale worktree entry whose checkout has since been re-linked to another repository.
	addWorktree(t, bare, filepath.Join(root, "org", "bared-worktrees", "team", "moved"), "team/moved")
	writeFile(t, filepath.Join(root, "org", "bared-worktrees", "team", "moved", ".git"),
		"gitdir: "+filepath.Join(gitDir, "worktrees", "moved")+"\n")

	// A checkout left behind by a repository that was renamed away: its ".git" file names a directory that is gone.
	writeFile(t, filepath.Join(root, "org", "orphaned-checkout", ".git"),
		"gitdir: "+filepath.Join(root, "org", "renamed.git", "worktrees", "orphan")+"\n")

	cfg := &config.Config{Roots: []string{root}, MaxDepth: 4}
	repos := Scan(cfg)

	if len(repos) != 2 {
		t.Fatalf("found %d repositories, want 2: %+v", len(repos), repos)
	}

	bared, plain := repos[0], repos[1]

	if bared.Name != "org/bared" {
		t.Errorf("bare repo name = %q, want %q", bared.Name, "org/bared")
	}
	if !bared.Bare {
		t.Error("bare repo not reported as bare")
	}
	if got := labels(bared); len(got) != 1 || got[0] != "team/topic" {
		t.Errorf("bare repo worktrees = %v, want [team/topic]", got)
	}

	if plain.Name != "org/plain" {
		t.Errorf("repo name = %q, want %q", plain.Name, "org/plain")
	}
	if plain.Bare {
		t.Error("plain repo reported as bare")
	}
	if !plain.Worktrees[0].Main {
		t.Error("first worktree of a plain repo should be the main one")
	}

	// main, the sibling worktree, and the out-of-root one; not the pruned one.
	want := []string{"main", "detached@0123456", "feature/x"}
	got := labels(plain)
	if len(got) != len(want) {
		t.Fatalf("worktrees = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("worktrees = %v, want %v", got, want)
		}
	}
}

// TestScanKeepsEmptyBareRepo pins that a bare repository is listed before it has any worktrees.
func TestScanKeepsEmptyBareRepo(t *testing.T) {
	root := t.TempDir()
	initBare(t, filepath.Join(root, "org", "lonely.git"))

	repos := Scan(&config.Config{Roots: []string{root}, MaxDepth: 4})
	if len(repos) != 1 {
		t.Fatalf("found %d repositories, want 1: %+v", len(repos), repos)
	}
	if !repos[0].Bare {
		t.Error("repository not reported as bare")
	}
	if repos[0].Name != "org/lonely" {
		t.Errorf("name = %q, want %q", repos[0].Name, "org/lonely")
	}
	if len(repos[0].Worktrees) != 0 {
		t.Errorf("worktrees = %v, want none", labels(repos[0]))
	}
}

func TestScanRespectsMaxDepth(t *testing.T) {
	root := t.TempDir()
	initRepo(t, filepath.Join(root, "a", "b", "c", "deep"), "main")

	if repos := Scan(&config.Config{Roots: []string{root}, MaxDepth: 2}); len(repos) != 0 {
		t.Fatalf("found %d repositories at max_depth 2, want 0", len(repos))
	}
	if repos := Scan(&config.Config{Roots: []string{root}, MaxDepth: 4}); len(repos) != 1 {
		t.Fatalf("found %d repositories at max_depth 4, want 1", len(repos))
	}
}

func TestScanSkipsIgnoredDirectories(t *testing.T) {
	root := t.TempDir()
	initRepo(t, filepath.Join(root, "vendor", "dependency"), "main")

	cfg := &config.Config{Roots: []string{root}, MaxDepth: 4, Ignore: []string{"vendor"}}
	if repos := Scan(cfg); len(repos) != 0 {
		t.Fatalf("found %d repositories, want 0: %+v", len(repos), repos)
	}
}

func labels(repo Repo) []string {
	out := make([]string, 0, len(repo.Worktrees))
	for _, worktree := range repo.Worktrees {
		out = append(out, worktree.Label())
	}

	return out
}

// initRepo writes the minimum git layout the scanner reads and returns the git directory.
func initRepo(t *testing.T, checkout, branch string) string {
	t.Helper()

	gitDir := filepath.Join(checkout, ".git")
	mkdirAll(t, filepath.Join(gitDir, "objects"))
	mkdirAll(t, filepath.Join(gitDir, "refs"))
	writeHead(t, gitDir, branch)

	return gitDir
}

func initBare(t *testing.T, dir string) string {
	t.Helper()

	mkdirAll(t, filepath.Join(dir, "objects"))
	mkdirAll(t, filepath.Join(dir, "refs"))
	writeHead(t, dir, "main")

	return dir
}

// addWorktree links a checkout to gitDir the way `git worktree add` does; an empty branch means detached.
func addWorktree(t *testing.T, gitDir, checkout, branch string) {
	t.Helper()

	admin := filepath.Join(gitDir, "worktrees", filepath.Base(checkout))
	mkdirAll(t, admin)
	writeHead(t, admin, branch)
	writeFile(t, filepath.Join(admin, "gitdir"), filepath.Join(checkout, ".git")+"\n")

	mkdirAll(t, checkout)
	writeFile(t, filepath.Join(checkout, ".git"), "gitdir: "+admin+"\n")
}

func writeHead(t *testing.T, gitDir, branch string) {
	t.Helper()

	content := "0123456789abcdef0123456789abcdef01234567\n"
	if branch != "" {
		content = "ref: refs/heads/" + branch + "\n"
	}
	writeFile(t, filepath.Join(gitDir, "HEAD"), content)
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
