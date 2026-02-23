# Dictation Endpoint Plan (`/dictate`)

Issue: `ISSUES.md` -> `WB-001` (workspace-local tracker)

## Goal
Provide an authenticated audio-transcription endpoint in `llm-proxy` so downstream apps can route dictation through proxy infrastructure instead of calling OpenAI directly.

## Target API Contract
- `POST /dictate?key=SERVICE_SECRET[&model=MODEL_ID]`
- `Content-Type: multipart/form-data`
- request audio part name: `audio` (alias `file` for compatibility)
- success response: JSON `{ "text": "..." }`

## Implementation Path
1. Add constants/config in `internal/proxy`:
   - default transcriptions URL (`https://api.openai.com/v1/audio/transcriptions`)
   - default dictation model (`gpt-4o-mini-transcribe`)
   - max input audio bytes (configurable)
2. Extend endpoint configuration in `internal/proxy/endpoints.go`:
   - add getter/setter/reset for transcriptions URL (parallel to responses/models)
3. Add transcription client flow in `internal/proxy/openai.go` (or `openai_dictation.go`):
   - build multipart upstream request (`model`, `file`)
   - set bearer auth from `OpenAIKey`
   - parse upstream payload (`text`, `transcript`, `output_text` fallback)
   - normalize and return trimmed text
4. Extend router in `internal/proxy/router.go`:
   - register `POST /dictate` route
   - reuse `secretMiddleware` (query `key`)
   - enforce multipart/size limits and missing-file validation
   - return proxy-mapped status codes (`400`, `502`, `504`)
5. Update README endpoint docs and curl examples for `/dictate`.

## Test Plan
- Unit tests:
  - route method/validation/missing audio
  - transcription parser shape fallbacks
  - upstream status and read/parse failure handling
- Integration tests:
  - successful `/dictate` proxy call
  - model override via query
  - shared-secret enforcement for `/dictate`

Run repository test command (per CI workflow): `go test ./...`

## Repo Management Notes
- Testing workflow in `.github/workflows/test.yml` runs `go test ./...` on PRs.
- Release process in `README.md` requires:
  1. update `CHANGELOG.md`
  2. tag `vX.Y.Z`
  3. push branch and tag (release workflow publishes binaries and release notes).
