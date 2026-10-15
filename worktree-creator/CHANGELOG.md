# Changelog

All notable changes to worktree-creator are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the plugin follows
[Semantic Versioning](https://semver.org).

## [0.1.1] - 2026-10-15

### Fixed

- Each side of a `/` in the branch filter may occur anywhere in its segment of the name: `feat/2811` finds
  `feat/PROJ-2811-status`.

## [0.1.0] - 2026-08-30

### Added

- Initial release: create a worktree from a branch name or an issue reference, on a base picked from freshly fetched
  branches.
