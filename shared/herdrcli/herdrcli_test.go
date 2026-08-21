// The fake Herdr is a shell script.
//go:build unix

package herdrcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeScript logs argv one argument per line with "--" between calls, then answers with the files dropped next to it.
const fakeScript = `#!/bin/sh
# $0 is the absolute path Herdr was invoked by, so strip the last segment
# rather than shelling out to dirname.
dir=${0%/*}
{
	for arg in "$@"; do printf '%s\n' "$arg"; done
	printf -- '--\n'
} >>"$dir/argv.log"

if [ -f "$dir/fail-$1" ]; then
	cat "$dir/fail-$1"
	exit 1
fi

[ -f "$dir/response" ] && cat "$dir/response"
exit 0
`

const openResponse = `{"result":{"already_open":true,` +
	`"workspace":{"workspace_id":"ws-7","label":"lorem (dev)"},` +
	`"worktree":{"path":"/p/lorem-dev"}}}`

type fake struct{ dir string }

func newFake(t *testing.T) *fake {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "herdr")
	if err := os.WriteFile(bin, []byte(fakeScript), 0o755); err != nil {
		t.Fatalf("writing the fake: %v", err)
	}
	t.Setenv("HERDR_BIN_PATH", bin)

	return &fake{dir: dir}
}

func (f *fake) answer(t *testing.T, body string) {
	t.Helper()
	f.write(t, "response", body)
}

// failOn makes the fake print body and exit 1 for calls whose first argument is subcommand.
func (f *fake) failOn(t *testing.T, subcommand, body string) {
	t.Helper()
	f.write(t, "fail-"+subcommand, body)
}

func (f *fake) write(t *testing.T, name, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(f.dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func (f *fake) calls(t *testing.T) [][]string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(f.dir, "argv.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		t.Fatalf("reading the argv log: %v", err)
	}

	var calls [][]string
	var current []string
	for line := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "--" {
			calls = append(calls, current)
			current = nil

			continue
		}
		current = append(current, line)
	}

	return calls
}

// wantCalls asserts the exact argv of every call; there is no SDK to type-check the contract.
func (f *fake) wantCalls(t *testing.T, want ...[]string) {
	t.Helper()

	got := f.calls(t)
	if len(got) != len(want) {
		t.Fatalf("saw %d calls, want %d:\ngot  %v\nwant %v", len(got), len(want), got, want)
	}

	for i := range want {
		if strings.Join(got[i], "\x00") != strings.Join(want[i], "\x00") {
			t.Fatalf("call %d argv =\n  %q\nwant\n  %q", i, got[i], want[i])
		}
	}
}

func TestBin(t *testing.T) {
	t.Run("HERDR_BIN_PATH wins", func(t *testing.T) {
		t.Setenv("HERDR_BIN_PATH", "/opt/herdr/bin/herdr")

		if got := Bin(); got != "/opt/herdr/bin/herdr" {
			t.Fatalf("Bin() = %q, want the injected path", got)
		}
	})

	t.Run("falls back to PATH", func(t *testing.T) {
		t.Setenv("HERDR_BIN_PATH", "")

		if got := Bin(); got != "herdr" {
			t.Fatalf("Bin() = %q, want \"herdr\"", got)
		}
	})
}

func TestOpenCheckout(t *testing.T) {
	fake := newFake(t)
	fake.answer(t, openResponse)

	got, err := OpenCheckout("/p/lorem/.git", "/p/lorem-dev")
	if err != nil {
		t.Fatalf("OpenCheckout: %v", err)
	}

	fake.wantCalls(t, []string{
		"worktree", "open", "--cwd", "/p/lorem/.git", "--path", "/p/lorem-dev", "--focus", "--json",
	})

	want := OpenResult{WorkspaceID: "ws-7", Label: "lorem (dev)", Path: "/p/lorem-dev", AlreadyOpen: true}
	if got != want {
		t.Fatalf("OpenCheckout = %+v, want %+v", got, want)
	}
}

// TestOpenCheckoutFallsBack pins the fallback argv, in particular no --json: `workspace create` rejects the flag.
func TestOpenCheckoutFallsBack(t *testing.T) {
	fake := newFake(t)
	fake.failOn(t, "worktree", `{"error":{"code":"not_a_worktree","message":"not a worktree"}}`)
	fake.answer(t, `{"result":{"workspace":{"workspace_id":"ws-plain","label":"lorem"}}}`)

	got, err := OpenCheckout("/p/lorem/.git", "/p/lorem")
	if err != nil {
		t.Fatalf("OpenCheckout: %v", err)
	}

	fake.wantCalls(t,
		[]string{"worktree", "open", "--cwd", "/p/lorem/.git", "--path", "/p/lorem", "--focus", "--json"},
		[]string{"workspace", "create", "--cwd", "/p/lorem", "--focus"},
	)

	want := OpenResult{WorkspaceID: "ws-plain", Label: "lorem"}
	if got != want {
		t.Fatalf("OpenCheckout = %+v, want the fallback's result %+v", got, want)
	}
}

