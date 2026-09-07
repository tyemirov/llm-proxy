# Current Gemini Candidates

F047 adds Developer API offerings for `gemini-3.8-flash` and `gemini-3.5-flash-lite`.
These offerings remain disabled.
F060 enables both models through the separate [Vertex provider](vertex-gemini.md).
The Gemini provider default remains `gemini-3.5-flash`.

## Request contract

Both candidates use the existing Gemini Interactions adapter and Gemini model family.
They accept text, JPEG, PNG, WebP, audio, and the existing structured output contract.
Gemini 3.8 Flash text requests use stored background interactions with retrieval and deletion.
Flash-Lite text requests use synchronous completion with `background: false` and `store: false`.
Google rejects background interactions for Flash-Lite with HTTP 400.
Media requests use synchronous interactions without storage.
Assistant history remains invalid for these routes.

| Model | Release date | Efforts | Omitted effort |
|---|---|---|---|
| `gemini-3.8-flash` | September 2, 2026 | `low`, `medium`, `high` | Provider default `medium` |
| `gemini-3.5-flash-lite` | July 21, 2026 | `minimal`, `low`, `medium`, `high` | Provider default `minimal` |

The adapter sends explicit effort in `generation_config.thinking_level`.
It rejects unsupported values before provider dispatch.

| Limit | Both candidates |
|---|---:|
| Input context | 1,048,576 tokens |
| Output | 65,536 tokens |
| Inline image or audio request | 20,000,000 encoded bytes |
| Images per request | 3,600 |
| File upload | 2,000,000,000 bytes per file |

Google does not specify an audio-file count in the reviewed audio guide.
The catalog records that count as unknown.
Both media-specific guides state a 20 MB total inline request limit.
The general file guide lists 100 MB but states that limits can vary by file type and model.
The catalog uses the media-specific bound for the supported image and audio inputs.
The existing Files API path handles media above that bound.

## Standard prices

These prices describe the paid standard tier, verified on September 5, 2026.
Input, output, and cache-read prices use USD per million tokens.
Cache storage uses USD per million token-hours.

| Model | Input | Output | Cache read | Cache storage |
|---|---:|---:|---:|---:|
| Gemini 3.8 Flash | 0.75 | 3.75 | 0.075 | 0.50 |
| Gemini 3.5 Flash-Lite | 0.30 | 2.50 | 0.03 | 1.00 |

The Gemini 3.8 Flash prices apply through December 31, 2026.
Google lists January 1, 2027 prices of 1.50, 7.50, 0.15, and 1.00 for these respective components.
The catalog stores the current snapshot and does not apply future prices automatically.
Refresh the price records before the published change date.

## Qualification

The existing repository key returned HTTP 200 metadata for both exact model IDs and token limits.
Both candidates remain disabled until all applicable qualification checks pass.

### September 7 Vertex qualification

I252 passed all 22 direct Vertex requests on the existing billed LLM Proxy project.
The matrix covered Pro, Flash, and Flash-Lite reasoning, structured output, image input, and audio input.
The [Vertex qualification report](vertex-gemini-qualification.md) contains the exact results and required proxy changes.
F060 owns the proxy transport migration and its separate acceptance checks.
These direct results do not activate the existing Developer API routes.

### September 6 diagnosis

I251 adds bounded provider error details to the standard candidate command.
The command excludes credentials, request input, generated output, and unrelated response fields from diagnostics.
It reports quota details and retry metadata when the provider returns those fields.

Gemini 3.8 Flash passed omitted, `low`, and `medium` effort.
Its `high` request returned HTTP 429 with `generate_content_free_tier_requests, limit: 5`.
The provider requested a retry after approximately 41 seconds.
This error identifies a request quota, not an unsupported reasoning value.
The stored lifecycle and live audio still require qualification.
Evidence: `/tmp/llm-proxy-f047-gemini38-diagnosis.log`.
A separate lifecycle probe timed out after 45,003 milliseconds at its omitted-effort request.
It produced no further lifecycle evidence.
Evidence: `/tmp/llm-proxy-f047-flash-lifecycle-diagnosis.log`.

