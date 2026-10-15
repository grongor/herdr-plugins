# Authoring Herdr plugins

Reference for working in this repository. Upstream docs: [herdr.dev/docs/plugins](https://herdr.dev/docs/plugins/) and
[herdr.dev/docs/cli-reference](https://herdr.dev/docs/cli-reference/).

## How plugins work

A Herdr plugin is a directory with a `herdr-plugin.toml` manifest and commands Herdr can launch. There is no SDK: **the
Herdr CLI is the plugin API**. Herdr passes the binary to call in `HERDR_BIN_PATH`.

Herdr owns installation, manifest validation, keybindings, panes, events, invocation context and command logging. The
plugin owns its language, dependencies and durable state. Commands are argv arrays run **without a shell**, with the
plugin directory as the working directory.

### Manifest

```toml
id = "owner.plugin-name"          # required; ASCII letters, digits, . : _ -
name = "Human Name"               # required
version = "0.1.0"                 # required
min_herdr_version = "0.7.5"       # required; oldest release that supports what you use
description = "One line"          # optional
platforms = ["linux", "macos"]    # optional; omitting it links with a warning

[[build]]                         # run by `plugin install`, NOT by `plugin link`
command = ["go", "build", "-o", "bin/thing", "."]

[[startup]]                       # one-shot, once per session after restore
command = ["./bin/thing", "restore"]

[[events]]                        # run on a Herdr event; no id key
on = "worktree.created"
command = ["./bin/thing", "on-worktree"]

[[actions]]                       # user-invokable; bindable to a key
id = "open"                       # no dots; qualified as <plugin.id>.<action>
title = "Open the thing"
contexts = ["global", "workspace", "tab", "pane", "selection"]
command = ["./bin/thing", "open"]

[[panes]]                         # terminal UI
id = "picker"
title = "Picker"
placement = "popup"               # overlay (default) | popup | split | tab | zoomed
width = "70%"                     # popup only; number = cells, string = percentage
height = "60%"
command = ["./bin/thing", "pick"]

[[link_handlers]]                 # ctrl+click on a matching terminal URL
id = "issue"
title = "Open issue"
pattern = '^https://github\.com/[^/]+/[^/]+/issues/[0-9]+$'
action = "open"                   # must be an action of this plugin
```

Every item can carry its own `platforms`, overriding the top-level list. Action, pane and link-handler ids are local to
the plugin and must not contain dots.

`[[events]].on` takes **dotted** names — `worktree.created`, `worktree.opened`, `workspace.focused`,
`pane.agent_status_changed` (present in 0.7.5 and 0.8.2). The socket API's event stream spells the same events with
underscores (`worktree_created`); only the dotted form belongs in a manifest.

Placements:

- **`popup`** — session-modal, leaves the tiled layout alone, receives every key including Escape, has no pane id,
  closes when the command exits. Right for pickers and prompts.
- **`overlay`** — temporary zoomed pane over the active one; restores focus and zoom on close.
- **`split` / `tab` / `zoomed`** — ordinary panes afterwards, so `pane.move`, `pane.resize`, … apply.

`herdr plugin pane open --placement` overrides the manifest for the tiled placements, so `--placement split` is a
convenient way to inspect a popup UI while developing. On 0.8.2 it takes `overlay`, `split`, `tab` or `zoomed` — not
`popup` — and an overlay always opens over the active pane, refusing `--workspace` and `--target-pane`.

### Environment

| Variable | Meaning |
| --- | --- |
| `HERDR_BIN_PATH` | the Herdr binary to call — use this, not `herdr` on `PATH` |
| `HERDR_SOCKET_PATH` | raw socket / named pipe, if you want to skip the CLI |
| `HERDR_ENV=1` | marks a Herdr-managed process |
| `HERDR_PLUGIN_ID` | this plugin's id |
| `HERDR_PLUGIN_ROOT` | the installed or linked directory — do **not** write state here |
| `HERDR_PLUGIN_CONFIG_DIR` | user-editable config |
| `HERDR_PLUGIN_STATE_DIR` | durable runtime state |
| `HERDR_PLUGIN_CONTEXT_JSON` | full invocation context |
| `HERDR_WORKSPACE_ID`, `HERDR_TAB_ID`, `HERDR_PANE_ID` | current scope — an action gets all three; a popup pane gets two of them **naming the popup, not the workspace**; see below |
| `HERDR_PLUGIN_ACTION_ID` | actions only |
| `HERDR_PLUGIN_ENTRYPOINT_ID` | pane commands only |
| `HERDR_PLUGIN_EVENT`, `HERDR_PLUGIN_EVENT_JSON` | startup and event hooks |
| `HERDR_PLUGIN_CLICKED_URL` | the URL a `[[link_handlers]]` match produced |
| `HERDR_SESSION` | the named session, absent for the default one |
| `HERDR_ACTIVE_WORKSPACE_ID`, `HERDR_ACTIVE_PANE_ID`, `HERDR_ACTIVE_PANE_CWD` | context for `[[keys.command]]` shell bindings |

The bottom three are undocumented upstream but present in the 0.7.5 and 0.8.2 binaries and read by published plugins
(`madarco/agentbox`, `osolmaz/ghzinga`, `nikok6/herdr-mirror`, Herdr's own examples).

**Herdr never sets `HERDR_CONFIG_PATH`**, although several published plugins read it as the path to Herdr's
`config.toml`. It is an input Herdr *reads* ("overrides config file path" in the binary's help); environment dumps of an
action, a popup, an event hook and a startup hook on 0.7.5 and 0.8.2 all lack it. Honour it when the user exported it
and derive the location otherwise — `shared/herdrcli` does both. `HERDR_CELL_WIDTH_PX` / `HERDR_CELL_HEIGHT_PX`,
reported by `herdr-browser`, are unverified: neither string appears in either binary.

**A popup pane is not told which workspace it belongs to, and the two variables that look like they say so are wrong.**
On 0.7.5, 0.8.0 and 0.8.2 an action gets `HERDR_WORKSPACE_ID`, `HERDR_TAB_ID` and `HERDR_PANE_ID` naming the invoking
workspace; a popup gets no `HERDR_PANE_ID` and a `HERDR_WORKSPACE_ID` of `w11` with `HERDR_TAB_ID` `w11:t1` — the
popup's own host, the same in every session, never an id `workspace list` returns. A `split` pane does get the real
three. All of them get `HERDR_PLUGIN_CONTEXT_JSON`, which carries the true `workspace_id`, the workspace's label, cwd
and focused pane, and — for a git-backed workspace — a `worktree` object with `repo_key`, `repo_name`, `repo_root`,
`checkout_path` and `is_linked_worktree`. Read the context, never the variable; `shared/plugin` parses it.

**Herdr kills a pane's process when the pane goes.** A command whose own work closes its workspace — `worktree remove` —
gets SIGHUP, SIGHUP, SIGTERM while still running (0.7.5 and 0.8.2; ignoring them only delays an outright kill), and its
children with it: a `herdr` subprocess in flight returns `signal: hangup` although the server-side work succeeded.
Neither deferring teardown nor a new process group helps; the kill is aimed at the process. Starting the child in a
*session* of its own works — `shared/proc.Detach`, which `worktree-remover` uses.

`HERDR_PLUGIN_ROOT` is a managed checkout for GitHub installs and is replaced on reinstall. Config goes in
`HERDR_PLUGIN_CONFIG_DIR`, state in `HERDR_PLUGIN_STATE_DIR`; Herdr creates both and otherwise leaves them alone. There
is no Herdr-managed storage API.

### What the CLI prints

`workspace list`, `pane list` and `agent list` print JSON on stdout **by default**; there is no `--json` flag, and
`agent list --json` exits 2 with a usage error (0.7.5 and 0.8.2). `worktree open|create|remove`, `workspace create` and
`plugin pane open` print the JSON envelope unprompted too. The `worktree` commands ignore a stray `--json`; `workspace
create` refuses it with `unknown option: --json`, exit 2 — which is why `shared/herdrcli` leaves it off that fallback.

### The socket API

`HERDR_SOCKET_PATH` is a Unix socket speaking newline-delimited JSON; the CLI is a client of it. A request is `{"id",
"method", "params"}`; the answer is `{"id", "result"}` or `{"id", "error": {"code", "message"}}`.

**One request per connection** (0.7.5 and 0.8.2): the server closes the socket after the first answer. The exception is
`events.subscribe`, which acknowledges and then pushes `{"event", "data"}` frames.

`herdr api schema --output schema.json` dumps the authoritative method list: 91 methods in 0.8.2 (89 in 0.7.5), dotted
(`workspace.list`, `worktree.create`, `plugin.pane.open`, `notification.show`, `events.subscribe`).

The plugins here use the CLI, not the socket.

## Conventions in this repository

- One directory per plugin at the top level, so `herdr plugin install grongor/herdr-plugins/<plugin>` installs one
  plugin — the same shape as Herdr's own [examples repository](https://github.com/ogulcancelik/herdr-plugin-examples).
- Each plugin is its own Go module and reaches the shared one through `replace ../shared`. No root `go.work`, ever — see
  below.
- Binaries build to `<plugin>/bin/<plugin>` (gitignored); the manifest refers to `./bin/<plugin>`.
- Nothing runs unprompted: no `[[startup]]`, `[[events]]` or `[[link_handlers]]`. `shared/manifesttest` fails if one
  appears; adding one is a decision to record there and in the README.
- `PLUGINS` in the `Makefile` lists the plugins. `MODULES` is that list plus `shared`, and is what `make check` and
  `make tidy-check` walk.
- Ship a `README.md` and, if the plugin has settings, a `config.example.toml`.

### The shared module

`shared/` holds whatever a second plugin would otherwise have had to copy:

| Package | What it owns |
| --- | --- |
| `shared/tui` | the terminal: raw mode, key and SGR mouse decoding, bracketed paste, resize and signal handling, a frame writer, colours, column-accurate truncation and wrapping, the clickable footer and filter line |
| `shared/herdrcli` | the Herdr CLI calls, their JSON envelope, and the typed error carrying Herdr's error code |
| `shared/plugin` | the `HERDR_*` runtime environment, including the invocation context |
| `shared/config` | loading `$HERDR_PLUGIN_CONFIG_DIR/config.toml` into a plugin's own struct |
| `shared/git` | reading a checkout's HEAD off disk, listing branches, deleting a branch |
| `shared/worktree` | reading a repository's worktrees and inferring its layout |
| `shared/newworktree` | the new-worktree dialog both worktree-creator and workspace-picker open |
| `shared/issue` | resolving an issue reference into a proposed branch name |
| `shared/search` | the query matcher behind the picker and the branch list |
| `shared/proc` | starting a subprocess that outlives the pane |
| `shared/paths` | `~` expansion |
| `shared/manifesttest` | the manifest checks every plugin's `manifest_test.go` runs |

A plugin depends on it like this:

```
require github.com/grongor/herdr-plugins/shared v0.0.0

replace github.com/grongor/herdr-plugins/shared => ../shared
```

A `replace` rather than a version because the shared module and the plugins are released together, and because it
resolves at install time: `herdr plugin install owner/repo/subdir` clones the whole repository (0.7.5: `git init`,
`remote add`, `fetch --depth 1`, checkout — no sparse checkout), so `../shared` is there. Unlike a `go.work`, a
`replace` is written by the plugin, names one module, and is pinned by `go.sum`.

Nothing plugin-specific goes in `shared/`; what a particular dialog looks like belongs to that plugin's `internal/ui`.
`make check` and `make tidy-check` treat it like a plugin, because a break there breaks every plugin.

### Why there is no `go.work`

The builds resolve `shared` through the `replace`, and gopls (since v0.15) and GoLand handle the multi-module layout on
their own.

Herdr's install build is `go build` inside a single plugin directory, and Go finds `go.work` by walking *up*, so a
committed root workspace silently joins that build. Measured on a two-module scratch copy:

- A sibling plugin whose directory is missing or whose `go.mod` does not parse fails the build of an unrelated plugin:
  `go: cannot load module ../<other> listed in go.work file`.
- A sibling's unfetchable dependency is reported against *this* plugin's source files.
- Workspace mode resolves the union of every module's requirements, so a plugin can build against a dependency version
  its own `go.mod`/`go.sum` do not mention.
- `go work init` writes the `go` directive of whoever ran it, which becomes everyone's floor: `GOTOOLCHAIN=local`
  refuses, `GOTOOLCHAIN=auto` downloads a toolchain mid-install.

`GOWORK=off` fixes all four. If you generate one locally, keep it out of your commits.

### Adding a plugin

1. `mkdir <plugin>` with `herdr-plugin.toml`, `README.md`, `CHANGELOG.md` and the code. The `[[build]]` fallback is `sh
   ../shared/scripts/build-with-throwaway-go.sh`, the same line as every sibling; it builds `bin/<plugin>` from the
   directory it is run in. `main.go` takes its version from the manifest:
   `//go:embed herdr-plugin.toml` into `var manifest []byte`, then `var version = plugin.Version(manifest)`.
2. `go mod init github.com/grongor/herdr-plugins/<plugin>`, then add the shared module's `require` and `replace` lines.
3. `manifest_test.go` is `manifesttest.Check(t, manifesttest.Dir(t))` — it covers the manifest, the build
   command, the fallback script and the dispatch switch.
4. Add the directory to `PLUGINS` in the `Makefile`.
5. `make build && make link`.
6. `herdr plugin action list --plugin <id>` to confirm what Herdr registered.

### Development loop

```bash
make build                                   # plugin link does not build for you
make link
herdr plugin list --plugin <id> --json       # check for manifest warnings
herdr plugin action invoke <id>.<action>
herdr plugin log list --plugin <id>          # exit status, stdout, stderr
make unlink
```

Relink only when the manifest changes; code changes take effect on the next invocation. `plugin link` and `plugin
install` work with no server running, and registration is global to your user. `herdr pane read <pane-id>` dumps a
pane's screen, useful for checking a TUI's layout from a script.

### The registry

`plugin link` and `plugin install` both write `~/.config/herdr/plugins.json`: a **snapshot of each manifest** plus
`plugin_root` and `source`. So editing `herdr-plugin.toml` does nothing until you relink, and every session sees the
same plugins. `source.kind` is `local` for a linked working tree or `github` for an installed plugin (whose
`plugin_root` is a Herdr-managed clone).

Config and state live outside the registry, keyed by plugin id — `herdr plugin config-dir <id>` prints
`~/.config/herdr/plugins/config/<id>` — and survive unlink, uninstall and reinstall.

### Switching between a local checkout and the published plugin

One registration per plugin id, and installing over a linked plugin is refused, so switching is
unregister-then-register:

```bash
# local working tree -> published
herdr plugin unlink grongor.workspace-picker
herdr plugin install grongor/herdr-plugins/workspace-picker

# published -> local working tree
herdr plugin uninstall grongor.workspace-picker   # also deletes the managed clone
make link
```

Neither touches config or state. Check which is live:

```bash
herdr plugin list --json | jq '.result.plugins[] | {plugin_id, source: .source.kind, plugin_root}'
```

`herdr plugin disable <id>` / `enable <id>` keeps a plugin registered but inert. To run a local build *alongside* the
published one, change `id` in the local manifest; the two then coexist with separate config directories.

### A throwaway session for testing

A named session has its own server, socket and persisted layout, and plugins are global, so a linked plugin is already
there:

```bash
herdr --session pluginlab
```

**Start it from a plain terminal, not from inside a Herdr pane.** Herdr refuses to nest by default (`[experimental]
allow_nested = false`), and the refusal extends to the `herdr` CLI inside the nested session's panes.

Inside the session any shell pane can run `herdr plugin action invoke <id>.<action>` — Herdr injects the session's
socket, `HERDR_ENV=1`, `HERDR_SESSION` and the current `HERDR_WORKSPACE_ID` / `HERDR_TAB_ID` / `HERDR_PANE_ID` into
every pane. From outside, point the CLI at the socket:

```bash
export HERDR_SOCKET_PATH=~/.config/herdr/sessions/pluginlab/herdr.sock
herdr plugin action invoke grongor.workspace-picker.open
herdr plugin log list --plugin grongor.workspace-picker
```

- `herdr plugin pane open --env KEY=VALUE` passes extra environment into a pane command, so debug switches need no
  manifest edit.
- `herdr pane run <pane-id> <command>...` types a command into that pane's shell, so the syntax has to suit that shell.

Teardown: `herdr session stop pluginlab`, wait for `herdr session list` to read "stopped", then `herdr session delete
pluginlab`. Deleting before the stop has settled can leave the entry and its server alive.

## Agent skills

- **Herdr's own agent skill** teaches an agent to *operate* Herdr from inside a managed pane: `npx skills add
  ogulcancelik/herdr --skill herdr -g`. It refuses to act unless `HERDR_ENV=1`.
- **[`.agents/skills/herdr-plugin-dev`](../.agents/skills/herdr-plugin-dev/SKILL.md)** is the extension side: the
  manifest contract, the runtime environment, and the build/link/invoke/log loop.

## Publishing

Nothing here is published yet. To publish:

1. **Push to a public GitHub repository.** There is no registry; installation is `herdr plugin install
   owner/repo[/subdir]`.
2. **Add the `herdr-plugin` GitHub topic.** That is the whole listing process for the
   [marketplace](https://herdr.dev/plugins/); the index refreshes every 30 minutes. A monorepo is listed once.
3. **Add a `LICENSE`.**
4. **Set `min_herdr_version` honestly.** Herdr refuses to install a plugin that claims a version newer than the running
   binary.
5. **Declare `platforms`.** Omitting it links with a warning.
6. **Document build dependencies — or remove them.** `[[build]]` runs on the user's machine and Herdr installs no
   toolchains, so a compiled plugin excludes everyone without that language. Prebuilt release binaries mean a release
   workflow, per-platform artifacts, checksums and a manifest version kept in step with a tag. The plugins here instead
   fetch a throwaway toolchain at install time through one shared
   [`shared/scripts/build-with-throwaway-go.sh`](../shared/scripts/build-with-throwaway-go.sh): it always builds the ref
   that was cloned, and the cost is bandwidth once. The cleanup is where that goes wrong: Go's module cache is written
   read-only, so `rm -rf` alone cannot remove it; a signal trap that calls cleanup directly *returns* into a script
   whose toolchain is gone, so signals should exit and leave the work to one `EXIT` trap; and telemetry ignores
   `GOTELEMETRY` and `GOTELEMETRYDIR`, so `XDG_CONFIG_HOME` and `HOME` have to point at the throwaway directory.
7. **Keep `[[build]]` self-contained.** A failing build aborts the install and registers nothing.
8. **Do not mutate `herdr-plugin.toml` from a build command** — changing it after the install preview aborts the
   install.
9. **Version each plugin independently.** There is no `plugin update`; users reinstall, optionally pinned with `--ref`.
   Installs track `main`, so every merge is a release: a user-facing change bumps the plugin's version and adds that
   version's section to its `CHANGELOG.md` in the same PR.

Users installing from GitHub see the source and every command that will run before confirming, so keep the manifest
readable.
