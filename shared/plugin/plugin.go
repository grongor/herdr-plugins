// Package plugin reads the HERDR_* environment Herdr hands a plugin command.
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const (
	envID           = "HERDR_PLUGIN_ID"
	envConfigDir    = "HERDR_PLUGIN_CONFIG_DIR"
	envStateDir     = "HERDR_PLUGIN_STATE_DIR"
	envEntrypointID = "HERDR_PLUGIN_ENTRYPOINT_ID"
	envContextJSON  = "HERDR_PLUGIN_CONTEXT_JSON"
)

// ID is the plugin id from the manifest; empty outside Herdr.
func ID() string { return os.Getenv(envID) }

// ConfigDir is the user's config directory for this plugin. Herdr creates it and never writes to it.
func ConfigDir() string { return os.Getenv(envConfigDir) }

// StateDir is where durable runtime state belongs; HERDR_PLUGIN_ROOT is a managed checkout a reinstall replaces.
func StateDir() string { return os.Getenv(envStateDir) }

// InPane reports whether this command is drawing a Herdr pane, which disappears the moment the process exits.
func InPane() bool { return os.Getenv(envEntrypointID) != "" }

// Worktree is what Herdr knows about the git checkout a workspace is backed by.
type Worktree struct {
	// RepoKey is the git common directory.
	RepoKey  string `json:"repo_key"`
	RepoName string `json:"repo_name"`
	// RepoRoot is the main checkout, or the repository itself when bare.
	RepoRoot     string `json:"repo_root"`
	CheckoutPath string `json:"checkout_path"`
	// IsLinkedWorktree is false for the main checkout; Herdr's worktree removal only applies to linked ones.
	IsLinkedWorktree bool `json:"is_linked_worktree"`
}

// Context is the subset of HERDR_PLUGIN_CONTEXT_JSON these plugins read.
type Context struct {
	// WorkspaceID is the workspace the command was invoked from; the only reliable source of it for a pane, see Load.
	WorkspaceID    string `json:"workspace_id"`
	WorkspaceLabel string `json:"workspace_label"`
	WorkspaceCwd   string `json:"workspace_cwd"`
	// Worktree is nil when the workspace is not backed by a git repository.
	Worktree *Worktree `json:"worktree"`
}

// ErrNoContext means the binary was run outside Herdr.
var ErrNoContext = errors.New(envContextJSON + " is not set: run this through Herdr, not directly")

// Load reads the invocation context. Do not read HERDR_WORKSPACE_ID instead: in a popup pane it names the popup's own
// host, an id no workspace listing returns (verified on Herdr 0.7.5, 0.8.0, 0.8.2). The context JSON is correct in both
// actions and popups.
func Load() (Context, error) {
	raw := os.Getenv(envContextJSON)
	if raw == "" {
		return Context{}, ErrNoContext
	}

	var ctx Context
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		return Context{}, fmt.Errorf("parse %s: %w", envContextJSON, err)
	}

	return ctx, nil
}

// Fail reports an error as name and exits non-zero. In a pane the message is held until the user presses enter,
// since the pane vanishes on exit and few users check `herdr plugin log list`.
func Fail(name string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)

	if InPane() {
		fmt.Fprint(os.Stderr, "\npress enter to close ")
		_, _ = fmt.Fscanln(os.Stdin)
	}

	os.Exit(1)
}
