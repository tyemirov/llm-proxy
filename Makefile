SHELL := /bin/bash

GO ?= go
GOFMT ?= gofmt
NPM ?= npm
UV ?= uv
PLAYWRIGHT_INSTALL_FLAGS ?= --with-deps
BIN_DIR ?= bin
BINARY_NAME ?= llm-proxy
PYTHON_PROJECT_DIR ?= python
PLAYWRIGHT_BROWSERS_PATH := $(CURDIR)/node_modules/.cache/ms-playwright
FRONTEND_DEPENDENCY_STAMP := $(PLAYWRIGHT_BROWSERS_PATH)/.llm-proxy-frontend-dependencies

export PLAYWRIGHT_BROWSERS_PATH

GO_SOURCES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: fmt check-format lint go-lint python-lint frontend-dependencies frontend-lint test go-test python-test python-package-install-test frontend-test test-frontend-dependency-contract test-openapi-pages-artifact test-management-auth-blackbox test-live-provider-harness test-live-providers test-live-provider-media test-live-gemini test-live-local-providers test-live-local-gemini live-test build clean ci up down

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

frontend-dependencies: $(FRONTEND_DEPENDENCY_STAMP)

$(FRONTEND_DEPENDENCY_STAMP): package.json package-lock.json
	$(NPM) ci
	./node_modules/.bin/playwright install $(PLAYWRIGHT_INSTALL_FLAGS) chromium
	@touch "$@"

frontend-lint: frontend-dependencies
	$(NPM) run frontend:lint

test: go-test python-test frontend-test test-openapi-pages-artifact test-management-auth-blackbox test-live-provider-harness

go-test: frontend-dependencies
	@GO="$(GO)" ./scripts/check_coverage.sh

python-test:
	cd $(PYTHON_PROJECT_DIR) && $(UV) run --group dev pytest
	$(MAKE) python-package-install-test

python-package-install-test:
	UV="$(UV)" $(UV) run --no-project --with 'pytest>=8.4.0' python -m pytest tests/python_package_contract_test.py

frontend-test: frontend-dependencies
	$(NPM) run frontend:test

test-frontend-dependency-contract:
	$(GO) test ./tests -run '^TestOperationalFrontendValidationPreparesPinnedDependencies$$' -count=1

test-openapi-pages-artifact:
	@./scripts/test-openapi-pages-artifact.sh

test-management-auth-blackbox: frontend-dependencies
	$(NPM) run frontend:test:blackbox

test-live-provider-harness:
	@GO="$(GO)" ./scripts/test_live_providers.sh --preflight

test-live-providers:
	@GO="$(GO)" ./scripts/test_live_providers.sh

test-live-provider-media:
	@GO="$(GO)" ./scripts/test_live_providers.sh --media

test-live-gemini:
	@GO="$(GO)" ./scripts/test_live_gemini.sh

test-live-local-providers:
	@GO="$(GO)" ./scripts/test_live_local.sh

test-live-local-gemini:
	@LLM_PROXY_LIVE_PROVIDERS=gemini \
		LLM_PROXY_LIVE_ALL_MODELS=true \
		LLM_PROXY_LIVE_REASONING_MATRIX=true \
		GO="$(GO)" ./scripts/test_live_local.sh

live-test:
	@./scripts/live_test.sh

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/cli

up:
	@./scripts/up.sh

down:
	@./scripts/down.sh

clean:
	rm -rf $(BIN_DIR)

ci:
	@MAKE_BIN="$(MAKE)" GO="$(GO)" GOFMT="$(GOFMT)" NPM="$(NPM)" UV="$(UV)" \
		PYTHON_PROJECT_DIR="$(PYTHON_PROJECT_DIR)" ./scripts/run_ci.sh

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

.PHONY: test-client-protocols
test-client-protocols: frontend-dependencies
	$(GO) test ./internal/proxy -run '^TestClientProtocols' -count=1

.PHONY: test-client-contracts generate-api-docs
test-client-contracts: frontend-dependencies
	$(GO) test ./internal/proxy ./pkg/llmproxyclient -run 'Test(ClientProtocols|OpenAPI|MessagesRequest|CoverageOpenAILifecycle|ManagementDashScopeWorkspaceChangeVerifiesRetainedKeyAndRoutesWithStoredURL)' -count=1

generate-api-docs:
	$(NPM) exec -- node scripts/generate_openapi_docs.mjs

.PHONY: test-release-policy
test-release-policy:
	$(GO) test ./tests -run '^TestOperationalReleaseDecisionUsesGixVersion$$' -count=1
