// Command worktree-remover is Herdr's delete-worktree confirmation plus a checkbox that deletes the branch too.
package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/grongor/herdr-plugins/shared/git"
	"github.com/grongor/herdr-plugins/shared/herdrcli"
	"github.com/grongor/herdr-plugins/shared/plugin"
	"github.com/grongor/herdr-plugins/shared/proc"

	"github.com/grongor/herdr-plugins/worktree-remover/internal/config"
	"github.com/grongor/herdr-plugins/worktree-remover/internal/ui"
)

//go:embed herdr-plugin.toml
var manifest []byte

var version = plugin.Version(manifest)

const name = "worktree-remover"

// paneEntrypoint matches the [[panes]] id in herdr-plugin.toml.
const paneEntrypoint = "confirm"

// Exit statuses of "apply"; the dialog answers each differently.
const (
	exitFailed = 1
	// exitDirty is Herdr refusing a checkout with uncommitted or untracked files.
	exitDirty = 2
	// exitBranch is the checkout removed but its branch not deleted.
	exitBranch = 3
)

func main() {
	command := "confirm"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	var err error
	switch command {
	case "confirm":
		err = runConfirm()
	case "open-pane":
		err = herdrcli.OpenPane(paneEntrypoint)
	case "apply":
		// Not plugin.Fail: this runs with no terminal, and waiting for enter would hang forever.
		os.Exit(runApply(os.Args[2:]))
	case "version", "-v", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", command)
	}

	if err != nil {
		plugin.Fail(name, err)
	}
}

func usage() {
	fmt.Print(`worktree-remover — Herdr worktree removal, with the branch

Usage:
  worktree-remover confirm     Show the dialog for the current workspace's checkout
  worktree-remover open-pane   Ask Herdr to open the dialog pane (used by the action)
  worktree-remover apply ...   Do the removal; the dialog spawns this, see the README
  worktree-remover version     Print the plugin version

Configuration is read from $HERDR_PLUGIN_CONFIG_DIR/config.toml.
`)
}

func runConfirm() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, err := plugin.Load()
	if err != nil {
		return err
	}

	worktree := ctx.Worktree
	switch {
	case worktree == nil:
		return errors.New("this workspace is not backed by a git repository")
	case !worktree.IsLinkedWorktree:
		// Herdr would refuse this too ("workspace is not a linked worktree checkout"); refusing here avoids a dialog
		// offering something that cannot happen.
		return fmt.Errorf(
			"%s is the repository's main checkout, not a worktree — there is nothing to remove",
			worktree.CheckoutPath,
		)
	}

	branch, err := checkoutBranch(worktree.CheckoutPath)
	if err != nil {
		return err
	}

	// Read up front so the loss warning is on the first frame. Unreadable reads as clean; Herdr refusing the removal
	// still escalates the dialog.
	dirty, _ := git.IsDirty(worktree.CheckoutPath)

	target := ui.Target{Repo: worktree.RepoName, Branch: branch, Checkout: worktree.CheckoutPath, Dirty: dirty}

	return ui.Run(target, cfg.DeleteBranchDefault(), ui.Actions{
		Remove: func(req ui.Request) error {
			return apply(ctx.WorkspaceID, worktree.RepoRoot, branch, req)
		},
	})
}

// checkoutBranch is empty for a detached checkout. The invocation context does not carry the branch; HEAD is a file,
// not a subprocess. An unreadable HEAD is refused rather than shown as detached: it is what a checkout moved or deleted
// behind Herdr's back looks like, and the dialog would offer to remove a directory that is not there.
func checkoutBranch(checkout string) (string, error) {
	branch, _, detached := git.HeadOf(checkout)
	if branch == "" && !detached {
		return "", fmt.Errorf("cannot read the checkout at %s: was it moved or deleted? This workspace still points there",
			checkout)
	}

	return branch, nil
}

// apply runs the removal in a copy of this binary in its own session and waits for it.
//
// Removing the checkout closes the workspace and its pane, and Herdr then kills the pane's process (SIGHUP, SIGHUP,
// SIGTERM on 0.7.5, measured) before the branch is deleted. A new process group does not help — the kill targets the
// process, not the terminal — so the child needs a session of its own. Waiting is safe: everything that can fail does
// so before anything closes, so failures reach a dialog still on screen.
func apply(workspaceID, repoRoot, branch string, req ui.Request) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate this binary to run the removal: %w", err)
	}

	args := []string{"apply", "--workspace", workspaceID, "--repo", repoRoot}
	if req.DeleteBranch && branch != "" {
		args = append(args, "--branch", branch)
	}
	if req.Force {
		args = append(args, "--force")
	}

	cmd := exec.Command(self, args...)
	proc.Detach(cmd)

	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return fmt.Errorf("cannot run the removal: %w", err)
	}

	said := errors.New(reason(string(out)))
	switch exit.ExitCode() {
	case exitDirty:
		return ui.ErrNeedsForce
	case exitBranch:
		return fmt.Errorf("%w, but its branch was not: %w", ui.ErrRemoved, said)
	default:
		return said
	}
}

// runApply does the removal and reports which half failed as its exit status.
func runApply(args []string) int {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	workspace := flags.String("workspace", "", "the Herdr workspace whose checkout to remove")
	repo := flags.String("repo", "", "the repository the branch lives in")
	branch := flags.String("branch", "", "the branch to delete once the checkout is gone; empty keeps it")
	force := flags.Bool("force", false, "remove a checkout with uncommitted or untracked files")

	if err := flags.Parse(args); err != nil {
		return exitFailed
	}
	if *workspace == "" {
		fmt.Fprintln(os.Stderr, "apply: --workspace is required")

		return exitFailed
	}

	if err := herdrcli.RemoveWorktree(*workspace, *force); err != nil {
		fmt.Fprintln(os.Stderr, err)

		if herdrcli.Code(err) == herdrcli.CodeDirtyWorktree {
			return exitDirty
		}

		return exitFailed
	}

	if *branch == "" {
		return 0
	}

	// After the removal: git refuses to delete a branch that is still checked out.
	if err := git.DeleteBranch(*repo, *branch); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return exitBranch
	}

	return 0
}

// reason is the first non-blank line of the child's output.
func reason(out string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}

	return "the removal failed without saying why"
}
