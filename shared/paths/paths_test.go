package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// os.UserHomeDir reads USERPROFILE on Windows.
	t.Setenv("USERPROFILE", home)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare tilde", "~", home},
		{"under the home directory", filepath.Join("~", "src", "repo"), filepath.Join(home, "src", "repo")},
		{"absolute paths are left alone", filepath.Join(home, "src"), filepath.Join(home, "src")},
		{"relative paths are left alone", filepath.Join("src", "repo"), filepath.Join("src", "repo")},
		{"a name starting with a tilde", "~work/repo", "~work/repo"},
		{"a tilde further in", filepath.Join("src", "~", "repo"), filepath.Join("src", "~", "repo")},
		{"empty", "", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ExpandHome(test.in)
			if err != nil {
				t.Fatalf("ExpandHome(%q) errored: %v", test.in, err)
			}
			if got != test.want {
				t.Fatalf("ExpandHome(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

// TestExpandHomeWithoutAHomeDirectory pins that the path comes back unexpanded alongside the error.
func TestExpandHomeWithoutAHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	// On some platforms os.UserHomeDir finds a home directory regardless of the environment.
	if _, err := os.UserHomeDir(); err == nil {
		t.Skip("the home directory is discoverable without the environment")
	}

	got, err := ExpandHome("~/src")
	if err == nil {
		t.Fatal("ExpandHome succeeded without a home directory")
	}
	if got != "~/src" {
		t.Fatalf("ExpandHome = %q, want the path unexpanded", got)
	}
}
