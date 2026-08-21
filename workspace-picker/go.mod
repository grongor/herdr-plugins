module github.com/grongor/herdr-plugins/workspace-picker

go 1.25.0

require github.com/grongor/herdr-plugins/shared v0.0.0

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/mattn/go-runewidth v0.0.27 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
)

// The shared module lives beside this one in the same repository, and Herdr
// clones the whole repository before building a plugin from its subdirectory —
// so the relative path resolves at install time as well as here. It is a replace
// rather than a version because there is nothing to version: the two are always
// released together, as one commit.
replace github.com/grongor/herdr-plugins/shared => ../shared
