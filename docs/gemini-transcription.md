# Gemini File Transcription

F053 enables `gemini-3.5-transcribe` for the existing file-dictation contract.
The route uses the tenant Gemini API key.
The Gemini text default remains `gemini-3.5-flash`.
Existing tenant defaults remain unchanged, including an empty dictation selection.
An empty dictation pair remains valid after a catalog capability change.
A tenant can select Gemini and `gemini-3.5-transcribe` as its dictation default in Settings.

## Public requests

Use `POST /dictate?provider=gemini&model=gemini-3.5-transcribe` with the multipart `audio` field and a tenant secret.
If an explicit Gemini provider omits the model, it selects `gemini-3.5-transcribe`.
The client API accepts `POST /v1/audio/transcriptions` with a multipart `file` field and `model=gemini/gemini-3.5-transcribe`.
Both return the existing JSON `text` field.
Request accounting uses the existing dictation usage records.
The route does not add token counts to those records.

The filename extension selects the audio MIME type.
Supported extensions are `.wav`, `.mp3`, `.aiff`, `.aif`, `.aac`, `.ogg`, `.flac`, `.mpeg`, `.m4a`, `.l16`, `.opus`, `.alaw`, `.mulaw`, and `.webm`.
An unsupported extension returns HTTP 400 with `invalid_audio_input` before provider dispatch.
The configured `max_input_audio_bytes` limit still applies.
Google limits synchronous files to one hour.
The provider checks audio duration.

## Provider execution

The route calls native `POST /v1beta/interactions` with `background: false` and `store: false`.
It sends one audio input with the exact uploaded bytes.
It leaves transcription configuration absent for default verbatim output and automatic language detection.
This route returns plain transcripts.
Live streaming, speaker labels, timestamps, custom vocabulary, and smart formatting require separate public contracts.

The inline JSON request limit is 20,000,000 bytes, including base64 and the JSON envelope.
Larger files use the existing Gemini Files API.
The proxy deletes uploaded provider files after success, failure, or request cancellation.
A cleanup failure returns an error without the transcript.
Provider file URIs and interaction IDs remain private.
An empty, incomplete, pending, or malformed provider result returns an error.

## Prices and acceptance

Google lists standard rates of USD 2 per million audio input tokens and USD 12 per million text output tokens.
The catalog records these rates as verified on September 5, 2026.
Google estimates a blended rate near USD 0.005 per audio minute.
Billing uses actual tokens.

On September 5, 2026, paid acceptance returned the expected words from a generated English PCM WAV sample.
The candidate harness checks the exact audio bytes and compares the returned word sequence.
Case and punctuation do not affect that comparison.

```sh
LLM_PROXY_LIVE_GEMINI_MODEL=gemini-3.5-transcribe \
LLM_PROXY_LIVE_GEMINI_AUDIO_FILE=/absolute/path/speech.wav \
LLM_PROXY_LIVE_GEMINI_EXPECTED_TRANSCRIPT='Expected spoken words.' \
make test-live-gemini-candidate LIVE_ENV_FILE=configs/.env
```

Local public HTTP tests cover both endpoints, the provider default, management selection, discovery, failure responses, and file cleanup.
Deployment and production acceptance remain operator-owned.

## Sources

- [Transcription model](https://ai.google.dev/gemini-api/docs/models/gemini-3.5-transcribe)
- [Audio transcription](https://ai.google.dev/gemini-api/docs/transcribe)
- [Gemini pricing](https://ai.google.dev/gemini-api/docs/pricing)
- [Audio input limits](https://ai.google.dev/gemini-api/docs/audio)
- [Files API](https://ai.google.dev/gemini-api/docs/files)
