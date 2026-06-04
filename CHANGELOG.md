# Changelog

All notable changes to this project will be documented in this file.

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

