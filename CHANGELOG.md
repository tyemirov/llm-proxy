# Changelog

All notable changes to this project will be documented in this file.

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

