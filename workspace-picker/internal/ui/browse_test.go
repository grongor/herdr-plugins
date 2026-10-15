package ui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/grongor/herdr-plugins/shared/tui"
)

func TestPathQuery(t *testing.T) {
	for _, test := range []struct {
		query, base, leaf string
		ok                bool
	}{
		{"~", "~/", "", true},
		{"~/", "~/", "", true},
		{"~/src/fo", "~/src/", "fo", true},
		{"/tmp/", "/tmp/", "", true},
		{"/", "/", "", true},
		{"fix/PROJ-1", "", "", false},
		{"~work", "", "", false},
		{"store 2478", "", "", false},
	} {
		base, leaf, ok := pathQuery(test.query)
		if base != test.base || leaf != test.leaf || ok != test.ok {
			t.Errorf("pathQuery(%q) = %q, %q, %v; want %q, %q, %v",
				test.query, base, leaf, ok, test.base, test.leaf, test.ok)
		}
	}
}

// folders lays out a home directory: plain folders, a hidden one, a file, and a checkout of its own.
func folders(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, dir := range []string{"notes", "nixos/hosts", ".config", "repo/.git/objects", "repo/.git/refs"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for file, content := range map[string]string{
		"readme.txt":     "",
		"repo/.git/HEAD": "ref: refs/heads/main\n",
	} {
		if err := os.WriteFile(filepath.Join(home, file), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return home
}

func browsing(t *testing.T, query string) *pickerState {
	t.Helper()

	picker := &pickerState{rows: BuildRows(twoRepos())}
	picker.query.Set(query)
	picker.refresh()

	return picker
}

func TestBrowseListsTheFolderThenItsSubfolders(t *testing.T) {
	home := folders(t)

	picker := browsing(t, "~/")
	if got, want := labels(picker), []string{"~/", "nixos/", "notes/", "repo/"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}

	row := picker.selected()
	if row == nil || row.Dir != home {
		t.Fatalf("selected = %+v, want the home directory itself", row)
	}

	repo := picker.rows[picker.visible[3].index]
	if repo.CommonDir != filepath.Join(home, "repo", ".git") || repo.Detail != gitMarker {
		t.Fatalf("repo row = %+v, want it recognised as a checkout", repo)
	}
}

func TestBrowseFiltersByThePartialName(t *testing.T) {
	folders(t)

	if got, want := labels(browsing(t, "~/no")), []string{"notes/"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if got, want := labels(browsing(t, "~/.co")), []string{".config/"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want the hidden folder once the name starts with a dot", got)
	}
	if got := labels(browsing(t, "~/missing/")); len(got) != 0 {
		t.Fatalf("rows = %v, want none for a folder that does not exist", got)
	}
}

// TestBrowseShowsARepositoryWithItsCheckouts pins that a path to a repository lists it as the repository list would,
// so it opens as a checkout and ctrl-g works on it.
func TestBrowseShowsARepositoryWithItsCheckouts(t *testing.T) {
	home := folders(t)

	picker := browsing(t, "~/repo/")
	row := picker.selected()
	if row == nil || row.Worktree == nil || !row.Worktree.Main || row.Repo.CommonDir != filepath.Join(home, "repo", ".git") {
		t.Fatalf("selected = %+v, want the repository's main checkout", row)
	}

	picker.handle(tui.Ctrl('g'))
	if picker.form == nil {
		t.Fatal("ctrl-g did not open the dialog on a repository found by path")
	}
}

func TestTabCompletesAndCtrlOToggles(t *testing.T) {
	folders(t)

	picker := &pickerState{rows: BuildRows(twoRepos())}
	picker.refresh()

	picker.handle(tui.Ctrl('o'))
	if !picker.browsing || picker.query.String() != "~/" {
		t.Fatalf("ctrl-o: browsing %v, query %q; want a path query from home", picker.browsing, picker.query.String())
	}

	for _, r := range "nix" {
		picker.handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	picker.handle(tui.Key{Kind: tui.KeyTab})
	if got := picker.query.String(); got != "~/nixos/" {
		t.Fatalf("query after tab = %q, want the folder completed", got)
	}
	if got, want := labels(picker), []string{"~/nixos/", "hosts/"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}

	// Tab on the folder itself has nothing to complete.
	picker.handle(tui.Key{Kind: tui.KeyTab})
	if got := picker.query.String(); got != "~/nixos/" {
		t.Fatalf("query after tab on the folder itself = %q", got)
	}

	picker.handle(tui.Ctrl('o'))
	if picker.browsing || picker.query.String() != "" {
		t.Fatalf("second ctrl-o: browsing %v, query %q; want the repositories back", picker.browsing, picker.query.String())
	}
	if got, want := labels(picker), []string{"org/a", "topic", "org/b"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want the repositories back: %v", got, want)
	}
}

func TestEnterOpensTheFolder(t *testing.T) {
	home := folders(t)

	picker := browsing(t, "~/no")
	var opened Row
	picker.actions = Actions{Open: func(row Row) error {
		opened = row

		return nil
	}}

	if !picker.Handle(tui.Key{Kind: tui.KeyEnter}) {
		t.Fatal("enter did not finish the picker")
	}
	if opened.Dir != filepath.Join(home, "notes") {
		t.Fatalf("opened %+v, want the notes folder", opened)
	}
}

// TestFetchLandingWhileBrowsing pins that remote branches arriving during a path query update the repositories, not
// the rows on screen, and are there once the query is cleared.
func TestFetchLandingWhileBrowsing(t *testing.T) {
	folders(t)

	picker := newPicker(twoRepos(), Options{
		LoadRemote: answers(map[string][]string{"/p/a": {"origin/feature"}}, ""),
	}, Actions{})
	picker.query.Set("~/")
	picker.refresh()

	picker.handle(tui.Ctrl('r'))
	settle(t, picker)

	if got, want := labels(picker), []string{"~/", "nixos/", "notes/", "repo/"}; !slices.Equal(got, want) {
		t.Fatalf("rows while browsing = %v, want %v", got, want)
	}

	picker.handle(tui.Ctrl('u'))
	if got := labels(picker); !slices.Contains(got, "origin/feature") {
		t.Fatalf("rows after clearing = %v, want the fetched branch listed", got)
	}
}
