SHELL := /bin/bash

GO ?= go
GOFMT ?= gofmt
UV ?= uv
BIN_DIR ?= bin
BINARY_NAME ?= llm-proxy
PYTHON_PROJECT_DIR ?= python
PUBLISH_ARGS ?=
RELEASE_ARGS ?=
DEPLOY_ARGS ?=
PUBLISH_PLATFORMS ?= linux/amd64,linux/arm64
DOCKER_IMAGE ?= ghcr.io/tyemirov/llm-proxy
PUBLISH_REMOTE ?= origin
PUBLISH_BRANCH ?= master
GATEWAY_DIR ?=
GATEWAY_DEPLOY_TARGET ?= deploy-gateway

GO_SOURCES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: fmt check-format lint go-lint python-lint test go-test python-test test-live-gemini build clean ci release publish deploy

fmt:
	$(GOFMT) -w $(GO_SOURCES)

check-format:
	@formatted="$$($(GOFMT) -l $(GO_SOURCES))"; \
	if [ -n "$$formatted" ]; then \
		echo "Go files require formatting:"; \
		echo "$$formatted"; \
		exit 1; \
	fi

lint: go-lint python-lint

go-lint:
	$(GO) vet ./...
	$(GO) run honnef.co/go/tools/cmd/staticcheck@latest ./...
	$(GO) run github.com/gordonklaus/ineffassign@latest ./...

python-lint:
	cd $(PYTHON_PROJECT_DIR) && $(UV) run --group dev mypy --strict llm_proxy_client

test: go-test python-test

go-test:
	@GO="$(GO)" ./scripts/check_coverage.sh

python-test:
	cd $(PYTHON_PROJECT_DIR) && $(UV) run --group dev pytest

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
	@DOCKER_IMAGE="$(DOCKER_IMAGE)" PUBLISH_PLATFORMS="$(PUBLISH_PLATFORMS)" PUBLISH_REMOTE="$(PUBLISH_REMOTE)" PUBLISH_BRANCH="$(PUBLISH_BRANCH)" ./scripts/publish.sh $(PUBLISH_ARGS)

deploy:
	@GATEWAY_DIR="$(GATEWAY_DIR)" DOCKER_IMAGE="$(DOCKER_IMAGE)" GATEWAY_DEPLOY_TARGET="$(GATEWAY_DEPLOY_TARGET)" ./scripts/deploy.sh $(DEPLOY_ARGS)
