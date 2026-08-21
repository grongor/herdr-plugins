package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pluginconfig "github.com/grongor/herdr-plugins/shared/config"
	"github.com/grongor/herdr-plugins/shared/paths"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/worktree"
)

// Config is the user configuration; every field is optional, defaults are documented in config.example.toml.
type Config struct {
	Roots    []string `toml:"roots"`
	MaxDepth int      `toml:"max_depth"`
	Ignore   []string `toml:"ignore"`
	// NewWorktreeLayout is proposed for a repository whose existing checkouts do not imply a layout.
	NewWorktreeLayout string `toml:"new_worktree_layout"`
	// ShowDetached lists linked detached checkouts from the start; ctrl-t flips it for one run either way.
	ShowDetached bool `toml:"show_detached"`

	// Path is the file the config was loaded from, empty when defaults were used.
	Path string `toml:"-"`
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

const defaultLayout = worktree.ModeSibling

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
	if !cfg.Layout().Valid() {
		return nil, fmt.Errorf(
			"new_worktree_layout %q in %s is not one of %s",
			cfg.NewWorktreeLayout, pluginconfig.Hint(cfg.Path), layoutNames(),
		)
	}

	return cfg, nil
}

func layoutNames() string {
	names := make([]string, 0, len(worktree.Modes))
	for _, mode := range worktree.Modes {
		names = append(names, string(mode))
	}

	return strings.Join(names, ", ")
}

func (c *Config) Layout() worktree.Mode { return worktree.Mode(c.NewWorktreeLayout) }

func (c *Config) applyDefaults() {
	if c.MaxDepth <= 0 {
		c.MaxDepth = defaultMaxDepth
	}
	if c.NewWorktreeLayout == "" {
		c.NewWorktreeLayout = string(defaultLayout)
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
