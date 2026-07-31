# Disappearing Messages — Mattermost plugin build.
#
# Uses the canonical Mattermost build toolchain vendored under build/
# (build/setup.mk auto-compiles build/manifest and build/pluginctl on every
# invocation; Go's cache makes this instant). Deploy goes through pluginctl,
# which uploads with ?force=true and enables the plugin, and auto-discovers the
# local-mode unix socket so dev deploy is zero-config when Mattermost runs local.

GO ?= $(shell command -v go 2> /dev/null)
NPM ?= $(shell command -v npm 2> /dev/null)
MM_DEBUG ?=
GOPATH ?= $(shell go env GOPATH)
GO_TEST_FLAGS ?= -race
GO_BUILD_FLAGS ?=
DEFAULT_GOOS := $(shell go env GOOS)
DEFAULT_GOARCH := $(shell go env GOARCH)

export GO111MODULE=on

# GOBIN allows the build tools (golangci-lint, gotestsum, ...) to be installed locally.
export GOBIN ?= $(PWD)/bin

# Optional assets directory; bundle copies it into the tarball if present.
ASSETS_DIR ?= assets

## Define the default target (make all)
.PHONY: default
default: all

# Verify environment, and define PLUGIN_ID, PLUGIN_VERSION, HAS_SERVER, HAS_WEBAPP
# and HAS_PUBLIC as needed. Also compiles build/bin/manifest and build/bin/pluginctl.
include build/setup.mk

BUNDLE_NAME ?= $(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz

# Include custom targets and environment variables, if present.
ifneq ($(wildcard build/custom.mk),)
	include build/custom.mk
endif

ifneq ($(MM_DEBUG),)
	GO_BUILD_GCFLAGS = -gcflags "all=-N -l"
else
	GO_BUILD_GCFLAGS =
endif

.DEFAULT_GOAL := default

.PHONY: all
all: check-style test dist  ## Build, lint, test and bundle the plugin

.PHONY: apply
apply:  ## Propagate plugin manifest info into server/manifest.go and webapp/src/manifest.ts
	./build/bin/manifest apply

.PHONY: check-style
check-style: apply webapp/node_modules  ## Run gofmt + golangci-lint + webapp lint/typecheck
	@echo Checking for style guide compliance
	@unformatted=$$(gofmt -l server build/manifest build/pluginctl); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: files need formatting:"; echo "$$unformatted"; exit 1; \
	fi
	cd webapp && $(NPM) run --if-present lint
	cd webapp && $(NPM) run check-types
	$(GO) vet ./...
	golangci-lint run ./...

.PHONY: test
test: apply webapp/node_modules  ## Run server tests (coverage profile) + webapp tests
	$(GO) test $(GO_TEST_FLAGS) -covermode=atomic -coverprofile=server/coverage.out ./server/...
	cd webapp && $(NPM) run --if-present test

.PHONY: coverage
coverage: test  ## Report total server coverage and fail below 80% (project target)
	@cov=$$($(GO) tool cover -func=server/coverage.out | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}'); \
	echo "coverage: $$cov% (target >= 80%)"; \
	awk "BEGIN {exit !($$cov >= 80)}" || { echo "coverage $$cov% below 80%"; exit 1; }

.PHONY: server
server:  ## Cross-compile the server for all bundled architectures
	rm -rf server/dist
	mkdir -p server/dist
ifneq ($(MM_SERVICESETTINGS_ENABLEDEVELOPER),)
	@echo Building plugin only for $(DEFAULT_GOOS)-$(DEFAULT_GOARCH) because MM_SERVICESETTINGS_ENABLEDEVELOPER is enabled
	cd server && env CGO_ENABLED=0 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o dist/plugin-$(DEFAULT_GOOS)-$(DEFAULT_GOARCH)
else
	cd server && env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o dist/plugin-linux-amd64
	cd server && env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o dist/plugin-linux-arm64
	cd server && env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o dist/plugin-darwin-amd64
	cd server && env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o dist/plugin-darwin-arm64
	cd server && env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o dist/plugin-windows-amd64.exe
endif

.PHONY: server-linux-amd64
server-linux-amd64:  ## Cross-compile only the linux/amd64 server binary (for linux/amd64 Mattermost, e.g. on k8s)
	rm -rf server/dist
	mkdir -p server/dist
	cd server && env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) $(GO_BUILD_GCFLAGS) -trimpath -o dist/plugin-linux-amd64

