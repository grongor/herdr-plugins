# Worktree Creator

A Herdr plugin that creates a git worktree: type a branch name — or paste the issue you are about to work on — pick what
it branches from, and the checkout is created, seeded and opened as a focused workspace.

```
New worktree
acme/storefront
────────────────────────────────────────────────────
❯ Branch   fix/ticket-2568-billing-dashboard
  Base     origin/main   ←/→ feat/ticket-2584-perf
  Layout   sibling · shared · herdr   detected from existing worktrees
  Path     /home/u/projects/storefront-ticket-2568-billing-dashboard

enter create · tab field · esc back
```

- **A paste is not a submit.** Bracketed paste is on, so a branch name copied with its trailing newline lands in the
  field.
- **The line has a caret** — arrows, `ctrl-←`/`→`, `home`/`end`, `ctrl-w`, `ctrl-u` — and **Escape cancels** from
  anywhere, having done nothing.
- **An issue reference resolves in place.** Paste `TICKET-2568` (or `ticket-2568`) or its browse URL and press enter:
  the issue is fetched, a branch name is proposed with the key in `issue.key_case`, and it lands back in the field for
  you to edit.
- **The base is a list, fetched first.** The default is the remote's default branch — whatever it is called — read off
  `refs/remotes/<remote>/HEAD`, which the fetch records if a hand-added remote never got one. `init.defaultBranch` and
  then `main`/`master` are tried only for a repository with no remote.
- **A remote base names the branch.** Choose `origin/feat/ticket-2584-perf` and the Branch field reads
  `feat/ticket-2584-perf`: that is the branch being checked out. A name you typed yourself stays.

## Install

```bash
herdr plugin install grongor/herdr-plugins/worktree-creator
```

Or from a checkout of this repository: `make build && make link`.

## Bind a key

Add to `~/.config/herdr/config.toml`, then `herdr server reload-config` (or rebuild, if Nix home-manager or another
config manager generates the file):

```toml
[[keys.command]]
key = "prefix+alt+g"
type = "plugin_action"
command = "grongor.worktree-creator.create"
description = "new worktree"
```

Without a keybinding: `herdr plugin action invoke grongor.worktree-creator.create`.

Herdr's own `new_worktree` (`prefix+shift+g`) stays where it is; this sits beside it.

## The dialog

| Field | What it holds |
| --- | --- |
| Branch | the branch to check out, created from Base when it does not exist yet — or an issue key or browse URL to resolve into one |
| Base | the ref it starts from: the fetched default branch, `HEAD`, or a branch list pick, where typed refs go; a remote branch chosen here fills an empty Branch with its name |
| Layout | where the checkout goes: beside the repository, in a shared directory, or wherever Herdr files them |
| Path | the resulting directory, proposed from the branch and the layout until you type one |

## Keys

| Key | Action |
| --- | --- |
| `enter` | create the worktree — or, on an issue reference, resolve it into a branch name |
| `tab` / `shift-tab`, `↑` / `↓` | move between fields |
| `←` / `→` | in Branch or Path, move the caret; on Base or Layout, step through the choices |
| `ctrl-←` / `ctrl-→`, `home` / `end` | move the caret by a word, or to either end |
| `space` | on Base, open the branch list; on Layout, step it |
| `ctrl-r` | show or hide remote-tracking branches — `keys.remote_branches` in the config moves it |
| `backspace`, `delete` | delete before or under the caret |
| `ctrl-w`, `ctrl-u` | delete the word before the caret, delete back to the start |
| `esc`, `ctrl-c` | cancel, having done nothing |

In the branch list: type to filter — each word has to occur in the name, in any order, as in the picker; typing on Base
opens the list with that character — `↑` / `↓` and `pgup` / `pgdn` to move, `enter` to use the branch, or the filter
text itself when nothing matches it (a tag, `origin/foo`, a sha), `ctrl-r` to show or hide remote ones, `esc` to go
back.

## Mouse

| Action | Result |
| --- | --- |
| click a field | move to it |
| click the field the cursor is on | in Branch or Path, put the caret there; on Base, open the branch list |
| click a layout name | choose that layout |
| click the offered base — the dimmed `←/→ <ref>` | swap to it |
| click the filter, in the branch list | put the caret there, without refiltering |
| click a branch | move to it; clicking it again uses it |
| wheel | between fields, and through the branch list |
| click a footer entry | what the key it names does |

The first click on a field only moves to it, the way a click on a list row only moves the cursor: a stray click neither
opens the branch list nor moves a caret you are typing at. Shift+click is left to the terminal for text selection.

## Resolving an issue

Enter with an issue key or browse URL in the Branch field runs two commands: one that prints the issue's title, one that
hands it to a model and prints a branch name. The defaults expect [`jira`][jira-cli], `jq` and `claude` (haiku, no
tools, no thinking); the fetch command, the agent command, the system prompt and the prompt are all configurable in
[config.example.toml](config.example.toml).

Nothing runs unless the field holds something shaped like a reference, in either case. `issue.enabled = false` turns
recognition off, and the field then takes branch names only.

`issue.key_case` decides how the key is spelt from there on — in both commands, the branch name and so the worktree's
directory: `"lower"` (the default) or `"upper"`.

The proposal is checked before it is offered: the last output line that is a valid branch name and contains the issue
key, in any case — it is respelt in `key_case`, since a model tends to copy the tracker's spelling from the title. Type
and slug are the prompt's business; a name without the key is refused, since a branch that cannot be traced back to its
ticket has lost the point of resolving it.

[jira-cli]: https://github.com/ankitpokhrel/jira-cli

## Seeding

A new checkout is given the gitignored files that make a project usable — editor settings, run configurations — since no
clone brings them along. What is copied is configurable and skips anything already there; the default is
JetBrains-shaped. See [config.example.toml](config.example.toml) for what is deliberately *not* copied.

The `herdr` layout cannot be seeded: Herdr chooses that path asynchronously, so there is no directory until after the
workspace is open.

## Configuration

See [config.example.toml](config.example.toml). The config is read on each invocation.

The workspace picker's `ctrl-g` opens this same dialog and reads this same file, from this plugin's config directory (a
sibling of its own), so the dialog is configured once, here. Its `ctrl-r`, which fetches every repository before listing
remote branches, takes `fetch` and `fetch_timeout` from here too. The picker does not need this plugin installed: a
missing file means the defaults.
