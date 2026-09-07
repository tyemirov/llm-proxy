# OpenAI transcription retirement

I246 replaces both deprecated OpenAI transcription selectors with `gpt-transcribe`.
The replacement applies to the existing complete-response file contract.
The current key passed model discovery on September 6, 2026.
Both public transcription interfaces passed on September 7, 2026.

## Provider contract

The [file guide](https://developers.openai.com/api/docs/guides/speech-to-text) recommends `gpt-transcribe` for completed recordings.
The [API reference](https://developers.openai.com/api/reference/resources/audio/subresources/transcriptions/methods/create) defines multipart `model` and `file` fields.
The request uses bearer authentication at `POST https://api.openai.com/v1/audio/transcriptions`.
The default buffered JSON response contains `text` and detected `languages`.
The API can report duration usage as `usage.type: duration` and `usage.seconds`.
The current proxy result remains a complete JSON `text` response.
The managed ledger retains its existing request, outcome, latency, and token-estimate contract.
Provider duration billing remains distinct from those token estimates.

The guide publishes a 25 MB file limit.
The API reference lists FLAC, MP3, MP4, MPEG, MPGA, M4A, OGG, WAV, and WebM input.
The file name must identify its format.
The configured `server.max_input_audio_bytes` limit remains the public admission bound.
The provider validates file encoding and its published upload limit.

The [model page](https://developers.openai.com/api/docs/models/gpt-transcribe) publishes USD 0.0045 per audio minute.
The catalog records that duration rate, its source, and the verification date.
`GET /v1/models/gpt-transcribe` returned HTTP 200 and `created=1785168027` with the existing key.
The adapter sends only the model and file.
New language controls, streaming, timestamps, and speaker results require separate public contracts.
The current public multipart field validation rejects unsupported caller fields.

## Selection migration

Managed schema version 16 maps both `gpt-4o-mini-transcribe` and `gpt-4o-transcribe` to `gpt-transcribe` for OpenAI dictation.
The startup transaction changes only matching tenant dictation defaults.
It preserves provider connections, encrypted credentials, text profiles, prompts, timestamps, and historical usage identities.
Repeated startup keeps the migrated selection unchanged.
A failed tenant update or migration record rolls back the transaction.
Existing predecessor migrations use the same bounded replacement policy before current-schema validation.

The deprecated selectors are absent from routing, public discovery, and management selection.
Explicit requests for either retired selector fail before provider dispatch.
Current client examples and browser fixtures use `gpt-transcribe`.
Historical usage records retain the original model names.

## Acceptance

`make test-openai-transcription-retirement` checks public replacement routing, default routing, retired-route rejection, discovery, prices, and SQLite migration.
The migration scenarios include schema version 15, the earlier ownership schema, repeated startup, and transaction rollback.
The provider fixture returns the current JSON shape with language and duration fields.

Use a WAV fixture that says: "The quick brown fox jumps over the lazy dog."
Run the live test with the existing authorized key:

```sh
LIVE_TRANSCRIPTION_AUDIO=/absolute/path/speech.wav make test-live-openai-transcription
```

The live target sends that fixture through both public HTTP interfaces to the actual OpenAI endpoint.
It checks the recognized words and excludes transcript content and credentials from its output.
Both interfaces passed before and after catalog replacement on September 7, 2026.
Local CI and production activation are separate acceptance results.

## Operator activation

OpenAI [announced removal](https://developers.openai.com/api/docs/deprecations) of both old models for February 26, 2027.
Complete the production migration before that date.

1. Select the reviewed revision after final CI and provider acceptance pass.
2. Back up the managed SQLite database through the repository's existing operator procedure.
3. Inventory OpenAI tenant defaults that use either retired selector.
4. Apply the release and start the service through the operator-owned deployment procedure.
5. Verify schema version 16 and the migrated tenant defaults.
6. Verify that historical usage, provider connections, and unrelated settings remain intact.
7. Confirm `gpt-transcribe` in public discovery and management selection.
8. Transcribe a known fixture through each public interface with the selected tenant key.
9. Confirm that retired selectors fail before provider dispatch.

A read-only inventory on September 7, 2026, found one local tenant with `gpt-4o-mini-transcribe`.
The local database is `configs/llm-proxy-management.sqlite` and uses the earlier ownership schema.
The inventory did not change that database.
Production inventory and activation remain pending.
The user owns production deployment and production acceptance.
