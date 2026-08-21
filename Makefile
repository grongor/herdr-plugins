PLUGINS := workspace-picker
# shared is not a plugin but is vetted, gofmt-ed, tested and tidied like one.
MODULES := shared $(PLUGINS)
HERDR ?= herdr
# CI runs `make check TESTFLAGS=-race`; locally the plain run is faster.
TESTFLAGS ?=

.PHONY: help build build-without-go check tidy-check link unlink list clean

help:
	@echo "make build       Build every plugin binary (what 'plugin install' does for you)"
	@echo "make build-without-go  Exercise the no-toolchain install path (downloads a Go)"
	@echo "make check       go vet + gofmt + tests across all modules"
	@echo "make tidy-check  Fail if any module's go.mod/go.sum needs tidying"
	@echo "make link        Link every plugin into Herdr from this working tree"
	@echo "make unlink      Unlink every plugin"
	@echo "make list        Show what Herdr currently has registered"
	@echo "make clean       Remove built binaries"

# `herdr plugin link` does not run [[build]]; local authors build their own working tree.
build:
	@for plugin in $(PLUGINS); do \
		echo "==> $$plugin"; \
		(cd $$plugin && go build -trimpath -o bin/$$plugin .) || exit 1; \
	done

# The `plugin install` path on a machine with no Go, runnable by hand. Downloads ~60 MiB, so never the default.
build-without-go:
	@for plugin in $(PLUGINS); do \
		echo "==> $$plugin (no toolchain)"; \
		(cd $$plugin && sh scripts/build-with-throwaway-go.sh) || exit 1; \
	done

check:
	@for module in $(MODULES); do \
		echo "==> $$module"; \
		(cd $$module && go vet ./... && gofmt -l . | (! grep .) && go test $(TESTFLAGS) ./...) || exit 1; \
	done

# `-diff` reports and exits non-zero without touching the working tree.
tidy-check:
	@for module in $(MODULES); do \
		echo "==> $$module"; \
		(cd $$module && go mod tidy -diff) \
			|| { echo "$$module: go.mod/go.sum need tidying — run 'cd $$module && go mod tidy'"; exit 1; }; \
	done

link: build
	@for plugin in $(PLUGINS); do \
		$(HERDR) plugin link "$(CURDIR)/$$plugin"; \
	done

unlink:
	@for plugin in $(PLUGINS); do \
		id=$$(awk -F'"' '/^id *=/ {print $$2; exit}' $$plugin/herdr-plugin.toml); \
		$(HERDR) plugin unlink "$$id"; \
	done

list:
	@$(HERDR) plugin list

clean:
	@for plugin in $(PLUGINS); do rm -rf $$plugin/bin; done
