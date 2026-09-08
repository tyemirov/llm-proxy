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

frontend-lint: frontend-dependencies check-brand-icons
	$(NPM) run frontend:lint

.PHONY: check-brand-icons test-brand-icons
check-brand-icons: frontend-dependencies
	node scripts/validate_brand_icons.mjs

test-brand-icons: frontend-dependencies
	$(MAKE) frontend-test FRONTEND_TEST_ARGS='--grep "brand icons"'

test: go-test python-test frontend-test test-openapi-pages-artifact test-management-auth-blackbox test-live-provider-harness

go-test: frontend-dependencies
	@GO="$(GO)" ./scripts/check_coverage.sh

python-test:
	cd $(PYTHON_PROJECT_DIR) && $(UV) run --group dev pytest
	$(MAKE) python-package-install-test

python-package-install-test:
	UV="$(UV)" $(UV) run --no-project --with 'pytest>=8.4.0' python -m pytest tests/python_package_contract_test.py

frontend-test: frontend-dependencies
	$(NPM) run frontend:test $(if $(FRONTEND_TEST_ARGS),-- $(FRONTEND_TEST_ARGS))

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

.PHONY: test-live-provider-candidate test-live-provider-candidate-media
test-live-provider-candidate:
	@GO="$(GO)" ./scripts/test_live_providers.sh --candidate-model "$(LIVE_CANDIDATE_MODEL)"

test-live-provider-candidate-media:
	@GO="$(GO)" ./scripts/test_live_providers.sh --candidate-model "$(LIVE_CANDIDATE_MODEL)" --media

test-live-gemini:
	@GO="$(GO)" ./scripts/test_live_gemini.sh

.PHONY: test-live-gemini-candidate
test-live-gemini-candidate:
	@GO="$(GO)" ./scripts/test_live_providers.sh --gemini-candidates

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

.PHONY: ci-backend ci-frontend
ci-backend: test-release-policy check-format go-lint python-lint go-test python-test test-live-provider-harness

ci-frontend: frontend-lint frontend-test test-openapi-pages-artifact test-management-auth-blackbox

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
.PHONY: test-mcp
test-mcp:
	$(GO) test ./internal/proxy -run '^TestMCP' -count=1

.PHONY: test-mcp-oauth
test-mcp-oauth: frontend-dependencies
	$(NPM) run frontend:test:blackbox -- --grep 'MCP OAuth'

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

.PHONY: test-provider-catalog
test-provider-catalog: frontend-dependencies
	$(GO) test ./internal/proxy ./tests ./cmd/cli -run 'Test(ProviderCatalog|CatalogDefined|ModelActivation|RootCommandPrintsCatalogDerivedLiveDiscovery)' -count=1

.PHONY: test-deepseek-retirement
test-deepseek-retirement: frontend-dependencies
	$(GO) test ./internal/proxy -run 'TestDeepSeekRetirement' -count=1

.PHONY: test-claude-retirement
test-claude-retirement: frontend-dependencies
	$(GO) test ./internal/proxy -run 'TestClaudeRetirement' -count=1

.PHONY: test-xai-responses
test-xai-responses: frontend-dependencies
	$(GO) test ./internal/proxy -run 'Test(XAIResponses|ManagementXAIResponses|ClientProtocolsSynchronous|ProviderImageSerialization|SynchronousResponses)' -count=1

.PHONY: test-dashscope-responses
test-dashscope-responses:
	$(GO) test ./internal/proxy -run 'Test(DashScopeResponses|ManagementDashScope|ManagementProviderKeyVerificationUsesEveryCanonical|V2.*Media|ProviderImageSerialization|SynchronousResponses)'  -count=1

.PHONY: test-minimax-reasoning
test-minimax-reasoning:
	$(GO) test ./internal/proxy -run 'Test(MiniMaxReasoning|ManagementProviderKeyVerificationUsesEveryCanonical)' -count=1

.PHONY: test-minimax-m3
test-minimax-m3:
	$(GO) test ./internal/proxy -run TestMiniMaxM3 -count=1

