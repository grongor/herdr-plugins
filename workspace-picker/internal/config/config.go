package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pluginconfig "github.com/grongor/herdr-plugins/shared/config"
	"github.com/grongor/herdr-plugins/shared/paths"
	"github.com/grongor/herdr-plugins/shared/tui"
)

// Config is the user configuration; every field is optional, defaults are documented in config.example.toml.
type Config struct {
	Roots    []string `toml:"roots"`
	MaxDepth int      `toml:"max_depth"`
	Ignore   []string `toml:"ignore"`
	// ShowDetached lists linked detached checkouts from the start; ctrl-t flips it for one run either way.
	ShowDetached bool `toml:"show_detached"`
	// ShowRemoteBranches lists remote branches from the start, fetching every repository as the picker opens; the
	// remote_branches key flips it for one run either way.
	ShowRemoteBranches bool `toml:"show_remote_branches"`

	Keys KeysConfig `toml:"keys"`

	// Path is the file the config was loaded from, empty when defaults were used.
	Path string `toml:"-"`
	// Shortcuts is Keys parsed, with the defaults filled in.
	Shortcuts Shortcuts `toml:"-"`
}

// KeysConfig is the list's shortcuts as written in the file: "ctrl-<letter>" or "alt-<character>", see tui.ParseKey.
// Empty keeps the default. Editing keys and enter, esc, tab are not shortcuts and stay where they are.
type KeysConfig struct {
	NewWorktree    string `toml:"new_worktree"`
	RemoteBranches string `toml:"remote_branches"`
	Detached       string `toml:"detached"`
}

// Shortcuts is the list's shortcuts as keystrokes.
type Shortcuts struct {
	NewWorktree    tui.Key
	RemoteBranches tui.Key
	Detached       tui.Key
}

var defaultShortcuts = Shortcuts{
	NewWorktree:    tui.Ctrl('g'),
	RemoteBranches: tui.Ctrl('r'),
	Detached:       tui.Ctrl('t'),
}

// parse fills in the defaults and refuses two actions on one key: the second would be unreachable.
func (k KeysConfig) parse() (Shortcuts, error) {
	shortcuts := defaultShortcuts

	for _, entry := range []struct {
		name string
		text string
		key  *tui.Key
	}{
		{"new_worktree", k.NewWorktree, &shortcuts.NewWorktree},
		{"remote_branches", k.RemoteBranches, &shortcuts.RemoteBranches},
		{"detached", k.Detached, &shortcuts.Detached},
	} {
		if entry.text == "" {
			continue
		}
		key, err := tui.ParseKey(entry.text)
		if err != nil {
			return shortcuts, fmt.Errorf("keys.%s: %w", entry.name, err)
		}
		*entry.key = key
	}

	seen := make(map[string]string, 3)
	for _, entry := range []struct{ name, key string }{
		{"new_worktree", shortcuts.NewWorktree.String()},
		{"remote_branches", shortcuts.RemoteBranches.String()},
		{"detached", shortcuts.Detached.String()},
	} {
		if other, taken := seen[entry.key]; taken {
			return shortcuts, fmt.Errorf("keys.%s and keys.%s are both %s", other, entry.name, entry.key)
		}
		seen[entry.key] = entry.name
	}

	return shortcuts, nil
}

var defaultIgnore = []string{
	".cache",
	".venv",
	"Library",
	"__pycache__",
	"build",
	"dist",
	"node_modules",
	"target",
	"vendor",
}

// candidateRoots are probed when the user has no config yet.
var candidateRoots = []string{"~/projects", "~/src", "~/code", "~/work", "~/dev", "~/git"}

const defaultMaxDepth = 4

// Load reads the plugin's config.toml and applies defaults; a missing file is not an error.
func Load() (*Config, error) {
	cfg := &Config{}

	path, err := pluginconfig.Load(cfg)
	if err != nil {
		return nil, err
	}
	cfg.Path = path

	if env := os.Getenv("HERDR_WORKSPACE_PICKER_ROOTS"); env != "" {
		cfg.Roots = strings.Split(env, string(os.PathListSeparator))
	}

	cfg.applyDefaults()

	if len(cfg.Roots) == 0 {
		return nil, fmt.Errorf("no project roots found; set roots in %s", pluginconfig.Hint(cfg.Path))
	}

	shortcuts, err := cfg.Keys.parse()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pluginconfig.Hint(cfg.Path), err)
	}
	cfg.Shortcuts = shortcuts

	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.MaxDepth <= 0 {
		c.MaxDepth = defaultMaxDepth
	}
	if c.Ignore == nil {
		c.Ignore = defaultIgnore
	}

	roots := c.Roots
	if len(roots) == 0 {
		roots = candidateRoots
	}

	seen := make(map[string]bool, len(roots))
	c.Roots = c.Roots[:0]
	for _, root := range roots {
		abs, err := resolve(root)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		c.Roots = append(c.Roots, abs)
	}
}

// IsIgnored reports whether a directory name should not be descended into.
func (c *Config) IsIgnored(name string) bool {
	for _, ignored := range c.Ignore {
		if strings.EqualFold(ignored, name) {
			return true
		}
	}

	return false
}

func resolve(path string) (string, error) {
	path = os.ExpandEnv(strings.TrimSpace(path))
	if path == "" {
		return "", errors.New("empty path")
	}

	// Before filepath.Abs, which would otherwise make a leading ~ a directory under the working directory.
	path, err := paths.ExpandHome(path)
	if err != nil {
		return "", err
	}

	return filepath.Abs(path)
}
