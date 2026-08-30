package newworktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeed(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()

	write(t, filepath.Join(from, ".idea", "codeStyles", "Project.xml"), "styles")
	write(t, filepath.Join(from, ".idea", "vcs.xml"), "vcs")
	write(t, filepath.Join(from, ".run", "app.run.xml"), "run")
	// Deliberately not in the seed list: copying it makes the IDE treat every worktree as one project.
	write(t, filepath.Join(from, ".idea", "workspace.xml"), "workspace")
	// Already in the new checkout, so must not be overwritten.
	write(t, filepath.Join(to, ".idea", "vcs.xml"), "tracked")

	creator := Creator{Seed: defaultSeed}
	if err := creator.seed(from, to); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for _, test := range []struct{ path, want string }{
		{filepath.Join(".idea", "codeStyles", "Project.xml"), "styles"},
		{filepath.Join(".run", "app.run.xml"), "run"},
		{filepath.Join(".idea", "vcs.xml"), "tracked"},
	} {
		data, err := os.ReadFile(filepath.Join(to, test.path))
		if err != nil {
			t.Errorf("%s: %v", test.path, err)

			continue
		}
		if string(data) != test.want {
			t.Errorf("%s = %q, want %q", test.path, data, test.want)
		}
	}

	if _, err := os.Stat(filepath.Join(to, ".idea", "workspace.xml")); err == nil {
		t.Error("workspace.xml was copied, which makes every worktree one project")
	}
}

func TestSeedRefusesToClimbOut(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	write(t, filepath.Join(filepath.Dir(from), "outside.txt"), "secret")

	err := Creator{Seed: []string{"../outside.txt", "/etc/hostname"}}.seed(from, to)
	if err == nil {
		t.Fatal("seed accepted a path outside the checkout")
	}

	if _, err := os.Stat(filepath.Join(to, "outside.txt")); err == nil {
		t.Error("a path outside the checkout was copied in")
	}
}

// TestSeedWithNothingConfigured pins that an empty list turns seeding off.
func TestSeedWithNothingConfigured(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	write(t, filepath.Join(from, ".idea", "vcs.xml"), "vcs")

	if err := (Creator{Seed: []string{}}).seed(from, to); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(to, ".idea")); err == nil {
		t.Error("seeding was configured off and copied anyway")
	}
}

// TestSeedPathsDefault pins that an absent list takes the default and an empty one seeds nothing.
func TestSeedPathsDefault(t *testing.T) {
	if got := (Config{}).SeedPaths(); len(got) != len(defaultSeed) {
		t.Errorf("an absent list = %q, want the default", got)
	}
	if got := (Config{Seed: []string{}}).SeedPaths(); len(got) != 0 {
		t.Errorf("an empty list = %q, want nothing seeded", got)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
