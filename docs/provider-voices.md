# Provider Voices

One provider resource defines voice discovery in `configs/providers.yml`.
Dictator and ElevenLabs use the same tenant voice API.
ElevenLabs uses the existing provider connection and `api_key` field.
Voice observations do not create model offerings.

## Collection Contract

`GET /model/v1/voices` returns `voices`, `has_more`, `total_count`, and `next_cursor`.
The request requires the tenant bearer key and the `provider` query field.
The first page accepts these optional query fields:

| Field | Values |
| --- | --- |
| `page_size` | Integer from 1 through 100. The default is 10. |
| `search` | Voice name search, with at most 1000 characters |
| `sort` | `name` or `created_at_unix` |
| `sort_direction` | `asc` or `desc` |
| `voice_type` | `personal`, `community`, `default`, `account`, `non-default`, `non-community`, or `saved` |
| `category` | `premade`, `cloned`, `generated`, or `professional` |
| `include_total_count` | `true` or `false` |

Use only `provider` and `cursor` for a continuation request.
The encrypted cursor retains the first-page query and private native page token.
It binds the page to the tenant, provider, account authority, and catalog revision.
It expires after 15 minutes.
An invalid or stale cursor returns `400` before native discovery.

A missing count uses `total_count: null`.
The final page uses `has_more: false` and `next_cursor: null`.
The gateway preserves native page order and page boundaries.
ElevenLabs can return default voices in addition to the requested page size.
The public `account` voice type selects the native `workspace` voice type.
The public API does not accept the private native value.

Dictator discovery includes preset voices and retained extracted voices under the current account authority.
Its local pages support name search, name sorting, direction, page size, and total count.
Dictator rejects provider-specific category, voice type, and creation-time filters.

## Voice Observations

Each voice has a tenant-owned `voi_` identifier.
The response preserves descriptions, categories, labels, high-quality model observations, and verified languages.
Each verified language preserves its language, model, accent, locale, and preview link.
An absent language or default sample rate uses `null`.
Unknown sample rates use an empty array.
The gateway does not invent speech-engine values for hosted voices.

Native voice IDs and preview URLs remain private.
A connection change does not rebind an existing voice ID to a different account authority.
`GET /model/v1/voices/{voice_id}` rejects a voice from an absent or obsolete connection with `404`.
Retained Dictator records receive the current empty metadata and preview shapes during database migration.
The gateway uses only the current stored shape after this bounded migration.

## Preview Access

A `preview` field contains a gateway path or `null`.
`GET /model/v1/voices/{voice_id}/previews/{preview}` returns authenticated audio content.
Index zero selects the default preview.
Subsequent indices select verified-language previews in response order.
The response uses `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.

The gateway checks tenant ownership and current account authority before the native request.
The native URL must use an artifact origin declared in the voice transport.
The ElevenLabs transport declares `https://storage.googleapis.com`.
The gateway sends no provider credential to that origin.
Preview downloads use shared transfer admission, a 30-second timeout, and an 8 MiB limit.
Invalid native responses return a sanitized `502` response.

## Clients And Validation

The Go client uses `GetMediaVoices` with `MediaVoiceQuery` and returns `MediaVoicePage`.
The Python client uses `get_media_voices` with `ClientMediaVoiceQuery` and returns `ClientMediaVoicePage`.
Both clients preserve all page and voice observations.
The CLI `media voices` command accepts the same query fields as flags and prints the complete page.

Local tests use the real gateway, local HTTP and gRPC servers, and both official clients.
They verify native queries, pagination, private references, account authority, previews, and retained Dictator records.
These tests do not establish live ElevenLabs connectivity or MediaOps consumer acceptance.

## Native References

- [ElevenLabs voice search](https://elevenlabs.io/docs/api-reference/voices/search)
- [ElevenLabs voice and preview representation](https://elevenlabs.io/docs/api-reference/voices/get)
