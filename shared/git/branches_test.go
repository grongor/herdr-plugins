package git

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestBranchesAndDefaultBase uses a clone: the only shape with a remote HEAD and a tracking ref beside a same-named
// local branch.
func TestBranchesAndDefaultBase(t *testing.T) {
	origin := newRepo(t)
	run(t, origin, "branch", "topic")

	clone := filepath.Join(t.TempDir(), "clone")
	run(t, t.TempDir(), "clone", "--quiet", origin, clone)

	local, remote, err := Branches(clone)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}

	if !slices.Contains(local, "main") {
		t.Errorf("local = %q, want it to hold main", local)
	}
	if slices.Contains(local, "topic") {
		t.Errorf("local = %q, want the untracked branch left to the remote list", local)
	}
	for _, want := range []string{"origin/main", "origin/topic"} {
		if !slices.Contains(remote, want) {
			t.Errorf("remote = %q, want it to hold %s", remote, want)
		}
	}

	if slices.Contains(remote, "origin/HEAD") {
		t.Errorf("remote = %q, want origin/HEAD left out", remote)
	}

	if got := DefaultBase(clone); got != "origin/main" {
		t.Errorf("DefaultBase = %q, want the remote's own default branch", got)
	}
}

func TestDefaultBaseWithoutARemote(t *testing.T) {
	repo := newRepo(t)

	if got := DefaultBase(repo); got != "main" {
		t.Errorf("DefaultBase = %q, want the local trunk", got)
	}
}

func TestFetchWithoutARemote(t *testing.T) {
	if err := Fetch(newRepo(t), 5*time.Second); err != nil {
		t.Fatalf("Fetch in a repository with no remote: %v", err)
	}
}

// TestAddWorktree pins that a new branch is created from the base and an existing one is checked out as is.
func TestAddWorktree(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "branch", "existing")
	commit(t, repo, "second.txt")

	dir := t.TempDir()

	fresh := filepath.Join(dir, "fresh")
	if err := AddWorktree(repo, fresh, "topic", "existing"); err != nil {
		t.Fatalf("AddWorktree for a new branch: %v", err)
	}
	if branch, _, _ := HeadOf(fresh); branch != "topic" {
		t.Errorf("branch = %q, want topic", branch)
	}
	if _, err := os.ReadFile(filepath.Join(fresh, "second.txt")); err == nil {
		t.Error("the checkout has the commit its base was made before")
	}

	taken := filepath.Join(dir, "taken")
	if err := AddWorktree(repo, taken, "existing", ""); err != nil {
		t.Fatalf("AddWorktree for an existing branch: %v", err)
	}
	if branch, _, _ := HeadOf(taken); branch != "existing" {
		t.Errorf("branch = %q, want existing", branch)
	}

	checkouts, err := Worktrees(repo)
	if err != nil {
		t.Fatalf("Worktrees: %v", err)
	}
	if len(checkouts) != 3 {
		t.Fatalf("Worktrees = %+v, want three", checkouts)
	}
	if !checkouts[0].Main || checkouts[0].Path != repo {
		t.Errorf("first checkout = %+v, want the main one", checkouts[0])
	}
	for _, checkout := range checkouts[1:] {
		if checkout.Main {
			t.Errorf("checkout %+v is reported as the main one", checkout)
		}
		if checkout.Branch == "" {
			t.Errorf("checkout %+v has no branch", checkout)
		}
	}
}

// TestAddWorktreeReportsGit pins that git's own reason survives into the error.
func TestAddWorktreeReportsGit(t *testing.T) {
	repo := newRepo(t)

	err := AddWorktree(repo, filepath.Join(t.TempDir(), "x"), "main", "")
	if err == nil {
		t.Fatal("AddWorktree checked out a branch that is already checked out")
	}
	if got := err.Error(); !strings.Contains(got, "main") {
		t.Fatalf("error %q does not name the branch", got)
	}
}

// TestCommonDir pins that a linked worktree answers with the repository's git directory, not its per-checkout one.
func TestCommonDir(t *testing.T) {
	repo := newRepo(t)

	linked := filepath.Join(t.TempDir(), "linked")
	if err := AddWorktree(repo, linked, "topic", ""); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	want := filepath.Join(repo, ".git")
	for _, dir := range []string{repo, linked} {
		if got := CommonDir(dir); got != want {
			t.Errorf("CommonDir(%s) = %q, want %q", dir, got, want)
		}
	}

	if got := CommonDir(t.TempDir()); got != "" {
		t.Errorf("CommonDir outside a repository = %q, want empty", got)
	}
}

// TestFetchGivesUp pins the timeout; the hang is an ssh command that sleeps instead of connecting.
func TestFetchGivesUp(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "remote", "add", "origin", "nowhere:repo.git")
	t.Setenv("GIT_SSH_COMMAND", "sh -c 'sleep 30' ssh")

	start := time.Now()
	err := Fetch(repo, 200*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Fetch against a hanging remote succeeded")
	}
	if !strings.Contains(err.Error(), "gave up") {
		t.Fatalf("error = %q, want it to say the fetch was given up on", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Fetch took %s, want it bounded by the timeout", elapsed)
	}
}

// TestDefaultBaseIsNotANameGuess pins that a trunk called "test" is found via the remote HEAD (cloned or recorded by
// Fetch after a hand-added remote) or init.defaultBranch.
func TestDefaultBaseIsNotANameGuess(t *testing.T) {
	upstream := t.TempDir()
	run(t, upstream, "init", "--initial-branch=test", ".")
	run(t, upstream, "config", "user.email", "test@example.com")
	run(t, upstream, "config", "user.name", "Test")
	commit(t, upstream, "first.txt")
	// A newer branch, so "newest first" cannot be what picks the right answer.
	run(t, upstream, "checkout", "-b", "main")
	commit(t, upstream, "second.txt")
	run(t, upstream, "checkout", "test")

	t.Run("a clone", func(t *testing.T) {
		clone := t.TempDir()
		run(t, clone, "clone", "--quiet", upstream, ".")

		if got := DefaultBase(clone); got != "origin/test" {
			t.Errorf("DefaultBase = %q, want origin/test", got)
		}
	})

	t.Run("a remote added by hand, then fetched", func(t *testing.T) {
		repo := newRepo(t)
		run(t, repo, "remote", "add", "upstream", upstream)
		if HasRef(repo, "refs/remotes/upstream/HEAD") {
			t.Fatal("remote add recorded a HEAD; the case under test is gone")
		}

		if err := Fetch(repo, 10*time.Second); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if got := DefaultBase(repo); got != "upstream/test" {
			t.Errorf("DefaultBase = %q, want upstream/test, read off the remote", got)
		}
	})

	t.Run("no remote, init.defaultBranch", func(t *testing.T) {
		repo := t.TempDir()
		run(t, repo, "init", "--initial-branch=test", ".")
		run(t, repo, "config", "user.email", "test@example.com")
		run(t, repo, "config", "user.name", "Test")
		run(t, repo, "config", "init.defaultBranch", "test")
		commit(t, repo, "first.txt")
		run(t, repo, "branch", "main")

		if got := DefaultBase(repo); got != "test" {
			t.Errorf("DefaultBase = %q, want test", got)
		}
	})
}
