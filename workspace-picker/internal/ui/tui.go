package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/grongor/herdr-plugins/shared/git"
	"github.com/grongor/herdr-plugins/shared/newworktree"
	"github.com/grongor/herdr-plugins/shared/tui"
	"github.com/grongor/herdr-plugins/shared/worktree"

	"github.com/grongor/herdr-plugins/workspace-picker/internal/discover"
)

const (
	promptText = "❯ "
	markerOn   = "❯ "
	markerOff  = "  "

	// Prompt, divider and hint.
	chromeHeight = 3
	// firstRow is the screen line the list starts on; clicks on rows are resolved from it by arithmetic.
	firstRow = 2
	// Minimum gap between the label and the dimmed right column.
	columnGap = 2

	// fetchParallel bounds the repositories fetched at once: a hundred at a time is a hundred SSH connections.
	fetchParallel = 8
)

// zoneQuery is the click zone kind for the query itself; zero is the shared layer's "just the keystroke".
const zoneQuery = 1

// Keys is the list's shortcuts. A zero Key takes the default, so a caller may set only what it configures.
type Keys struct {
	NewWorktree tui.Key
	Remote      tui.Key
	Detached    tui.Key
	Folder      tui.Key
}

var defaultKeys = Keys{NewWorktree: tui.Ctrl('g'), Remote: tui.Ctrl('r'), Detached: tui.Ctrl('t'), Folder: tui.Ctrl('o')}

func (k Keys) withDefaults() Keys {
	if k.NewWorktree.Kind == tui.KeyNone {
		k.NewWorktree = defaultKeys.NewWorktree
	}
	if k.Remote.Kind == tui.KeyNone {
		k.Remote = defaultKeys.Remote
	}
	if k.Detached.Kind == tui.KeyNone {
		k.Detached = defaultKeys.Detached
	}
	if k.Folder.Kind == tui.KeyNone {
		k.Folder = defaultKeys.Folder
	}

	return k
}

// hints is the footer; the toggles name what they would show next.
func hints(keys Keys, showRemote, browsing bool) []tui.Hint {
	remote := keys.Remote.String() + " remote"
	if showRemote {
		remote = keys.Remote.String() + " local only"
	}
	folder := keys.Folder.String() + " folder"
	if browsing {
		folder = keys.Folder.String() + " projects"
	}

	items := []tui.Hint{{Text: "enter open", Key: tui.Key{Kind: tui.KeyEnter}}}
	if browsing {
		items = append(items, tui.Hint{Text: "tab complete", Key: tui.Key{Kind: tui.KeyTab}})
	}

	return append(items, []tui.Hint{
		{Text: keys.NewWorktree.String() + " new worktree", Key: keys.NewWorktree},
		{Text: remote, Key: keys.Remote},
		{Text: keys.Detached.String() + " detached", Key: keys.Detached},
		{Text: folder, Key: keys.Folder},
		{Text: "ctrl-u clear", Key: tui.Ctrl('u')},
		{Text: "esc cancel", Key: tui.Key{Kind: tui.KeyEsc}},
	}...)
}

// Options configures one run of the picker.
type Options struct {
	// Status is an extra note shown next to the row counter.
	Status string
	// HerdrWorktreesDir is where Herdr places worktrees itself; used to recognise a Herdr-managed repository.
	HerdrWorktreesDir string
	// NewWorktree configures the dialog ctrl-g opens.
	NewWorktree newworktree.Config
	// ShowDetached is whether linked detached checkouts start out listed; ctrl-t flips it either way.
	ShowDetached bool
	// ShowRemote is whether remote branches start out listed, which fetches every repository; ctrl-r flips it either
	// way.
	ShowRemote bool
	// Fetch bounds the fetch of one repository before its remote branches are listed; zero lists what is on disk.
	Fetch time.Duration
	// LoadRemote nil uses git; tests pass one to stay off the disk.
	LoadRemote RemoteLoader
	// Keys are the list's shortcuts; unset ones take the defaults.
	Keys Keys
	// Roots name a repository a path query finds the way the list names those under them.
	Roots []string
}

