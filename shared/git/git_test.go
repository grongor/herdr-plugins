package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestHeadOf pins that a linked worktree's HEAD is read from its own administrative directory, not the repository's.
func TestHeadOf(t *testing.T) {
	tests := []struct {
		name     string
		head     string
		linked   bool
		branch   string
		short    string
		detached bool
	}{
		{name: "a branch", head: "ref: refs/heads/main\n", branch: "main"},
		{name: "a branch with slashes", head: "ref: refs/heads/feature/PROJ-1\n", branch: "feature/PROJ-1"},
		{
			name:     "detached",
			head:     "3f786850e387550fdab836ed7e6dc881de23001b\n",
			short:    "3f78685",
			detached: true,
		},
		{name: "a linked worktree", head: "ref: refs/heads/topic\n", linked: true, branch: "topic"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkout := t.TempDir()

			gitDir := filepath.Join(checkout, ".git")
			if test.linked {
				gitDir = filepath.Join(t.TempDir(), "worktrees", "topic")
				write(t, filepath.Join(checkout, ".git"), "gitdir: "+gitDir+"\n")
			}
			if err := os.MkdirAll(gitDir, 0o755); err != nil {
				t.Fatalf("creating %s: %v", gitDir, err)
			}
			write(t, filepath.Join(gitDir, "HEAD"), test.head)

			branch, short, detached := HeadOf(checkout)
			if branch != test.branch || short != test.short || detached != test.detached {
				t.Fatalf("HeadOf = (%q, %q, %v), want (%q, %q, %v)",
					branch, short, detached, test.branch, test.short, test.detached)
			}
		})
	}
}

// TestHeadOfNothing pins that a non-repository answers quietly rather than failing.
func TestHeadOfNothing(t *testing.T) {
	branch, short, detached := HeadOf(t.TempDir())
	if branch != "" || short != "" || detached {
		t.Fatalf("HeadOf = (%q, %q, %v), want everything empty", branch, short, detached)
	}
}

// TestDeleteBranch pins that an unmerged branch is deleted, which plain `git branch -d` would refuse.
func TestDeleteBranch(t *testing.T) {
	repo := newRepo(t)

	run(t, repo, "checkout", "-b", "topic")
	commit(t, repo, "second.txt")
	run(t, repo, "checkout", "main")

	if branches := run(t, repo, "branch", "--format=%(refname:short)"); !strings.Contains(branches, "topic") {
		t.Fatalf("the fixture has no topic branch: %q", branches)
	}

	if err := DeleteBranch(repo, "topic"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	if branches := run(t, repo, "branch", "--format=%(refname:short)"); strings.Contains(branches, "topic") {
		t.Fatalf("topic survived: %q", branches)
	}
}

// TestDeleteBranchReportsGit pins that git's stderr reaches the caller.
func TestDeleteBranchReportsGit(t *testing.T) {
	repo := newRepo(t)

	err := DeleteBranch(repo, "nonexistent")
	if err == nil {
		t.Fatal("DeleteBranch succeeded on a branch that does not exist")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("error %q does not name the branch", err)
	}
}

func TestIsDirty(t *testing.T) {
	repo := newRepo(t)

	if dirty, err := IsDirty(repo); err != nil || dirty {
		t.Fatalf("IsDirty on a fresh commit = %v, %v; want clean", dirty, err)
	}

	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dirty, err := IsDirty(repo); err != nil || !dirty {
		t.Fatalf("IsDirty with an untracked file = %v, %v; want dirty", dirty, err)
	}
}

func TestIsDirtyOutsideARepository(t *testing.T) {
	if _, err := IsDirty(t.TempDir()); err == nil {
		t.Fatal("IsDirty succeeded outside a repository")
	}
}

func TestDeleteBranchWithoutABranch(t *testing.T) {
	if err := DeleteBranch(t.TempDir(), ""); err == nil {
		t.Fatal("DeleteBranch succeeded with no branch named")
	}
}

// newRepo builds a repository with one commit on "main".
func newRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	// git reports resolved paths; macOS temp dirs sit behind a symlink.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run(t, dir, "init", "--initial-branch=main", ".")
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "Test")
	run(t, dir, "config", "commit.gpgsign", "false")
	commit(t, dir, "first.txt")

	return dir
}

func commit(t *testing.T, dir, file string) {
	t.Helper()

	write(t, filepath.Join(dir, file), file+"\n")
	run(t, dir, "add", file)
	run(t, dir, "commit", "-m", "add "+file)
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()

	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	return string(out)
}

func write(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// TestBranchesOmitsRemoteHead pins that the symbolic origin/HEAD is not offered as a branch: %(refname:short) renders
// it as a bare "origin", which used to slip past the /HEAD filter.
func TestBranchesOmitsRemoteHead(t *testing.T) {
	upstream := newRepo(t)
	run(t, upstream, "branch", "feature")

	dir := t.TempDir()
	run(t, dir, "clone", "--quiet", upstream, ".")
	if !HasRef(dir, "refs/remotes/origin/HEAD") {
		t.Fatal("clone did not record origin/HEAD")
	}

	local, remote, err := Branches(dir)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(local, ",") != "main" {
		t.Errorf("local = %q, want [main]", local)
	}

	want := []string{"origin/feature", "origin/main"}
	slices.Sort(remote)
	if !slices.Equal(remote, want) {
		t.Errorf("remote = %q, want %q", remote, want)
	}
}
