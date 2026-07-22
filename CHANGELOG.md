# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

- Declare the llm-proxy TAuth tenant requirements in the app-owned deployment manifest for gateway assembly.

## [v0.2.36] - 2026-07-21

- Merge pull request #216 from tyemirov/gix/add-bounded-docker-pages-readiness-checks-to-release-and
- fix(gitrelease): bound inspection timeouts and build selection logic
- docs: document CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS in README
- docs(ISSUES): clarify Pages artifact handling and Docker manifest timeouts
- test(management-ui): improve reasoning effort, notice, and sign-in coverage
- refactor: simplify reasoning effort controls and clean up settings grid UI
- fix(management): validate reasoning effort for specific provider/model pair
- docs: clarify managed effort validation and update UI/routing-defaults behavior
- feat(config): expand reasoning_effort efforts for GPT-5 models
- test(cli): reject stale provider-level reasoning_effort in config files
- docs: clarify reasoning_effort as route-bound and update OpenAI model options
- docs(issues): document routing effort, header notice, and auto-dismiss tasks
- feat(gitrelease): verify public marker after Pages build, not branch push
- refactor(deploy): use external script for container manifest digest resolution
- docs: clarify GitHub Pages build state and marker verification steps
- docs: clarify publish and deploy wait logic and public readiness boundaries
- docs(issues): record authoritative readiness contracts for GHCR and Pages

## [v0.2.35] - 2026-07-21

- Merge pull request #215 from tyemirov/gix/add-reasoning-effort-default-and-header-notification
- test: expand and update management UI E2E coverage for header, copy, notices
- feat: add Reasoning Effort support to routing, types, and UI styles
- feat(management): expose reasoning effort options and capability metadata
- docs: describe reasoning_effort default routing and UI contract enhancements
- feat(config): add reasoning_effort to gpt-5 and 5.x provider configs
- feat(config): add reasoning_effort support to config file and model catalog
- docs: document catalog reasoning_effort and tenant default contract
- docs(issues): add B041 header notification, B042 header brand, B043 copy icon

## [v0.2.34] - 2026-07-21

- Merge pull request #204 from tyemirov/maintenance/M001R-backlog-hygiene
- Merge pull request #214 from tyemirov/bugfix/B040-web-search-log-privacy
- fix(logging): keep web search query values out of logs
- Merge pull request #205 from tyemirov/bugfix/B036-routing-default-pairs
- Merge pull request #206 from tyemirov/bugfix/B035-notification-placement
- Merge pull request #207 from tyemirov/improvement/I025-reveal-provider-keys
- Merge pull request #208 from tyemirov/maintenance/M004R-dependency-security-audit
- Merge pull request #209 from tyemirov/bugfix/B039-path-only-request-logs
- Merge pull request #210 from tyemirov/maintenance/M014-go-security-toolchain
- Merge pull request #211 from tyemirov/maintenance/M015-qpack-security-update
- Merge pull request #212 from tyemirov/maintenance/M016-pgx-security-update
- Merge pull request #213 from tyemirov/maintenance/M017-mapstructure-security-update
- chore(deps): patch mapstructure security update
- chore(deps): patch pgx security update
- chore(deps): patch QPACK dependency
- chore(deps): patch Go toolchain security release
- fix(logging): remove query content from request logs
- chore(issues): record M004 security audit findings
- feat(management): reveal saved provider keys
- fix(ui): move workspace notices above footer
- fix(routing): enforce managed default pairs
- docs(issues): normalize backlog identifiers
- Merge pull request #203 from tyemirov/gix/add-reloadable-per-user-model-profiles-to-go-python
- fix(client): normalize all model profile reader failures before HTTP request
- test(llmproxyclient): reject model profile files with invalid UTF-8 encoding
- docs(issues): note UTF-8 profile checks and Python error handling improvements
- feat: add support for dynamic model profiles via JSON file in client config
- feat: support dynamic model profiles with atomic reload and config validation
- feat(cli): add --model-profile option and tests for dynamic provider/model selection
- docs: clarify model-profile config, runtime override, and exclusivity rules
- docs: document application-user model profile support in Go and Python clients
- chore(makefile): add LLMProxyModelProfileError to python import test
- docs(issues): capture and resolve F015 reloadable client model-profile contract

