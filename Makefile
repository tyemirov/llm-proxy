SHELL := /bin/bash

GO ?= go
GOFMT ?= gofmt
NPM ?= npm
UV ?= uv
BIN_DIR ?= bin
BINARY_NAME ?= llm-proxy
PYTHON_PROJECT_DIR ?= python

GO_SOURCES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: fmt check-format lint go-lint python-lint frontend-lint test go-test python-test python-package-install-test frontend-test test-openapi-pages-artifact test-management-auth-blackbox test-live-provider-harness test-live-providers test-live-gemini live-test build clean ci up

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

test: go-test python-test frontend-test test-openapi-pages-artifact test-management-auth-blackbox test-live-provider-harness

go-test:
	@GO="$(GO)" ./scripts/check_coverage.sh

python-test:
	cd $(PYTHON_PROJECT_DIR) && $(UV) run --group dev pytest
	$(MAKE) python-package-install-test

python-package-install-test:
	@set -eu; temporary_package_directory="$$(mktemp -d)"; trap 'rm -rf "$$temporary_package_directory"' 0; cp "$(PYTHON_PROJECT_DIR)/pyproject.toml" "$$temporary_package_directory/pyproject.toml"; cp -R "$(PYTHON_PROJECT_DIR)/llm_proxy_client" "$$temporary_package_directory/llm_proxy_client"; LLM_PROXY_CLIENT_PROJECT_PATH="$$temporary_package_directory/pyproject.toml" $(UV) run --no-project --with "$$temporary_package_directory" python -c 'from importlib.metadata import version; from pathlib import Path; import os, tomllib; from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest, LLMProxyModelProfileError; assert version("llm-proxy-client") == tomllib.loads(Path(os.environ["LLM_PROXY_CLIENT_PROJECT_PATH"]).read_text(encoding="utf-8"))["project"]["version"]; assert Client and ClientConfig and ClientMessage and ClientMessagesRequest and LLMProxyModelProfileError'

frontend-test:
	$(NPM) run frontend:test

test-openapi-pages-artifact:
	@./scripts/test-openapi-pages-artifact.sh

test-management-auth-blackbox:
	$(NPM) run frontend:test:blackbox

test-live-provider-harness:
	@GO="$(GO)" ./scripts/test_live_providers.sh --preflight

test-live-providers:
	@GO="$(GO)" ./scripts/test_live_providers.sh

test-live-gemini:
	@GO="$(GO)" ./scripts/test_live_gemini.sh

live-test:
	@./scripts/live_test.sh

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/cli

up:
	@./scripts/up.sh

clean:
	rm -rf $(BIN_DIR)

ci: check-format lint test

.PHONY: release publish deploy

release publish deploy:
	@application_root="$$(git rev-parse --show-toplevel)"; \
	gateway_root="$$(dirname "$${application_root}")/mprlab-gateway"; \
	if [ ! -d "$${gateway_root}" ]; then \
		printf "required sibling gateway is missing: %s; clone mprlab-gateway at exactly %s\n" \
			"$${gateway_root}" "$${gateway_root}" >&2; \
		exit 2; \
	fi; \
	$(MAKE) --no-print-directory -C "$${gateway_root}" "app-$@" \
		MPRLAB_APP_ROOT="$${application_root}"
