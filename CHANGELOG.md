# Changelog

All notable changes to this project will be documented in this file.

## [v0.2.16] - 2026-06-08

### Changes
- Merge pull request #173 from tyemirov/gix/add-canonical-post-v2-messages-only-chat-endpoint-and
- docs: update ISSUES.md with recent bugfixes, improvements, and features
- feat: add Anthropic provider support with message-based API client
- docs: update provider routing plan with Anthropic and Grok details
- feat(cli): add Anthropic and Grok providers to config and tests
- docs: add Anthropic and Grok provider support details to README
- docs: add detailed feature spec for self-service API key and tenant UI
- feat: add v2 messages-only API support with ClientMessage and validation
- feat: add v2 messages API support with ordered chat messages
- feat: add /v2 messages-only API with explicit ordering and OpenRouter support
- feat(proxy): add structured chat message handling with validation and ordering
- docs: update provider routing plan for multi-provider and message ordering
- docs: update README with canonical POST /v2 chat messages endpoint
- test: add ClientMessagesRequest to python-root-import-test check

## [v0.2.15] - 2026-06-06

### Changes
- Merge pull request #172 from tyemirov/gix/replace-global-defaults-with-per-tenant-secrets-and
- docs: clarify model default selection between tenant and provider in README
- test: update tests to use Tenants config instead of ServiceSecret
- fix: update test configs to use tenants structure for coverage and live tests
- feat: add tenant-authenticated defaults with per-tenant secrets and settings
- refactor: replace service secret with multi-tenant configuration support
- docs: update provider routing plan for tenant-based defaults and validation
- refactor(cli): replace global defaults with per-tenant configuration
- docs: update README to use tenant secrets and tenant-based defaults

## [v0.2.14] - 2026-06-06

### Changes
- chore: remove stale Python bytecode files from repository

## [v0.2.13] - 2026-06-06

### Changes
- Merge pull request #171 from tyemirov/gix/refactor-slow-release-gate-tests-for-deterministic
- test: improve integration tests for queue, long requests, and retries
- scripts: add coverage probe timeout and reduce CI default timeouts to 350s
- docs: update ISSUES.md with refactor plan for slow release-gate tests
- test: improve timeout handling and error mapping in proxy tests and utils
- docs: update default timeout for release lifecycle commands to 350s
- Merge pull request #170 from tyemirov/gix/extend-release-lifecycle-scripts-with-configurable-make
- chore: add pyproject.toml for llm-proxy-client packaging
- ci(scripts): add configurable CI timeout with validation for deploy, publish, release
- ci: extend and unify local CI timeout in release, publish, deploy scripts
- docs: update README with Python test, lint, and release timeout info
- test: add python-root-import-test to verify root imports in python-test
- chore: update .gitignore to exclude Python cache and egg-info files
- Merge pull request #169 from tyemirov/gix/add-importable-python-llm-proxy-client-package
- handle TimeoutError as LLMProxyTransportError in client requests
- docs: update resolution note to include timeout coverage improvement
- ci: add Python setup and uv install to GitHub Actions test workflow
- feat: add Python client for llm-proxy JSON POST text requests
- feat: add importable Python llm-proxy client package with dataclasses and tests
- docs: add Python client package usage and local development instructions
- build: add Python lint and test targets to Makefile
- chore: ignore Python environment and cache directories in .gitignore
- Merge pull request #168 from tyemirov/gix/add-installable-llm-proxy-client-command-and-reusable-go
- feat: add llmproxyclient package with HTTP JSON POST client
- feat: add llm-proxy-client command to send JSON POST prompt requests
- fix: update import paths from temirov to tyemirov in test files
- build: add client coverage to check_coverage.sh script
- feat: add installable llm-proxy-client command and reusable Go client library
- fix: update import paths from temirov to tyemirov across codebase
- chore: update module path to github.com/tyemirov/llm-proxy
- fix: update import paths from temirov to tyemirov in CLI package
- docs: add usage instructions for installable llm-proxy-client prompt client

## [v0.2.12] - 2026-06-05

### Changes
- Merge pull request #167 from tyemirov/gix/switch-to-yaml-config-file-for-service-configuration
- test: update expected error messages in integration2_test.go
- test: improve coverage checks and live Gemini test config handling
- chore: complete config.yml as sole service configuration source
- refactor(config): add validated flag and unify configuration validation
- docs: update provider routing plan with detailed config.yml schema
- refactor: replace CLI config helpers with unified YAML config file loader
- docs: update README to use YAML config file for service configuration

## [v0.2.11] - 2026-06-04

