# Disappearing Messages — Mattermost plugin build.
#
# Self-contained alternative to the canonical mattermost-plugin-starter-template
# build/ subtree. Targets mirror the template (all / check-style / test / dist /
# deploy) but parse the manifest with jq, so no vendored build tools are required.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

GO ?= go
NPM ?= npm
JQ ?= jq

export GO111MODULE = on
export CGO_ENABLED = 0

PLUGIN_ID := $(shell $(JQ) -r .id plugin.json)
PLUGIN_VERSION := $(shell $(JQ) -r .version plugin.json)
BUNDLE := dist/$(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz

.DEFAULT_GOAL := help

.PHONY: all
all: check-style test dist  ## Build, lint, test and bundle the plugin

.PHONY: check-style
check-style: gofmt golangci-lint webapp-check  ## Run gofmt + golangci-lint + webapp typecheck

.PHONY: gofmt
gofmt:  ## Verify gofmt formatting of the server
	@unformatted=$$(gofmt -l server); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: files need formatting:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: golangci-lint
golangci-lint:  ## Run go vet + golangci-lint on the server
	$(GO) vet ./server/...
	golangci-lint run ./server/...

.PHONY: webapp-check
webapp-check: webapp/node_modules  ## Typecheck the webapp
	cd webapp && $(NPM) run check-types

.PHONY: test
test:  ## Run server tests with a coverage profile
	$(GO) test -race -covermode=atomic -coverprofile=server/coverage.out ./server/...

.PHONY: coverage
coverage: test  ## Report total coverage and fail below 80% (project target)
	@cov=$$($(GO) tool cover -func=server/coverage.out | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}'); \
	echo "coverage: $$cov% (target >= 80%)"; \
	awk "BEGIN {exit !($$cov >= 80)}" || { echo "coverage $$cov% below 80%"; exit 1; }

.PHONY: server
server:  ## Cross-compile the server for all bundled architectures
	@rm -rf server/dist && mkdir -p server/dist
	@set -e; \
	for target in linux:amd64 linux:arm64 darwin:amd64 darwin:arm64; do \
	  goos=$${target%%:*}; goarch=$${target##*:}; \
	  echo "  --> server/dist/plugin-$$goos-$$goarch"; \
	  ( cd server && GOOS=$$goos GOARCH=$$goarch $(GO) build -trimpath -o dist/plugin-$$goos-$$goarch . ); \
	done
	@echo "  --> server/dist/plugin-windows-amd64.exe"
	@cd server && GOOS=windows GOARCH=amd64 $(GO) build -trimpath -o dist/plugin-windows-amd64.exe .

webapp/node_modules: webapp/package.json
	cd webapp && $(NPM) install
	@touch $@

.PHONY: webapp
webapp: webapp/node_modules  ## Build the webapp bundle (webapp/dist/main.js)
	cd webapp && $(NPM) run build

.PHONY: bundle
bundle:  ## Assemble the installable plugin tarball
	@rm -rf dist && mkdir -p dist/$(PLUGIN_ID)/server dist/$(PLUGIN_ID)/webapp
	cp -r server/dist dist/$(PLUGIN_ID)/server/
	cp -r webapp/dist dist/$(PLUGIN_ID)/webapp/
	cp plugin.json dist/$(PLUGIN_ID)/
	cd dist && tar -czf $(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz $(PLUGIN_ID)
	@echo "bundle: $(BUNDLE)"

.PHONY: dist
dist: server webapp bundle  ## Cross-compile server, build webapp and assemble the tarball

.PHONY: deploy
deploy: dist  ## Build and deploy the plugin to a running Mattermost server
	@if command -v pluginctl >/dev/null 2>&1; then \
	  pluginctl deploy $(PLUGIN_ID) $(BUNDLE); \
	elif [ -n "$$MM_SERVICESETTINGS_SITEURL" ] && [ -n "$$MM_ADMIN_TOKEN" ]; then \
	  echo "Deploying $(BUNDLE) to $$MM_SERVICESETTINGS_SITEURL ..."; \
	  curl -fsS -X POST "$$MM_SERVICESETTINGS_SITEURL/api/v4/plugins" \
	    -H "Authorization: Bearer $$MM_ADMIN_TOKEN" \
	    -F "plugin=@$(BUNDLE)" && echo "uploaded — activate via System Console or mmctl plugin enable $(PLUGIN_ID)"; \
	else \
	  echo "No deploy target configured. Install $(BUNDLE) via:"; \
	  echo "  mmctl plugin upload $(BUNDLE)   # or"; \
	  echo "  System Console > Plugins > Plugin Management > Upload"; \
	fi

.PHONY: clean
clean:  ## Remove all build artifacts
	rm -rf server/dist server/coverage.out webapp/dist webapp/node_modules dist

.PHONY: help
help:  ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  %-16s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
