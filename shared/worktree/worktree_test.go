package worktree

import "testing"

const herdrDir = "/home/u/.herdr/worktrees"

func TestSlug(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   string
	}{
		{"plain", "main", "main"},
		{"slashes become dashes", "feature/PROJ-2512-add-retry-budget", "feature-PROJ-2512-add-retry-budget"},
		{"case is kept", "fix/ABC-1", "fix-ABC-1"},
		{"runs collapse", "a//b  c", "a-b-c"},
		{"trimmed", "/wip/", "wip"},
		{"nothing usable", "///", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Slug(test.branch); got != test.want {
				t.Fatalf("Slug(%q) = %q, want %q", test.branch, got, test.want)
			}
		})
	}
}

func TestForDetectsLayout(t *testing.T) {
	tests := []struct {
		name      string
		repo      Repo
		wantMode  Mode
		wantPath  string
		wantFound bool
	}{
		{
			name: "sibling directories carrying the repository name",
			repo: Repo{Anchor: "/p/lorem", Checkouts: []Checkout{
				{"/p/lorem-fix-issue", "fix-issue"}, {"/p/lorem-dev", "dev"},
			}},
			wantMode:  ModeSibling,
			wantPath:  "/p/lorem-thing",
			wantFound: true,
		},
		{
			name: "one directory holding every checkout",
			repo: Repo{Anchor: "/p/ipsum.git", Checkouts: []Checkout{
				{"/p/ipsum-worktrees/main", "main"}, {"/p/ipsum-worktrees/dev", "dev"},
			}},
			wantMode:  ModeShared,
			wantPath:  "/p/ipsum-worktrees/new-thing",
			wantFound: true,
		},
		{
			// Branches with unlike prefixes settle on the directory above all of them.
			name: "a shared directory that spells branches out as directories",
			repo: Repo{Anchor: "/p/dolor", Checkouts: []Checkout{
				{"/p/dolor-worktrees/proposal/di", "proposal/di"},
				{"/p/dolor-worktrees/fix/bug", "fix/bug"},
				{"/p/dolor-worktrees/main", "main"},
			}},
			wantMode:  ModeShared,
			wantPath:  "/p/dolor-worktrees/new/thing",
			wantFound: true,
		},
		{
			name: "a shared directory named by hand",
			repo: Repo{Anchor: "/p/dolor", Checkouts: []Checkout{
				{"/p/dolor-worktrees/perf-test", "feature/PROJ-2584-perf-test-sg"},
				{"/p/dolor-worktrees/main", "main"},
			}},
			wantMode:  ModeShared,
			wantPath:  "/p/dolor-worktrees/new-thing",
			wantFound: true,
		},
		{
			name: "a shared directory that flattens branches",
			repo: Repo{Anchor: "/p/dolor", Checkouts: []Checkout{
				{"/p/dolor-worktrees/proposal-di", "proposal/di"},
				{"/p/dolor-worktrees/main", "main"},
			}},
			wantMode:  ModeShared,
			wantPath:  "/p/dolor-worktrees/new-thing",
			wantFound: true,
		},
		{
			name:      "checkouts Herdr placed itself",
			repo:      Repo{Anchor: "/p/lorem", Checkouts: []Checkout{{herdrDir + "/lorem/dev", "dev"}}},
			wantMode:  ModeHerdr,
			wantPath:  "",
			wantFound: true,
		},
		{
			name:      "no linked checkouts falls back",
			repo:      Repo{Anchor: "/p/lorem"},
			wantMode:  ModeSibling,
			wantPath:  "/p/lorem-thing",
			wantFound: false,
		},
		{
			name: "the majority wins",
			repo: Repo{Anchor: "/p/lorem", Checkouts: []Checkout{
				{"/p/lorem-a", "a"}, {"/p/lorem-b", "b"}, {herdrDir + "/lorem/c", "c"},
			}},
			wantMode:  ModeSibling,
			wantPath:  "/p/lorem-thing",
			wantFound: true,
		},
		{
			name:      "a lone detached checkout falls back",
			repo:      Repo{Anchor: "/p/lorem", Checkouts: []Checkout{{"/tmp/scratch/abc1234", ""}}},
			wantMode:  ModeSibling,
			wantPath:  "/p/lorem-thing",
			wantFound: false,
		},
		{
			name: "detached strays do not move the shared directory",
			repo: Repo{Anchor: "/p/dolor.git", Checkouts: []Checkout{
				{"/p/dolor-worktrees/proposal/di", "proposal/di"},
				{"/p/dolor-worktrees/main", "main"},
				{"/tmp/scratch/src-pr", ""},
			}},
			wantMode:  ModeShared,
			wantPath:  "/p/dolor-worktrees/new/thing",
			wantFound: true,
		},
		{
			// A single-segment branch is no evidence against nesting, so it must not outvote the nested checkout.
			name: "one nested checkout among flat ones",
			repo: Repo{Anchor: "/p/lorem", Checkouts: []Checkout{
				{"/p/trees/proposal/di", "proposal/di"},
				{"/p/trees/topic", "topic"},
				{"/p/trees/dev", "dev"},
			}},
			wantMode:  ModeShared,
			wantPath:  "/p/trees/new/thing",
			wantFound: true,
		},
		{
			name: "flat checkouts only",
			repo: Repo{Anchor: "/p/lorem", Checkouts: []Checkout{
				{"/p/trees/topic", "topic"}, {"/p/trees/dev", "dev"},
			}},
			wantMode:  ModeShared,
			wantPath:  "/p/trees/new-thing",
			wantFound: true,
		},
		{
			// One checkout cannot say whether "proposal" is shared directory or branch; it is taken as directory.
			name: "a lone nested checkout reads as flat",
			repo: Repo{Anchor: "/p/dolor", Checkouts: []Checkout{
				{"/p/dolor-worktrees/proposal/di", "proposal/di"},
			}},
			wantMode:  ModeShared,
			wantPath:  "/p/dolor-worktrees/proposal/new-thing",
			wantFound: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			layout := For(test.repo, herdrDir, ModeSibling)

			if layout.Mode != test.wantMode {
				t.Fatalf("Mode = %q, want %q", layout.Mode, test.wantMode)
			}
			if layout.Detected != test.wantFound {
				t.Fatalf("Detected = %v, want %v", layout.Detected, test.wantFound)
			}
			if got := layout.Path("new/thing"); got != test.wantPath {
				t.Fatalf("Path = %q, want %q", got, test.wantPath)
			}
		})
	}
}

