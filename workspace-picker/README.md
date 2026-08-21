# workspace-picker

A Herdr plugin that opens a picker over every git repository under your project roots — each repository's
worktrees nested underneath — and turns the one you choose into a Herdr workspace.

```
❯ store 2478                                                                    2/102
─────────────────────────────────────────────────────────────────────────────────────
❯ acme/storefront                                                                main
    └ feature/PROJ-2478-grpc-add-single-correction storefront-PROJ-2478-grpc-add-sin…

enter open · ctrl-g new worktree · ctrl-t detached · ctrl-u clear · esc cancel
```

Every footer entry is clickable. The left column is the repository name (top level) or the branch (nested); the dimmed
right column is the branch for a repository and the checkout path for a worktree. Queries match against all of it, so
`store 2478` narrows straight to one worktree.

Each word of the query has to occur as typed, case aside, and the words can come in any order: `retry fix` or
`fix/PROJ-2568` finds `fix/PROJ-2568-retry-budget`, `fixretry` does not. Not a subsequence, as fzf matches by default,
because branch names are full of ticket numbers and a few digits are a subsequence of most of them: `2811` would list
`PROJ-2481-…` too. A word found only in the checkout path matches but ranks below one found in the name or the branch,
and a repository lists its best matches first.

## New worktrees

`ctrl-g` — or a right click, on any row including a repository header — opens a dialog that creates a worktree in the
repository the row belongs to and opens it as a workspace.


```
New worktree
acme/storefront
─────────────────────────────────────────────────────────────────────────────────────

❯ Branch  PROJ-2512-add-retry-budget
  Base    feature/PROJ-2478-grpc-add-single-correction   ←/→ master
  Layout  sibling · shared · herdr   detected from existing worktrees
  Path    /home/user/projects/acme/storefront-PROJ-2512-add-retry-budget

enter create · tab field · esc back
```

Branch and path are text, with a caret: arrows, `home`/`end` and `ctrl-←`/`→` move it, and both can be left empty — an
empty branch has Herdr generate a name, an empty path lets Herdr choose the location. Base and layout are choices, which
is what `←/→` steps through there.

The base starts as the checkout of the row you opened the dialog from. `←/→` swaps it for the repository's own `HEAD` —
the branch its main checkout has out, or the default branch of a bare repository — and back. A detached checkout offers
its commit instead of a branch name.

The layout decides the path proposed for the branch you type. The starting layout is read off the worktrees the
repository already has, so a repository keeps the convention it follows:

| layout | path | recognised by |
| --- | --- | --- |
| `sibling` | `<parent>/<repo>-<branch>` | checkouts sitting next to the repository, named after it |
| `shared` | `<dir>/<branch>` | checkouts sharing one directory of their own, the usual bare-repository setup |
| `herdr` | Herdr's own worktrees directory | checkouts already under it |


The shared directory is the deepest one every checkout sits under: a repository holding `main`, `proposal/di` and
`fix/bug` shares the directory above all three, and gets `<dir>/fix/some-bug` for `fix/some-bug`. A checkout deeper than
the shared directory means branches are spelled out as directories; checkouts kept directly in it mean flattening.
Nothing is read back off a branch name, so a directory you named yourself is understood too.

Checkouts that disagree about the layout get the one most of them follow. A repository with a single worktree cannot say
whether the directory above it is shared or part of the branch, so it is taken as shared — `←/→` corrects that in one
keystroke. A repository with no worktrees gets [`new_worktree_layout`](config.example.toml), `sibling` by default. The
proposed path is always on screen, and typing over it wins.

A bare repository with no worktrees yet is listed too, showing `bare` and nothing under it. `enter` does nothing there,
but `ctrl-g` creates the first one.

## Requirements

- Herdr 0.7.5 or newer, Linux or macOS
- `curl` or `wget`, and `sha256sum` or `shasum` — only if you do not have Go
- **A Go toolchain is not required.** It is used if you have one.

