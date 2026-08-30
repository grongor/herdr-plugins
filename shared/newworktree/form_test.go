package newworktree

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/grongor/herdr-plugins/shared/tui"
	"github.com/grongor/herdr-plugins/shared/worktree"
)

func lorem() Repo {
	return Repo{
		Name:      "org/lorem",
		CommonDir: "/p/lorem/.git",
		Anchor:    "/p/lorem",
		Head:      "main",
		Checkouts: []worktree.Checkout{{Path: "/p/lorem-topic", Branch: "proposal/di"}},
	}
}

// open stubs branch loading and applies its result, so a test never races the goroutine.
func open(t *testing.T, opts Options, branches BranchList) *Form {
	t.Helper()

	if opts.Repo.Anchor == "" && opts.Repo.CommonDir == "" {
		opts.Repo = lorem()
	}
	if opts.Create == nil {
		opts.Create = func(Request) error { return nil }
	}
	opts.LoadBranches = func(string, time.Duration) (BranchList, error) { return branches, nil }

	f := New(opts)
	<-f.Wake()
	f.drain()

	return f
}

func TestBasesOfferedInOrder(t *testing.T) {
	tests := []struct {
		name      string
		preferred string
		want      []string
	}{
		{"a caller's base keeps the front", "proposal/di", []string{"proposal/di", "origin/main", "main"}},
		{"without one, the default branch leads", "", []string{"origin/main", "main"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := open(t, Options{Base: test.preferred}, BranchList{Default: "origin/main"})

			if len(f.bases) != len(test.want) {
				t.Fatalf("bases = %q, want %q", f.bases, test.want)
			}
			for i, want := range test.want {
				if f.bases[i] != want {
					t.Fatalf("bases = %q, want %q", f.bases, test.want)
				}
			}
			if got := f.Request().Base; got != test.want[0] {
				t.Fatalf("base = %q, want the first choice %q", got, test.want[0])
			}
		})
	}
}

// TestDefaultBaseWaitsForTheFetch pins that HEAD is shown until the fetch lands, then the default branch replaces it.
func TestDefaultBaseWaitsForTheFetch(t *testing.T) {
	f := New(Options{
		Repo:         lorem(),
		Create:       func(Request) error { return nil },
		LoadBranches: func(string, time.Duration) (BranchList, error) { return BranchList{Default: "origin/main"}, nil },
	})

	if got := f.Request().Base; got != "main" {
		t.Fatalf("base before the fetch = %q, want the repository HEAD", got)
	}
	if !f.loading {
		t.Fatal("the dialog is not reporting the fetch it started")
	}

	<-f.Wake()
	f.drain()

	if got := f.Request().Base; got != "origin/main" {
		t.Fatalf("base after the fetch = %q, want the fetched default branch", got)
	}
}

// TestTypingOnBaseGoesThroughTheList: Base is a choice, so a keystroke opens the branch list with that character as
// the filter; enter on a filter nothing matches uses the text, and the fetch landing afterwards does not undo it.
func TestTypingOnBaseGoesThroughTheList(t *testing.T) {
	f := New(Options{
		Repo:         Repo{Name: "org/lorem", Anchor: "/p/lorem"},
		Create:       func(Request) error { return nil },
		LoadBranches: func(string, time.Duration) (BranchList, error) { return BranchList{Default: "origin/main"}, nil },
	})

	f.field = fieldBase
	for _, r := range "v1.2.3" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}

	if f.picker == nil {
		t.Fatal("typing on Base did not open the branch list")
	}
	if got := f.picker.query.String(); got != "v1.2.3" {
		t.Fatalf("filter = %q, want everything typed, first character included", got)
	}
	if len(f.picker.matches) != 0 {
		t.Fatalf("matches = %q, want none for a tag", names(f))
	}

	f.Handle(tui.Key{Kind: tui.KeyEnter})
	if f.picker != nil {
		t.Fatal("enter on an unmatched filter left the list up")
	}
	if got := f.Request().Base; got != "v1.2.3" {
		t.Fatalf("base = %q, want the filter text", got)
	}

	<-f.Wake()
	f.drain()

	if got := f.Request().Base; got != "v1.2.3" {
		t.Fatalf("base after the fetch = %q, want the chosen one kept", got)
	}
	if !slices.Contains(f.bases, "v1.2.3") {
		t.Fatalf("bases = %q, want the chosen ref among the choices", f.bases)
	}
}