func TestForFallback(t *testing.T) {
	repo := Repo{Anchor: "/p/lorem"}

	tests := []struct {
		fallback Mode
		want     string
	}{
		{ModeSibling, "/p/lorem-dev"},
		{ModeShared, "/p/lorem-worktrees/dev"},
		{ModeHerdr, ""},
	}

	for _, test := range tests {
		t.Run(string(test.fallback), func(t *testing.T) {
			if got := For(repo, herdrDir, test.fallback).Path("dev"); got != test.want {
				t.Fatalf("Path = %q, want %q", got, test.want)
			}
		})
	}
}

// TestSwitchKeepsDetectedDir pins that Switch proposes the default shared directory, not the detected one.
func TestSwitchKeepsDetectedDir(t *testing.T) {
	repo := Repo{Anchor: "/p/lorem", Checkouts: []Checkout{{"/elsewhere/trees/dev", "dev"}}}

	detected := For(repo, herdrDir, ModeSibling)
	if got := detected.Path("wip"); got != "/elsewhere/trees/wip" {
		t.Fatalf("detected path = %q, want /elsewhere/trees/wip", got)
	}

	if got := Switch(repo, ModeShared).Path("wip"); got != "/p/lorem-worktrees/wip" {
		t.Fatalf("switched path = %q, want /p/lorem-worktrees/wip", got)
	}
}

func TestPathWithoutBranch(t *testing.T) {
	if got := Switch(Repo{Anchor: "/p/lorem"}, ModeSibling).Path(""); got != "" {
		t.Fatalf("Path(\"\") = %q, want empty so Herdr names it", got)
	}
}