## [v0.2.33] - 2026-07-20

- Merge pull request #202 from tyemirov/gix/add-qwen-cloud-and-minimax-as-selectable-text-providers
- test: add coverage for qwencloud and minimax env keys in live config
- docs(resources): add Qwen Cloud and MiniMax routes, update provider list
- feat: add Qwen Cloud and MiniMax live provider support in scripts
- feat: add Qwen Cloud and MiniMax provider support
- docs: document Qwen Cloud Token Plan and MiniMax provider support
- feat(config): add QwenCloud and MiniMax providers to config.yml
- feat(cli/config): add QwenCloud and MiniMax provider config parsing and tests
- docs: document Qwen Cloud Token Plan and MiniMax provider support
- docs(issues): track addition of Qwen Cloud and MiniMax text providers
- Merge pull request #201 from tyemirov/gix/refresh-provider-catalogs-with-current-model-releases
- test: update DashScope model references to Qwen Plus and exclude old models
- chore(config): remove qwen3.7-max and qwen3.7-plus from qwen models list
- docs(readme): remove qwen3.7-max and qwen3.7-plus references
- docs(issues): add resolution for DashScope catalog and endpoint validity
- test: add coverage for Moonshot K3, K2.7, Zhipu GLM-5.2, and model limits
- docs: clarify Kimi K3 and GLM-5.2 routing and control behavior
- chore(config): add kimi-k3, kimi-k2.7-code, and glm-5.2 model configs
- docs: document addition of Kimi K3, Kimi code models, and GLM-5.2 with limits
- docs(issues): document addition of GLM-5.2 and Kimi K3 models to catalogs
- test: update test to use gpt-5.6 for temperature suppression scenario
- test: verify support for current model catalog across providers
- docs: clarify model catalog sync and token limits in provider routing plan
- feat(config): add latest model variants for OpenAI, Qwen, Gemini, Claude, and Grok
- docs: refresh model catalog and capability matrix in README
- docs(issues): document model catalog refresh for all supported providers
- docs(issues): add user-revealable provider keys, notification placement, usage intervals
- Merge pull request #200 from tyemirov/bugfix/B037-app-owned-orchestration-manifest
- Merge pull request #199 from tyemirov/improvement/I020-declare-tauth-tenant
- Fix B037 app-owned orchestration manifest
- improvement(I020): declare app-owned TAuth tenant

## [v0.2.32] - 2026-07-15

- Merge pull request #198 from tyemirov/gix/hydrate-dashboard-from-canonical-mpr-ui-auth-lifecycle
- test: cover MPR UI auth event, contrast ratio, and error fallback flows
- fix(ui): improve error handling for profile loading and unauthenticated state
- docs(seo): clarify legacy tenant claim and replacement conditions
- fix(management): allow legacy tenant claim to replace empty destination accounts
- docs: clarify MPR UI browser authentication boundaries and session flow
- docs: clarify browser auth flow, management claim migration, and tests in README
- docs(issues): record completed MPR UI auth, legacy migration, and contrast fixes

## [v0.2.31] - 2026-07-13

- Merge pull request #197 from tyemirov/gix/exercise-real-tauth-session-boundary-with-local-stack
- test: refactor management-auth blackbox spec for Playwright browser integration
- docs: clarify management auth blackbox test flow in README
- docs(issues): add real-stack browser sign-in scenario and resolution
- test: add Playwright config for blackbox tests
- test(blackbox): add local stack harness and frontend auth E2E test
- feat(ui): support new mpr-ui auth-change events and orchestrator readiness
- chore: add new test files to syntax check and update mpr-ui stylesheet version
- chore: add mpr-ui dependency and frontend:test:blackbox script to package.json
- chore: add mpr-ui dev dependency to package-lock.json
- test: update management page test for mpr-ui v3.11.1 bundle reference
- docs: clarify management UI session restoration and sign-out behavior
- docs: document session boundary and blackbox management auth test instructions
- chore(makefile): add management auth blackbox test target to test suite
- docs(issues): add session, sign-in, and auth black-box test resolutions
- ci: trigger test workflow on any change in tests directory

