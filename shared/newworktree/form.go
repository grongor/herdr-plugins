package newworktree

import (
	"slices"
	"strings"
	"time"

	"github.com/grongor/herdr-plugins/shared/git"
	"github.com/grongor/herdr-plugins/shared/issue"
	"github.com/grongor/herdr-plugins/shared/tui"
	"github.com/grongor/herdr-plugins/shared/worktree"
)

// Repo is the dialog's own view of a repository, so neither caller hands over its richer type.
type Repo struct {
	Name string
	// CommonDir is what Herdr's worktree API takes as --cwd.
	CommonDir string
	// Anchor is the main checkout, or the repository itself when bare. Git runs here; layouts are measured from it.
	Anchor string
	Head   string
	// Checkouts are the existing linked worktrees the proposed layout is read off.
	Checkouts []worktree.Checkout
}

type Request struct {
	Repo Repo
	// Branch is created from Base when it does not exist yet.
	Branch string
	// Base empty branches from the repository's HEAD.
	Base string
	// Path empty hands the choice to Herdr (the "herdr" layout).
	Path string
}

type Options struct {
	Repo Repo
	// Branch prefills the branch field. The picker passes the remote branch chosen to check out.
	Branch string
	// Base is offered ahead of the repository's default branch. The picker passes the checkout the cursor was on.
	Base string
	// HerdrWorktreesDir is where Herdr files worktrees it places itself; recognises a Herdr-managed repository.
	HerdrWorktreesDir string
	Config            Config
	// Create's error keeps the dialog open with the message on screen.
	Create func(Request) error
	// Wake is for a caller that owns the tui.Run loop and had to hand it a channel before the dialog existed. Nil
	// makes one. Must be buffered: sends come from the slow-work goroutines and must never block.
	Wake chan struct{}
	// LoadBranches nil uses git; tests pass one to stay off the disk.
	LoadBranches BranchLoader
}

type BranchList struct {
	// Local and Remote are most recently committed first.
	Local  []string
	Remote []string
	// Default is the remote's default branch.
	Default string
}

// BranchLoader runs off the input loop. The git-backed one fetches first, for at most fetch; zero skips the fetch.
type BranchLoader func(dir string, fetch time.Duration) (BranchList, error)

// gitBranches lists even when the fetch failed: a repository with no network is normal.
func gitBranches(dir string, fetch time.Duration) (BranchList, error) {
	var failure error
	if fetch > 0 {
		failure = git.Fetch(dir, fetch)
	}

	local, remote, err := git.Branches(dir)
	if err != nil && failure == nil {
		failure = err
	}

	return BranchList{Local: local, Remote: remote, Default: git.DefaultBase(dir)}, failure
}

// Action is what one keystroke did to the dialog.
type Action int

const (
	ActionNone Action = iota
	// ActionCancel: the picker returns to its list; the creator plugin exits.
	ActionCancel
	ActionQuit
	ActionDone
)

// Field order is tab order.
const (
	fieldBranch = iota
	fieldBase
	fieldLayout
	fieldPath
	fieldCount
)

var fieldLabels = [fieldCount]string{"Branch", "Base", "Layout", "Path"}

// Layout is never empty; Branch depends on whether issue references are recognised, see placeholder.
var placeholders = [fieldCount]string{
	fieldBase: "(repository HEAD)",
	fieldPath: "(Herdr chooses the path)",
}

const (
	placeholderBranch      = "(branch name)"
	placeholderBranchIssue = "(issue key, link, or branch name)"
)

// Status lines stand where an error would; the dialog stays usable while either is on screen.
const (
	statusFetching  = "fetching…"
	statusResolving = "resolving "
)

// Form is a surface rather than a tui.App: it renders into the area it is handed.
type Form struct {
	opts Options

	// layouts is built once so the layout the checkouts imply keeps its directory across a trip through the field.
	layouts []worktree.Layout
	mode    int
	// proposed is the mode the dialog opened on; only that one gets the note beside the layout names.
	proposed int

	// bases are the caller's preference, the default branch and HEAD; the full list is behind the picker.
	bases []string

	branch tui.LineEdit
	// base is one of bases, or empty when there are none: a ref typed by hand arrives through the branch list, which
	// adds it to the choices.
	base string
	path tui.LineEdit
	// pathEdited keeps a hand-written path from being overwritten by a later proposal; baseChosen keeps a base picked
	// by hand from being overwritten by the fetch.
	pathEdited bool
	baseChosen bool
	// proposedBranch is the name a remote base put in the branch field; only that, or an empty field, is replaced when
	// another remote base is chosen, so a name typed by hand stays.
	proposedBranch string

	field int
	err   string

	local  []string
	remote []string
	// showRemote starts from the config; ctrl-r flips it for the rest of the dialog.
	showRemote bool
	loading    bool

	// resolving is the issue key being resolved, empty when none is.
	resolving string

	// picker is non-nil while the branch list is on screen.
	picker *branchPicker

	// results carries closures from goroutines to the input loop, the only place the form is touched; no locks.
	results chan func()
	wake    chan struct{}

	zones tui.Zones
}

