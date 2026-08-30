// Package git reads repository metadata straight off disk where a scan of hundreds of repositories cannot afford a
// process each, and shells out to git for everything else.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
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

// fallbackBranches are tried after the remote HEAD and init.defaultBranch.
var fallbackBranches = []string{"main", "master"}

// Fetch updates dir's remote-tracking refs, pruning gone ones. A repository with no remote is not an error. Past
// timeout git is killed and an error returned; the refs already on disk still serve.
func Fetch(dir string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", dir, "fetch", "--all", "--prune", "--quiet")
	// A credential or host-key prompt would sit unanswered under the dialog.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	out, err := cmd.CombinedOutput()
	if err == nil {
		recordRemoteHead(ctx, dir)

		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("git fetch: gave up after %s", timeout)
	}

	text := strings.TrimSpace(string(out))
	if text == "" {
		text = err.Error()
	}

	return fmt.Errorf("git fetch: %s", text)
}

// Branches lists local and remote branches, newest commit first, without the symbolic <remote>/HEAD.
func Branches(dir string) (local, remote []string, err error) {
	local, err = refs(dir, "refs/heads")
	if err != nil {
		return nil, nil, err
	}

	remote, err = refs(dir, "refs/remotes")
	if err != nil {
		return nil, nil, err
	}

	remote = slices.DeleteFunc(remote, func(name string) bool {
		return strings.HasSuffix(name, "/HEAD")
	})

	return local, remote, nil
}

// refs lists one ref namespace, newest commit first, with the namespace prefix stripped.
// The full refname is used rather than %(refname:short), which renders the symbolic refs/remotes/origin/HEAD as
// just "origin" and so hides it from the /HEAD filter in Branches.
func refs(dir, namespace string) ([]string, error) {
	cmd := exec.Command("git", "-C", dir,
		"for-each-ref", "--sort=-committerdate", "--format=%(refname)", namespace)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git for-each-ref %s: %w", namespace, err)
	}

	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, nil
	}

	names := strings.Split(text, "\n")
	for i, name := range names {
		names[i] = strings.TrimPrefix(name, namespace+"/")
	}

	return names, nil
}

// recordRemoteHead writes refs/remotes/<remote>/HEAD when missing (a remote added by hand has none), for DefaultBase.
// An existing but stale ref is not refreshed: `git remote set-head <remote> --auto` fixes it once.
func recordRemoteHead(ctx context.Context, dir string) {
	remote := Remote(dir)
	if remote == "" || HasRef(dir, "refs/remotes/"+remote+"/HEAD") {
		return
	}

	_ = exec.CommandContext(ctx, "git", "-C", dir, "remote", "set-head", remote, "--auto").Run()
}

// Remote is "origin" when present, otherwise the first remote git lists, empty when there are none.
func Remote(dir string) string {
	out, err := exec.Command("git", "-C", dir, "remote").Output()
	if err != nil {
		return ""
	}

	remotes := strings.Fields(string(out))
	if len(remotes) == 0 {
		return ""
	}
	if slices.Contains(remotes, "origin") {
		return "origin"
	}

	return remotes[0]
}

// DefaultBase is the repository's default branch as a ref name: the remote's HEAD, then init.defaultBranch, then
// fallbackBranches. The remote-tracking ref is preferred over the local branch of the same name, which may be behind.
// Empty means nothing answered; fall back to HEAD.
func DefaultBase(dir string) string {
	remote := Remote(dir)

	if remote != "" {
		out, err := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "refs/remotes/"+remote+"/HEAD").Output()
		if name := strings.TrimSpace(string(out)); err == nil && name != "" {
			return name
		}
	}

	candidates := fallbackBranches
	if configured, err := exec.Command("git", "-C", dir, "config", "--get", "init.defaultBranch").Output(); err == nil {
		if name := strings.TrimSpace(string(configured)); name != "" {
			candidates = append([]string{name}, candidates...)
		}
	}

	for _, name := range candidates {
		if remote != "" && HasRef(dir, remote+"/"+name) {
			return remote + "/" + name
		}
		if HasRef(dir, "refs/heads/"+name) {
			return name
		}
	}

	return ""
}

// HasRef reports whether dir resolves name to a commit.
func HasRef(dir, name string) bool {
	if name == "" {
		return false
	}

	// ^{commit}: tags and branches answer alike, a directory of refs does not.
	err := exec.Command("git", "-C", dir, "rev-parse", "--verify", "--quiet", name+"^{commit}").Run()

	return err == nil
}

// AddWorktree creates a checkout of branch at path, branching from base when the branch does not exist yet. Herdr's
// own `worktree create` is not used: it returns before the directory exists.
func AddWorktree(dir, path, branch, base string) error {
	args := []string{"-C", dir, "worktree", "add"}
	if HasRef(dir, "refs/heads/"+branch) {
		args = append(args, "--", path, branch)
	} else {
		args = append(args, "-b", branch, "--", path)
		if base != "" {
			args = append(args, base)
		}
	}

	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	text := strings.TrimSpace(string(out))
	if text == "" {
		text = err.Error()
	}

	return fmt.Errorf("git worktree add: %s", text)
}

// Checkout is one worktree of a repository, as git lists them.
type Checkout struct {
	Path string
	// Branch is empty when detached.
	Branch string
	// Main is true for the primary, non-linked checkout.
	Main bool
}

// Worktrees lists a repository's checkouts.
func Worktrees(dir string) ([]Checkout, error) {
	out, err := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var (
		checkouts []Checkout
		current   Checkout
	)
	flush := func() {
		if current.Path != "" {
			checkouts = append(checkouts, current)
		}
		current = Checkout{}
	}

	for _, line := range strings.Split(string(out), "\n") {
		key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch key {
		case "worktree":
			flush()
			// Git prints the main checkout first; there is no field for it.
			current = Checkout{Path: value, Main: len(checkouts) == 0}
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		}
	}
	flush()

	return checkouts, nil
}

// CommonDir is the git common directory for a path inside a work tree (what Herdr's worktree API takes as --cwd);
// empty outside a repository.
func CommonDir(path string) string {
	out, err := exec.Command("git", "-C", path,
		"rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}
