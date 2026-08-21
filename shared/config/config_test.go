package config

import (
	"os"
	"path/filepath"
	"testing"
)

type settings struct {
	Roots []string `toml:"roots"`
	Depth int      `toml:"max_depth"`
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)

	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("roots = [\"~/projects\"]\nmax_depth = 2\n"), 0o644); err != nil {
		t.Fatalf("writing %s: %v", FileName, err)
	}

	var got settings
	loaded, err := Load(&got)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded != path {
		t.Errorf("Load reported %q, want %q", loaded, path)
	}
	if len(got.Roots) != 1 || got.Roots[0] != "~/projects" || got.Depth != 2 {
		t.Errorf("decoded %+v, want the file's contents", got)
	}
}

// TestLoadWithoutAFile pins that a missing file and a missing config directory are both silent.
func TestLoadWithoutAFile(t *testing.T) {
	t.Run("no file in the config directory", func(t *testing.T) {
		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", t.TempDir())

		var got settings
		loaded, err := Load(&got)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if loaded != "" {
			t.Fatalf("Load reported %q, want no file", loaded)
		}
	})

	t.Run("no config directory at all", func(t *testing.T) {
		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")

		var got settings
		loaded, err := Load(&got)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if loaded != "" {
			t.Fatalf("Load reported %q, want no file", loaded)
		}
	})
}

// TestLoadReportsAParseError pins that a typo is not silently replaced by the defaults.
func TestLoadReportsAParseError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)

	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("roots = [\n"), 0o644); err != nil {
		t.Fatalf("writing %s: %v", FileName, err)
	}

	var got settings
	if _, err := Load(&got); err == nil {
		t.Fatal("Load accepted an unparseable file")
	}
}

func TestHint(t *testing.T) {
	dir := t.TempDir()

	t.Run("the file that was read", func(t *testing.T) {
		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)

		if got := Hint("/somewhere/config.toml"); got != "/somewhere/config.toml" {
			t.Fatalf("Hint = %q, want the loaded path", got)
		}
	})

	t.Run("the file that should exist", func(t *testing.T) {
		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)

		if got, want := Hint(""), filepath.Join(dir, FileName); got != want {
			t.Fatalf("Hint = %q, want %q", got, want)
		}
	})

	t.Run("outside Herdr", func(t *testing.T) {
		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")

		if got := Hint(""); got != FileName {
			t.Fatalf("Hint = %q, want %q", got, FileName)
		}
	})
}