func New(opts Options) *Form {
	wake := opts.Wake
	if wake == nil {
		wake = make(chan struct{}, 1)
	}

	f := &Form{
		opts:       opts,
		showRemote: opts.Config.ShowRemote(),
		results:    make(chan func(), 4),
		wake:       wake,
	}

	repo := worktree.Repo{Anchor: opts.Repo.Anchor, Checkouts: opts.Repo.Checkouts}
	detected := worktree.For(repo, opts.HerdrWorktreesDir, opts.Config.LayoutMode())

	f.layouts = make([]worktree.Layout, len(worktree.Modes))
	for i, mode := range worktree.Modes {
		f.layouts[i] = worktree.Switch(repo, mode)
	}
	if i := slices.Index(worktree.Modes, detected.Mode); i >= 0 {
		f.layouts[i] = detected
		f.mode, f.proposed = i, i
	}

	f.bases = choices(opts.Base, opts.Repo.Head)
	if len(f.bases) > 0 {
		f.base = f.bases[0]
	}
	f.branch.Set(opts.Branch)
	f.proposedBranch = opts.Branch
	f.proposePath()

	f.startLoad()

	return f
}

// preferenceSlots is where the default branch is inserted: behind the caller's preference if any.
func preferenceSlots(preferred string) int {
	if preferred == "" {
		return 0
	}

	return 1
}

func choices(preferred ...string) []string {
	list := make([]string, 0, len(preferred))
	for _, base := range preferred {
		if base != "" && !slices.Contains(list, base) {
			list = append(list, base)
		}
	}

	return list
}

// Wake signals a repaint for a reason nobody typed.
func (f *Form) Wake() <-chan struct{} { return f.wake }

func (f *Form) Zones() tui.Zones { return f.zones }

func (f *Form) Request() Request {
	return Request{
		Repo:   f.opts.Repo,
		Branch: strings.TrimSpace(f.branch.String()),
		Base:   f.base,
		Path:   strings.TrimSpace(f.path.String()),
	}
}

func (f *Form) Fail(err error) { f.err = err.Error() }

// post is called from goroutines, so neither send may block; an uncollected wake already says what a second would.
func (f *Form) post(apply func()) {
	f.results <- apply

	select {
	case f.wake <- struct{}{}:
	default:
	}
}

// drain runs at the top of every keystroke and frame, the only two moments the form is looked at.
func (f *Form) drain() {
	for {
		select {
		case apply := <-f.results:
			apply()
		default:
			return
		}
	}
}

func (f *Form) startLoad() {
	dir := f.opts.Repo.Anchor
	if dir == "" {
		dir = f.opts.Repo.CommonDir
	}
	if dir == "" {
		return
	}

	load := f.opts.LoadBranches
	if load == nil {
		load = gitBranches
	}

	f.loading = true
	fetching := f.opts.Config.FetchFor()

	go func() {
		branches, err := load(dir, fetching)
		f.post(func() { f.loaded(branches, err) })
	}()
}

// loaded reports a failure but keeps whatever list came back with it.
func (f *Form) loaded(branches BranchList, failure error) {
	f.loading = false
	f.local, f.remote = branches.Local, branches.Remote

	if failure != nil && f.err == "" {
		f.err = failure.Error()
	}

	if f.picker != nil {
		f.refreshPicker()
	}

	fallback := branches.Default
	if fallback == "" {
		return
	}

	// The default branch goes behind the caller's preference but ahead of HEAD, which is only what the main checkout
	// happens to have out.
	if !slices.Contains(f.bases, fallback) {
		f.bases = slices.Insert(f.bases, min(len(f.bases), preferenceSlots(f.opts.Base)), fallback)
	}

	if !f.baseChosen && f.opts.Base == "" {
		f.base = f.bases[0]
	}
}

func (f *Form) Handle(k tui.Key) Action {
	f.drain()

	if f.picker != nil {
		return f.handlePicker(k)
	}

	// Any keystroke means the message has been read; status lines are separate and stay until the work finishes.
	f.err = ""

	// Shortcuts come before the editing keys: a shortcut put on ctrl-w is meant to win.
	if k.Is(f.opts.Config.RemoteKey()) {
		f.showRemote = !f.showRemote

		return ActionNone
	}

	switch k.Kind {
	case tui.KeyEnter:
		// Enter only ever submits, on every field: the branch list is opened with space, or by typing into it.
		return f.submit()
	case tui.KeyCtrlC:
		return ActionQuit
	case tui.KeyEsc:
		return ActionCancel
	case tui.KeyTab, tui.KeyDown:
		f.field = (f.field + 1) % fieldCount
	case tui.KeyShiftTab, tui.KeyUp:
		f.field = (f.field + fieldCount - 1) % fieldCount
	case tui.KeyLeft, tui.KeyRight:
		// In a text field the arrows move the caret; in a choice field they choose. No field is both.
		if !f.choice() {
			f.edit(k)

			break
		}
		if k.Kind == tui.KeyLeft {
			f.cycle(-1)
		} else {
			f.cycle(1)
		}
	case tui.KeyRune:
		switch f.field {
		case fieldLayout:
			if k.Rune == ' ' {
				f.cycle(1)
			}
		case fieldBase:
			// Base is chosen, not typed: a keystroke opens the list, and a character starts the filter.
			f.openPicker()
			if k.Rune != ' ' {
				f.handlePicker(k)
			}
		default:
			f.edit(k)
		}
	default:
		f.edit(k)
	}

	return ActionNone
}