// RemoteLoader lists a repository's remote-tracking branches, newest commit first. It runs off the input loop; the
// git-backed one fetches first, for at most fetch, and zero skips the fetch.
type RemoteLoader func(dir string, fetch time.Duration) ([]string, error)

// gitRemote lists even when the fetch failed: a repository with no network is normal, and the refs on disk still serve.
func gitRemote(dir string, fetch time.Duration) ([]string, error) {
	var failure error
	if fetch > 0 {
		failure = git.Fetch(dir, fetch)
	}

	_, remote, err := git.Branches(dir)
	if err != nil && failure == nil {
		failure = err
	}

	return remote, failure
}

// Actions are what the picker calls when the user commits. An error is shown and the surface stays open.
type Actions struct {
	Open   func(row Row) error
	Create func(req newworktree.Request) error
}

// Run shows the picker and blocks until an action succeeds or the user cancels. Cancelling is not an error.
func Run(repos []discover.Repo, opts Options, actions Actions) error {
	return tui.Run(newPicker(repos, opts, actions))
}

func newPicker(repos []discover.Repo, opts Options, actions Actions) *pickerState {
	picker := &pickerState{
		repos:   repos,
		rows:    BuildRows(repos),
		opts:    opts,
		actions: actions,
		show:    toggles{detached: opts.ShowDetached, remote: opts.ShowRemote},
		// A goroutine holds its slot until its result is taken, so this many can be waiting on the next frame.
		results: make(chan func(), fetchParallel),
		wake:    make(chan struct{}, 1),
	}
	picker.refresh()
	if picker.show.remote {
		picker.startFetch()
	}

	return picker
}

// Wake is the repaint channel shared with the dialog, held by the picker because tui.Run reads it once, before any
// dialog exists.
func (s *pickerState) Wake() <-chan struct{} { return s.wake }

// Handle routes one keystroke to whichever surface is on screen and reports whether the picker is done.
func (s *pickerState) Handle(k tui.Key) bool {
	s.drain()

	// The dialog owns the whole pane while up, so it resolves its own clicks.
	if s.form != nil {
		if k.Kind == tui.KeyMouse {
			synthesised, ok := s.form.Click(k.Mouse)
			if !ok {
				return false
			}
			k = synthesised
		}

		switch s.form.Handle(k) {
		case newworktree.ActionQuit, newworktree.ActionDone:
			return true
		case newworktree.ActionCancel:
			s.form = nil
		case newworktree.ActionNone:
		}

		return false
	}

	if k.Kind == tui.KeyMouse {
		synthesised, ok := s.click(k.Mouse)
		if !ok {
			s.prioritise()

			return false
		}
		k = synthesised
	}

	switch s.handle(k) {
	case actionCancel, actionQuit:
		return true
	case actionSelect:
		if err := s.actions.Open(*s.selected()); err != nil {
			s.err = err.Error()

			return false
		}

		return true
	case actionNone:
		// The cursor may have moved onto a repository still waiting its turn.
		s.prioritise()
	}

	return false
}

type pickerState struct {
	// repos backs rows; rows are rebuilt from it as fetched remote branches arrive.
	repos []discover.Repo
	// rows is what the list shows: the repositories, or while browsing what the path query names.
	rows []Row
	// browsing is set while the query is a path; listed keeps the repositories' rows meanwhile, and browsed is the
	// query the rows were read for, so the directory is read again only when the query changes.
	browsing bool
	listed   []Row
	browsed  string

	opts    Options
	actions Actions
	query   tui.LineEdit
	visible []visibleRow
	cursor  int
	offset  int
	err     string
	show    toggles

	// fetch is each repository's place in the fetch, nil until the first time remote branches are asked for: the fetch
	// happens once per run however often they are hidden and shown again. running is how many are in flight.
	fetch   []fetchState
	running int
	// failures counts the repositories whose fetch failed; failure is the first message, reported when the last lands.
	failures int
	failure  string

	// form is the new-worktree dialog, non-nil while it is on screen.
	form *newworktree.Form
	// results carries closures from the fetch goroutines to the input loop, the only place the state is touched.
	results chan func()
	wake    chan struct{}
	// zones is what the last frame put where, for resolving clicks. Cleared when the surface changes: one chunk of input
	// can hold two events, and the first may have replaced what the second was aimed at.
	zones tui.Zones
	// listHeight is how many rows the last frame had room for; bounds the lines a click can name a row on.
	listHeight int
}