## [v0.2.30] - 2026-07-13

- Merge pull request #196 from tyemirov/gix/use-canonical-tauth-sessionvalidator-align-deployment-go
- test: add deployment contract tests for gateway integration scenarios
- test: extract gateway repo fixture setup into reusable helper
- feat(deploy): enforce gateway contract and branch state before deployment
- fix(proxy): improve management session validator error handling and test coverage
- docs: update deploy section to clarify gateway and preflight contract steps
- chore(makefile): remove unused GATEWAY_DEPLOY_TARGET variable from deploy target
- docs(ISSUES): document contract coupling and gateway deployment checks
- refactor(proxy): delegate session validation to tauth/sessionvalidator
- chore(deps): update Go module dependencies in go.sum
- chore(deps): update Go to 1.25.4 and upgrade dependencies in go.mod
- docs: document backend use of tauth sessionvalidator and rollout coordination
- docs: clarify TAuth session validation and deployment contract in README
- build: update Dockerfile to use golang:1.25-bookworm as builder image
- docs(issues): document session validation and deployment contract unification
- ci: update Go version to 1.25.4 in test workflow

## [v0.2.29] - 2026-07-12

- Merge pull request #195 from tyemirov/gix/retry-management-profile-after-mpr-ui-authenticated
- test(management-ui): verify MPR dashboard loads after authentication refresh
- refactor(ui): migrate mpr-ui shell integration to auto orchestration
- chore: update build-pages-artifact.sh for stricter artifact validation
- test(management): update required and forbidden fragments for index.html
- docs: clarify static management UI config contract and app orchestration
- feat(cli): validate and inject --site-config-url in static site renders
- docs: clarify static UI config injection and PAGES_CONFIG_URL handling in README
- build: add PAGES_CONFIG_URL to Makefile and pass to pages-artifact target
- docs(issues): record fix for management profile retry on delayed auth event

## [v0.2.28] - 2026-07-12

