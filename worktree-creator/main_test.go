package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grongor/herdr-plugins/shared/plugin"
)

func TestRepoFrom(t *testing.T) {
	main, linked := repoWithWorktree(t)

	tests := []struct {
		name string
		ctx  plugin.Context
	}{
		{
			name: "from Herdr's invocation context",
			ctx: plugin.Context{Worktree: &plugin.Worktree{
				RepoName:     "lorem",
				RepoKey:      filepath.Join(main, ".git"),
				RepoRoot:     main,
				CheckoutPath: linked,
			}},
		},
		{
			// From the linked checkout, whose ".git" is a file pointing elsewhere — the case a naive answer gets wrong.
			name: "from the workspace directory alone",
			ctx:  plugin.Context{WorkspaceCwd: linked},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, err := repoFrom(test.ctx)
			if err != nil {
				t.Fatalf("repoFrom: %v", err)
			}

			if want := filepath.Join(main, ".git"); repo.CommonDir != want {
				t.Errorf("CommonDir = %q, want %q", repo.CommonDir, want)
			}
			if repo.Anchor != main {
				t.Errorf("Anchor = %q, want the main checkout %q", repo.Anchor, main)
			}
			if repo.Head != "main" {
				t.Errorf("Head = %q, want main", repo.Head)
			}

			// The main checkout is not layout evidence.
			if len(repo.Checkouts) != 1 {
				t.Fatalf("Checkouts = %+v, want the linked one alone", repo.Checkouts)
			}
			if repo.Checkouts[0].Path != linked || repo.Checkouts[0].Branch != "topic" {
				t.Errorf("Checkouts[0] = %+v, want %s on topic", repo.Checkouts[0], linked)
			}
		})
	}
}

func TestRepoFromOutsideARepository(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  plugin.Context
	}{
		{"nothing at all", plugin.Context{}},
		{"a directory that is not a checkout", plugin.Context{WorkspaceCwd: t.TempDir()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repoFrom(test.ctx); err == nil {
				t.Fatal("repoFrom succeeded outside a repository")
			}
		})
	}
}

// repoWithWorktree builds a repository on "main" with one linked checkout on "topic".
func repoWithWorktree(t *testing.T) (main, linked string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	// git reports resolved paths; macOS temp dirs sit behind a symlink.
	tempDir := func() string {
		t.Helper()

		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}

		return dir
	}

	main = tempDir()
	git := func(args ...string) {
		t.Helper()

		out, err := exec.Command("git", append([]string{"-C", main}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	git("init", "--initial-branch=main", ".")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("config", "commit.gpgsign", "false")
	git("commit", "--allow-empty", "-m", "first")

	linked = filepath.Join(tempDir(), "topic")
	git("worktree", "add", "-b", "topic", linked)

	return main, linked
}
