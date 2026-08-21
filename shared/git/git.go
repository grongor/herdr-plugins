// Package git reads repository metadata straight off disk where a scan of hundreds of repositories cannot afford a
// process each, and shells out to git for everything else.
package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// shortHead matches git's default abbreviation.
const shortHead = 7

// Head parses gitDir/HEAD: "ref: refs/heads/<branch>", or a raw commit id when detached. For a linked worktree gitDir
// is "<common>/worktrees/<name>"; see Dir.
func Head(gitDir string) (branch, head string, detached bool) {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", "", false
	}

	value := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(value, "ref:"); ok {
		return strings.TrimPrefix(strings.TrimSpace(ref), "refs/heads/"), "", false
	}

	if len(value) > shortHead {
		value = value[:shortHead]
	}

	return "", value, true
}

// HeadOf is Head for a checkout directory.
func HeadOf(checkout string) (branch, head string, detached bool) {
	dir, ok := Dir(checkout)
	if !ok {
		return "", "", false
	}

	return Head(dir)
}

// Dir resolves a checkout's git directory: ".git" itself, or the "gitdir:" target of a linked worktree's ".git" file.
func Dir(checkout string) (string, bool) {
	gitPath := filepath.Join(checkout, ".git")

	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return gitPath, true
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}

	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
	if target == "" {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(checkout, target)
	}

	return filepath.Clean(target), true
}

// IsDirty reports whether checkout has uncommitted or untracked files.
func IsDirty(checkout string) (bool, error) {
	out, err := exec.Command("git", "-C", checkout, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("git status in %s: %w", checkout, err)
	}

	return len(strings.TrimSpace(string(out))) > 0, nil
}

// DeleteBranch force-deletes a branch: it runs after the branch's checkout is gone, so "not fully merged" is the
// ordinary case and a refusal would leave the branch behind with no UI left to act on it. The reflog keeps it
// recoverable.
func DeleteBranch(dir, branch string) error {
	if branch == "" {
		return errors.New("no branch to delete")
	}

	cmd := exec.Command("git", "-C", dir, "branch", "--delete", "--force", "--", branch)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// Empty output means git itself could not be run.
	text := strings.TrimSpace(string(out))
	if text == "" {
		text = err.Error()
	}

	return fmt.Errorf("git branch -D %s: %s", branch, text)
}
