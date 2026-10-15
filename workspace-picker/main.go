// Command workspace-picker picks a git repository or worktree and opens it as a Herdr workspace.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/grongor/herdr-plugins/shared/herdrcli"
	"github.com/grongor/herdr-plugins/shared/newworktree"
	"github.com/grongor/herdr-plugins/shared/plugin"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/config"
	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
	"github.com/grongor/herdr-plugins/workspace-picker/internal/ui"
)

//go:embed herdr-plugin.toml
var manifest []byte

var version = plugin.Version(manifest)

// paneEntrypoint matches the [[panes]] id in herdr-plugin.toml.
const paneEntrypoint = "picker"

func main() {
	command := "pick"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	var err error
	switch command {
	case "pick":
		err = runPick()
	case "open-pane":
		err = herdrcli.OpenPane(paneEntrypoint)
	case "list":
		err = runList()
	case "version", "-v", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", command)
	}

	if err != nil {
		plugin.Fail("workspace-picker", err)
	}
}

func usage() {
	fmt.Print(`workspace-picker — Herdr workspace picker

Usage:
  workspace-picker pick        Show the picker and open the chosen checkout
  workspace-picker open-pane   Ask Herdr to open the picker pane (used by the action)
  workspace-picker list        Print discovered checkouts as TSV and exit
  workspace-picker version     Print the plugin version

Configuration is read from $HERDR_PLUGIN_CONFIG_DIR/config.toml.
Set HERDR_WORKSPACE_PICKER_ROOTS to override the roots for one run.
`)
}

func runPick() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	repos, err := reposFor(cfg)
	if err != nil {
		return err
	}

	// The ctrl-g dialog is the worktree-creator's and reads that plugin's config, whichever plugin opens it. Its fetch
	// settings also govern the fetch behind ctrl-r, so one file says how long a repository may take.
	dialog, _, err := newworktree.LoadConfig()
	if err != nil {
		return err
	}

	options := ui.Options{
		HerdrWorktreesDir: herdrcli.WorktreesDir(),
		NewWorktree:       dialog,
		ShowDetached:      cfg.ShowDetached,
		ShowRemote:        cfg.ShowRemoteBranches,
		Fetch:             dialog.FetchFor(),
		Keys: ui.Keys{
			NewWorktree: cfg.Shortcuts.NewWorktree,
			Remote:      cfg.Shortcuts.RemoteBranches,
			Detached:    cfg.Shortcuts.Detached,
			Folder:      cfg.Shortcuts.Folder,
		},
		Roots: cfg.Roots,
	}

	return ui.Run(repos, options, ui.Actions{
		Open: func(row ui.Row) error {
			var err error
			switch {
			case row.Worktree != nil:
				_, err = herdrcli.OpenCheckout(row.Repo.CommonDir, row.Worktree.Path)
			case row.CommonDir != "":
				_, err = herdrcli.OpenCheckout(row.CommonDir, row.Dir)
			default:
				_, err = herdrcli.OpenDirectory(row.Dir)
			}

			return err
		},
		Create: newworktree.Creator{Seed: dialog.SeedPaths()}.Create,
	})
}

func runList() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	repos, err := reposFor(cfg)
	if err != nil {
		return err
	}

	for _, row := range ui.BuildRows(repos) {
		kind := "bare"
		path := ""
		if row.Worktree != nil {
			kind = "worktree"
			if row.Worktree.Main {
				kind = "main"
			}
			path = row.Worktree.Path
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", kind, row.Repo.Name, row.Label, path)
	}

	return nil
}

func reposFor(cfg *config.Config) ([]discover.Repo, error) {
	repos := discover.Scan(cfg)
	if len(repos) == 0 {
		return nil, fmt.Errorf("no git repositories found under %s", strings.Join(cfg.Roots, ", "))
	}

	return repos, nil
}
