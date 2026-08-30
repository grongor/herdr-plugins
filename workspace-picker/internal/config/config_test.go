package config

import (
	"strings"
	"testing"

	"github.com/grongor/herdr-plugins/shared/tui"
)

func TestKeysParse(t *testing.T) {
	shortcuts, err := KeysConfig{}.parse()
	if err != nil {
		t.Fatal(err)
	}
	if shortcuts != defaultShortcuts {
		t.Fatalf("empty keys = %+v, want the defaults", shortcuts)
	}

	shortcuts, err = KeysConfig{Detached: "Alt+D"}.parse()
	if err != nil {
		t.Fatal(err)
	}
	if !shortcuts.Detached.Is(tui.Alt('d')) || !shortcuts.NewWorktree.Is(tui.Ctrl('g')) {
		t.Fatalf("keys = %+v, want alt-d for detached and the default elsewhere", shortcuts)
	}

	for _, test := range []struct {
		name string
		keys KeysConfig
		want string
	}{
		{"two actions on one key", KeysConfig{Detached: "ctrl-g"}, "keys.new_worktree and keys.detached are both ctrl-g"},
		{"a reserved letter", KeysConfig{NewWorktree: "ctrl-a"}, "keys.new_worktree: \"ctrl-a\" is home"},
		{"not a shortcut", KeysConfig{RemoteBranches: "f5"}, "keys.remote_branches: \"f5\" is not a shortcut"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.keys.parse()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
