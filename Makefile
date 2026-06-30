SHELL := /bin/bash

GO ?= go
GOFMT ?= gofmt
NPM ?= npm
UV ?= uv
BIN_DIR ?= bin
BINARY_NAME ?= llm-proxy
PYTHON_PROJECT_DIR ?= python
PUBLISH_ARGS ?=
PUBLISH_PAGES_ARGS ?=
RELEASE_ARGS ?=
DEPLOY_ARGS ?=
PUBLISH_PLATFORMS ?= linux/amd64,linux/arm64
DOCKER_IMAGE ?= ghcr.io/tyemirov/llm-proxy
PUBLISH_REMOTE ?= origin
PUBLISH_BRANCH ?= master
PAGES_BRANCH ?= gh-pages
PAGES_DOMAIN ?= llm-proxy.mprlab.com
GATEWAY_DIR ?=
GATEWAY_DEPLOY_TARGET ?= deploy-llm-proxy-backend

GO_SOURCES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: fmt check-format lint go-lint python-lint frontend-lint test go-test python-test python-root-import-test frontend-test test-live-providers test-live-gemini build clean ci release publish publish-pages deploy

fmt:
	$(GOFMT) -w $(GO_SOURCES)

check-format:
	@formatted="$$($(GOFMT) -l $(GO_SOURCES))"; \
	if [ -n "$$formatted" ]; then \
		echo "Go files require formatting:"; \
		echo "$$formatted"; \
		exit 1; \
	fi

lint: go-lint python-lint frontend-lint

go-lint:
	$(GO) vet ./...
	$(GO) run honnef.co/go/tools/cmd/staticcheck@latest ./...
	$(GO) run github.com/gordonklaus/ineffassign@latest ./...

python-lint:
	cd $(PYTHON_PROJECT_DIR) && $(UV) run --group dev mypy --strict llm_proxy_client

frontend-lint:
	$(NPM) run frontend:lint

test: go-test python-test frontend-test

go-test:
	@GO="$(GO)" ./scripts/check_coverage.sh

python-test:
	cd $(PYTHON_PROJECT_DIR) && $(UV) run --group dev pytest
	$(MAKE) python-root-import-test

python-root-import-test:
	$(UV) run --no-project --with-editable . python -c 'from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest; assert Client and ClientConfig and ClientMessage and ClientMessagesRequest'

frontend-test:
	$(NPM) run frontend:test

test-live-providers:
	@GO="$(GO)" ./scripts/test_live_providers.sh

test-live-gemini:
	@GO="$(GO)" ./scripts/test_live_gemini.sh

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/cli

clean:
	rm -rf $(BIN_DIR)

ci: check-format lint test

release:
	@./scripts/release.sh $(RELEASE_ARGS)

publish:
	@DOCKER_IMAGE="$(DOCKER_IMAGE)" PUBLISH_PLATFORMS="$(PUBLISH_PLATFORMS)" PUBLISH_REMOTE="$(PUBLISH_REMOTE)" PUBLISH_BRANCH="$(PUBLISH_BRANCH)" PAGES_BRANCH="$(PAGES_BRANCH)" PAGES_DOMAIN="$(PAGES_DOMAIN)" ./scripts/publish.sh $(PUBLISH_ARGS)

publish-pages:
	@PAGES_REMOTE="$(PUBLISH_REMOTE)" PAGES_BRANCH="$(PAGES_BRANCH)" PAGES_DOMAIN="$(PAGES_DOMAIN)" ./scripts/publish_pages.sh $(PUBLISH_PAGES_ARGS)

deploy:
	@GATEWAY_DIR="$(GATEWAY_DIR)" DOCKER_IMAGE="$(DOCKER_IMAGE)" GATEWAY_DEPLOY_TARGET="$(GATEWAY_DEPLOY_TARGET)" PAGES_BRANCH="$(PAGES_BRANCH)" PAGES_DOMAIN="$(PAGES_DOMAIN)" ./scripts/deploy.sh $(DEPLOY_ARGS)
