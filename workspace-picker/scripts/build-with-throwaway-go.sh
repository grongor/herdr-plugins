#!/bin/sh
#
# Build the plugin with a throwaway Go toolchain: fetch the latest release into a temp dir, build, delete it. Lets
# `plugin install` work on a machine with no Go (release binaries were rejected: a second, untested code path with a
# version to keep in step). Download is ~64 MiB, unpacking to ~239 MiB plus a ~50 MiB build cache (GOCACHE=off has
# been refused since Go 1.12). POSIX sh: nothing is known about the machine yet.

set -eu

# The plugin directory is derived from this script's location, so the build does not depend on the invoking cwd.
plugin_dir=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
binary=bin/workspace-picker

fail() {
	echo "build-with-throwaway-go: $*" >&2
	exit 1
}

# macOS has curl and no wget; minimal Linux images may have either.
if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL --retry 3 --retry-all-errors -o "$1" -- "$2"; }
	fetch_stdout() { curl -fsSL --retry 3 --retry-all-errors -- "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q -O "$1" -- "$2"; }
	fetch_stdout() { wget -q -O - -- "$1"; }
else
	fail "neither curl nor wget is available, so a Go toolchain cannot be fetched.
Install Go 1.25 or newer instead: https://go.dev/dl/"
fi

# sha256sum on Linux, shasum on macOS. Without one the tarball cannot be verified, so no build.
if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum -- "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 -- "$1" | cut -d' ' -f1; }
else
	fail "neither sha256sum nor shasum is available, so the Go toolchain download
cannot be verified. Install Go 1.25 or newer instead: https://go.dev/dl/"
fi

case $(uname -s) in
Linux) goos=linux ;;
Darwin) goos=darwin ;;
*) fail "unsupported operating system $(uname -s); this plugin declares linux and macos" ;;
esac

case $(uname -m) in
x86_64 | amd64) goarch=amd64 ;;
arm64 | aarch64) goarch=arm64 ;;
*) fail "unsupported architecture $(uname -m); prebuilt Go toolchains cover amd64 and arm64" ;;
esac

# Latest stable, not a version pinned here: go.mod's `go` directive pins the language version, so a newer toolchain
# still compiles under 1.25 semantics and nothing here goes stale.
version=$(fetch_stdout 'https://go.dev/VERSION?m=text' | head -1)
case $version in
go[0-9]*) ;;
*) fail "could not determine the latest Go version (got '${version}')" ;;
esac

tarball="${version}.${goos}-${goarch}.tar.gz"
url="https://dl.google.com/go/${tarball}"

work=$(mktemp -d 2>/dev/null) || fail "could not create a temporary directory"

# ${work:?} so an empty variable fails instead of expanding into `rm -rf /`.
# Go writes the module cache read-only, directories included; without the chmod `rm -rf` fails with "Permission denied".
cleanup() {
	chmod -R u+w -- "${work:?}" 2>/dev/null || :
	rm -rf -- "${work:?}"
}
# Signal traps only exit: calling cleanup from one would return, and POSIX sh would resume the script with GOROOT gone.
# Exiting reaches the EXIT trap, so cleanup runs exactly once. Statuses are 128 + signal number.
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

echo "==> fetching ${version} (${goos}-${goarch}) into a temporary directory"
fetch "${work}/${tarball}" "$url" || fail "could not download ${url}"

want=$(fetch_stdout "${url}.sha256" | tr -d '[:space:]') ||
	fail "could not download the published digest for ${tarball}"
got=$(sha256 "${work}/${tarball}")
[ "$want" = "$got" ] || fail "checksum mismatch for ${tarball}:
  published ${want}
  download  ${got}"

echo "==> unpacking"
# Verified first, unpacked second: a stream cannot be checked before tar consumes it. go/test, go/api and go/misc
# (~30 MiB) are never read by `go build`. Excludes precede -f so bsdtar (macOS) and GNU tar parse this the same way.
tar --exclude='go/test' --exclude='go/api' --exclude='go/misc' \
	-xzf "${work}/${tarball}" -C "$work" || fail "could not unpack ${tarball}"

rm -f -- "${work}/${tarball}"

# Every path Go writes goes into the temp dir; missing one leaves a cache or go env file on a machine with no Go.
GOROOT="${work}/go"
GOPATH="${work}/gopath"
GOMODCACHE="${work}/gopath/pkg/mod"
GOCACHE="${work}/gocache"
GOENV="${work}/goenv"

# Telemetry cannot be disabled from outside: GOTELEMETRY/GOTELEMETRYDIR are derived values the go command never reads
# (checkEnvWrite rejects them as "cannot be modified"), and `go telemetry off` creates the very directory to be avoided.
# The directory comes from os.UserConfigDir — XDG_CONFIG_HOME on Linux, HOME on macOS — so both point into the temp dir.
# Moving HOME is safe only because this path runs where there is no Go: no go env file to honour, no credentials wanted.
XDG_CONFIG_HOME="${work}/config"
HOME="${work}/home"
mkdir -p -- "$HOME"

# GOTOOLCHAIN=local stops the fetched toolchain downloading another on top.
GOTOOLCHAIN=local
export GOROOT GOPATH GOMODCACHE GOCACHE GOENV XDG_CONFIG_HOME HOME GOTOOLCHAIN

echo "==> building ${binary}"
cd -- "$plugin_dir"
"${GOROOT}/bin/go" build -trimpath -o "$binary" . || fail "the build failed"

echo "==> built ${binary}; removing the temporary toolchain"