type action int

const (
	actionNone action = iota
	actionSelect
	// actionCancel leaves the current surface: the dialog goes back to the list, the list closes the picker.
	actionCancel
	// actionQuit closes the picker from wherever it is.
	actionQuit
)

func (s *pickerState) refresh() {
	query := s.query.String()
	if _, leaf, ok := pathQuery(query); ok {
		if !s.browsing {
			s.listed, s.browsing = s.rows, true
		}
		if s.browsed != query || s.rows == nil {
			s.rows, s.browsed = browse(query, s.opts.Roots), query
		}
		s.visible = browsed(s.rows, leaf, s.show)
	} else {
		if s.browsing {
			s.rows, s.listed, s.browsing, s.browsed = s.listed, nil, false, ""
		}
		s.visible = filter(s.rows, query, s.show)
	}

	s.cursor = 0
	s.offset = 0
	if len(s.visible) > 0 && !s.rows[s.visible[0].index].Targetable() {
		s.move(1)
	}
}

// relist refilters after the toggles changed, rebuilding the rows first when the repositories did, and keeps the
// cursor on the row it was on when that row is still shown: a fetch landing must not send the cursor back to the top.
func (s *pickerState) relist(rebuild bool) {
	var at Row
	held := s.cursor < len(s.visible)
	if held {
		at = s.rows[s.visible[s.cursor].index]
	}

	if rebuild {
		if s.browsing {
			s.listed = BuildRows(s.repos)
		} else {
			s.rows = BuildRows(s.repos)
		}
	}
	s.refresh()

	if !held {
		return
	}
	for i, v := range s.visible {
		if s.rows[v.index].same(at) {
			s.cursor = i

			return
		}
	}
}

// same reports whether two rows, possibly from different builds of the list, name the same thing.
func (r Row) same(o Row) bool {
	return r.Repo == o.Repo && r.Remote == o.Remote && r.Dir == o.Dir && r.Label == o.Label
}