// TestBaseIsAChoice: ←/→ on Base steps through the offered bases and never leaves free text there, while the same
// arrows in Branch move the caret.
func TestBaseIsAChoice(t *testing.T) {
	f := open(t, Options{Base: "proposal/di"}, BranchList{Default: "origin/main"})

	f.field = fieldBase
	f.Handle(tui.Key{Kind: tui.KeyLeft})
	if got := f.Request().Base; got != "main" {
		t.Fatalf("base after ← = %q, want the last choice", got)
	}
	f.Handle(tui.Key{Kind: tui.KeyRight})
	f.Handle(tui.Key{Kind: tui.KeyRight})
	if got := f.Request().Base; got != "origin/main" {
		t.Fatalf("base after ← → → = %q, want the second choice", got)
	}
	if f.picker != nil {
		t.Fatal("cycling opened the branch list")
	}

	f.field = fieldBranch
	for _, r := range "ab" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	f.Handle(tui.Key{Kind: tui.KeyLeft})
	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: 'x'})
	if got := f.Request().Branch; got != "axb" {
		t.Fatalf("branch = %q, want ← to have moved the caret, not chosen anything", got)
	}
	if got := f.Request().Base; got != "origin/main" {
		t.Fatalf("base = %q, want it untouched by an arrow in Branch", got)
	}
}

// TestBranchEditsAtTheCaret: word jumps, deletions and home/end all act where the caret is, and the path follows.
func TestBranchEditsAtTheCaret(t *testing.T) {
	f := open(t, Options{}, BranchList{})

	for _, r := range "fix/TICKET-1-x" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}

	f.Handle(tui.Key{Kind: tui.KeyLeft, Mod: tui.ModCtrl})
	f.Handle(tui.Key{Kind: tui.KeyLeft, Mod: tui.ModCtrl})
	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: 'y'})
	if got := f.Request().Branch; got != "fix/TICKET-y1-x" {
		t.Fatalf("branch = %q, want fix/TICKET-y1-x", got)
	}
	if got := f.Request().Path; got != "/p/lorem-TICKET-y1-x" {
		t.Fatalf("path = %q, want it reproposed from the edited branch", got)
	}

	// The caret is right after the y, so ctrl-w takes the y and then the word before it.
	f.Handle(tui.Ctrl('w'))
	f.Handle(tui.Ctrl('w'))
	if got := f.Request().Branch; got != "fix/1-x" {
		t.Fatalf("branch after ctrl-w twice = %q, want the two words before the caret gone", got)
	}

	f.Handle(tui.Key{Kind: tui.KeyHome})
	f.Handle(tui.Key{Kind: tui.KeyDelete})
	f.Handle(tui.Key{Kind: tui.KeyEnd})
	f.Handle(tui.Key{Kind: tui.KeyBackspace})
	if got := f.Request().Branch; got != "ix/1-" {
		t.Fatalf("branch after home, delete, end, backspace = %q, want ix/1-", got)
	}
}

// TestPathEditsAtTheCaret: an insertion in the middle of the path is a hand edit like any other.
func TestPathEditsAtTheCaret(t *testing.T) {
	f := open(t, Options{}, BranchList{})

	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: 'a'})
	f.field = fieldPath
	f.Handle(tui.Key{Kind: tui.KeyLeft, Mod: tui.ModCtrl})
	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: 'x'})
	if got := f.Request().Path; got != "/p/lorem-xa" {
		t.Fatalf("path = %q, want x before the last word", got)
	}

	f.field = fieldBranch
	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: 'b'})
	if got := f.Request().Path; got != "/p/lorem-xa" {
		t.Fatalf("path = %q, want the hand-edited one kept", got)
	}
}

// TestPathFollowsTheBranchUntilItIsTyped: a hand-written path stops tracking the branch, but a layout change
// still reproposes it.
func TestPathFollowsTheBranchUntilItIsTyped(t *testing.T) {
	f := open(t, Options{}, BranchList{})

	f.field = fieldBranch
	for _, r := range "wip" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	if got := f.Request().Path; got != "/p/lorem-wip" {
		t.Fatalf("path = %q, want it proposed from the branch", got)
	}

	f.field = fieldPath
	f.Handle(tui.Ctrl('u'))
	for _, r := range "/elsewhere/x" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}

	f.field = fieldBranch
	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: '2'})
	if got := f.Request().Path; got != "/elsewhere/x" {
		t.Fatalf("path = %q, want the hand-written one kept", got)
	}

	f.field = fieldLayout
	f.Handle(tui.Key{Kind: tui.KeyRight})
	if got := f.Request().Path; got != "/p/lorem-worktrees/wip2" {
		t.Fatalf("path = %q, want the newly chosen layout's proposal", got)
	}
}

func TestCycleOnlyTouchesTheFocusedField(t *testing.T) {
	f := open(t, Options{Base: "proposal/di"}, BranchList{})

	f.field = fieldLayout
	f.Handle(tui.Key{Kind: tui.KeyRight})

	if got := f.Request().Base; got != "proposal/di" {
		t.Errorf("base = %q, want it left alone while Layout has focus", got)
	}
	if worktree.Modes[f.mode] != worktree.ModeShared {
		t.Errorf("mode = %q, want shared", worktree.Modes[f.mode])
	}
}

