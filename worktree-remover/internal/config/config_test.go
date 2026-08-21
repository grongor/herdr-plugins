package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDeleteBranchDefault pins that "delete_branch = false" and an absent key are told apart.
func TestDeleteBranchDefault(t *testing.T) {
	tests := []struct {
		name string
		// Empty means no file at all.
		config string
		want   bool
	}{
		{"no file", "", true},
		{"no key", "# nothing here\n", true},
		{"turned off", "delete_branch = false\n", false},
		{"turned on", "delete_branch = true\n", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)

			if test.config != "" {
				path := filepath.Join(dir, "config.toml")
				if err := os.WriteFile(path, []byte(test.config), 0o644); err != nil {
					t.Fatalf("writing config.toml: %v", err)
				}
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := cfg.DeleteBranchDefault(); got != test.want {
				t.Fatalf("DeleteBranchDefault() = %v, want %v", got, test.want)
			}
		})
	}
}

// TestLoadRejectsBrokenConfig pins that a typo is reported rather than silently replaced by the defaults.
func TestLoadRejectsBrokenConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)

	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("delete_branch = yes\n"), 0o644); err != nil {
		t.Fatalf("writing config.toml: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted an unparseable config")
	}
}

// TestLoadWithoutHerdr covers running the binary by hand, with no config directory injected.
func TestLoadWithoutHerdr(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.DeleteBranchDefault() {
		t.Fatal("DeleteBranchDefault() = false with no config at all")
	}
}
