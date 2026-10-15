// Package herdrcli wraps the subset of the Herdr CLI these plugins drive; Herdr has no plugin SDK.
package herdrcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/grongor/herdr-plugins/shared/paths"
	"github.com/grongor/herdr-plugins/shared/plugin"
)

// defaultWorktreesDir matches Herdr's own default.
const defaultWorktreesDir = "~/.herdr/worktrees"

const configFileName = "config.toml"

// CodeDirtyWorktree is Herdr's refusal to remove a checkout with uncommitted or untracked files without --force.
const CodeDirtyWorktree = "dirty_worktree_requires_force"

// Bin is the Herdr binary: HERDR_BIN_PATH, or "herdr" when running a plugin by hand.
func Bin() string {
	if bin := os.Getenv("HERDR_BIN_PATH"); bin != "" {
		return bin
	}

	return "herdr"
}

// response is the envelope Herdr's CLI commands print on stdout.
type response struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Result struct {
		AlreadyOpen bool `json:"already_open"`
		Workspace   struct {
			ID    string `json:"workspace_id"`
			Label string `json:"label"`
		} `json:"workspace"`
		Worktree struct {
			Path string `json:"path"`
		} `json:"worktree"`
	} `json:"result"`
}

// Error is a failure Herdr described in its JSON envelope; match on Code, the message is prose that gets reworded.
type Error struct {
	// Command names the call by subcommand only.
	Command string
	// Code is empty when the command died without printing an envelope.
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Command + ": " + e.Message }

// Code reports the code Herdr gave for err, empty when the failure carried none.
func Code(err error) string {
	var reported *Error
	if errors.As(err, &reported) {
		return reported.Code
	}

	return ""
}

// OpenResult describes the workspace a call ended up on.
type OpenResult struct {
	WorkspaceID string
	Label       string
	Path        string
	AlreadyOpen bool
}

// OpenCheckout opens a git checkout as a focused worktree-aware workspace, focusing an existing one rather than
// duplicating it. Repositories Herdr does not recognise as worktrees fall back to a plain workspace.
func OpenCheckout(commonDir, checkout string) (OpenResult, error) {
	result, err := run("worktree", "open", "--cwd", commonDir, "--path", checkout, "--focus", "--json")
	if err == nil {
		return result, nil
	}

	fallback, fallbackErr := OpenDirectory(checkout)
	if fallbackErr != nil {
		return OpenResult{}, fmt.Errorf("%w (workspace create also failed: %w)", err, fallbackErr)
	}

	return fallback, nil
}

// OpenDirectory opens any directory as a focused plain workspace. It must not pass --json: `workspace create` rejects
// the flag with exit 2 on 0.7.5, 0.8.0 and 0.8.2 (the envelope is printed regardless).
func OpenDirectory(dir string) (OpenResult, error) {
	return run("workspace", "create", "--cwd", dir, "--focus")
}

// CreateWorktree creates a checkout and opens it as a focused workspace. Empty branch, base and path are omitted and
// left to Herdr: a generated branch, the current HEAD, its own worktrees directory.
func CreateWorktree(commonDir, branch, base, path string) (OpenResult, error) {
	args := []string{"worktree", "create", "--cwd", commonDir}
	for _, option := range []struct{ flag, value string }{
		{"--branch", branch},
		{"--base", base},
		{"--path", path},
	} {
		if option.value != "" {
			args = append(args, option.flag, option.value)
		}
	}
	args = append(args, "--focus", "--json")

	return run(args...)
}

// RemoveWorktree deletes a linked workspace's checkout and closes the workspace. A dirty checkout is refused with
// CodeDirtyWorktree unless force is set. The branch is left alone; delete it afterwards, git refuses while it is
// checked out.
func RemoveWorktree(workspaceID string, force bool) error {
	args := []string{"worktree", "remove", "--workspace", workspaceID}
	if force {
		args = append(args, "--force")
	}
	args = append(args, "--json")

	_, err := run(args...)

	return err
}

// WorktreesDir is where Herdr files worktrees when it picks the path itself. No CLI call reports it, so Herdr's own
// config.toml is read; if the key is renamed upstream this falls back to the default and Herdr-placed checkouts merely
// stop being recognised.
func WorktreesDir() string {
	dir := defaultWorktreesDir

	if path := configFile(); path != "" {
		var parsed struct {
			Worktrees struct {
				Directory string `toml:"directory"`
			} `toml:"worktrees"`
		}
		if data, err := os.ReadFile(path); err == nil {
			if err := toml.Unmarshal(data, &parsed); err == nil && parsed.Worktrees.Directory != "" {
				dir = parsed.Worktrees.Directory
			}
		}
	}

	// An unexpanded ~ only costs a failed comparison, never an operation.
	expanded, _ := paths.ExpandHome(dir)

	return expanded
}

// configFile locates Herdr's own config.toml. HERDR_CONFIG_PATH is the user's override, not injected by Herdr (env
// dumps on 0.7.5 and 0.8.2); madarco/agentbox and andrewchng/herdr-sessionizer treat it as the file path, a directory
// is accepted too.
func configFile() string {
	if path := os.Getenv("HERDR_CONFIG_PATH"); path != "" {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return filepath.Join(path, configFileName)
		}

		return path
	}

	if root := configRoot(); root != "" {
		return filepath.Join(root, configFileName)
	}

	return ""
}

// configRoot locates Herdr's configuration directory. The plugin config dir is "<root>/plugins/config/<plugin id>", a
// layout nothing promises.
func configRoot() string {
	if dir := plugin.ConfigDir(); dir != "" {
		return filepath.Dir(filepath.Dir(filepath.Dir(dir)))
	}

	config, err := os.UserConfigDir()
	if err != nil {
		return ""
	}

	return filepath.Join(config, "herdr")
}

// OpenPane opens one of the calling plugin's manifest panes; actions run without a terminal and cannot draw UI.
func OpenPane(entrypoint string) error {
	pluginID := plugin.ID()
	if pluginID == "" {
		return errors.New("HERDR_PLUGIN_ID is not set: run this through Herdr, not directly")
	}

	_, err := run("plugin", "pane", "open", "--plugin", pluginID, "--entrypoint", entrypoint, "--focus")

	return err
}

func run(args ...string) (OpenResult, error) {
	cmd := exec.Command(Bin(), args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))

	// Flags would push the actual reason off the end of a pane.
	name := "herdr " + strings.Join(args[:min(2, len(args))], " ")

	var parsed response
	if jsonErr := json.Unmarshal([]byte(text), &parsed); jsonErr == nil && parsed.Error != nil {
		return OpenResult{}, &Error{Command: name, Code: parsed.Error.Code, Message: parsed.Error.Message}
	}

	if err != nil {
		if text == "" {
			text = err.Error()
		}

		return OpenResult{}, &Error{Command: name, Message: text}
	}

	return OpenResult{
		WorkspaceID: parsed.Result.Workspace.ID,
		Label:       parsed.Result.Workspace.Label,
		Path:        parsed.Result.Worktree.Path,
		AlreadyOpen: parsed.Result.AlreadyOpen,
	}, nil
}