// TestOpenCheckoutBothFail pins that both failures survive into the error.
func TestOpenCheckoutBothFail(t *testing.T) {
	fake := newFake(t)
	fake.failOn(t, "worktree", `{"error":{"code":"not_a_worktree","message":"not a worktree"}}`)
	fake.failOn(t, "workspace", `{"error":{"code":"busy","message":"server is busy"}}`)

	_, err := OpenCheckout("/p/lorem/.git", "/p/lorem")
	if err == nil {
		t.Fatal("OpenCheckout succeeded, want an error")
	}

	for _, want := range []string{"not a worktree", "server is busy"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

func TestCreateWorktreeArgv(t *testing.T) {
	tests := []struct {
		name               string
		branch, base, path string
		want               []string
	}{
		{
			name: "nothing but the repository",
			want: []string{"worktree", "create", "--cwd", "/p/lorem/.git", "--focus", "--json"},
		},
		{
			name:   "a branch only",
			branch: "dev",
			want: []string{
				"worktree", "create", "--cwd", "/p/lorem/.git", "--branch", "dev", "--focus", "--json",
			},
		},
		{
			name: "a base only",
			base: "origin/main",
			want: []string{
				"worktree", "create", "--cwd", "/p/lorem/.git", "--base", "origin/main", "--focus", "--json",
			},
		},
		{
			name: "a path only",
			path: "/p/lorem-dev",
			want: []string{
				"worktree", "create", "--cwd", "/p/lorem/.git", "--path", "/p/lorem-dev", "--focus", "--json",
			},
		},
		{
			name:   "everything, in a stable order",
			branch: "dev",
			base:   "main",
			path:   "/p/lorem-dev",
			want: []string{
				"worktree", "create", "--cwd", "/p/lorem/.git",
				"--branch", "dev", "--base", "main", "--path", "/p/lorem-dev",
				"--focus", "--json",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFake(t)
			fake.answer(t, openResponse)

			if _, err := CreateWorktree("/p/lorem/.git", test.branch, test.base, test.path); err != nil {
				t.Fatalf("CreateWorktree: %v", err)
			}

			fake.wantCalls(t, test.want)
		})
	}
}

func TestOpenPane(t *testing.T) {
	t.Run("asks Herdr for the pane", func(t *testing.T) {
		fake := newFake(t)
		t.Setenv("HERDR_PLUGIN_ID", "grongor.workspace-picker")

		if err := OpenPane("picker"); err != nil {
			t.Fatalf("OpenPane: %v", err)
		}

		fake.wantCalls(t, []string{
			"plugin", "pane", "open",
			"--plugin", "grongor.workspace-picker",
			"--entrypoint", "picker", "--focus",
		})
	})

	t.Run("refuses without a plugin id", func(t *testing.T) {
		fake := newFake(t)
		t.Setenv("HERDR_PLUGIN_ID", "")

		if err := OpenPane("picker"); err == nil {
			t.Fatal("OpenPane succeeded outside Herdr, want an error")
		}

		fake.wantCalls(t)
	})
}

func TestWorktreesDir(t *testing.T) {
	tests := []struct {
		name string
		// config is written to <root>/config.toml; empty means no file.
		config string
		want   string
	}{
		{
			name:   "the configured directory",
			config: "[worktrees]\ndirectory = \"/srv/trees\"\n",
			want:   "/srv/trees",
		},
		{
			name:   "~ expands",
			config: "[worktrees]\ndirectory = \"~/trees\"\n",
			want:   "$HOME/trees",
		},
		{
			name:   "no file",
			config: "",
			want:   "$HOME/.herdr/worktrees",
		},
		{
			name:   "no section",
			config: "[ui]\ntheme = \"dark\"\n",
			want:   "$HOME/.herdr/worktrees",
		},
		{
			name:   "no key",
			config: "[worktrees]\nauto_prune = true\n",
			want:   "$HOME/.herdr/worktrees",
		},
		{
			name:   "unparseable config",
			config: "[worktrees\n",
			want:   "$HOME/.herdr/worktrees",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			root := t.TempDir()
			t.Setenv("HERDR_CONFIG_PATH", "")
			t.Setenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(root, "plugins", "config", "grongor.workspace-picker"))

			if test.config != "" {
				if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(test.config), 0o644); err != nil {
					t.Fatalf("writing config.toml: %v", err)
				}
			}

			want := strings.ReplaceAll(test.want, "$HOME", home)
			if got := WorktreesDir(); got != want {
				t.Fatalf("WorktreesDir() = %q, want %q", got, want)
			}
		})
	}
}

