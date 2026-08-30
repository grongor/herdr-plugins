package newworktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grongor/herdr-plugins/shared/tui"
)

// TestLoadConfigReadsTheCreatorsFile pins that whichever plugin asks, the creator's config.toml is read.
func TestLoadConfigReadsTheCreatorsFile(t *testing.T) {
	root := t.TempDir()
	creator := filepath.Join(root, PluginID)
	if err := os.MkdirAll(creator, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creator, "config.toml"),
		[]byte("fetch_timeout = \"250ms\"\n[issue]\nenabled = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, asker := range []string{PluginID, "grongor.workspace-picker"} {
		t.Run(asker, func(t *testing.T) {
			t.Setenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(root, asker))

			cfg, path, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if path != filepath.Join(creator, "config.toml") {
				t.Fatalf("path = %q, want the creator's file", path)
			}
			if got := cfg.FetchFor(); got != 250*time.Millisecond {
				t.Fatalf("FetchFor = %s, want the configured timeout", got)
			}
			if cfg.IssueEnabled() {
				t.Fatal("issue detection is on despite enabled = false")
			}
		})
	}
}

// TestLoadConfigWithoutAFile pins that a missing file yields defaults, not an error.
func TestLoadConfigWithoutAFile(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(t.TempDir(), "grongor.workspace-picker"))

	cfg, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.FetchFor(); got != defaultFetchTimeout {
		t.Fatalf("FetchFor = %s, want the default", got)
	}
	if !cfg.IssueEnabled() {
		t.Fatal("issue detection is off by default")
	}
}

// TestLoadConfigRefusesBadSettings pins that bad values are reported at load time, naming the file.
func TestLoadConfigRefusesBadSettings(t *testing.T) {
	for name, body := range map[string]string{
		"a timeout that is not a duration": "fetch_timeout = \"soon\"",
		"a timeout of zero":                "fetch_timeout = \"0s\"",
		"an unknown layout":                "layout = \"stacked\"",
		"an unknown key case":              "[issue]\nkey_case = \"title\"",
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), PluginID)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)

			_, _, err := LoadConfig()
			if err == nil {
				t.Fatal("LoadConfig accepted the file")
			}
			if !strings.Contains(err.Error(), "config.toml") {
				t.Fatalf("error = %q, want it to name the file", err)
			}
		})
	}
}

func TestFetchForHonoursFetchOff(t *testing.T) {
	off := false
	if got := (Config{Fetch: &off, FetchTimeout: "1m"}).FetchFor(); got != 0 {
		t.Fatalf("FetchFor with fetch = false = %s, want 0", got)
	}
}

// TestRemoteKeyIsConfigurable pins that keys.remote_branches replaces ctrl-r in both the form and its branch list, and
// that a key the terminal owns is refused with what it does.
func TestRemoteKeyIsConfigurable(t *testing.T) {
	root := t.TempDir()
	creator := filepath.Join(root, PluginID)
	if err := os.MkdirAll(creator, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", creator)
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(creator, "config.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("[keys]\nremote_branches = \"ctrl-m\"\n")
	if _, _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "keys.remote_branches: \"ctrl-m\" is enter") {
		t.Fatalf("error = %v, want the reserved key refused", err)
	}

	write("[keys]\nremote_branches = \"alt-r\"\n")
	cfg, _, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RemoteKey().Is(tui.Alt('r')) {
		t.Fatalf("RemoteKey = %v, want alt-r", cfg.RemoteKey())
	}

	f := open(t, Options{Config: cfg}, BranchList{Local: []string{"main"}, Remote: []string{"origin/topic"}})
	f.Handle(tui.Ctrl('r'))
	if f.showRemote {
		t.Fatal("ctrl-r still toggles remote branches")
	}
	f.Handle(tui.Alt('r'))
	if !f.showRemote {
		t.Fatal("alt-r does not toggle remote branches")
	}

	f.field = fieldBase
	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: ' '})
	f.Handle(tui.Alt('r'))
	if f.showRemote || len(f.picker.matches) != 1 {
		t.Fatalf("alt-r in the list: showRemote = %v, matches = %d, want local only", f.showRemote, len(f.picker.matches))
	}
	if hint := f.pickerHints()[1].Text; hint != "alt-r remote" {
		t.Fatalf("hint = %q, want alt-r remote", hint)
	}
}
