---
name: herdr-plugin-dev
description: Build, link, test, and debug the Herdr plugins in this repository. Use when adding a plugin, editing a herdr-plugin.toml manifest, changing plugin code, or diagnosing a plugin that does not appear or does not run. Covers the manifest contract, the HERDR_* runtime environment, and the link/build/invoke/log loop.
---

# Herdr plugin development

Full reference: [docs/authoring-plugins.md](../../../docs/authoring-plugins.md). Upstream:
<https://herdr.dev/docs/plugins/>.

Herdr has no plugin SDK. **The whole Herdr CLI is the plugin API.** A plugin is a directory with `herdr-plugin.toml`
plus argv commands Herdr launches; they call back through `$HERDR_BIN_PATH`. Commands run with the plugin directory as
cwd and **without a shell**.

## Loop

```bash
make build      # REQUIRED: herdr plugin link does not run [[build]] commands
make link
herdr plugin list --plugin <id> --json          # check the `warnings` array
herdr plugin action list --plugin <id>
herdr plugin action invoke <id>.<action>
herdr plugin log list --plugin <id>             # exit status, stdout, stderr
```

- Code change → `make build`, invoke again. No relink needed.
- Manifest change → relink.
- New plugin → add it to `PLUGINS` in the `Makefile`. Never commit a `go.work`: Herdr builds one plugin from its own
  directory, and a committed workspace joins that build uninvited.

## The shared module

Everything reusable lives in `shared/`, a Go module the plugins reach through `replace
github.com/grongor/herdr-plugins/shared => ../shared` (resolves at install time because `herdr plugin install
owner/repo/subdir` clones the whole repository). `shared/tui` (terminal, keys, mouse, frames, colours, column-accurate
text), `shared/herdrcli` (CLI calls and error codes), `shared/plugin` (the `HERDR_*` environment), `shared/config`,
`shared/git`, `shared/proc`, `shared/paths`, `shared/manifesttest`.

A new plugin is a dialog plus a manifest. Anything not about *this* plugin's screen or decision belongs in `shared/`,
which `make check` treats like a plugin.

## Test in a throwaway session, not the live one

A named session has its own server, socket and layout; plugins are global, so a linked plugin is already available in
it:

```bash
herdr --session pluginlab   # from a plain terminal — Herdr refuses to nest
```

Drive it from outside:

```bash
export HERDR_SOCKET_PATH=~/.config/herdr/sessions/pluginlab/herdr.sock
herdr plugin action invoke <id>.<action>
herdr plugin log list --plugin <id>
```

Teardown: `herdr session stop pluginlab`, wait for `herdr session list` to read "stopped", then `herdr session delete
pluginlab`.

If you must act on the live session, prefer `--no-focus` and clean up everything you create (`herdr pane close`, `herdr
workspace close <id>`). Opening a linked worktree also opens its repository's parent workspace, so that is usually two
workspaces.

## Testing a pane UI without hijacking the session

A `popup` pane is session-modal and takes every keystroke. Open it as a normal pane instead and read it back:

```bash
herdr plugin pane open --plugin <id> --entrypoint <pane-id> \
  --placement split --direction down --no-focus
herdr pane read <pane-id> --source visible
herdr pane close <pane-id>
```

`herdr api snapshot` prints live workspaces, tabs, panes and agents as JSON; `herdr api schema --output schema.json`
dumps the socket API schema.

## Manifest gotchas

- `id`, `name`, `version`, `min_herdr_version` are required. `min_herdr_version` must be a release you verified — Herdr
  refuses a plugin newer than the running binary.
- Manifest `version` must equal the `version` const in `main.go`; `manifest_test.go` pins them. Every user-facing change
  bumps it and adds that version's section to the plugin's `CHANGELOG.md` in the same PR: installs track `main`, so a
  merge is a release.
- Action / pane / link-handler ids must not contain dots; actions are qualified as `<plugin.id>.<action>`.
- Declare `platforms`, or linking warns.
- No `[[startup]]`, `[[events]]` or `[[link_handlers]]` here: `shared/manifesttest` fails if one appears. Adding one is
  a decision to record there and in the README.
- `[[build]]` runs on `plugin install`, never on `plugin link`.
- `[[startup]]` is one-shot init and must exit.
- Actions run detached with no terminal; an action that needs UI asks Herdr for a pane (`herdr plugin pane open`).

## Runtime environment

`HERDR_BIN_PATH`, `HERDR_SOCKET_PATH`, `HERDR_ENV=1`, `HERDR_PLUGIN_ID`, `HERDR_PLUGIN_ROOT`, `HERDR_PLUGIN_CONFIG_DIR`,
`HERDR_PLUGIN_STATE_DIR`, `HERDR_PLUGIN_CONTEXT_JSON`, plus `HERDR_WORKSPACE_ID` / `HERDR_TAB_ID` / `HERDR_PANE_ID` when
in scope, `HERDR_PLUGIN_ACTION_ID` for actions, `HERDR_PLUGIN_ENTRYPOINT_ID` for panes.

**A popup pane's `HERDR_WORKSPACE_ID` is not its workspace.** Actions get all three scope variables naming the invoking
workspace. A popup gets no `HERDR_PANE_ID` and a `HERDR_WORKSPACE_ID` / `HERDR_TAB_ID` of `w11` / `w11:t1` — the popup's
own host, in every session, never an id `workspace list` returns. A `split` pane gets the real three. All of them get
`HERDR_PLUGIN_CONTEXT_JSON` with the true `workspace_id` and, for a git-backed workspace, a `worktree` object
(`repo_root`, `checkout_path`, `is_linked_worktree`, …). Read the context via `shared/plugin.Load()`, never the
variable.

**Herdr kills a pane's process the moment the pane goes** — SIGHUP, SIGHUP, SIGTERM, then an outright kill — and its
children with it: a `herdr` subprocess in flight returns `signal: hangup` although the server-side work succeeded. A
command whose own work closes its workspace cannot finish in the pane; deferring teardown or a new process group does
not help. Start the work in a session of its own (`shared/proc.Detach`, `Setsid`) and wait for it, with every failure
happening before anything closes.

**Herdr never sets `HERDR_CONFIG_PATH`** — it is an input Herdr reads to relocate its own `config.toml`. Honour it when
the user exported it and derive the location otherwise; `shared/herdrcli` does both. Evidence for these claims:
[docs/authoring-plugins.md](../../../docs/authoring-plugins.md#environment).

Never write durable state into `HERDR_PLUGIN_ROOT` — for GitHub installs it is a managed checkout that gets replaced.
Config goes in `HERDR_PLUGIN_CONFIG_DIR`, state in `HERDR_PLUGIN_STATE_DIR`.

## Related

Herdr's own agent skill (`npx skills add ogulcancelik/herdr --skill herdr -g`) teaches an agent to *operate* Herdr from
inside a managed pane and guards on `HERDR_ENV=1`. This skill is the *extension* side.
