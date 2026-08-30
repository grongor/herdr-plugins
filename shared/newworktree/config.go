// Package newworktree is the new-worktree dialog shared by the creator and picker plugins. It renders into the area
// it is handed and never opens a surface of its own.
package newworktree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	pluginconfig "github.com/grongor/herdr-plugins/shared/config"
	"github.com/grongor/herdr-plugins/shared/issue"
	"github.com/grongor/herdr-plugins/shared/plugin"
	"github.com/grongor/herdr-plugins/shared/tui"
	"github.com/grongor/herdr-plugins/shared/worktree"
)

// PluginID is the plugin whose config.toml configures the dialog; the picker reads the same file rather than a copy.
const PluginID = "grongor.worktree-creator"

// Config is the whole of the worktree-creator's config file. Every field is optional.
type Config struct {
	// ShowRemoteBranches is whether the base list starts out including remote-tracking branches. Default false.
	ShowRemoteBranches *bool `toml:"show_remote_branches"`

	// Fetch is whether remote-tracking refs are updated before offering a base. Default true.
	Fetch *bool `toml:"fetch"`

	// FetchTimeout is a duration string ("5s", "1m"); past it the branches already on disk are offered.
	FetchTimeout string `toml:"fetch_timeout"`

	// Layout is proposed when existing checkouts imply nothing: "sibling", "shared" or "herdr".
	Layout string `toml:"layout"`

	// Seed lists paths copied from the main checkout into a new one, relative to the checkout root (gitignored editor
	// settings, run configurations). Missing paths are skipped: one list serves every repository.
	Seed []string `toml:"seed"`

	Issue IssueConfig `toml:"issue"`

	Keys KeysConfig `toml:"keys"`
}

// KeysConfig is the dialog's shortcuts, spelled as tui.ParseKey reads them. Empty keeps the default.
type KeysConfig struct {
	// RemoteBranches shows and hides remote-tracking branches in the base list. Default "ctrl-r".
	RemoteBranches string `toml:"remote_branches"`
}

var defaultRemoteKey = tui.Ctrl('r')

// RemoteKey is the shortcut that toggles remote branches; a misspelt one is caught by LoadConfig, so this falls back
// silently for a Config built without it.
func (c Config) RemoteKey() tui.Key {
	if key, err := tui.ParseKey(c.Keys.RemoteBranches); err == nil {
		return key
	}

	return defaultRemoteKey
}

// defaultSeed deliberately omits ".idea/workspace.xml" (copying it duplicates ProjectId, so the IDE treats every
// worktree as one project), "*.iml" and ".idea/modules.xml" (regenerated from the directory name).
var defaultSeed = []string{
	".idea/codeStyles",
	".idea/inspectionProfiles",
	".idea/runConfigurations",
	".idea/go.imports.xml",
	".idea/golinter.xml",
	".idea/vcs.xml",
	".run",
}

const defaultFetchTimeout = 5 * time.Second

func (c Config) ShowRemote() bool {
	return c.ShowRemoteBranches != nil && *c.ShowRemoteBranches
}

// FetchFor is zero when fetching is off. A misspelt timeout is caught by LoadConfig; the fallback is for a Config
// built without it.
func (c Config) FetchFor() time.Duration {
	if c.Fetch != nil && !*c.Fetch {
		return 0
	}

	if timeout, err := parseFetchTimeout(c.FetchTimeout); err == nil {
		return timeout
	}

	return defaultFetchTimeout
}

// parseFetchTimeout refuses non-positive values rather than reading them as "no timeout".
func parseFetchTimeout(text string) (time.Duration, error) {
	if text == "" {
		return defaultFetchTimeout, nil
	}

	timeout, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("fetch_timeout %q is not a duration such as \"5s\"", text)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("fetch_timeout %q must be positive; set fetch = false to skip fetching", text)
	}

	return timeout, nil
}

func (c Config) IssueEnabled() bool {
	return c.Issue.Enabled == nil || *c.Issue.Enabled
}