### About the build

`herdr plugin install` runs the `[[build]]` command on your machine. There is no prebuilt binary: what runs is always
compiled from the ref you cloned, so there are no release assets to keep in step and nothing that can be stale. The
build has two paths:

- **You have Go 1.25 or newer.** A plain `go build`, about a second. This is what Herdr shows you before it runs
  anything.
- **You have no Go, or one too old.** The build fetches the current Go toolchain into a temporary directory, verifies it
  against the digest Google publishes beside it, builds, and deletes the toolchain and every cache again, so the plugin
  binary is the only thing added to your machine. About 64 MiB downloaded and 300 MiB of temporary disk, once. See
  [`scripts/build-with-throwaway-go.sh`](scripts/build-with-throwaway-go.sh). "Nothing left behind" is checked: CI runs
  the script on both platforms with no Go installed, and it completes under a sandbox with the whole filesystem
  read-only apart from its temporary directory and `bin/`.

With Go 1.21 to 1.24, Go itself downloads the toolchain `go.mod` asks for and keeps it in your module cache — Go's own
behaviour, not the plugin's.

A failing build aborts the install and registers nothing.

## Install

```bash
herdr plugin install grongor/herdr-plugins/workspace-picker
```

That clones the repository, runs the `[[build]]` command, and registers the plugin globally for your user, in every
session including running ones. For a local checkout, see [Local development](#local-development).

## Configure

The defaults scan whichever of `~/projects`, `~/src`, `~/code`, `~/work`, `~/dev`, `~/git` exist. To set your own roots:

```bash
cp config.example.toml "$(herdr plugin config-dir grongor.workspace-picker)/config.toml"
```

```toml
roots = ["~/projects"]
max_depth = 4
```

See [config.example.toml](config.example.toml) for every setting. The config is read on each invocation.

## Bind a key

Add to `~/.config/herdr/config.toml`, then `herdr server reload-config` (or rebuild, if Nix home-manager or another
config manager generates the file):

```toml
[[keys.command]]
key = "prefix+alt+p"
type = "plugin_action"
command = "grongor.workspace-picker.open"
description = "open workspace picker"
```

Without a keybinding: `herdr plugin action invoke grongor.workspace-picker.open`.

## Keys

| Key | Action |
| --- | --- |
| type | filter; whitespace splits the query into terms that must all occur, in any order |
| `↑` / `↓`, `ctrl-p` / `ctrl-n` | move between openable rows |
| `pgup` / `pgdn` | jump ten rows |
| `←` / `→`, `ctrl-←` / `ctrl-→`, `home` / `end` | move the caret by a character, a word, or to either end |
| `backspace`, `delete` | delete before or under the caret |
| `ctrl-w` | delete the word before the caret |
| `ctrl-u` | delete back to the start; from the end, the whole query |
| `ctrl-g` | new worktree in the repository under the cursor |
| `ctrl-t` | show or hide detached checkouts |
| `enter` | open the highlighted checkout |
| `esc`, `ctrl-c` | cancel |

## Mouse

| Action | Result |
| --- | --- |
| click a row | move the cursor to it |
| click the row the cursor is on | open it — a stray first click never opens a workspace |
| right-click a row | new worktree in that repository, header rows included |
| click the query | put the caret there, without refiltering |
| wheel | move between rows |
| click a footer entry | the key it names |
| click a field, a layout name, or the offered base | in the new-worktree dialog, choose it |

## What it finds

Repositories are found by walking each root and reading git metadata directly — no `git` subprocess, so a hundred
repositories scan instantly. Worktrees come from the repository's own `worktrees` administrative directory rather than
from the walk, so every layout works:

| Layout | Result |
| --- | --- |
| plain repository | one row |
| worktrees as sibling directories (`repo`, `repo-feature-x`) | `repo` with `feature-x` nested under it |
| worktrees in a container (`repo-worktrees/<branch>`) | nested, however deep the branch path goes |
| worktrees hidden inside the repository (`repo/.git-worktrees/...`) | nested |
| worktrees outside every configured root (`~/.herdr/worktrees`, `/tmp/...`) | nested — git metadata knows about them |
| bare repository plus worktrees, no main checkout | a dimmed, unselectable header row with the worktrees beneath it |

Detached checkouts show as `detached@<short-sha>` and start out hidden, being mostly what review tools and coding agents
leave in temp directories. `ctrl-t` shows and hides them, [`show_detached`](config.example.toml) sets the default; a
main checkout on a detached HEAD is always listed. The row counter keeps counting hidden checkouts, so `12/15` with an
empty query means some exist. Worktrees whose directory has been deleted but not pruned are skipped. Submodules are not
listed.

## What happens when you pick a row

```bash
herdr worktree open --cwd <git-common-dir> --path <checkout> --focus
```

Going through the worktree API rather than `workspace create` gives the workspace its worktree identity, so it shows up
in `herdr worktree list` and `herdr worktree remove` works on it; Herdr also focuses an existing workspace instead of
creating a duplicate. Opening a linked worktree makes Herdr open its repository's parent workspace too — Herdr's own
model, not something this plugin adds. If Herdr does not recognise a directory as a worktree, the plugin falls back to
`herdr workspace create --cwd <checkout> --focus`.

## Local development

`herdr plugin link` does not run `[[build]]` commands, so build first:

```bash
make build            # from the repository root
make link             # herdr plugin link <this directory>
```

Then:

```bash
# Discovery only, no TUI and no Herdr needed. Prints kind, repo, label, path.
HERDR_WORKSPACE_PICKER_ROOTS=~/projects ./bin/workspace-picker list

# The real popup.
herdr plugin action invoke grongor.workspace-picker.open

# The picker in a normal split instead of a modal popup, easier to inspect while iterating.
herdr plugin pane open --plugin grongor.workspace-picker --entrypoint picker \
  --placement split --direction down

# Exit status, stdout and stderr of every plugin command Herdr has run.
herdr plugin log list --plugin grongor.workspace-picker
```

After changing the code: `make build`, then reopen the pane. Relinking is only needed when `herdr-plugin.toml` changes —
Herdr snapshots the manifest into `~/.config/herdr/plugins.json` at link time. `make unlink` removes it from Herdr and
leaves your files alone.

To test in isolation, start a named session from a plain terminal — plugins are registered globally, so it is already
available there:

```bash
herdr --session pluginlab
```

Inside that session any shell pane can run `herdr plugin action invoke grongor.workspace-picker.open`; from another
terminal, prefix it with `HERDR_SOCKET_PATH=~/.config/herdr/sessions/pluginlab/herdr.sock`. Tear it down with `herdr
session stop pluginlab`, wait for `herdr session list` to show it stopped, then `herdr session delete pluginlab`.

Switching between this working tree and the published plugin is unregister-then-register, since only one registration
per plugin id exists:

```bash
herdr plugin unlink grongor.workspace-picker    # then: herdr plugin install ...
herdr plugin uninstall grongor.workspace-picker # then: make link
```

Config and state survive both. See [docs/authoring-plugins.md](../docs/authoring-plugins.md).

### Troubleshooting

- **The popup flashes and disappears.** The picker prints the error and waits for `enter` when running as a pane, so
  read that first; failures are also in `herdr plugin log list`.
- **"no git repositories found".** Check `roots` in the config directory printed by `herdr plugin config-dir
  grongor.workspace-picker`, and that `max_depth` reaches your repositories.
- **A worktree is missing.** Confirm git knows about it: `git -C <repo> worktree list`. The plugin lists exactly what
  git reports, minus checkouts whose directory no longer exists.
- **Colours look wrong.** The picker draws with your terminal's own ANSI colours and its highlight for the selected row,
  so a badly tuned palette shows through. `NO_COLOR=1` renders plain.
