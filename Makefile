SHELL := /bin/bash

GO ?= go
GOFMT ?= gofmt
NPM ?= npm
UV ?= uv
BIN_DIR ?= bin
BINARY_NAME ?= llm-proxy
PYTHON_PROJECT_DIR ?= python
PUBLISH_RELEASE_ARGS ?=
DEPLOY_PAGES_ARGS ?=
RELEASE_ARGS ?=
RELEASE_HELPER ?=
RELEASE_ARTIFACT_TARGETS ?= container-artifacts pages-artifact
RELEASE_TOOL_DIR ?= $(abspath $(CURDIR)/tools/gitrelease/scripts)
DEPLOY_ARGS ?=
PUBLISH_PLATFORMS ?= linux/amd64,linux/arm64
DOCKER_IMAGE ?= ghcr.io/tyemirov/llm-proxy
PUBLISH_REMOTE ?= origin
PUBLISH_BRANCH ?= master
PAGES_BRANCH ?= gh-pages
PAGES_DOMAIN ?= llm-proxy.mprlab.com
PAGES_CONFIG_URL ?= https://llm-proxy-api.mprlab.com/config-ui.yaml
PAGES_URL ?= https://llm-proxy.mprlab.com/
PAGES_VERSION ?=
GATEWAY_DIR ?=
GATEWAY_DEPLOY_TARGET ?= deploy-llm-proxy-backend

GO_SOURCES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: fmt check-format lint go-lint python-lint frontend-lint test go-test python-test python-root-import-test frontend-test release-test test-live-provider-harness test-live-providers test-live-gemini build clean ci release container-artifacts pages-artifact publish-release publish pages-deploy deploy

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

test: go-test python-test frontend-test release-test test-live-provider-harness

go-test:
	@GO="$(GO)" ./scripts/check_coverage.sh

python-test:
	cd $(PYTHON_PROJECT_DIR) && $(UV) run --group dev pytest
	$(MAKE) python-root-import-test

python-root-import-test:
	$(UV) run --no-project --with-editable . python -c 'from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest; assert Client and ClientConfig and ClientMessage and ClientMessagesRequest'

frontend-test:
	$(NPM) run frontend:test

release-test:
	python3 -m unittest discover -s tools/gitrelease/tests -p 'test_*.py'

test-live-provider-harness:
	@GO="$(GO)" ./scripts/test_live_providers.sh --preflight

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
	@RELEASE_HELPER="$(RELEASE_HELPER)" RELEASE_ARTIFACT_TARGETS="$(RELEASE_ARTIFACT_TARGETS)" ./scripts/release.sh $(RELEASE_ARGS)

container-artifacts:
	@RELEASE_TOOL_DIR="$(RELEASE_TOOL_DIR)" DOCKER_IMAGE="$(DOCKER_IMAGE)" PUBLISH_PLATFORMS="$(PUBLISH_PLATFORMS)" ./scripts/build-container-artifact.sh

pages-artifact:
	@RELEASE_TOOL_DIR="$(RELEASE_TOOL_DIR)" PAGES_CONFIG_URL="$(PAGES_CONFIG_URL)" PAGES_DOMAIN="$(PAGES_DOMAIN)" ./scripts/build-pages-artifact.sh

publish-release:
	@RELEASE_HELPER="$(RELEASE_HELPER)" ./scripts/publish-release.sh --remote "$(PUBLISH_REMOTE)" $(PUBLISH_RELEASE_ARGS)

publish: publish-release
	@"$(RELEASE_TOOL_DIR)/publish_container_artifacts.sh"

pages-deploy:
	@"$(RELEASE_TOOL_DIR)/deploy_pages_artifact.sh" --remote "$(PUBLISH_REMOTE)" --branch "$(PAGES_BRANCH)" --url "$(PAGES_URL)" $(if $(PAGES_VERSION),--version "$(PAGES_VERSION)") $(DEPLOY_PAGES_ARGS)

deploy:
	@GATEWAY_DIR="$(GATEWAY_DIR)" DOCKER_IMAGE="$(DOCKER_IMAGE)" GATEWAY_DEPLOY_TARGET="$(GATEWAY_DEPLOY_TARGET)" PAGES_BRANCH="$(PAGES_BRANCH)" PAGES_URL="$(PAGES_URL)" ./scripts/deploy.sh $(DEPLOY_ARGS)
