package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/grongor/herdr-plugins/shared/tui"
)

const (
	testWidth  = 60
	testHeight = 16
)

func target() Target {
	return Target{
		Repo:     "acme/storefront",
		Branch:   "feature/PROJ-2478",
		Checkout: "/home/u/projects/acme/storefront-PROJ-2478",
	}
}

// recorder is a Remove that records its requests and answers from a scripted list, one per call.
type recorder struct {
	got     []Request
	answers []error
}

func (r *recorder) remove(req Request) error {
	r.got = append(r.got, req)

	if len(r.got) <= len(r.answers) {
		return r.answers[len(r.got)-1]
	}

	return nil
}

func (r *recorder) actions() Actions { return Actions{Remove: r.remove} }

func enter(d *dialog) bool { return d.Handle(tui.Key{Kind: tui.KeyEnter}) }
func space(d *dialog)      { d.Handle(tui.Key{Kind: tui.KeyRune, Rune: ' '}) }

func TestEnterRemoves(t *testing.T) {
	tests := []struct {
		name         string
		deleteBranch bool
	}{
		{"the branch goes too", true},
		{"the branch stays", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls recorder
			d := &dialog{target: target(), deleteBranch: test.deleteBranch, actions: calls.actions()}

			if !enter(d) {
				t.Fatal("enter did not finish the dialog")
			}

			want := []Request{{DeleteBranch: test.deleteBranch}}
			if len(calls.got) != 1 || calls.got[0] != want[0] {
				t.Fatalf("removed with %+v, want %+v", calls.got, want)
			}
		})
	}
}

func TestSpaceTogglesTheCheckbox(t *testing.T) {
	var calls recorder
	d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}

	space(d)
	if d.deleteBranch {
		t.Fatal("space did not clear the checkbox")
	}

	space(d)
	if !d.deleteBranch {
		t.Fatal("space did not set the checkbox again")
	}

	enter(d)
	if len(calls.got) != 1 || !calls.got[0].DeleteBranch {
		t.Fatalf("removed with %+v, want the branch deleted", calls.got)
	}
}

// TestDetachedCheckoutCannotToggle pins that the box cannot be ticked when there is no branch to delete.
func TestDetachedCheckoutCannotToggle(t *testing.T) {
	detached := target()
	detached.Branch = ""

	var calls recorder
	d := &dialog{target: detached, deleteBranch: true, actions: calls.actions()}

	space(d)
	if !d.deleteBranch {
		t.Fatal("space changed a checkbox that cannot apply to anything")
	}

	lines := tui.Capture(d, testWidth, testHeight)
	if !contains(lines, emptyBox) {
		t.Fatalf("the box is not drawn cleared:\n%s", strings.Join(lines, "\n"))
	}
	if contains(lines, checkedBox) {
		t.Fatalf("the box is drawn ticked with no branch to delete:\n%s", strings.Join(lines, "\n"))
	}
	if !contains(lines, noBranch) {
		t.Fatalf("nothing on screen says there is no branch:\n%s", strings.Join(lines, "\n"))
	}
}

// TestDirtyCheckoutAsksAgain pins that Herdr's refusal of a dirty checkout becomes a second confirmation, not --force.
func TestDirtyCheckoutAsksAgain(t *testing.T) {
	calls := recorder{answers: []error{ErrNeedsForce}}
	d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}

	if enter(d) {
		t.Fatal("a refused removal finished the dialog")
	}
	if d.stage != stageForce {
		t.Fatalf("stage = %d, want stageForce", d.stage)
	}

	lines := tui.Capture(d, testWidth, testHeight)
	if !contains(lines, "uncommitted") {
		t.Fatalf("the warning is not on screen:\n%s", strings.Join(lines, "\n"))
	}
	if !contains(lines, "delete anyway") {
		t.Fatalf("the footer still reads as the safe first ask:\n%s", strings.Join(lines, "\n"))
	}

	if !enter(d) {
		t.Fatal("the second enter did not finish the dialog")
	}

	want := []Request{{DeleteBranch: true}, {DeleteBranch: true, Force: true}}
	if len(calls.got) != 2 || calls.got[0] != want[0] || calls.got[1] != want[1] {
		t.Fatalf("removed with %+v, want %+v", calls.got, want)
	}
}