Flash-Lite returned HTTP 400 with this exact provider message:
`Model 'gemini-3.5-flash-lite' does not support background interactions.`
B197 changes its text transport to synchronous completion.
The corrected candidate command passed omitted, `minimal`, `low`, `medium`, and `high` effort with HTTP 200.
An earlier synchronous request returned HTTP 500 because the model had high demand.
That transient response differs from the reproducible background request defect.
Evidence: `/tmp/llm-proxy-f047-lite-background-diagnosis.log` and `/tmp/llm-proxy-b197-lite-live.log`.
The disposable proxy then passed Flash-Lite key verification and omitted-effort text with HTTP 200.
Its next reasoning request timed out after 45,003 milliseconds without response bytes.
This proxy matrix remains incomplete despite the successful direct matrix.
Evidence: `/tmp/llm-proxy-b197-lite-proxy-live.log`.
Flash-Lite then passed disposable key verification and the image smoke test with HTTP 200.
That run used `.mprlab/deploy/.env` through the standard credential loader.
The first media attempt stopped at a dotenv parse error in `configs/.env`.
Live audio remains unqualified for both candidates.
Evidence: `/tmp/llm-proxy-b197-lite-media-declared-env.log`.

I234 remains blocked by the credential project's Gemini 3.1 Pro quota.
Its omitted-effort request reports zero free-tier input-token quota and zero free-tier request quota.
The exact API model remains `gemini-3.1-pro-preview`.
The quota message names the `gemini-3.1-pro` quota group.
The repository contains one distinct Gemini credential across its declared environment inputs.
The operator must provide a project with nonzero quota before Pro acceptance can continue.
Evidence: `/tmp/llm-proxy-i234-diagnosis.log`.

Run each lifecycle check separately:

```sh
LLM_PROXY_LIVE_GEMINI_MODEL=gemini-3.8-flash \
  make test-live-gemini-candidate LIVE_ENV_FILE=configs/.env
LLM_PROXY_LIVE_GEMINI_MODEL=gemini-3.5-flash-lite \
  make test-live-gemini-candidate LIVE_ENV_FILE=configs/.env
```

The existing candidate harness accepts one exact model through `LLM_PROXY_LIVE_GEMINI_MODEL`.
Without that selection, it retains the Gemini 3.6 and 3.7 matrix required by I233.
An unknown candidate fails before a provider request.

For each disabled model, run the disposable proxy qualification:

```sh
LLM_PROXY_LIVE_REASONING_MATRIX=true make test-live-provider-candidate \
  LIVE_CANDIDATE_MODEL=gemini/MODEL_ID LIVE_ENV_FILE=configs/.env
make test-live-provider-candidate-media \
  LIVE_CANDIDATE_MODEL=gemini/MODEL_ID LIVE_ENV_FILE=configs/.env
```

Activate a candidate only after reasoning, media, and its declared completion lifecycle pass.
For Gemini 3.8 Flash, also verify active retrieval, cancellation, and deletion.
For Flash-Lite, verify synchronous key checks and text completion without stored interactions.
Production deployment remains operator-owned.

## Local validation

B197 and I251 passed the focused HTTP and CLI checks.
Final CI passed all 12 gates with 100.0% Go statement coverage in 245 seconds.
The tests cover synchronous key verification, reasoning, output-limit errors, media, public discovery, and the required Gemini default.
The diagnostic tests verify provider error details and private data exclusion.
These source checks do not replace the remaining F047 live qualification.
Evidence: `/tmp/llm-proxy-b197-ci-verified.log`.

## Sources

- [Gemini release dates](https://ai.google.dev/gemini-api/docs/changelog)
- [Gemini 3.8 Flash](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash)
- [Gemini 3.5 Flash-Lite](https://ai.google.dev/gemini-api/docs/models/gemini-3.5-flash-lite)
- [Thinking levels](https://ai.google.dev/gemini-api/docs/thinking)
- [Standard prices](https://ai.google.dev/gemini-api/docs/pricing)
- [Inline input limits](https://ai.google.dev/gemini-api/docs/file-input-methods)
- [Image count](https://ai.google.dev/gemini-api/docs/image-understanding)
- [Files API limits](https://ai.google.dev/gemini-api/docs/files)
- [Audio input](https://ai.google.dev/gemini-api/docs/audio)

## Vertex qualification

F060 adds qualified Vertex routes for Gemini 3.8 Flash, 3.5 Flash-Lite, and 3.1 Pro Preview.
The Developer API offerings for Flash 3.8 and Flash-Lite remain disabled.
The [Vertex contract](vertex-gemini.md) records service-account acceptance and the explicit tenant cutover.