.PHONY: test-claude-current
test-claude-current:
	$(GO) test ./internal/proxy -run TestClaudeCurrentModels -count=1
	$(GO) test ./tests -run '^TestPublicCapabilityCatalog' -count=1

.PHONY: test-gemini-current test-gemini-candidate-contract
test-gemini-current:
	$(GO) test ./internal/proxy -run '^TestGemini(CurrentModels|InlineRequestLimit)' -count=1

test-gemini-candidate-contract:
	$(GO) test ./tests -run '^TestOperational(GeminiCandidateHarness|ShellScriptsDoNotUseHeredocs)' -count=1

.PHONY: test-gemini-transcription
test-gemini-transcription:
	$(GO) test ./internal/proxy -run '^TestGeminiTranscription' -count=1

.PHONY: test-live-candidate-contract
test-live-candidate-contract:
	$(GO) test ./tests -run TestOperationalLiveCandidateCatalogIsolation -count=1

.PHONY: test-live-minimax-m3
test-live-minimax-m3:
	LIVE_ENV_FILE="$(LIVE_ENV_FILE)" ./scripts/test_live_providers.sh --candidate-model minimax/minimax-m3
	LIVE_ENV_FILE="$(LIVE_ENV_FILE)" ./scripts/test_live_providers.sh --candidate-model minimax/minimax-m3 --media

.PHONY: test-grok-current
test-grok-current:
	$(GO) test ./internal/proxy -run '^TestGrokCurrent' -count=1

.PHONY: test-meta-current
test-meta-current:
	$(GO) test ./internal/proxy -run '^TestMetaCurrent' -count=1

.PHONY: test-meta-transcription
test-meta-transcription:
	$(GO) test ./internal/proxy -run '^(TestMetaTranscription|TestModelActivation)' -count=1

.PHONY: test-openai-transcription-retirement test-live-openai-transcription
test-openai-transcription-retirement:
	$(GO) test ./internal/proxy -run '^TestOpenAITranscriptionRetirement' -count=1

test-live-openai-transcription:
	LLM_PROXY_LIVE_OPENAI_TRANSCRIPTION=true $(GO) test ./internal/proxy -run '^TestOpenAITranscriptionRetirementLive$$' -count=1 -v

.PHONY: test-dashscope-media-limits
test-dashscope-media-limits:
	$(GO) test ./internal/proxy -run '^TestDashScopeMedia' -count=1

.PHONY: test-qwen-current
test-qwen-current:
	$(GO) test ./internal/proxy -run '^TestQwenCurrent' -count=1

.PHONY: test-zai-current
test-zai-current:
	$(GO) test ./internal/proxy -run '^TestZAICurrent' -count=1

.PHONY: test-zai-images
test-zai-images:
	$(GO) test ./internal/proxy -run '^TestZAIImage' -count=1

.PHONY: test-baidu
test-baidu:
	$(GO) test ./internal/proxy -run '^TestBaidu' -count=1
	$(GO) test ./cmd/cli -run 'TestRootCommand(PrintsCatalogDerivedLiveDiscovery|RejectsObsoleteTenantConfiguration)' -count=1
	$(GO) test ./tests -run '^TestPublicCapabilityCatalog' -count=1

.PHONY: test-live-provider-defaults
test-live-provider-defaults:
	$(GO) test ./tests -run '^TestOperationalLiveHarnessCatalogDefault' -count=1

.PHONY: test-gemini-qualified
test-gemini-qualified:
	$(GO) test ./internal/proxy ./tests -run '^Test(GeminiQualified|GeminiCurrent|PublicCapabilityCatalog|ManagementProfileListsCurrentCatalogModels)' -count=1

.PHONY: test-astra
test-astra:
	$(GO) test ./internal/proxy -run '^TestAstra' -count=1

.PHONY: test-live-astra-capabilities
test-live-astra-capabilities:
	LLM_PROXY_LIVE_ASTRA=true $(GO) test ./internal/proxy -run '^TestAstraLive$$' -count=1 -v
