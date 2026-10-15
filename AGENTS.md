# Agent instructions

Reference: `docs/authoring-plugins.md` (manifest contract, runtime environment, build/link loop).

## Live Herdr

The user's Herdr session is live. Never run a state-changing `herdr` command against it: run tests against an isolated
instance (`HERDR_SOCKET_PATH=<iso.sock>` set on the command itself) or ask the user to run it. Never pattern-kill
(`pkill`, `killall`, `pgrep | kill`); stop a test instance by its PID or with
`HERDR_SOCKET_PATH=<iso.sock> herdr server stop`.

## Checks

`make check` before committing; `make tidy-check` after touching a `go.mod`.

## Docs

A user-visible change updates, in the same branch: the plugin's README (feature text, key/mouse tables, footer
examples), `config.example.toml` for a new setting, the plugin's row in the root README and the action `description` in
`herdr-plugin.toml` when what the plugin does changes.

## Versioning

Every change a plugin's users can notice bumps that plugin's version in the same branch: minor for a feature, patch
for a fix (pre-1.0). Docs-only, test-only and CI-only changes do not bump. Installs track `main`, so every merge is a
release.

- The version is written only in `<plugin>/herdr-plugin.toml`; the binary embeds the manifest and reads it from there.
- The bump adds that version's section to `<plugin>/CHANGELOG.md` (Keep a Changelog: `Added`, `Changed`, `Fixed`, …).
- A change in `shared/` bumps each plugin whose behaviour it changes, and only those.
- Bump once per branch, not once per commit.

## Examples

Tests, comments and docs use placeholder names (`lorem/ipsum`, `org/a`, `PROJ-2811`), never real repositories,
organisations, people or local paths — not even the ones from the report that prompted the change.
