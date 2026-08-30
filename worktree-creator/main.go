// Command worktree-creator shows the new-worktree dialog for the current workspace's repository.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grongor/herdr-plugins/shared/git"
	"github.com/grongor/herdr-plugins/shared/herdrcli"
	"github.com/grongor/herdr-plugins/shared/newworktree"
	"github.com/grongor/herdr-plugins/shared/plugin"
	"github.com/grongor/herdr-plugins/shared/worktree"
)

const version = "0.1.0"

const name = "worktree-creator"

// paneEntrypoint matches the [[panes]] id in herdr-plugin.toml.
const paneEntrypoint = "dialog"

func main() {
	command := "dialog"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	var err error
	switch command {
	case "dialog":
		err = runDialog()
	case "open-pane":
		err = herdrcli.OpenPane(paneEntrypoint)
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
	fmt.Print(`worktree-creator — Herdr new-worktree dialog

Usage:
  worktree-creator dialog      Show the dialog for the current workspace's repository
  worktree-creator open-pane   Ask Herdr to open the dialog pane (used by the action)
  worktree-creator version     Print the plugin version

Configuration is read from $HERDR_PLUGIN_CONFIG_DIR/config.toml.
`)
}

func runDialog() error {
	cfg, _, err := newworktree.LoadConfig()
	if err != nil {
		return err
	}

	ctx, err := plugin.Load()
	if err != nil {
		return err
	}

	repo, err := repoFrom(ctx)
	if err != nil {
		return err
	}

	return newworktree.Run(newworktree.Options{
		Repo:              repo,
		HerdrWorktreesDir: herdrcli.WorktreesDir(),
		Config:            cfg,
		Create:            newworktree.Creator{Seed: cfg.SeedPaths()}.Create,
	})
}

// repoFrom takes the repository from Herdr's invocation context, falling back to asking git about the workspace
// directory for a workspace Herdr does not recognise as a checkout (one opened inside a repository, say).
func repoFrom(ctx plugin.Context) (newworktree.Repo, error) {
	repo := newworktree.Repo{}

	if w := ctx.Worktree; w != nil {
		repo = newworktree.Repo{Name: w.RepoName, CommonDir: w.RepoKey, Anchor: w.RepoRoot}
	} else {
		if ctx.WorkspaceCwd == "" {
			return repo, errors.New("this workspace is not backed by a git repository")
		}

		common := git.CommonDir(ctx.WorkspaceCwd)
		if common == "" {
			return repo, fmt.Errorf("%s is not inside a git repository", ctx.WorkspaceCwd)
		}
		repo.CommonDir = common
	}

	checkouts, err := git.Worktrees(anyOf(repo.Anchor, repo.CommonDir))
	if err != nil {
		return repo, err
	}

	for _, checkout := range checkouts {
		if checkout.Main {
			// A bare repository has no main checkout; the anchor then stays the repository itself.
			if repo.Anchor == "" {
				repo.Anchor = checkout.Path
			}

			continue
		}
		repo.Checkouts = append(repo.Checkouts, worktree.Checkout{Path: checkout.Path, Branch: checkout.Branch})
	}

	if repo.Anchor == "" {
		repo.Anchor = repo.CommonDir
	}
	if repo.Name == "" {
		repo.Name = filepath.Base(repo.Anchor)
	}

	repo.Head, _, _ = git.HeadOf(repo.Anchor)

	return repo, nil
}

func anyOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