### Changes
- Merge pull request #166 from tyemirov/gix/exclude-thought-parts-and-enforce-finishreason-in
- test: add default OpenAI API key and dictation provider in Gemini live test script
- docs: clarify live Gemini test setup with placeholder OpenAI key
- add script to test live Gemini generateContent API with llm-proxy
- fix: stop sending Gemini response-only `thought` fields in requests
- fix(gemini): update request/response structs and omit 'thought' in requests
- docs: add make target for live Gemini testing in README.md
- build: add test-live-gemini target to Makefile
- fix(gemini): return only final answer text and error on partial output

## [v0.2.10] - 2026-06-03

### Changes
- Merge pull request #165 from tyemirov/gix/enforce-provider-specific-max-tokens-limits-for-gemini
- docs: add detailed ISSUES.md backlog with Gemini POST response bug B001 analysis
- feat(gemini): enforce finishReason validation and filter internal thoughts
- docs: document max_tokens validation and Gemini token ceiling limit
- docs: document Gemini max_tokens limit and 400 Bad Request behavior

## [v0.2.9] - 2026-06-03

### Changes
- Merge pull request #164 from tyemirov/gix/remove-global-max-output-tokens-config-and-add-per
- test: update semantic review test and remove unused token config
- docs: update issues.md with max_tokens config removal and request-level usage
- feat(proxy): support max_tokens query and JSON param mapping to max_output_tokens
- docs: document optional max_tokens parameter in provider routing plan
- remove max_output_tokens config flag and environment binding from CLI root command
- docs: document per-request max_tokens output length cap

## [v0.2.8] - 2026-06-03

### Changes
- Merge pull request #163 from tyemirov/gix/add-gemini-provider-support-with-native-generatecontent
- docs: add Gemini native text provider feature to issues.md
- feat: add Gemini provider support for text generation
- docs: add Gemini provider to routing plan and configuration
- feat(cli): add Gemini API key and base URL configuration flags
- docs: add Gemini provider details and usage examples to README

## [v0.2.7] - 2026-06-03

### Changes
- feat: add normalized token usage headers and JSON usage field

## [v0.2.6] - 2026-06-03

### Changes
- test: update coverage_edges_test.go and ISSUES.md for accuracy
- Merge pull request #162 from tyemirov/sync/llm-proxy/internal
- test: add integration test for gateway context timeout canceling upstream request
- fix: cancel upstream text generation on downstream request timeout
- fix: propagate context for OpenAI requests and improve poll timeout handling
- Merge pull request #161 from tyemirov/sync/llm-proxy/readme
- docs: document request timeout knobs for gateway alignment
- docs: add request_timeout and upstream_poll_timeout to README variables table

## [v0.2.5] - 2026-05-25

### Changes
- Merge pull request #160 from tyemirov/bugfix/B405-large-semantic-review-budget
- Fix B405 large semantic review budget

## [v0.2.4] - 2026-05-24

### Changes
- fix proxy constants and improve test coverage for router and openai modules

## [v0.2.3] - 2026-05-24

### Changes
- test: add extensive coverage tests for proxy request handling and formats

## [v0.2.2] - 2026-05-24

### Changes
- Merge pull request #159 from tyemirov/codex/multi-provider-default-provider-and-env-renames
- fix repository gitignore defaults
- fix multi-provider review follow-ups
- add multi-provider default and env var updates
- Merge pull request #157 from tyemirov/maintenance/issues-md-init/master
- Merge pull request #158 from tyemirov/improvement/B001-llm-proxy-model-aliases-json-body
- Standardize Makefile automation targets
- Improve model support and JSON chat request handling
- Future development
- Future development
- chore: seed planning templates
- chore: seed planning templates
- ci: validate changelog before building release image

## [v0.2.1] - 2026-02-22 

### Bug Fixes 🐛                                                                                                                                      - Fill up for the missed puch of CHANGELOG fore v0.2.0

## [v0.2.0] - 2026-02-22 

### Features ✨
- Add authenticated `/dictate` POST endpoint for audio transcription with model override support
- Support multipart form-data audio upload with size limits and shared-secret authentication

### Improvements ⚙️
- Enhance README with `/dictate` endpoint documentation and usage examples
- Add new configuration options for dictation model default and max audio input size
- Extend CLI flags and environment variables for dictation endpoint settings
- Introduce implementation plan document for the dictation endpoint

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- Add comprehensive unit and integration tests for `/dictate` handler, including validation and upstream error handling
- Verify reasoning fields and model compatibility for GPT-5 web search testing

### Docs 📚
- Document GPT-5 web search support and new dictation endpoint in README
- Add implementation planning document outlining `/dictate` endpoint design and test plan

## [v0.1.0] - 2025-09-06

### Added
- HTTP service that forwards prompts to OpenAI's Responses API.
- Per-request model selection, regular prompt handling, and optional web search.
- Responses available as plain text, JSON, XML, or CSV.

### Limitations
- Supports only OpenAI models; no other providers currently.

