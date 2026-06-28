# Changelog

All notable changes to this project will be documented in this file.

## [v0.2.19] - 2026-06-28

### Changes
- Merge pull request #178 from tyemirov/gix/add-self-service-tenant-management-ui-and-db-migration
- fix: set Content-Type header only for non-GET requests in backendClient.js
- feat(management): add mutation middleware to enforce CORS and JSON content type
- fix: reject invalid origins and content types in management API mutations
- fix(ui): remove unused noDictationDefault copy and option element
- validate tenant defaults separately for runtime and catalog management modes
- docs: add detailed management mode issues and resolutions in .mprlab/ISSUES.md
- feat: integrate MPR UI shell with dynamic YAML config loading
- feat(management): add UI config YAML endpoint and expand management settings
- docs: update management UI and API hosting details in provider-routing-plan.md
- chore: add new management server config options to config.yml
- feat(cli): add management UI config and site rendering command
- docs: update management UI config and GitHub Pages deployment instructions
- chore: move management frontend to GitHub Pages with API-served config
- chore: update .gitignore to exclude new config and site files
- ci: enhance GitHub Pages workflow to build site from CLI
- docs: add detailed recurring maintenance issues to ISSUES.md
- fix: update Google client ID in UI config for authentication integration
- chore: replace gorm.io/driver/sqlite with github.com/glebarez/sqlite in proxy packages
- chore: update dependencies and replace go-sqlite3 with glebarez/go-sqlite
- chore: replace mattn/go-sqlite3 with glebarez/sqlite and update deps
- docs: clarify management mode SQLite driver and config environment requirements
- fix(ansible): update llm-proxy resource names and hosts to llm-proxy-api
- chore: parameterize management config with environment variables
- test: enhance root command test for management environment config
- docs: update README with environment variable placeholders and deployment notes
- fix: update default gateway deploy target in Makefile to backend
- docs: add forward-only contract discipline to AGENTS.md
- chore: remove AGENTS.md, update AGENTS.GO.md and POLICY.md for DB and libs
- feat(site): add key management UI and backend client integration
- add Ansible resource definitions for llm-proxy deployment
- fix: update default deploy target and env var handling in deploy and release scripts
- fix: increase default client timeout from 260s to 390s
- chore: increase default timeout from 260s to 390s in llm-proxy-client
- feat(management): add authenticated management API and configuration support
- chore: update go.sum with new dependencies and module versions
- chore: add JWT and GORM dependencies to go.mod for auth and DB support
- docs: document management mode config, validation, and UI deployment details
- config: increase request timeout and add management API config section
- feat: add management API config support with validation and tests
- docs: add detailed self-service management UI and split-origin setup instructions
- chore: enforce GORM-only DB access and update data driver in backend guidance
- chore: ignore SQLite database files in configs directory
- ci: add GitHub Actions workflow to deploy GitHub Pages

## [v0.2.18] - 2026-06-10

### Changes
- Merge pull request #177 from tyemirov/gix/update-default-gateway-deploy-target-to-deploy-llm-proxy
- fix(deploy): update default gateway target to deploy-llm-proxy
- docs: correct default deploy target to deploy-llm-proxy in README
- fix Makefile default deploy target to deploy-llm-proxy
- docs: update ISSUE.md with live retry and gateway timeout analysis

## [v0.2.17] - 2026-06-09

### Changes
- Merge pull request #175 from tyemirov/gix/support-resumable-openai-background-responses-with
- test: verify timeout releases queue slot before worker acquisition in coverage provider
- docs: add issue B005 describing PR merge CI limiter coverage gap and fix
- fix: clarify model label in live provider smoke test logs
- refactor: migrate client to llm-proxy v2 messages-only JSON POST API
- refactor: remove legacy JSON POST support and enforce v2 messages-only API
- refactor(llm-proxy-client): switch to v2 JSON POST request format
- refactor(openai): unify response handling with snapshot abstraction
- docs: clarify provider default model and bundled client POST /v2 usage
- test(cli): add case for blank gemini text default model validation error
- docs: update README to reflect v2-only client API changes
- fix: correct import names in python-root-import-test Makefile target
- docs: update issue tracker with OpenAI polling fix and default model handling
- test: add integration test for concurrency and queue behavior
- feat(proxy): add concurrency-limited HTTP client for upstream requests
- docs: clarify server.workers and server.queue_size limits in routing plan
- docs: clarify server.workers and server.queue_size concurrency limits
- docs: add detailed issue I010 on decoupling OpenAI polling from worker occupancy
- Merge branch 'master' into gix/support-resumable-openai-background-responses-with
- Merge pull request #176 from tyemirov/issues-md-1781049635220
- Update ISSUES.md
- Update ISSUES.md
- test: remove UpstreamPollTimeoutSeconds and simplify semantic review tests
- refactor: remove background response resume logic and lower default timeout
- refactor: remove background response resume logic and related tests
- chore: reduce default timeout from 600s to 260s in llm-proxy-client
- refactor: remove deprecated upstream poll timeout feature and related code
- docs: clarify OpenAI Responses polling and timeout behavior in provider routing plan
- chore: increase server request timeout from 180s to 240s
- refactor: remove UpstreamPollTimeoutSeconds from server config
- docs: clarify REST contract and update request timeout in README
- fix: remove manual timeout tuning for OpenAI background semantic-review calls
- test: add integration tests for background semantic review responses
- feat(client): add support for resumable OpenAI background responses
- feat: add support for resuming OpenAI background responses on 504 Gateway Timeout
- chore: increase default timeout from 120s to 600s in llm-proxy-client
- feat(proxy): add resume endpoint for polling incomplete OpenAI responses
- docs: document OpenAI Responses background polling and resumable 504 errors
- docs: document resumable OpenAI background response 504 handling
- fix: enable resumable OpenAI background responses to avoid manual timeout tuning
- Merge pull request #174 from tyemirov/gix/make-static-provider-config-explicit-and-add-dynamic
- docs: clarify config loader behavior for missing provider api_key placeholders
- fix(config): error on missing config placeholders except optional API keys
- docs: clarify handling of missing provider API key placeholders
- docs: add detailed issue B002 on long semantic-review POST transport failures
- test: unify integration test router setup with integrationConfiguration helper
- test_live_providers.sh: allow empty model override and improve logging
- test: unify tests to use explicit provider model catalogs
- docs: clarify config validation and provider API key requirements
- fix: revert default Gemini model to gemini-2.5-flash in config.yml
- fix: enforce API key validation for tenant default providers with aliases
- docs: update README to reflect Gemini default model change and API key handling
- docs: update ISSUES.md with provider config improvements and API key rules
- chore: add comprehensive default configuration file with providers and models
- test: remove deprecated adaptive tools tests and add web_search capability rejection test
- test: unify live provider smoke tests into a single script
- feat: add provider model catalog and enhance config with OpenAI URLs
- docs: update provider routing plan with multi-provider model catalog config
- feat(config): add comprehensive provider config validation and model catalogs
- docs: update README with detailed provider model catalog schema and defaults
- build: add test-live-providers target to Makefile
- chore: update ISSUES.md with completed provider config and live test improvements
- Runbook consolidated

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