// TestForcedRemovalWaitsForItsFrame pins that a held enter, arriving in the same input chunk as the refusal, does not
// answer a warning never drawn.
func TestForcedRemovalWaitsForItsFrame(t *testing.T) {
	calls := recorder{answers: []error{ErrNeedsForce}}
	d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}

	// Both enters arrive in one chunk: no render between them.
	if enter(d) {
		t.Fatal("the refused removal finished the dialog")
	}
	if enter(d) {
		t.Fatal("the dialog finished before the warning was ever drawn")
	}
	if len(calls.got) != 1 {
		t.Fatalf("removed %d times before the warning was drawn, want 1", len(calls.got))
	}

	tui.Capture(d, testWidth, testHeight)

	if !enter(d) {
		t.Fatal("enter after the warning was drawn did not finish the dialog")
	}
	if len(calls.got) != 2 || calls.got[1] != (Request{DeleteBranch: true, Force: true}) {
		t.Fatalf("removed with %+v, want a forced removal at the end", calls.got)
	}
}

// TestDirtyCheckoutWarnsUpFront pins that a known-dirty checkout's loss warning is shown before the first enter and
// is not dropped for room.
func TestDirtyCheckoutWarnsUpFront(t *testing.T) {
	dirty := target()
	dirty.Dirty = true

	// Height 6 is short enough that the ordinary consequences are dropped.
	clean := tui.Capture(&dialog{target: target(), deleteBranch: true}, testWidth, 6)
	if contains(clean, "permanently deleted") {
		t.Fatalf("the fixture is not short enough to drop the prose:\n%s", strings.Join(clean, "\n"))
	}

	lines := tui.Capture(&dialog{target: dirty, deleteBranch: true}, testWidth, 6)
	if !contains(lines, "permanently deleted") {
		t.Fatalf("a dirty checkout's warning was cut:\n%s", strings.Join(lines, "\n"))
	}
}

// TestForceStageKeepsTheCheckbox pins that the checkbox is live in the force stage and toggling keeps the warning.
func TestForceStageKeepsTheCheckbox(t *testing.T) {
	calls := recorder{answers: []error{ErrNeedsForce}}
	d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}

	enter(d)
	space(d)

	if d.deleteBranch {
		t.Fatal("space did not clear the checkbox in the force stage")
	}
	if lines := tui.Capture(d, testWidth, testHeight); !contains(lines, "uncommitted") {
		t.Fatalf("toggling cleared the warning:\n%s", strings.Join(lines, "\n"))
	}

	enter(d)
	if len(calls.got) != 2 || calls.got[1] != (Request{Force: true}) {
		t.Fatalf("removed with %+v, want a forced removal keeping the branch", calls.got)
	}
}

// TestFailureKeepsTheDialogOpen pins that an ordinary failure is shown and can be retried.
func TestFailureKeepsTheDialogOpen(t *testing.T) {
	calls := recorder{answers: []error{errors.New("worktree operation is already in progress")}}
	d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}

	if enter(d) {
		t.Fatal("a failed removal finished the dialog")
	}
	if d.stage != stageConfirm {
		t.Fatalf("stage = %d, want the dialog still on its first ask", d.stage)
	}
	if lines := tui.Capture(d, testWidth, testHeight); !contains(lines, "already in progress") {
		t.Fatalf("the failure is not on screen:\n%s", strings.Join(lines, "\n"))
	}

	if !enter(d) {
		t.Fatal("retrying did not finish the dialog")
	}
	if len(calls.got) != 2 {
		t.Fatalf("removed %d times, want 2", len(calls.got))
	}
}

// TestBranchFailureIsTerminal pins that with the checkout already gone the dialog reports and stops offering.
func TestBranchFailureIsTerminal(t *testing.T) {
	failure := fmt.Errorf("%w, but its branch was not: %w", ErrRemoved, errors.New("git branch -D topic: nope"))
	calls := recorder{answers: []error{failure}}
	d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}

	if enter(d) {
		t.Fatal("the dialog closed on a message nobody could have read")
	}
	if d.stage != stageDone {
		t.Fatalf("stage = %d, want stageDone", d.stage)
	}

	lines := tui.Capture(d, testWidth, testHeight)
	if !contains(lines, "its branch was not") {
		t.Fatalf("the failure is not on screen:\n%s", strings.Join(lines, "\n"))
	}
	if contains(lines, "enter delete") {
		t.Fatalf("the footer still offers a removal that has already happened:\n%s", strings.Join(lines, "\n"))
	}

	space(d)
	if len(calls.got) != 1 {
		t.Fatalf("the dialog acted again after the checkout was gone: %+v", calls.got)
	}
}

func TestEscapeCancels(t *testing.T) {
	for _, key := range []tui.Key{{Kind: tui.KeyEsc}, {Kind: tui.KeyCtrlC}} {
		var calls recorder
		d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}

		if !d.Handle(key) {
			t.Fatalf("%+v did not close the dialog", key)
		}
		if len(calls.got) != 0 {
			t.Fatalf("%+v removed something: %+v", key, calls.got)
		}
	}
}

