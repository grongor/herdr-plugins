// Command workspace-picker picks a git repository or worktree and opens it as a Herdr workspace.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/grongor/herdr-plugins/shared/herdrcli"
	"github.com/grongor/herdr-plugins/shared/plugin"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/config"
	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
	"github.com/grongor/herdr-plugins/workspace-picker/internal/ui"
)

const version = "0.1.0"

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

	rows, err := rowsFor(cfg)
	if err != nil {
		return err
	}

	options := ui.Options{
		HerdrWorktreesDir: herdrcli.WorktreesDir(),
		Layout:            cfg.Layout(),
		ShowDetached:      cfg.ShowDetached,
	}

	return ui.Run(rows, options, ui.Actions{
		Open: func(row ui.Row) error {
			_, err := herdrcli.OpenCheckout(row.Repo.CommonDir, row.Worktree.Path)

			return err
		},
		Create: func(req ui.Create) error {
			_, err := herdrcli.CreateWorktree(req.Repo.CommonDir, req.Branch, req.Base, req.Path)

			return err
		},
	})
}

func runList() error {
	rows, err := load()
	if err != nil {
		return err
	}

	for _, row := range rows {
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

func load() ([]ui.Row, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	return rowsFor(cfg)
}

func rowsFor(cfg *config.Config) ([]ui.Row, error) {
	repos := discover.Scan(cfg)
	if len(repos) == 0 {
		return nil, fmt.Errorf("no git repositories found under %s", strings.Join(cfg.Roots, ", "))
	}

	return ui.BuildRows(repos), nil
}