func TestEnterResolvesAReferenceInsteadOfCreating(t *testing.T) {
	created := false
	f := open(t, Options{Create: func(Request) error {
		created = true

		return nil
	}}, BranchList{})

	f.opts.Config.Issue.FetchCommand = []string{"printf", "TICKET-2568: Kafka retries"}
	f.opts.Config.Issue.ProposeCommand = []string{"printf", "fix/TICKET-2568-drop-billing-dashboard"}

	for _, r := range "https://example.atlassian.net/browse/TICKET-2568" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}

	// The rule is about what the branch field holds, not where the cursor is.
	f.field = fieldPath
	if got := f.Handle(tui.Key{Kind: tui.KeyEnter}); got != ActionNone {
		t.Fatalf("enter on a reference = %v, want the dialog to stay open", got)
	}
	if created {
		t.Fatal("enter on an issue reference created a worktree")
	}

	<-f.Wake()
	f.drain()

	// The tracker's uppercase key, respelt in the default key_case.
	if got := f.Request().Branch; got != "fix/ticket-2568-drop-billing-dashboard" {
		t.Fatalf("branch = %q, want the proposal", got)
	}
	if f.field != fieldBranch {
		t.Fatal("the proposal is not where the cursor is, so it cannot be edited")
	}
	if got := f.Request().Path; got != "/p/lorem-ticket-2568-drop-billing-dashboard" {
		t.Fatalf("path = %q, want it reproposed from the resolved branch", got)
	}

	if got := f.Handle(tui.Key{Kind: tui.KeyEnter}); got != ActionDone {
		t.Fatalf("enter on the proposal = %v, want it created", got)
	}
	if !created {
		t.Fatal("the second enter did not create the worktree")
	}
}

func TestCreateFailureKeepsTheDialog(t *testing.T) {
	f := open(t, Options{Create: func(Request) error {
		return errors.New("fatal: a branch named 'wip' already exists")
	}}, BranchList{})

	for _, r := range "wip" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}

	if got := f.Handle(tui.Key{Kind: tui.KeyEnter}); got != ActionNone {
		t.Fatalf("enter = %v, want the dialog kept open", got)
	}

	status, failed := f.status()
	if !failed || status == "" {
		t.Fatalf("status = %q, failed = %v, want the refusal on screen", status, failed)
	}

	f.Handle(tui.Key{Kind: tui.KeyRune, Rune: '2'})
	if status, _ := f.status(); status != "" {
		t.Fatalf("status = %q, want it cleared once read", status)
	}
}

// TestFieldsFitTheWidth uses double-width text, where a budget spent in runes rather than columns shows up.
func TestFieldsFitTheWidth(t *testing.T) {
	f := open(t, Options{Repo: Repo{
		Name:      "org/設定",
		Anchor:    "/p/設定",
		Head:      "主ブランチ",
		Checkouts: []worktree.Checkout{{Path: "/p/設定-機能", Branch: "機能/ブランチ"}},
	}}, BranchList{Default: "origin/主ブランチ"})

	f.branch.Set("機能/新しいブランチ")
	f.proposePath()

	sc := &tui.Screen{}
	for _, width := range []int{20, 21, 40} {
		for field := range fieldCount {
			line, _ := f.fieldLine(sc, field, fieldsTop+field, width)
			if got := tui.Cells(line); got > width {
				t.Errorf("field %d is %d columns at width %d: %q", field, got, width, line)
			}
		}
	}
}

// TestIssueDetectionOff: a key is taken literally, nothing is fetched, and the placeholder does not invite one.
func TestIssueDetectionOff(t *testing.T) {
	off := false
	var created Request
	f := open(t, Options{
		Config: Config{Issue: IssueConfig{Enabled: &off, FetchCommand: []string{"false"}}},
		Create: func(req Request) error {
			created = req

			return nil
		},
	}, BranchList{})

	if got, _ := f.value(fieldBranch); got != placeholderBranch {
		t.Fatalf("empty branch field shows %q, want %q", got, placeholderBranch)
	}

	for _, r := range "TICKET-2568" {
		f.Handle(tui.Key{Kind: tui.KeyRune, Rune: r})
	}
	if got := f.Handle(tui.Key{Kind: tui.KeyEnter}); got != ActionDone {
		t.Fatalf("enter = %v, want the branch created as typed", got)
	}
	if created.Branch != "TICKET-2568" {
		t.Fatalf("created %q, want the key taken literally", created.Branch)
	}
}

// TestBranchPrefills pins that a caller's branch lands in the field with its path proposed, and that its base survives
// the fetch: the picker hands over a remote branch to check out from itself.
func TestBranchPrefills(t *testing.T) {
	f := open(t, Options{Branch: "fix/retry", Base: "origin/fix/retry"}, BranchList{Default: "origin/main"})

	req := f.Request()
	if req.Branch != "fix/retry" {
		t.Fatalf("branch = %q, want the prefilled one", req.Branch)
	}
	if req.Base != "origin/fix/retry" {
		t.Fatalf("base = %q, want the remote branch kept ahead of the default", req.Base)
	}
	if req.Path != "/p/lorem-retry" {
		t.Fatalf("path = %q, want it proposed from the branch", req.Path)
	}
}
