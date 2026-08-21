package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestCheckoutBranch pins that a checkout moved away from under its workspace is refused, not shown as detached.
func TestCheckoutBranch(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	gitIn(t, root, "init", "-q", "-b", "main", repo)
	gitIn(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")

	linked := filepath.Join(root, "repo-feat")
	gitIn(t, repo, "worktree", "add", "-q", "-b", "feat/x", linked)
	detached := filepath.Join(root, "repo-detached")
	gitIn(t, repo, "worktree", "add", "-q", "--detach", detached)

	if got, err := checkoutBranch(linked); err != nil || got != "feat/x" {
		t.Fatalf("checkoutBranch(linked) = %q, %v; want feat/x", got, err)
	}
	if got, err := checkoutBranch(detached); err != nil || got != "" {
		t.Fatalf("checkoutBranch(detached) = %q, %v; want no branch and no error", got, err)
	}

	moved := filepath.Join(root, "repo-moved")
	if err := os.Rename(linked, moved); err != nil {
		t.Fatal(err)
	}
	_, err := checkoutBranch(linked)
	if err == nil {
		t.Fatal("checkoutBranch accepted a checkout that is no longer there")
	}
	if !strings.Contains(err.Error(), linked) {
		t.Fatalf("error = %q, want it to name the path", err)
	}
}