// choice reports whether the focused field is chosen from rather than typed into.
func (f *Form) choice() bool { return f.field == fieldBase || f.field == fieldLayout }

// submit resolves an issue reference in the branch field whichever field the cursor is on: a key is never a branch
// name.
func (f *Form) submit() Action {
	if f.opts.Config.IssueEnabled() {
		if key, ok := issue.Reference(f.branch.String()); ok {
			f.startResolve(key)

			return ActionNone
		}
	}

	if f.resolving != "" {
		// Enter now would create the checkout the answer is about to rename.
		return ActionNone
	}

	if err := f.opts.Create(f.Request()); err != nil {
		f.err = err.Error()

		return ActionNone
	}

	return ActionDone
}

func (f *Form) startResolve(key string) {
	if f.resolving != "" {
		return
	}

	f.resolving = key
	resolver := f.opts.Config.Resolver()

	go func() {
		branch, err := resolver.Resolve(key)
		f.post(func() { f.resolved(branch, err) })
	}()
}

// resolved puts the proposal in the branch field for review rather than creating the checkout: the name came from
// a model.
func (f *Form) resolved(branch string, err error) {
	f.resolving = ""

	if err != nil {
		f.err = err.Error()

		return
	}

	f.branch.Set(branch)
	f.field = fieldBranch
	f.pathEdited = false
	f.proposePath()
}

func (f *Form) cycle(delta int) {
	switch f.field {
	case fieldLayout:
		f.setMode((f.mode + delta + len(worktree.Modes)) % len(worktree.Modes))
	case fieldBase:
		f.cycleBase(delta)
	}
}

func (f *Form) setMode(mode int) {
	f.mode = mode
	// A different layout means its path, even after a hand edit.
	f.pathEdited = false
	f.proposePath()
}

func (f *Form) cycleBase(delta int) {
	if len(f.bases) == 0 {
		return
	}

	f.setBase(f.bases[(f.baseIndex()+delta+len(f.bases))%len(f.bases)])
}

// setBase makes base the choice; one that was not offered — picked from the list or typed into its filter — joins the
// choices, so ←/→ can come back to it.
func (f *Form) setBase(base string) {
	if !slices.Contains(f.bases, base) {
		f.bases = slices.Insert(f.bases, 0, base)
	}
	f.base = base
	f.baseChosen = true
	f.followBase()
}

// followBase names the branch after a remote base — origin/feat/x is checked out as feat/x — unless the field holds a
// name typed by hand. A local base leaves the field alone: it is what the new branch starts from, not what it is.
func (f *Form) followBase() {
	if !slices.Contains(f.remote, f.base) {
		return
	}
	if current := f.branch.String(); current != "" && current != f.proposedBranch {
		return
	}

	_, name, _ := strings.Cut(f.base, "/")
	f.branch.Set(name)
	f.proposedBranch = name
	f.proposePath()
}

// baseIndex is -1 only while there is nothing to choose from.
func (f *Form) baseIndex() int { return slices.Index(f.bases, f.base) }

func (f *Form) layout() worktree.Layout { return f.layouts[f.mode] }

func (f *Form) proposePath() {
	if f.pathEdited {
		return
	}

	f.path.Set(f.layout().Path(f.branch.String()))
}

// editor is nil for the choice fields.
func (f *Form) editor(field int) *tui.LineEdit {
	switch field {
	case fieldBranch:
		return &f.branch
	case fieldPath:
		return &f.path
	}

	return nil
}

// edit hands an editing key to the focused text field; other keys, and keys on a choice field, do nothing.
func (f *Form) edit(k tui.Key) {
	e := f.editor(f.field)
	if e == nil || !e.Handle(k) {
		return
	}

	switch f.field {
	case fieldBranch:
		f.proposePath()
	case fieldPath:
		f.pathEdited = true
	}
}

func (f *Form) placeholder(field int) string {
	if field != fieldBranch {
		return placeholders[field]
	}
	if f.opts.Config.IssueEnabled() {
		return placeholderBranchIssue
	}

	return placeholderBranch
}

// value reports whether the text is a placeholder for an empty field.
func (f *Form) value(field int) (string, bool) {
	text := ""
	switch field {
	case fieldBase:
		text = f.base
	case fieldLayout:
		return "", false
	default:
		text = f.editor(field).String()
	}
	if text == "" {
		return f.placeholder(field), true
	}

	return text, false
}

// status reports whether the line is an error.
func (f *Form) status() (string, bool) {
	switch {
	case f.resolving != "":
		return statusResolving + f.resolving + "…", false
	case f.loading:
		return statusFetching, false
	case f.err != "":
		return f.err, true
	}

	return "", false
}
