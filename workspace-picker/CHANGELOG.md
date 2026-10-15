# Changelog

All notable changes to workspace-picker are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the plugin follows
[Semantic Versioning](https://semver.org).

## [0.2.1] - 2026-10-15

### Fixed

- Each side of a `/` in a query term may occur anywhere in its segment of the name: `lorem/dolor` finds
  `lorem/ipsum-condolor`, not only `lorem/dolores`.

## [0.2.0] - 2026-10-15

### Added

- A query starting with `/` or `~/` browses to [any folder](README.md#any-folder) and opens it as a workspace;
  `tab` completes the highlighted subdirectory.
- `ctrl-o` starts a path from the home directory and goes back to the repositories, configurable as `keys.folder`.
- A path naming a repository outside the project roots lists its checkouts, so they open with their worktree identity
  and `ctrl-g` creates worktrees in it.

## [0.1.0] - 2026-08-21

### Added

- Initial release: pick a git repository or one of its worktrees and open it as a workspace, or create a worktree in
  the layout that repository already uses.
