# worktree-remover

A Herdr plugin that replaces Herdr's "delete worktree checkout?" confirmation with the same dialog plus a checkbox:
delete the branch the checkout was on.

```
Delete worktree checkout?
storefront — feature/PROJ-2478
─────────────────────────────────────────────────────────────

/home/user/projects/acme/storefront-PROJ-2478

  The checkout folder is deleted.
  The Herdr workspace will close.
  Dirty or untracked files will be permanently deleted.

❯ [x] Delete the branch too   feature/PROJ-2478

enter delete · space toggle · esc cancel
```

Herdr's own dialog says "The branch is not deleted" and leaves that to you. This one has the missing box, ticked by
default and [configurable](config.example.toml). Every footer entry and the checkbox are clickable.

## What it acts on

The worktree of the workspace you invoke it from, like Herdr's own "Delete worktree checkout…". There is no picker.

A workspace that is the repository's **main** checkout is refused up front, with the path in the message; Herdr refuses
it too. A **detached** checkout has no branch, so the box is drawn cleared and inert beside `(detached — no branch)`.

## Dirty checkouts

Herdr refuses to remove a checkout with uncommitted or untracked files unless asked twice, and this keeps that. The
first `enter` comes back with

```
This checkout has uncommitted or untracked files. Deleting it loses them.

enter delete anyway · space toggle · esc cancel
```

and the second one goes through. The checkbox stays live in between.

## What it runs

```bash
herdr worktree remove --workspace <id> [--force]   # then, only if the box is ticked:
git -C <repo> branch --delete --force -- <branch>
```

- **The order.** Git refuses to delete a branch that is still checked out somewhere.
- **The force.** By then the checkout is gone, so "not fully merged" is the ordinary case, and a refusal would leave the
  branch behind with nothing on screen to act on it. The commits stay in the reflog.
- **Where it runs.** Not in the dialog's own process: removing the checkout closes the workspace, which closes the pane,
  and Herdr kills that pane's process (SIGHUP, SIGHUP, SIGTERM) with the branch still to be deleted. The dialog hands
  both steps to a copy of itself in a session of its own (`worktree-remover apply`) and waits. Everything that can fail
  does so before anything closes, so failures still reach a dialog that is on screen.

## Requirements

- Herdr 0.7.5 or newer, Linux or macOS
- `git` on `PATH` — only if you use the branch checkbox
- `curl` or `wget`, and `sha256sum` or `shasum` — only if you do not have Go
- **A Go toolchain is not required.** It is used if you have one.

### About the build

`herdr plugin install` runs the `[[build]]` command on your machine; there is no prebuilt binary, so what runs is always
compiled from the ref you cloned. With Go 1.25 or newer that is a plain `go build`. Without Go, the build fetches the
current toolchain into a temporary directory, verifies it against the digest Google publishes beside it, builds, and
deletes the toolchain and every cache again — see
[`scripts/build-with-throwaway-go.sh`](scripts/build-with-throwaway-go.sh) and [workspace-picker's
README](../workspace-picker/README.md#about-the-build). A failing build aborts the install and registers nothing.

## Install

```bash
herdr plugin install grongor/herdr-plugins/worktree-remover
```

That clones the repository (the build needs `shared/` beside this directory), runs the `[[build]]` command, and
registers the plugin globally for your user, in every session including running ones.

## Configure

```bash
cp config.example.toml "$(herdr plugin config-dir grongor.worktree-remover)/config.toml"
```

```toml
# Whether "Delete the branch too" starts out ticked. Default: true.
delete_branch = true
```

The config is read on each invocation.

## Bind a key

Add to `~/.config/herdr/config.toml`, then `herdr server reload-config` (or rebuild, if Nix home-manager or another
config manager generates the file):

```toml
[[keys.command]]
key = "prefix+alt+x"
type = "plugin_action"
command = "grongor.worktree-remover.remove"
description = "delete this worktree checkout"
```

Without a keybinding: `herdr plugin action invoke grongor.worktree-remover.remove`.

Herdr's own "Delete worktree checkout…" stays where it is; this sits beside it.

## Keys

| Key | Action |
| --- | --- |
| `enter` | delete the checkout — and, the second time, a dirty one |
| `space`, `x` | tick or clear "Delete the branch too" |
| `esc`, `ctrl-c` | cancel, having done nothing |

## Mouse

| Action | Result |
| --- | --- |
| click the checkbox | tick or clear it |
| click a footer entry | the key it names |

## Local development

```bash
make build            # from the repository root; plugin link does not build
make link
herdr plugin action invoke grongor.worktree-remover.remove   # from a workspace that is a linked worktree
```

The popup is session-modal and awkward to drive from a script. Open it as an ordinary pane instead and read it back:

```bash
herdr plugin pane open --plugin grongor.worktree-remover --entrypoint confirm \
  --placement split --direction down
herdr pane read <pane-id>
```

In a split the pane belongs to the workspace being removed, so Herdr kills it as part of the removal — exactly the case
`apply` exists for, and worth reproducing.

Use a throwaway session (`herdr --session pluginlab`, from a plain terminal) and a scratch repository, since this plugin
deletes things. `herdr plugin log list --plugin grongor.worktree-remover` has the exit status, stdout and stderr of
every command Herdr ran.

### Troubleshooting

- **"this workspace is not backed by a git repository".** The workspace was created with `workspace create` rather than
  `worktree open`. Reopen it through [workspace-picker](../workspace-picker/), which goes through the worktree API.
- **"… is the repository's main checkout".** Herdr's worktree removal only applies to linked checkouts.
- **The checkout went and the branch did not.** The dialog says so if it is still on screen; otherwise `herdr plugin log
  list --plugin grongor.worktree-remover` has what `apply` printed. The usual cause is the branch being checked out in a
  second worktree.
- **The popup flashes and disappears.** The plugin prints the error and waits for `enter` when running as a pane, so
  read that first.
