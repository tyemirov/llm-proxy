# Meta media assessment

P009 assessed the hosted Meta media models on September 5, 2026.
This document records provider facts and the full scope approved on September 6, 2026.
F054 through F058 own implementation. Paid provider acceptance remains pending.

The approved scope includes file dictation, structured file transcription, realtime sessions, image generation, image edits, and image conversations.
P011 specifies the shared media operation design. F022 and I046 implement its required storage, execution, and capacity contracts.

## Scope

| Exact model | Provider capability | Approved repository work |
| --- | --- | --- |
| `muse-voice-transcribe-1.0` | File transcription | A native codec for the current dictation interface. |
| `muse-image-1.0` | Image generation and image edits | Provider execution through the F022 media operation interface. |
| `muse-voice-transcribe-1.0` | Realtime transcription | A tenant-owned public session interface. |
| Muse Glimmer | Local inference from model weights | P008 owns this scope. |

The [Meta model list](https://dev.meta.ai/docs/models) confirms these capability boundaries.
The list does not prove access for a particular API key.

## File transcription

Meta uses `POST https://api.meta.ai/v1/asr/transcribe` with bearer authentication.
The multipart body contains a JSON `request` part and an `audio` part.
The JSON part requires `model` and `audioEncoding: "WAV"`.
Its `mode` defaults to `PUSH_TO_TALK`.
The other modes are `ENDPOINTING` and `DIARIZATION`.

| Boundary | Published contract |
| --- | --- |
| Audio | RIFF/WAVE, mono, signed 16-bit integer PCM, 16 kHz or 24 kHz. |
| Maximum request | 32 MB, including the multipart body. |
| Maximum audio duration | Ten minutes. |
| Recognition controls | Optional `keywords` and `languageBias` arrays. |
| Buffered result | JSON with `transcript`, `audioDurationMs`, `sessionId`, and `turns`. |
| Other response forms | Plain text or server-sent events, selected through `Accept`. |

The [file reference](https://dev.meta.ai/docs/api-reference/voice/transcribe) defines the request boundary.
The [speech guide](https://dev.meta.ai/docs/speech-to-text) defines result fields and endpoint limits.
Its JSON `transcript` contains the complete text.
The `turns` array is empty in `PUSH_TO_TALK` mode.

The native `meta_transcription` codec uses `PUSH_TO_TALK` with a buffered JSON result.
The codec maps `transcript` to the current public `text` field.
The existing `/dictate` and `/v1/audio/transcriptions` endpoints can retain their current shapes.

Implementation requirements:

1. Validate the WAV content, duration, and complete encoded request at the input boundary.
2. Preserve the existing public upload limit when it is smaller than the provider limit.
3. Reject unsupported audio formats with the current public validation contract.
4. Keep the provider session identifier private.
5. Preserve explicit tenant defaults and current usage records.
6. Add public integration tests for both dictation endpoints before the codec change.

F055 owns public speaker labels, timestamps, and file transcription controls.
The native file input remains WAV. Additional audio format conversion requires its own verified contract.
The current text result cannot represent speaker turns or timestamps.

### Candidate implementation

F054 registers `muse-voice-transcribe-1.0` as a disabled candidate.
Its `created` value records the catalog entry date, September 6, 2026.
The adapter sends explicit `PUSH_TO_TALK` settings and preserves valid WAV bytes.
It returns only the complete transcript through the existing public output formats.
It validates chunk sizes, PCM fields, sample rates, and duration before dispatch.
The gateway bounds the complete multipart body at 32,000,000 bytes.
This gateway bound does not resolve the provider's exact meaning of 32 MB.
The smaller configured public upload limit still applies.
Provider failures preserve safe status metadata and exclude response bodies from public errors.

`make test-meta-transcription` tests both HTTP interfaces, tenant defaults, usage, cancellation, upload limits, and candidate discovery.
The official Go client reads the activated transport through the real public capability endpoint.
B198 requires defaults only for enabled provider operations and validates all retained candidate metadata.

Activation requires verified provider access, file retention evidence, the exact request bound, and paid acceptance at both sample rates.
After acceptance, enable the candidate and assign its Meta dictation default in the same catalog change.

## Image generation and edits

Meta accepts JSON at `/v1/images/generations`.
Image edits use `/v1/images/edits` with either multipart images or a JSON `images` array.
Each JSON image item selects `image_url` or `file_id`.
The [edit reference](https://dev.meta.ai/docs/api-reference/images/edit-image) includes a Data URI example.
The multipart schema specifies PNG input.

| Control or result | Published contract |
| --- | --- |
| `n` | One through ten output images. Default: one. |
| `size` | Requested aspect ratio expressed as width and height. Returned pixels can differ. |
| `output_format` | `webp`, `png`, or `jpeg`. Default: `webp`. |
| `response_format` | `b64_json` or a temporary signed `url`. Default: `b64_json`. |
| `reasoning_strength` | `high` or `low`. Default: `high`. |
| `tool_enablement` | Image search, web search, and shell controls. All three default to enabled. |
| `stream` | Completed image events. Partial image requests have no effect. |
| Result | `data` image entries, creation time, output format, background, and token usage. |

The [image schemas](https://dev.meta.ai/docs/api-reference/images/schemas) define the controls.
The [image guide](https://dev.meta.ai/docs/image-generation) defines completed events and tool behavior.
The schema includes partial event types, but the guide does not promise partial output.

Implementation requirements:

1. Complete the F022 decisions in the [media design](media-gateway-consolidation.md) before image execution.
2. Use one immutable media operation plan for each accepted request.
3. Persist dispatch state before a provider call.
4. Keep ambiguous dispatch results in the F022 `uncertain` state.
5. Copy completed image bytes into tenant-owned artifacts before public success.
6. Keep provider URLs, file identifiers, and response identifiers private.
7. Declare aspect ratio support without an exact output resolution promise.
8. Set tool controls explicitly from the approved operation contract.
9. Verify artifact format, dimensions, output count, and tenant access through public integration tests.

F022 owns durable state, worker claims, cancellation, artifact retention, and recovery.
I046 owns fair capacity allocation.
The Google-specific storage work in F043 is not a Meta prerequisite.
MediaOps retains workflow ownership.

Meta also supports image conversations through Responses.
That path carries `image_generation_call` items and signed identifiers between turns.
The proposed first image scope uses the dedicated image endpoints.
F058 implements conversational image state after F057.

## Realtime transcription

The [realtime reference](https://dev.meta.ai/docs/api-reference/voice/realtime) defines a WebSocket session at `wss://api.meta.ai/v1/asr/realtime`.
The first JSON frame carries authentication and fixed session settings within ten seconds.
Audio frames contain raw mono 16-bit PCM at 16 kHz or 24 kHz.
The handshake selects `PCM_16KHZ` or `PCM_24KHZ`.
The client sends `endStream`, then consumes final events before normal closure.

Sessions have a sixty-minute limit and no resume token.
Close codes distinguish completion (`1000`), input or pacing errors (`1008`), server failure (`1011`), and rate limits (`1013`).
The speech guide limits queued audio to five seconds and requires realtime input pacing.
These behaviors require a public session resource, event semantics, capacity control, and cancellation rules.
They do not fit the current buffered dictation result.

## Retention and errors

The [file guide](https://dev.meta.ai/docs/file-handling) states that uploaded files persist by default.
An explicit `expires_after` permits a lifetime from one hour through thirty days.
The Files API also provides explicit deletion.
Its generic upload limit is 1 GiB, with 100 GiB of storage per team.
These Files limits do not increase the separate ASR request limit.

The [Responses guide](https://dev.meta.ai/docs/protocols/responses) defaults `store` to `true`.
It describes response deletion as a soft delete that removes retrieval and history access.
It does not give a physical deletion deadline.
The guide describes `store: false` as stateless execution.
This application-state control does not establish a general content-log retention period.

The realtime handshake supports `zdrOverride: true` for metadata-only logs.
When omitted, the authenticated caller's policy applies.
The file transcription request has no `zdrOverride` field.
Its content retention period remains an evidence gap.

The [error guide](https://dev.meta.ai/docs/error-handling) defines HTTP status and typed errors for image requests.
It distinguishes invalid input, authentication, billing, access, missing resources, rate limits, and server failures.
Unknown endpoint paths can return an empty HTTP 404 body.
ASR instead returns an HTTP status and a client-safe message without `type`, `param`, or `code`.
The speech guide specifies HTTP 413 for excess request bytes and HTTP 400 for excess duration or invalid WAV content.
It also specifies HTTP 406 for an unsupported `Accept` value and HTTP 429 for exhausted capacity.

Provider retry advice does not establish duplicate-free media execution.
F022 dispatch evidence controls retries after uncertain image results.

## Price and acceptance

The [pricing page](https://dev.meta.ai/docs/pricing-rate-limits) lists USD 0.01 for each returned image.
Image generation has a limit of 150 requests per minute per team.
Image tools have no separate charge.
Image token counts are informational.

Transcription costs USD 0.18 per processed audio hour.
The speech guide rounds processed duration down to whole seconds.
File and realtime transcription share eight concurrent sessions and 1,000 session starts per hour.
These limits apply across API keys in one provider team.

The existing Meta key is available as `MUSE_API_KEY` in `configs/.env`.
All live Meta tests use this catalog-defined environment binding.
Paid discovery, model access, output quality, latency, and cleanup acceptance remain pending.

Qualification sequence:

1. Resolve the provider evidence gaps required by the selected implementation issue.
2. Implement the approved outcomes through F054, F055, F056, F057, and F058 in dependency order.
3. Verify exact model access with the selected tenant connection.
4. Implement and test the selected public contract with controlled provider responses.
5. Qualify real transcription at both sample rates with known speech and boundary inputs.
6. Qualify image generation, edits, output formats, artifact access, and required provider cleanup.
7. Enable each model only after its required acceptance passes.

## Open Decisions

- Complete the F022 and I046 implementation dependencies for image operations.
- Define image input byte and count limits from endpoint-specific evidence.
- Verify the exact byte interpretation of the ASR 32 MB limit.
- Obtain the content-log retention policy for image and file transcription requests.
- Verify temporary image URL lifetime for supported URL output.
- Verify response and signed image identifier lifetimes for F058 image conversations.
- Define public realtime events, session ownership, disconnect behavior, and usage before realtime implementation.

These decisions remain separate from completion of the P009 assessment.
