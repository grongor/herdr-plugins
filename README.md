# herdr-plugins

Plugins for [Herdr](https://herdr.dev), the terminal workspace manager for AI coding agents.

## Plugins

| Plugin | What it does |
| --- | --- |
| [workspace-picker](workspace-picker/) | Pick a git repository or one of its worktrees — or any folder, by path — and open it as a workspace, or create a worktree in the layout that repository already uses |
| [worktree-creator](worktree-creator/) | Create a worktree from a branch name or an issue reference, on a base picked from freshly fetched branches |
| [worktree-remover](worktree-remover/) | Herdr's "delete worktree checkout?" dialog, with a checkbox that deletes the branch too |

## Install

Each plugin installs on its own:

```bash
herdr plugin install grongor/herdr-plugins/workspace-picker
herdr plugin install grongor/herdr-plugins/worktree-creator
herdr plugin install grongor/herdr-plugins/worktree-remover
```

Herdr clones the repository, runs that plugin's `[[build]]` commands, and registers it globally for your user. See the
plugin's README for configuration and keybindings.

There is no `herdr plugin update`: to update, run the install again, optionally pinned with `--ref`. Each plugin's
`CHANGELOG.md` lists what changed between versions.

## Develop

```bash
make build       # build every plugin binary
make check       # go vet + gofmt + tests, across every module
make tidy-check  # fail if a module's go.mod/go.sum needs tidying
make link        # register every plugin from this working tree
make list        # what Herdr currently has registered
make unlink      # deregister, leaving files alone
```

CI runs `make check TESTFLAGS=-race` and `make tidy-check` on Linux and macOS, and a second job runs each plugin's
`[[build]]` command on both platforms with no Go installed, to exercise the throwaway-toolchain install path.

`make link` uses `herdr plugin link`, which registers a plugin in place so code edits take effect on the next
invocation. Unlike `plugin install`, it does not build — that is what `make build` is for.

## Layout

```
herdr-plugins/
  Makefile                  build / link / check helpers for every module
  docs/authoring-plugins.md how Herdr plugins work, and how to add one here
  AGENTS.md                 agent instructions (CLAUDE.md links to it)
  .agents/skills/           the herdr-plugin-dev agent skill (.claude/skills links to it)
  shared/                   the module every plugin is built out of
  <plugin>/
    herdr-plugin.toml       the manifest Herdr reads
    README.md               what it does, how to configure it, how to test it
    CHANGELOG.md            what changed in each version
    config.example.toml     template for the plugin's user config
    ...                     implementation, one Go module per plugin
```

One directory per plugin, each with its own manifest and Go module, so `herdr plugin install
grongor/herdr-plugins/<plugin>` installs exactly one thing.

`shared/` is a module the plugins reach through `replace ../shared`: the terminal UI, the Herdr CLI wrapper, config
loading, the manifest checks, and the new-worktree dialog that both worktree-creator and workspace-picker open. Nothing
installs it, but `make check` treats it as a plugin. See
[docs/authoring-plugins.md](docs/authoring-plugins.md#the-shared-module).

There is no `go.work` and one must never be committed: Herdr builds a single plugin from that plugin's own directory,
and a committed workspace joins that build uninvited — see
[docs/authoring-plugins.md](docs/authoring-plugins.md#why-there-is-no-gowork).

## License

[MIT](LICENSE), covering every plugin in the repository.
