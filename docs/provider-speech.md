# Provider Speech

The provider catalog defines speech models, transports, controls, and account fields in `configs/providers.yml`.
ElevenLabs has one provider definition and one account connection for its current resources, services, and model routes.
The gateway and public route explorer use this catalog.

## Speech Generation

The `audio.speech.generate` capability adds these exact source models:

| Model | Native text limit | Speed mapping |
| --- | --- | --- |
| `eleven_multilingual_v2` | 10,000 characters | Native speed field |
| `eleven_flash_v2_5` | 40,000 characters | Native speed field |
| `eleven_turbo_v2_5` | 40,000 characters | Native speed field |
| `eleven_v3` | 5,000 characters | Source pacing tags |

The catalog selects `native_speed` or `text_pacing` through a typed request-codec variation.
The adapter does not select behavior from provider or model names.
Turbo v2.5 retains its exact source identifier.
Its declared limit follows the current native equivalence with Flash v2.5.

Use the common media operation API with an owned voice reference and nonempty text.
Set `output_format` and `timestamps` explicitly.
Optional controls preserve stability, similarity, style, speaker boost, speed, seed, language, and normalization.
The catalog declares the supported languages and account-dependent controls for each model.

The current native API does not support `language_code` for Multilingual v2.
Its offering omits that control and rejects a language override before submission.
This corrects the older source capability flag.
The model still determines the spoken language from input text.
Eleven v3 omits similarity and speaker boost because its native model does not support those controls.
The gateway rejects these controls for that offering before submission.

Optional operation input includes:

- `previous_text` and `next_text` for adjacent text.
- `previous_operation_ids` and `next_operation_ids` for owned continuity references.
- `dictionaries` with public `dictionary_id` and `version_id` pairs.

Each continuity direction accepts up to three references.
Each request accepts up to three dictionary references.
Continuity references require retained successful speech operations under the current tenant, provider, connection, and catalog authority.
Dictionary references require the current account and dictionary service binding.
Native dictionary, version, and request identifiers remain private.

Eleven v3 uses the source speed range from 0.7 to 1.2.
A non-default speed becomes a source pacing tag before the caller text.
The default speed of 1 and absent speed add no tag.
The native request omits its unsupported speed field.
The text limit includes the pacing prefix.

## Speech Conversion

The `audio.speech.convert` capability supports these exact source models:

- `eleven_english_sts_v2`
- `eleven_multilingual_sts_v2`

Create a media operation through `POST /model/v1/operations`.
Set the provider and exact model from authenticated capability discovery.
Supply an idempotency key for the request.

```json
{
  "capability": "audio.speech.convert",
  "provider": "elevenlabs",
  "model": "eleven_multilingual_sts_v2",
  "input": {
    "voice_id": "voi_0123456789abcdef0123456789abcdef",
    "audio_asset_id": "ast_0123456789abcdef0123456789abcdef"
  },
  "controls": {
    "input_format": "other",
    "output_format": "mp3_44100_128",
    "stability": 0.3,
    "similarity_boost": 0.6,
    "style": 0.2,
    "use_speaker_boost": false,
    "seed": 4294967295,
    "remove_background_noise": true
  }
}
```

Both input references must belong to the authenticated tenant.
The voice must belong to the selected provider and current account authority.
The gateway checks that authority before acceptance and native submission.
Native voice identifiers remain private.

The input and output formats are required.
The six other controls are optional.
The gateway omits absent settings from the native request.
The catalog defines the permitted values and numeric bounds.

| Input format | Source asset |
| --- | --- |
| `other` | MPEG audio, WAV, M4A, FLAC, or Ogg |
| `pcm_s16le_16` | Raw signed 16-bit little-endian PCM, 16 kHz, one channel |

Raw PCM input uses `application/octet-stream` and contains complete two-byte samples.
The gateway streams the stored asset into the native multipart request.
The configured asset byte limit bounds input and output.

## Output Artifacts

Speech generation and conversion preserve these first two ordered artifacts:

1. Exact native audio bytes with the selected format's MIME type.
2. A JSON audio description with `output_format`, `mime_type`, and raw audio interpretation when applicable.

| Output formats | MIME type |
| --- | --- |
| `mp3_22050_32`, `mp3_24000_48`, `mp3_44100_32`, `mp3_44100_64`, `mp3_44100_96`, `mp3_44100_128`, `mp3_44100_192` | `audio/mpeg` |
| `opus_48000_32`, `opus_48000_64`, `opus_48000_96`, `opus_48000_128`, `opus_48000_192` | `audio/ogg` |
| `wav_8000`, `wav_16000`, `wav_22050`, `wav_24000`, `wav_32000`, `wav_44100`, `wav_48000` | `audio/wav` |
| `pcm_8000`, `pcm_16000`, `pcm_22050`, `pcm_24000`, `pcm_32000`, `pcm_44100`, `pcm_48000` | `application/octet-stream` |
| `alaw_8000`, `ulaw_8000` | `application/octet-stream` |

Raw audio descriptions contain `encoding`, `sample_rate_hz`, and `channels`.
The encoding is `s16le`, `alaw`, or `mulaw`. The channel count is one.
The gateway preserves bytes. Local audio conversion remains in MediaOps.
The selected provider account can restrict output formats.

Generation with `timestamps: true` adds a third JSON artifact.
It contains `alignment` and `normalized_alignment`, including explicit null values when the provider omits an observation.
Each present alignment contains characters, start times, and end times.
The gateway validates array lengths, nonnegative times, character order, and pacing prefixes.
It removes a verified pacing prefix from both arrays and preserves the remaining provider times.

## Durable Execution

The gateway saves native request and history identifiers as private operation evidence before it publishes artifacts.
A rejected native request records a failure.
An ambiguous native result records an uncertain operation.
A process restart never repeats a dispatched speech request.
History retrieval remains separate, unfinished F026 work.

Queued operations support cancellation.
Native speech generation and conversion do not support cancellation after dispatch.
Repeated requests with the same idempotency key return the same operation.

## Validation Scope

Public HTTP tests cover both models and all 28 formats under two catalog provider identities.
They check multipart fields, exact bytes, artifact interpretation, private identifiers, account changes, cancellation, and restart behavior.
The tests use a local native protocol server.
They establish gateway behavior under controlled responses, not live account availability or audio quality.

Speech tests cover all four source models, owned dictionary and continuity references, timestamps, formats, and every source pacing interval.
History, voice-library operations, music methods, imports, and consumer acceptance remain open under F026.
The complete source inventory is in [media provider completeness](media-provider-completeness.md).

The native protocol reference is [ElevenLabs voice conversion](https://elevenlabs.io/docs/api-reference/speech-to-speech/convert).

Native references: [speech generation with timestamps](https://elevenlabs.io/docs/api-reference/text-to-speech/convert-with-timestamps) and [model limits](https://elevenlabs.io/docs/overview/models).

The [native speech guide](https://elevenlabs.io/docs/eleven-creative/playground/text-to-speech) describes the model-specific control limits.