func (c Config) LayoutMode() worktree.Mode {
	if mode := worktree.Mode(c.Layout); mode.Valid() {
		return mode
	}

	return worktree.ModeSibling
}

// SeedPaths: an explicitly empty list turns seeding off; an absent one takes the default.
func (c Config) SeedPaths() []string {
	if c.Seed != nil {
		return c.Seed
	}

	return defaultSeed
}

// IssueConfig mirrors issue.Resolver; every empty field falls back to that package's defaults.
type IssueConfig struct {
	// Enabled defaults to true. Off, the Branch field takes branch names only and says so.
	Enabled *bool `toml:"enabled"`
	// FetchCommand prints the issue's title on stdout; "{key}" is replaced in every argument and ISSUE_KEY is set.
	FetchCommand []string `toml:"fetch_command"`
	// ProposeCommand prints the branch name on stdout; "{key}", "{title}", "{prompt}" and "{system_prompt}" are
	// replaced in every argument, and ISSUE_KEY, ISSUE_TITLE, BRANCH_PROMPT and BRANCH_SYSTEM_PROMPT are set.
	ProposeCommand []string `toml:"propose_command"`
	SystemPrompt   string   `toml:"system_prompt"`
	// Prompt is the template with "{title}" for what FetchCommand printed and "{key}" for the issue key.
	Prompt string `toml:"prompt"`
	// KeyCase is "lower" or "upper": how the key is spelt in the branch name, whichever way it was typed. Default
	// "lower".
	KeyCase string `toml:"key_case"`
}

func (c Config) Resolver() issue.Resolver {
	return issue.Resolver{
		Fetch:        c.Issue.FetchCommand,
		Propose:      c.Issue.ProposeCommand,
		SystemPrompt: c.Issue.SystemPrompt,
		Prompt:       c.Issue.Prompt,
		KeyCase:      issue.KeyCase(c.Issue.KeyCase),
	}
}

// LoadConfig reads and validates the worktree-creator's config file. Herdr keeps every plugin's config directory
// under one parent named by plugin id, so the file is found as a sibling of the caller's own directory. A missing
// file is not an error. The path returned is the file read, or the one that would have been, for error messages.
func LoadConfig() (Config, string, error) {
	var cfg Config

	path := configPath()
	if path == "" {
		return cfg, pluginconfig.FileName, nil
	}

	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return cfg, path, nil
	case err != nil:
		return cfg, path, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, path, fmt.Errorf("parse %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return cfg, path, fmt.Errorf("%s: %w", path, err)
	}

	return cfg, path, nil
}

// configPath is empty outside Herdr.
func configPath() string {
	own := plugin.ConfigDir()
	if own == "" {
		return ""
	}

	return filepath.Join(filepath.Dir(own), PluginID, pluginconfig.FileName)
}

func (c Config) validate() error {
	if c.Layout != "" && !worktree.Mode(c.Layout).Valid() {
		return fmt.Errorf("layout %q is not one of %s", c.Layout, layoutNames())
	}

	if c.Issue.KeyCase != "" && !issue.KeyCase(c.Issue.KeyCase).Valid() {
		return fmt.Errorf("issue.key_case %q is not one of %s", c.Issue.KeyCase, keyCaseNames())
	}

	if _, err := parseFetchTimeout(c.FetchTimeout); err != nil {
		return err
	}

	if c.Keys.RemoteBranches != "" {
		if _, err := tui.ParseKey(c.Keys.RemoteBranches); err != nil {
			return fmt.Errorf("keys.remote_branches: %w", err)
		}
	}

	return nil
}

func keyCaseNames() string {
	names := make([]string, 0, len(issue.KeyCases))
	for _, c := range issue.KeyCases {
		names = append(names, string(c))
	}

	return strings.Join(names, ", ")
}

func layoutNames() string {
	names := make([]string, 0, len(worktree.Modes))
	for _, mode := range worktree.Modes {
		names = append(names, string(mode))
	}

	return strings.Join(names, ", ")
}