// TestClickTogglesTheCheckbox pins that the click zone is where the checkbox was drawn.
func TestClickTogglesTheCheckbox(t *testing.T) {
	var calls recorder
	d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}
	tui.Capture(d, testWidth, testHeight)

	click := tui.Mouse{Button: tui.MouseLeft, Col: tui.Cells(markerOn) + 6, Row: d.checkboxRow}

	if k, ok := d.click(click); ok {
		t.Fatalf("the click stood for %+v rather than toggling", k)
	}
	if d.deleteBranch {
		t.Fatal("the click did not clear the checkbox")
	}
}

// TestClickingTheFooterRunsIt pins that clicking a footer entry does what the key it names does.
func TestClickingTheFooterRunsIt(t *testing.T) {
	var calls recorder
	d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}
	tui.Capture(d, testWidth, testHeight)

	k, ok := d.click(tui.Mouse{Button: tui.MouseLeft, Col: 2, Row: testHeight})
	if !ok || k.Kind != tui.KeyEnter {
		t.Fatalf("footer click = %+v (%v), want enter", k, ok)
	}
}

// TestShiftedClicksAreIgnored pins that Shift+mouse is left to the terminal for selecting text.
func TestShiftedClicksAreIgnored(t *testing.T) {
	var calls recorder
	d := &dialog{target: target(), deleteBranch: true, actions: calls.actions()}
	tui.Capture(d, testWidth, testHeight)

	for _, test := range []struct {
		name string
		m    tui.Mouse
	}{
		{"the checkbox", tui.Mouse{Button: tui.MouseLeft, Col: 4, Row: d.checkboxRow, Shift: true}},
		{"a footer entry", tui.Mouse{Button: tui.MouseLeft, Col: 2, Row: testHeight, Shift: true}},
		{"a release", tui.Mouse{Button: tui.MouseLeft, Col: 4, Row: d.checkboxRow, Release: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if k, ok := d.click(test.m); ok {
				t.Fatalf("the click stood for %+v", k)
			}
			if !d.deleteBranch {
				t.Fatal("the click toggled the checkbox")
			}
		})
	}
}

// TestEveryLineFitsTheWidth pins that no line overruns the pane; a wrapped line would push the footer off the bottom.
func TestEveryLineFitsTheWidth(t *testing.T) {
	wide := Target{
		Repo:     "org/設定リポジトリ",
		Branch:   "機能/とても長い名前のブランチ",
		Checkout: "/home/u/projects/org/設定リポジトリ-機能-とても長い名前のブランチ",
	}

	for _, width := range []int{20, 21, 40, 80} {
		for _, d := range []*dialog{
			{target: wide, deleteBranch: true},
			{target: wide, stage: stageForce},
			{target: Target{Repo: wide.Repo, Checkout: wide.Checkout}},
		} {
			for i, line := range tui.Capture(d, width, testHeight) {
				if got := tui.Cells(line); got > width {
					t.Errorf("line %d is %d columns at width %d: %q", i, got, width, line)
				}
			}
		}
	}
}

func TestHeaderNamesWhatGoes(t *testing.T) {
	d := &dialog{target: target(), deleteBranch: true}
	lines := tui.Capture(d, 100, testHeight)

	for _, want := range []string{title, "acme/storefront", "feature/PROJ-2478", "storefront-PROJ-2478"} {
		if !contains(lines, want) {
			t.Errorf("%q is not on screen:\n%s", want, strings.Join(lines, "\n"))
		}
	}
}

func contains(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}

	return false
}

// TestShortPaneKeepsWhatMatters pins that a pane too short for the whole dialog keeps the checkbox.
func TestShortPaneKeepsWhatMatters(t *testing.T) {
	for _, height := range []int{6, 8, 10, 12, 16, 40} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			d := &dialog{target: target(), deleteBranch: true}
			lines := tui.Capture(d, testWidth, height)

			if len(lines) != height {
				t.Fatalf("drew %d lines into a pane %d tall", len(lines), height)
			}
			for _, want := range []string{title, checkedBox, "storefront-PROJ-2478"} {
				if !contains(lines, want) {
					t.Errorf("%q was cut:\n%s", want, strings.Join(lines, "\n"))
				}
			}

			// checkboxRow has to reflect what was dropped above it.
			if got := lines[d.checkboxRow-1]; !strings.Contains(got, checkedBox) {
				t.Errorf("checkboxRow points at %q", got)
			}
		})
	}
}

// TestShortPaneKeepsTheWarning pins the same for the second confirmation's warning.
func TestShortPaneKeepsTheWarning(t *testing.T) {
	d := &dialog{target: target(), deleteBranch: true, stage: stageForce}
	lines := tui.Capture(d, testWidth, 8)

	for _, want := range []string{"uncommitted", checkedBox, "delete anyway"} {
		if !contains(lines, want) {
			t.Errorf("%q was cut:\n%s", want, strings.Join(lines, "\n"))
		}
	}
}