- chore: update release script, tests, and documentation
- Merge pull request #194 from tyemirov/gix/enforce-help-output-contract-and-heredoc-prohibition-for
- refactor(gitrelease): simplify usage/help output and prefer python3 -c in scripts
- test: broaden operational contract coverage for shell script and container
- refactor(scripts): simplify heredoc usage to builtin printf for consistency
- docs: resolve B023 and B024 in issues log with implemented validation details
- Merge pull request #193 from tyemirov/gix/add-loopaware-traffic-pixel-to-all-site-and-resource
- test: validate .nojekyll and marker contract for Pages artifact deploys
- test(e2e): update management-ui tests for open attribute and asset route mocking
- feat(site): add analytics pixel to all index/resource pages
- feat(seo): inject Loopaware analytics pixel into generated resources
- docs: update SEO resource cluster report date to 2026-07-11
- docs(issues): document Pages marker preservation and LoopAware pixel addition
- Merge pull request #192 from tyemirov/gix/accurately-track-upstream-rate-limit-wait-durations-and
- test: validate Pages deploy logic for pushInsteadOf and remote mutation
- docs(issues): document Pages deployment push URL validation and enforcement
- refactor(gitrelease): extract and improve Pages artifact validation logic
- test: add operational and integration coverage for release tag and preflight logic
- docs: clarify role of upstream_rate_limits in reliability resources
- chore(scripts): validate release tag and require python3 in deploy.sh
- fix(proxy): improve rate limit wait time accounting in limited_http.doer
- chore(makefile): add --remote flag to publish-release target
- docs(issues): clarify upstream rate limit enforcement after worker acquisition
- Merge pull request #188 from tyemirov/gix/add-meta-model-api-support-and-upstream-http-rate
- Merge remote-tracking branch 'origin/master' into gix/add-meta-model-api-support-and-upstream-http-rate
- feat(release): add immutable release toolchain and black-box tests
- test: add operational contract and rate limit integration test coverage
- docs: update resource metadata dates and sitemap for July 9, 2026
- chore: improve release, deployment, and live provider test automation
- feat: add support for legacy tenant migration in management mode
- docs: document static-tenant validation, release implementation, migration flow
- refactor(config): remove old tenant and provider api_key config, enable legacy token migration
- test: add legacy_token_migration config to managementConfiguration struct
- docs: update README for stateless management migration and live test harness
- build(makefile): update targets and variables for new release tooling
- docs(issues): record Meta, upstream-rate, release regression fixes and token migration
- ci: trigger tests on changes in tools/gitrelease directory
- Merge pull request #191 from tyemirov/issues-md-1783632219288
- Update ISSUES.md
- Merge pull request #190 from tyemirov/issues-md-1783632144494
- Update ISSUES.md
- Merge pull request #189 from tyemirov/issues-md-1783632087239
- Update ISSUES.md
- test: add e2e and integration tests for Meta provider and upstream rate limits
- docs(resources): add Meta Muse Spark reference to provider lists and usage samples
- refactor(scripts): unify Pages release pipeline and update for Meta Muse
- feat(proxy): add support for Meta provider and configurable upstream rate limits
- docs: document Meta support and upstream rate limit configuration
- feat(config): add Meta provider and MODEL_API_KEY support in config files
- feat(cli): support and test Meta provider and upstream rate limit config
- docs: add Meta Muse Spark support and upstream rate limit docs
- chore(makefile): refactor release and publish targets for new artifact workflow
- docs(issues): document upstream HTTP rate limiting and Meta provider support
- ci: remove GitHub Actions release workflow

## [v0.2.27] - 2026-07-06

### Changes
- Merge pull request #187 from tyemirov/gix/add-45-repo-grounded-seo-resource-pages-hub-sitemap-and
- docs: add SEO resource cluster report for LLM Proxy use-case planning
- test(e2e): add SEO crawlability, sitemap, robots checks for resource pages
- feat(site): add public resources hub with repo-grounded implementation guides
- chore(scripts): add generate_seo_resources script for SEO resource generation
- docs: track SEO resource cluster and social campaign deliverables in ISSUES.md

## [v0.2.26] - 2026-07-03

### Changes
- Merge pull request #186 from tyemirov/gix/ensure-settings-modal-layers-above-mpr-header-footer
- test: add assertions for notice layer and z-index in settings modal overlay
- fix(ui): lower z-index of notice overlay to 2100
- docs: clarify notice and modal overlay stacking in Settings resolution
- test: verify settings modal overlays MPR header/footer with correct stacking
- fix(ui): update z-index for settings overlay and notices for better stacking
- docs(issues): mark settings modal layout as complete and add resolution details
- chore(gitignore): ignore Playwright output directory
- Merge pull request #185 from tyemirov/gix/fold-settings-request-examples-as-a-single-usage-segment
- Merge remote-tracking branch 'origin/master' into gix/fold-settings-request-examples-as-a-single-usage-segment
- test: expand request examples section in settings before asserting visibility
- feat(ui): add collapsible usage examples section to management UI
- chore(deploy): remove obsolete Ansible resources definition
- feat(ui): fold Settings request examples into collapsible usage segment
- Merge pull request #184 from tyemirov/issues-md-1783111819653
- Update ISSUES.md
- Update ISSUES.md
- Update ISSUES.md

## [v0.2.25] - 2026-06-30

### Changes
- feat: update key management UI and provider routing documentation