webapp/node_modules: $(wildcard webapp/package.json)
	cd webapp && $(NPM) install
	@touch $@

.PHONY: webapp
webapp: webapp/node_modules  ## Build the webapp bundle (webapp/dist/main.js)
ifeq ($(MM_DEBUG),)
	cd webapp && $(NPM) run build
else
	cd webapp && $(NPM) run debug
endif

.PHONY: bundle
bundle:  ## Assemble the installable plugin tarball
	rm -rf dist
	mkdir -p dist/$(PLUGIN_ID)
	./build/bin/manifest dist
ifneq ($(wildcard $(ASSETS_DIR)/.),)
	cp -r $(ASSETS_DIR) dist/$(PLUGIN_ID)/
endif
ifneq ($(HAS_PUBLIC),)
	cp -r public dist/$(PLUGIN_ID)/
endif
ifneq ($(HAS_SERVER),)
	mkdir -p dist/$(PLUGIN_ID)/server
	cp -r server/dist dist/$(PLUGIN_ID)/server/
endif
ifneq ($(HAS_WEBAPP),)
	mkdir -p dist/$(PLUGIN_ID)/webapp
	cp -r webapp/dist dist/$(PLUGIN_ID)/webapp/
endif
ifeq ($(shell uname),Darwin)
	cd dist && tar --disable-copyfile -cvzf $(BUNDLE_NAME) $(PLUGIN_ID)
else
	cd dist && tar -cvzf $(BUNDLE_NAME) $(PLUGIN_ID)
endif
	@echo plugin built at: dist/$(BUNDLE_NAME)

.PHONY: dist
dist: apply server webapp bundle  ## Cross-compile server (all platforms), build webapp and assemble the tarball

.PHONY: dist-linux-amd64
dist-linux-amd64: apply server-linux-amd64 webapp bundle  ## Build linux/amd64 only + webapp + tarball

.PHONY: deploy
deploy: dist  ## Build (all platforms) and deploy via pluginctl (force upload + enable)
	./build/bin/pluginctl deploy $(PLUGIN_ID) dist/$(BUNDLE_NAME)

.PHONY: deploy-linux-amd64
deploy-linux-amd64: dist-linux-amd64  ## Build linux/amd64 only and deploy (suited for Mattermost on k8s)
	./build/bin/pluginctl deploy $(PLUGIN_ID) dist/$(BUNDLE_NAME)

.PHONY: disable
disable:  ## Disable the plugin on the running server
	./build/bin/pluginctl disable $(PLUGIN_ID)

.PHONY: enable
enable:  ## Enable the plugin on the running server
	./build/bin/pluginctl enable $(PLUGIN_ID)

.PHONY: reset
reset:  ## Disable then re-enable the plugin on the running server
	./build/bin/pluginctl reset $(PLUGIN_ID)

.PHONY: logs
logs:  ## Print recent plugin logs from the running server
	./build/bin/pluginctl logs $(PLUGIN_ID)

.PHONY: logs-watch
logs-watch:  ## Tail plugin logs continuously from the running server
	./build/bin/pluginctl logs-watch $(PLUGIN_ID)

.PHONY: clean
clean:  ## Remove all build artifacts
	rm -rf dist/
	rm -rf server/coverage.out server/dist
	rm -rf webapp/dist webapp/node_modules
	rm -rf build/bin/

.PHONY: help
help:  ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} \
	  /^[a-zA-Z0-9_-]+:.*?##/ { printf "  %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