// post is called from the fetch goroutines; the wake send is dropped when one is already pending.
func (s *pickerState) post(apply func()) {
	s.results <- apply

	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// drain runs at the top of every keystroke and frame, the only two moments the state is looked at.
func (s *pickerState) drain() {
	for {
		select {
		case apply := <-s.results:
			apply()
		default:
			return
		}
	}
}

type fetchState uint8

const (
	fetchQueued fetchState = iota
	fetchRunning
	fetchDone
)

// startFetch queues every repository and starts the first few; the rest follow as slots free up, the repository under
// the cursor always first.
func (s *pickerState) startFetch() {
	if s.fetch != nil {
		return
	}

	s.fetch = make([]fetchState, len(s.repos))
	s.pump()
}

// pump fills the free slots in list order, after the repository under the cursor.
func (s *pickerState) pump() {
	if s.fetch == nil {
		return
	}

	s.prioritise()
	for i := range s.fetch {
		if s.running >= fetchParallel {
			return
		}
		if s.fetch[i] == fetchQueued {
			s.start(i)
		}
	}
}

// prioritise fetches the repository under the cursor at once when it is still waiting its turn, past the bound if need
// be: it is the one whose branches are about to be wanted. Runs after every keystroke, so moving the cursor onto another
// repository moves the fetch with it.
func (s *pickerState) prioritise() {
	// A path query's rows are not the repositories the fetch is indexed by.
	if s.fetch == nil || s.browsing || s.cursor >= len(s.visible) {
		return
	}

	if at := s.rows[s.visible[s.cursor].index].group; s.fetch[at] == fetchQueued {
		s.start(at)
	}
}

func (s *pickerState) start(index int) {
	s.fetch[index] = fetchRunning
	s.running++

	load := s.opts.LoadRemote
	if load == nil {
		load = gitRemote
	}

	dir := s.repos[index].Anchor
	if dir == "" {
		dir = s.repos[index].CommonDir
	}

	go func() {
		remote, err := load(dir, s.opts.Fetch)
		s.post(func() { s.landed(index, remote, err) })
	}()
}

// landed keeps whatever list came back with a failure and hands the slot on; the failures are reported once, when the
// last one lands.
func (s *pickerState) landed(index int, remote []string, err error) {
	s.fetch[index] = fetchDone
	s.running--
	s.repos[index].Remote = remote
	if err != nil {
		s.failures++
		if s.failure == "" {
			s.failure = err.Error()
		}
	}

	s.relist(true)
	s.pump()

	if s.pending() == 0 && s.failures > 0 {
		s.err = fmt.Sprintf("fetch failed in %d of %d repositories: %s", s.failures, len(s.repos), s.failure)
	}
}

// pending is the repositories queued or in flight.
func (s *pickerState) pending() int {
	count := 0
	for _, state := range s.fetch {
		if state != fetchDone {
			count++
		}
	}

	return count
}

func (s *pickerState) selected() *Row {
	if len(s.visible) == 0 {
		return nil
	}
	row := s.rows[s.visible[s.cursor].index]
	if !row.Selectable() {
		return nil
	}

	return &row
}

// move steps the cursor by delta, skipping untargetable rows.
func (s *pickerState) move(delta int) {
	if len(s.visible) == 0 {
		return
	}

	for i := s.cursor + delta; i >= 0 && i < len(s.visible); i += sign(delta) {
		if s.rows[s.visible[i].index].Targetable() {
			s.cursor = i

			return
		}
	}

	// Nothing that way; if parked on a header (only possible right after a refresh), search the other way.
	if !s.rows[s.visible[s.cursor].index].Targetable() {
		for i := s.cursor - sign(delta); i >= 0 && i < len(s.visible); i -= sign(delta) {
			if s.rows[s.visible[i].index].Targetable() {
				s.cursor = i

				return
			}
		}
	}
}

func (s *pickerState) handle(k tui.Key) action {
	s.err = ""

	// Shortcuts come before the editing keys: a shortcut put on ctrl-w is meant to win.
	keys := s.opts.Keys.withDefaults()
	switch {
	case k.Is(keys.NewWorktree):
		s.openForm()

		return actionNone
	case k.Is(keys.Remote):
		s.show.remote = !s.show.remote
		if s.show.remote {
			s.startFetch()
		}
		s.relist(false)

		return actionNone
	case k.Is(keys.Detached):
		s.show.detached = !s.show.detached
		s.relist(false)

		return actionNone
	case k.Is(keys.Folder):
		// Into a path query from the home directory, or back out of one to the repositories.
		if s.browsing {
			s.query.Set("")
		} else {
			s.query.Set("~/")
		}
		s.refresh()

		return actionNone
	}

	switch k.Kind {
	case tui.KeyEnter:
		row := s.selected()
		if row == nil {
			break
		}
		// A remote branch has nothing to open yet: the dialog gets it as the branch to check out, from itself.
		if row.Remote != "" {
			s.openFormAt(s.cursor, row.Branch())

			break
		}

		return actionSelect
	case tui.KeyTab:
		if row := s.selected(); row != nil {
			if query, ok := completion(s.query.String(), *row); ok {
				s.query.Set(query)
				s.refresh()
			}
		}
	case tui.KeyEsc, tui.KeyCtrlC:
		return actionCancel
	case tui.KeyUp:
		s.move(-1)
	case tui.KeyDown:
		s.move(1)
	case tui.KeyPageUp:
		s.move(-10)
	case tui.KeyPageDown:
		s.move(10)
	default:
		// Only a changed query refilters: moving the caret must not send the cursor back to the top.
		before := s.query.String()
		if s.query.Handle(k) && s.query.String() != before {
			s.refresh()
		}
	}

	return actionNone
}

func (s *pickerState) openForm() { s.openFormAt(s.cursor, "") }

// openFormAt starts the dialog for one visible row; the checkout or remote branch it names is the default base, and
// branch, when given, prefills the branch to create.
func (s *pickerState) openFormAt(index int, branch string) {
	if index < 0 || index >= len(s.visible) {
		return
	}

	row := s.rows[s.visible[index].index]
	// A directory has no repository to create a worktree in.
	if row.Repo == nil {
		return
	}
	s.form, s.zones = newworktree.New(newworktree.Options{
		Repo:              dialogRepo(row.Repo),
		Branch:            branch,
		Base:              baseFor(row),
		HerdrWorktreesDir: s.opts.HerdrWorktreesDir,
		Config:            s.opts.NewWorktree,
		Create:            s.actions.Create,
		Wake:              s.wake,
	}), nil
}

// dialogRepo is the dialog's view of a discovered repository.
func dialogRepo(repo *discover.Repo) newworktree.Repo {
	linked := repo.Linked()

	checkouts := make([]worktree.Checkout, 0, len(linked))
	for _, checkout := range linked {
		checkouts = append(checkouts, worktree.Checkout{Path: checkout.Path, Branch: checkout.Branch})
	}

	return newworktree.Repo{
		Name:      repo.Name,
		CommonDir: repo.CommonDir,
		Anchor:    repo.Anchor,
		Head:      repo.Head,
		Checkouts: checkouts,
	}
}

// baseFor is the default base the row offers. The main checkout is the repository's own HEAD, which the dialog is told
// separately; a detached checkout offers its commit; a remote branch offers itself.
func baseFor(row Row) string {
	if row.Remote != "" {
		return row.Remote
	}

	w := row.Worktree
	if w == nil || w.Main {
		return ""
	}
	if w.Branch != "" {
		return w.Branch
	}

	return w.Head
}

// click resolves a mouse report against the last frame. Cursor moves happen here; anything standing for a keystroke
// comes back as that keystroke.
func (s *pickerState) click(m tui.Mouse) (tui.Key, bool) {
	// Shift+mouse is the terminal's, for selecting text; Herdr's own mouse bindings leave it alone too.
	if m.Shift || m.Release || m.Button == tui.MouseMiddle {
		return tui.Key{}, false
	}

	switch m.Button {
	case tui.MouseWheelUp:
		return tui.Key{Kind: tui.KeyUp}, true
	case tui.MouseWheelDown:
		return tui.Key{Kind: tui.KeyDown}, true
	}

	if target, ok := s.zones.At(m); ok {
		if target.Kind == zoneQuery {
			s.query.Cursor = s.query.Locate(target.To-target.From, m.Col-1-target.From)

			return tui.Key{}, false
		}

		return target.Key, true
	}

	if row := m.Row - 1 - firstRow; row >= 0 && row < s.listHeight {
		return s.clickRow(s.offset+row, m.Button)
	}

	return tui.Key{}, false
}

func (s *pickerState) clickRow(index, button int) (tui.Key, bool) {
	if index >= len(s.visible) {
		return tui.Key{}, false
	}

	targetable := s.rows[s.visible[index].index].Targetable()
	onCursor := targetable && index == s.cursor
	if targetable {
		s.cursor = index
	}

	// Right-click is the mouse's ctrl-g; it works on a repository header too, where the cursor cannot rest.
	if button == tui.MouseRight {
		s.openFormAt(index, "")

		return tui.Key{}, false
	}

	// Only a click on the row already under the cursor opens, so a stray click does not open a workspace.
	if onCursor && s.selected() != nil {
		return tui.Key{Kind: tui.KeyEnter}, true
	}

	return tui.Key{}, false
}

func sign(n int) int {
	if n < 0 {
		return -1
	}

	return 1
}

// Render draws whichever surface is on screen.
func (s *pickerState) Render(sc *tui.Screen) {
	s.drain()

	width, height := sc.Width(), sc.Height()

	if s.form != nil {
		s.form.Render(sc, width, height)

		return
	}

	s.listHeight = height - chromeHeight
	s.scrollInto(s.listHeight)
	s.zones = s.zones[:0]

	frame := sc.Frame()
	prompt := sc.PromptLine(promptText, &s.query, s.counter(), width)
	frame.Line(prompt.Text)
	frame.Line(sc.Dim(strings.Repeat("─", width)))
	s.zones = append(s.zones, tui.Zone{Kind: zoneQuery, Line: 0, From: prompt.From, To: prompt.To})

	for i := range s.listHeight {
		index := s.offset + i
		if index >= len(s.visible) {
			frame.Line("")

			continue
		}
		frame.Line(s.rowLine(sc, s.visible[index], index == s.cursor, width))
	}

	footer, zones := sc.Footer(s.err, hints(s.opts.Keys.withDefaults(), s.show.remote, s.browsing), height-1, width)
	frame.Bottom(footer)
	s.zones = append(s.zones, zones...)

	frame.Cursor(1, prompt.From+prompt.Caret+1)
	frame.Flush()
}

func (s *pickerState) counter() string {
	counter := fmt.Sprintf("%d/%d", s.countVisible(), s.countRows())
	if pending := s.pending(); pending > 0 {
		counter = fmt.Sprintf("fetching %d/%d…  %s", len(s.repos)-pending, len(s.repos), counter)
	}
	if s.opts.Status != "" {
		return s.opts.Status + "  " + counter
	}

	return counter
}

// countVisible and countRows count targetable rows, so an empty bare repository is not reported as nothing.
func (s *pickerState) countVisible() int {
	count := 0
	for _, v := range s.visible {
		if s.rows[v.index].Targetable() {
			count++
		}
	}

	return count
}

// countRows keeps counting hidden detached checkouts, the one on-screen sign there are any, but not hidden remote
// branches: the footer already says ctrl-r lists them, and hundreds of them would swamp the count of checkouts.
func (s *pickerState) countRows() int {
	count := 0
	for _, row := range s.rows {
		if row.Targetable() && (row.Remote == "" || s.show.remote) {
			count++
		}
	}

	return count
}

func (s *pickerState) scrollInto(listHeight int) {
	if listHeight <= 0 {
		return
	}
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+listHeight {
		s.offset = s.cursor - listHeight + 1
	}
	// Keep the selected worktree's header on screen when it is one line above the viewport.
	if s.offset > 0 && s.offset == s.cursor {
		s.offset--
	}
	s.offset = max(s.offset, 0)
}

func (s *pickerState) rowLine(sc *tui.Screen, v visibleRow, selected bool, width int) string {
	row := s.rows[v.index]
	prefix := v.prefix(s.rows)

	marker := markerOff
	if selected {
		marker = markerOn
	}

	labelWidth, detailWidth := tui.Columns(
		max(width-tui.Cells(marker)-tui.Cells(prefix)-columnGap, 0),
		tui.Cells(row.Label),
		tui.Cells(row.Detail),
	)

	label, labelUsed := highlighted(sc, row.Label, v.highlights, labelWidth, !row.Selectable() && !selected)
	// From the left: the end of a path is what tells two checkouts apart.
	detail := tui.TruncateLeft(row.Detail, detailWidth)

	// Padded from what was drawn, not the budget: truncating before a double-width character leaves a column unspent.
	pad := max(width-tui.Cells(marker)-tui.Cells(prefix)-labelUsed-tui.Cells(detail), 0)

	var b strings.Builder
	b.WriteString(sc.AccentIf(marker, !selected))
	b.WriteString(sc.DimIf(prefix, !selected))
	b.WriteString(label)
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(sc.DimIf(detail, !selected))

	if selected {
		// Reverse video follows the terminal theme where a fixed grey suits half of them; bold stays free for matches.
		return sc.Reverse(b.String())
	}

	return b.String()
}

// highlighted renders a label with the matched runes emphasised and reports the columns used, which can be one
// short of the budget when it ends before a double-width character.
func highlighted(sc *tui.Screen, label string, positions []int, budget int, dimmed bool) (string, int) {
	if budget <= 0 {
		return "", 0
	}

	marks := make(map[int]bool, len(positions))
	for _, p := range positions {
		marks[p] = true
	}

	// Positions index runes, so the text is walked in runes and only measured in columns.
	truncated := tui.Cells(label) > budget
	limit := budget
	if truncated {
		limit--
	}

	var b strings.Builder
	used := 0
	for i, r := range []rune(label) {
		width := tui.RuneCells(r)
		if used+width > limit {
			break
		}
		used += width

		switch {
		case marks[i]:
			b.WriteString(sc.Bold(string(r)))
		case dimmed:
			b.WriteString(sc.Dim(string(r)))
		default:
			b.WriteRune(r)
		}
	}
	if truncated {
		b.WriteString("…")
		used++
	}

	return b.String(), used
}