## [v0.2.24] - 2026-06-30

### Changes
- Merge pull request #183 from tyemirov/gix/store-per-provider-text-model-and-system-prompt-in
- test: add management UI tests for settings request examples and secret generation
- feat(ui): add provider model and system prompt editing to settings panel
- feat(management): support tenant text model and system prompt per provider
- docs: update provider key record/storage contract for model and system prompt
- docs: clarify provider key UI, text model, and system prompt management
- feat: enable per-provider model and prompt settings in management UI

## [v0.2.23] - 2026-06-30

### Changes
- Merge pull request #182 from tyemirov/gix/make-management-admin-config-plural-and-env-backed
- test: update management UI E2E to assert config url is not exposed
- refactor(frontend): simplify config URL detection and remove placeholder attribute
- chore(scripts): add publish_pages.sh for static GitHub Pages publishing
- refactor: remove unused ManagementConfigUIURL function
- docs: clarify GitHub Pages deployment and artifacts in management UI section
- refactor(cli): decouple static site render from backend config, update tests
- docs: clarify GitHub Pages branch setup, publishing, and runtime config
- build(makefile): add publish-pages target and support for pages env vars
- docs(issues): record B015 Pages deployment exit from Actions, backend-owned config
- ci: remove GitHub Pages workflow and references from test workflow
- docs: clarify admin email env placeholder in management config section
- feat(config): add .env.sample and allow multiple admin emails via env variable
- test(cli): add support for multiple management admin emails in config
- docs: update admin_emails config usage and environment placeholder guidance
- docs(issues): document fix for plural and deployable management admin config

## [v0.2.22] - 2026-06-30

### Changes
- test: improve coverage and assertions in client main_test.go
- Merge pull request #180 from tyemirov/feature/F007-dashboard-settings-modal
- Merge branch 'master' into feature/F007-dashboard-settings-modal
- Merge pull request #181 from tyemirov/issues-md-1782774534841
- Apply ISSUES.md execution changes
- Update ISSUES.md
- test: verify usage refresh clears stale metrics on summary reload failure
- fix(key-management): reset usage summary on fetch error
- fix(proxy): record usage validation failures and add missing usage indexes
- docs(issues): document usage dashboard validation and index fixes in B012
- test: add admin users dashboard and asset route coverage to management UI
- feat: add admin dashboard view with managed user usage and UI polish
- feat(management): add admin user listing and encrypted provider key support
- docs: document admin_emails and provider_key_encryption_key requirements
- feat(config): add admin_emails and provider_key_encryption_key to management config
- feat(cli): add provider_key_encryption_key to management config
- docs: document admin_emails and provider_key_encryption_key config fields
- docs(issues): record B011 usage fixes, I015 icon, I016 key encryption, F009 admin UI
- ci: add admin email and provider key encryption to Pages workflow env
- test(e2e): add management UI end-to-end tests for dashboard and settings modal
- feat(scripts): add frontend JavaScript syntax check script
- chore: add Playwright config for end-to-end tests
- chore: add package.json with frontend lint and test scripts
- chore: add package-lock.json to lock dependencies versions
- feat(ui): add usage dashboard and settings menu integration
- feat(management): add usage summary API and usage event recording
- docs: add usage-focused details for authenticated management landing view
- docs: update management mode usage dashboard and API details
- build: add frontend lint and test targets to Makefile
- feat(management): add usage dashboard and Settings modal in avatar menu
- chore: update .gitignore to exclude node_modules and test artifacts
- ci: add Node.js and Playwright setup to test workflow

## [v0.2.21] - 2026-06-29

### Changes
- Merge pull request #179 from tyemirov/gix/align-management-header-avatar-to-right-edge-in-llm
- style: align header actions to the right in llm-proxy UI
- docs: document header avatar alignment to right edge in static site CSS

## [v0.2.20] - 2026-06-29

### Changes
- docs: update GitHub Pages workflow and documentation files

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
