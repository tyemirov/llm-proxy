# Current Gemini Candidates

F047 adds disabled catalog records for `gemini-3.8-flash` and `gemini-3.5-flash-lite`.
The public catalog excludes both models until their live qualification passes.
The Gemini provider default remains `gemini-3.5-flash`.

## Request contract

Both candidates use the existing Gemini Interactions adapter and Gemini model family.
They accept text, JPEG, PNG, WebP, audio, and the existing structured output contract.
Text requests use stored background interactions with retrieval and deletion.
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
Gemini 3.8 Flash's first omitted-effort request timed out after 45,004 milliseconds without response bytes.
Flash-Lite passed omitted effort and all four explicit efforts.
Its background completion request returned HTTP 400 at creation.
Neither candidate passed the full stored interaction lifecycle.
Gemini 3.8 Flash passed disposable key verification and the image smoke test.
Flash-Lite returned HTTP 422 `provider_key_rejected` during disposable key verification, before its image test.
Its successful metadata and direct text requests show that the key has access to the model.
Live audio remains unqualified for both candidates.

Independent acceptance runs on September 5 reproduced both blockers with the existing repository credential.
Gemini 3.8 Flash timed out after 45,005 milliseconds before its omitted-effort response.
Flash-Lite again passed all five reasoning cases, then returned HTTP 400 at background creation.
Both candidates remain disabled.

The latest Gemini 3.8 Flash run passed omitted, `low`, and `medium` effort with HTTP 200.
Its `high` effort request returned HTTP 429, which stopped the run before lifecycle checks.
This result replaces the timeout as the latest acceptance blocker.
Evidence: `/tmp/llm-proxy-f047-gemini38-goal-refresh.log`.

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

Activate a candidate only after reasoning, media, completion, active retrieval, cancellation, and deletion all pass.
Production deployment remains operator-owned.

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
