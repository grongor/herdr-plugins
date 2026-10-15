package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grongor/herdr-plugins/shared/tui"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
)

// twoRepos is a repository with a checked-out topic branch and one bare with no checkouts; the loader answers each.
func twoRepos() []discover.Repo {
	return []discover.Repo{
		{Name: "org/a", Anchor: "/p/a", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/a", Branch: "main", Main: true},
			{Path: "/p/a-topic", Branch: "topic"},
		}},
		{Name: "org/b", Anchor: "/p/b", Head: "main", Worktrees: []discover.Worktree{
			{Path: "/p/b", Branch: "main", Main: true},
		}},
	}
}

func answers(remote map[string][]string, failing string) RemoteLoader {
	return func(dir string, _ time.Duration) ([]string, error) {
		if dir == failing {
			return remote[dir], errors.New("git fetch: could not resolve host")
		}

		return remote[dir], nil
	}
}

// settle applies every fetch result the way the input loop would, so a test never races the goroutines.
func settle(t *testing.T, picker *pickerState) {
	t.Helper()

	for picker.pending() > 0 {
		select {
		case apply := <-picker.results:
			apply()
		case <-time.After(5 * time.Second):
			t.Fatal("the fetch never finished")
		}
	}
}

func labels(picker *pickerState) []string {
	visible := make([]string, 0, len(picker.visible))
	for _, v := range picker.visible {
		visible = append(visible, picker.rows[v.index].Label)
	}

	return visible
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// TestRemoteBranchesFetchOnFirstToggle pins that ctrl-r fetches once, lists what is not checked out under each
// repository, and hides it again without refetching.
func TestRemoteBranchesFetchOnFirstToggle(t *testing.T) {
	var calls atomic.Int32
	load := answers(map[string][]string{
		"/p/a": {"origin/feature/new", "origin/topic", "origin/main"},
		"/p/b": {"origin/main", "origin/fix"},
	}, "")
	picker := newPicker(twoRepos(), Options{LoadRemote: func(dir string, fetch time.Duration) ([]string, error) {
		calls.Add(1)

		return load(dir, fetch)
	}}, Actions{})

	if picker.fetch != nil {
		t.Fatal("the picker fetched before being asked to")
	}
	if got := labels(picker); !equal(got, []string{"org/a", "topic", "org/b"}) {
		t.Fatalf("local view = %v", got)
	}

	picker.handle(tui.Ctrl('r'))
	if picker.pending() != 2 {
		t.Fatalf("pending = %d repositories, want both", picker.pending())
	}
	if got := picker.counter(); !strings.HasPrefix(got, "fetching 0/2…") {
		t.Fatalf("counter while fetching = %q", got)
	}
	settle(t, picker)

	// Checked-out branches stay their checkout's row; the rest keep git's order.
	want := []string{"org/a", "topic", "origin/feature/new", "org/b", "origin/fix"}
	if got := labels(picker); !equal(got, want) {
		t.Fatalf("remote view = %v, want %v", got, want)
	}
	if got := picker.counter(); got != "5/5" {
		t.Fatalf("counter = %q, want 5/5", got)
	}
	if picker.err != "" {
		t.Fatalf("err = %q, want none", picker.err)
	}

	remote := picker.rows[picker.visible[2].index]
	if remote.Remote != "origin/feature/new" || remote.Detail != remoteMarker || !remote.Selectable() {
		t.Fatalf("remote row = %+v", remote)
	}
	if remote.Branch() != "feature/new" {
		t.Fatalf("Branch() = %q, want feature/new", remote.Branch())
	}
	if got := picker.visible[2].prefix(picker.rows); got != prefixLast {
		t.Errorf("the remote row closes the tree with %q, want %q", got, prefixLast)
	}

	picker.handle(tui.Ctrl('r'))
	if got := labels(picker); !equal(got, []string{"org/a", "topic", "org/b"}) {
		t.Fatalf("re-hidden view = %v", got)
	}
	if got := picker.counter(); got != "3/3" {
		t.Fatalf("counter with remote hidden = %q, want 3/3", got)
	}

	picker.handle(tui.Ctrl('r'))
	if calls.Load() != 2 {
		t.Fatalf("the loader ran %d times, want once per repository", calls.Load())
	}
	if got := labels(picker); !equal(got, want) {
		t.Fatalf("re-shown view = %v, want %v", got, want)
	}
}

// TestFetchLandingKeepsTheCursor pins that rows arriving under an earlier repository do not move the cursor off the
// row it was on.
func TestFetchLandingKeepsTheCursor(t *testing.T) {
	picker := newPicker(twoRepos(), Options{LoadRemote: answers(map[string][]string{
		"/p/a": {"origin/one", "origin/two"},
	}, "")}, Actions{})

	picker.move(2)
	if got := picker.rows[picker.visible[picker.cursor].index].Label; got != "org/b" {
		t.Fatalf("cursor on %q, want org/b", got)
	}

	picker.handle(tui.Ctrl('r'))
	settle(t, picker)

	if got := picker.rows[picker.visible[picker.cursor].index].Label; got != "org/b" {
		t.Fatalf("cursor on %q after the fetch, want org/b still", got)
	}
}

// TestEnterOnRemoteBranchOpensTheDialog pins that a remote branch is checked out through the dialog, prefilled to
// create the branch from its remote counterpart, while ctrl-g there only offers it as the base.
func TestEnterOnRemoteBranchOpensTheDialog(t *testing.T) {
	opened := false
	picker := newPicker(twoRepos(), Options{
		LoadRemote: answers(map[string][]string{"/p/b": {"origin/fix/retry"}}, ""),
	}, Actions{Open: func(Row) error {
		opened = true

		return nil
	}})
	picker.handle(tui.Ctrl('r'))
	settle(t, picker)

	picker.query.Set("retry")
	picker.refresh()
	if got := labels(picker); !equal(got, []string{"org/b", "origin/fix/retry"}) {
		t.Fatalf("filtered view = %v", got)
	}
	picker.move(1)

	if done := picker.Handle(tui.Key{Kind: tui.KeyEnter}); done || opened {
		t.Fatal("enter on a remote branch opened a workspace, want the dialog")
	}
	if picker.form == nil {
		t.Fatal("enter on a remote branch did not open the dialog")
	}
	req := picker.form.Request()
	if req.Repo.Name != "org/b" || req.Branch != "fix/retry" || req.Base != "origin/fix/retry" {
		t.Fatalf("dialog request = %+v", req)
	}
	if !strings.HasSuffix(req.Path, "b-retry") {
		t.Fatalf("proposed path = %q, want a sibling named after the branch", req.Path)
	}

	picker.form = nil
	picker.handle(tui.Ctrl('g'))
	if picker.form == nil {
		t.Fatal("ctrl-g on a remote branch did not open the dialog")
	}
	req = picker.form.Request()
	if req.Branch != "" || req.Base != "origin/fix/retry" {
		t.Fatalf("ctrl-g request = %+v, want an empty branch based on the remote one", req)
	}
}

// TestFetchFailuresAreReportedOnce pins that a failing repository still lists what it has on disk and that the
// failures are summed up in one message when the last fetch lands.
func TestFetchFailuresAreReportedOnce(t *testing.T) {
	picker := newPicker(twoRepos(), Options{ShowRemote: true, LoadRemote: answers(map[string][]string{
		"/p/a": {"origin/stale"},
		"/p/b": {"origin/fix"},
	}, "/p/a")}, Actions{})

	if picker.fetch == nil {
		t.Fatal("show_remote_branches did not start the fetch")
	}
	settle(t, picker)

	want := []string{"org/a", "topic", "origin/stale", "org/b", "origin/fix"}
	if got := labels(picker); !equal(got, want) {
		t.Fatalf("view = %v, want %v", got, want)
	}
	if picker.err != "fetch failed in 1 of 2 repositories: git fetch: could not resolve host" {
		t.Fatalf("err = %q", picker.err)
	}

	picker.handle(tui.Key{Kind: tui.KeyDown})
	if picker.err != "" {
		t.Fatal("a keystroke did not clear the failure")
	}
}

// TestRemoteRowsRender pins the frame: the tag on the right, the fetch status beside the counter, and the footer
// naming the toggle's other state.
func TestRemoteRowsRender(t *testing.T) {
	picker := newPicker(twoRepos(), Options{LoadRemote: answers(map[string][]string{
		"/p/a": {"origin/feature/new"},
	}, "")}, Actions{})
	picker.handle(tui.Ctrl('r'))

	lines := tui.Capture(picker, 60, 8)
	if !strings.Contains(lines[0], "fetching 0/2…  3/3") {
		t.Fatalf("prompt line while fetching = %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "ctrl-r local only") {
		t.Fatalf("footer = %q, want it to offer local only", lines[len(lines)-1])
	}

	settle(t, picker)
	lines = tui.Capture(picker, 60, 8)
	if !strings.HasSuffix(strings.TrimRight(lines[0], " "), "4/4") {
		t.Fatalf("prompt line = %q, want the counter alone", lines[0])
	}
	remote := lines[4]
	if !strings.Contains(remote, prefixLast+"origin/feature/new") || !strings.HasSuffix(remote, remoteMarker) {
		t.Fatalf("remote row = %q", remote)
	}
	for i, line := range lines {
		if tui.Cells(line) > 60 {
			t.Errorf("line %d is %d columns wide", i, tui.Cells(line))
		}
	}
}

// waitFor polls a condition the fetch goroutines bring about.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestCursorRepositoryFetchesFirst pins that the repository under the cursor is fetched ahead of the queue, and that
// moving the cursor onto a repository still waiting starts it at once, past the bound.
func TestCursorRepositoryFetchesFirst(t *testing.T) {
	repos := make([]discover.Repo, 12)
	for i := range repos {
		name := fmt.Sprintf("org/%02d", i)
		path := fmt.Sprintf("/p/%02d", i)
		repos[i] = discover.Repo{Name: name, Anchor: path, Head: "main", Worktrees: []discover.Worktree{
			{Path: path, Branch: "main", Main: true},
		}}
	}

	var (
		mu      sync.Mutex
		started []string
	)
	release := make(chan struct{})
	picker := newPicker(repos, Options{LoadRemote: func(dir string, _ time.Duration) ([]string, error) {
		mu.Lock()
		started = append(started, dir)
		mu.Unlock()
		<-release

		return []string{"origin/x"}, nil
	}}, Actions{})
	startedSoFar := func() []string {
		mu.Lock()
		defer mu.Unlock()

		return append([]string(nil), started...)
	}

	// The cursor rests on org/11, past the first eight, when the fetch is asked for.
	for range 11 {
		picker.Handle(tui.Key{Kind: tui.KeyDown})
	}
	picker.Handle(tui.Ctrl('r'))

	// Goroutines register in no particular order, so the first batch is checked as a set.
	waitFor(t, "the first slots to fill", func() bool { return len(startedSoFar()) == fetchParallel })
	first := startedSoFar()
	if !slices.Contains(first, "/p/11") || slices.Contains(first, "/p/07") {
		t.Fatalf("started = %v, want the cursor's repository in the first batch instead of the eighth in order", first)
	}
	if picker.running != fetchParallel {
		t.Fatalf("running = %d, want the bound", picker.running)
	}

	// Typing narrows the list to org/07, whose fetch is waiting; it must not wait for a slot.
	for _, r := range "07" {
		picker.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	waitFor(t, "the cursor's repository to jump the queue", func() bool { return len(startedSoFar()) == fetchParallel+1 })
	if got := startedSoFar(); got[fetchParallel] != "/p/07" || picker.running != fetchParallel+1 {
		t.Fatalf("started = %v (running %d), want org/07 started past the bound", got, picker.running)
	}

	close(release)
	settle(t, picker)

	if got := len(startedSoFar()); got != len(repos) {
		t.Fatalf("started %d fetches, want every repository once", got)
	}
	for i := range repos {
		if len(picker.repos[i].Remote) != 1 {
			t.Fatalf("repository %d has no remote branches after the fetch", i)
		}
	}
}

// TestRemoteRowsMatchByRepositoryOrBranchName pins that a query naming the repository lists its remote branches with
// its checkouts, that a word has to occur in a branch name rather than merely be a subsequence of it, and that a fetch
// landing under a query adds only what the query names.
func TestRemoteRowsMatchByRepositoryOrBranchName(t *testing.T) {
	load := answers(map[string][]string{
		"/p/a": {"origin/feature/new"},
		// "feat" is a subsequence of this name but does not occur in it.
		"/p/b": {"origin/fix-eat"},
	}, "")

	picker := newPicker(twoRepos(), Options{ShowRemote: true, LoadRemote: load}, Actions{})
	settle(t, picker)

	for query, want := range map[string][]string{
		"":           {"org/a", "topic", "origin/feature/new", "org/b", "origin/fix-eat"},
		"org/a":      {"org/a", "topic", "origin/feature/new"},
		"feat":       {"org/a", "origin/feature/new"},
		"org/a feat": {"org/a", "origin/feature/new"},
		"org/b feat": {},
		"eat":        {"org/b", "origin/fix-eat", "org/a", "origin/feature/new"}, // best match first
		"remote":     {},
	} {
		picker.query.Set(query)
		picker.refresh()
		if got := labels(picker); !equal(got, want) {
			t.Errorf("query %q lists %v, want %v", query, got, want)
		}
	}

	picker = newPicker(twoRepos(), Options{LoadRemote: load}, Actions{})
	for _, r := range "feat" {
		picker.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	picker.Handle(tui.Ctrl('r'))
	settle(t, picker)
	if got := labels(picker); !equal(got, []string{"org/a", "origin/feature/new"}) {
		t.Fatalf("landing under a query lists %v, want only the branch the query names", got)
	}
}

// TestShortcutsAreConfigurable pins that the configured keys act, the defaults no longer do, a shortcut beats the line
// editor's use of the same key, and the footer names the configured key.
func TestShortcutsAreConfigurable(t *testing.T) {
	picker := newPicker(twoRepos(), Options{
		LoadRemote: answers(nil, ""),
		Keys:       Keys{NewWorktree: tui.Ctrl('w'), Remote: tui.Alt('r')},
	}, Actions{})

	picker.query.Set("abc")
	picker.handle(tui.Ctrl('w'))
	if picker.form == nil {
		t.Fatal("the configured new-worktree key did not open the dialog")
	}
	if picker.query.String() != "abc" {
		t.Fatalf("query = %q, want the shortcut to beat ctrl-w's word deletion", picker.query.String())
	}
	picker.form = nil

	picker.handle(tui.Ctrl('g'))
	if picker.form != nil {
		t.Fatal("the default new-worktree key still opens the dialog")
	}

	picker.handle(tui.Ctrl('r'))
	if picker.show.remote {
		t.Fatal("the default remote key still toggles")
	}
	picker.handle(tui.Alt('r'))
	if !picker.show.remote {
		t.Fatal("the configured remote key did not toggle")
	}
	settle(t, picker)

	// The unconfigured toggle keeps its default.
	picker.handle(tui.Ctrl('t'))
	if !picker.show.detached {
		t.Fatal("the default detached key stopped working")
	}

	footer, _ := (&tui.Screen{}).HintLine(hints(picker.opts.Keys.withDefaults(), true, false), 0, 120)
	for _, want := range []string{"ctrl-w new worktree", "alt-r local only", "ctrl-t detached"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q lacks %q", footer, want)
		}
	}
}