// TestWorktreesDirPrefersConfigPath pins that HERDR_CONFIG_PATH outranks the walk up from HERDR_PLUGIN_CONFIG_DIR.
func TestWorktreesDirPrefersConfigPath(t *testing.T) {
	write := func(t *testing.T, path, directory string) {
		t.Helper()

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		body := "[worktrees]\ndirectory = \"" + directory + "\"\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}

	t.Run("the injected file wins over the derived one", func(t *testing.T) {
		derived, injected := t.TempDir(), t.TempDir()
		write(t, filepath.Join(derived, configFileName), "/from/derived")
		write(t, filepath.Join(injected, configFileName), "/from/injected")

		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(derived, "plugins", "config", "grongor.workspace-picker"))
		t.Setenv("HERDR_CONFIG_PATH", filepath.Join(injected, configFileName))

		if got := WorktreesDir(); got != "/from/injected" {
			t.Fatalf("WorktreesDir() = %q, want /from/injected", got)
		}
	})

	t.Run("a directory is understood too", func(t *testing.T) {
		injected := t.TempDir()
		write(t, filepath.Join(injected, configFileName), "/from/directory")

		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")
		t.Setenv("HERDR_CONFIG_PATH", injected)

		if got := WorktreesDir(); got != "/from/directory" {
			t.Fatalf("WorktreesDir() = %q, want /from/directory", got)
		}
	})

	t.Run("the derived path still works without it", func(t *testing.T) {
		derived := t.TempDir()
		write(t, filepath.Join(derived, configFileName), "/from/derived")

		t.Setenv("HERDR_CONFIG_PATH", "")
		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(derived, "plugins", "config", "grongor.workspace-picker"))

		if got := WorktreesDir(); got != "/from/derived" {
			t.Fatalf("WorktreesDir() = %q, want /from/derived", got)
		}
	})

	// A missing HERDR_CONFIG_PATH file does not fall through to the derived path.
	t.Run("a path that does not exist falls back to the default", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		derived := t.TempDir()
		write(t, filepath.Join(derived, configFileName), "/from/derived")

		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(derived, "plugins", "config", "grongor.workspace-picker"))
		t.Setenv("HERDR_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.toml"))

		if got, want := WorktreesDir(), filepath.Join(home, ".herdr", "worktrees"); got != want {
			t.Fatalf("WorktreesDir() = %q, want %q", got, want)
		}
	})
}

func TestRemoveWorktreeArgv(t *testing.T) {
	tests := []struct {
		name  string
		force bool
		want  []string
	}{
		{
			name: "the ordinary ask",
			want: []string{"worktree", "remove", "--workspace", "w1P", "--json"},
		},
		{
			name:  "after Herdr refused a dirty checkout",
			force: true,
			want:  []string{"worktree", "remove", "--workspace", "w1P", "--force", "--json"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFake(t)
			fake.answer(t, `{"result":{}}`)

			if err := RemoveWorktree("w1P", test.force); err != nil {
				t.Fatalf("RemoveWorktree: %v", err)
			}

			fake.wantCalls(t, test.want)
		})
	}
}

// TestRemoveWorktreeReportsTheCode pins that the dirty refusal is identifiable by code, not by Herdr's prose.
func TestRemoveWorktreeReportsTheCode(t *testing.T) {
	fake := newFake(t)
	fake.failOn(t, "worktree", `{"error":{"code":"`+CodeDirtyWorktree+`","message":"worktree is dirty"}}`)

	err := RemoveWorktree("w1P", false)
	if err == nil {
		t.Fatal("RemoveWorktree succeeded, want the refusal")
	}
	if got := Code(err); got != CodeDirtyWorktree {
		t.Fatalf("Code = %q, want %q", got, CodeDirtyWorktree)
	}
	if !strings.Contains(err.Error(), "worktree is dirty") {
		t.Fatalf("error %q does not carry Herdr's message", err)
	}
}

// TestCodeOfAnythingElse pins that a failure with no envelope reports no code.
func TestCodeOfAnythingElse(t *testing.T) {
	fake := newFake(t)
	fake.failOn(t, "worktree", "herdr: command not found")

	err := RemoveWorktree("w1P", false)
	if err == nil {
		t.Fatal("RemoveWorktree succeeded, want the failure")
	}
	if got := Code(err); got != "" {
		t.Fatalf("Code = %q, want it empty", got)
	}
	if got := Code(errors.New("nothing to do with Herdr")); got != "" {
		t.Fatalf("Code of a plain error = %q, want it empty", got)
	}
}
