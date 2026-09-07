# Current Claude Models

F046 adds `claude-fable-5-1` and `claude-opus-5` through the existing Anthropic connection.
Both models passed live qualification on 2026-09-05 and are enabled in the repository catalog.
`claude-sonnet-4-6` remains the provider default. Existing tenant selections remain unchanged.
Repository activation does not establish production deployment.
The [Opus 4.1 retirement migration](claude-retirement.md) replaces retired direct API selections with Opus 5.

## Request contract

Both offerings use the Messages API with the same canonical and upstream model ID.
They support text, ordered JPEG, PNG, and WebP attachments, and the existing structured output contract.
The public `reasoning_effort` field accepts `low`, `medium`, `high`, `xhigh`, and `max`.
The adapter sends the selected value in `output_config.effort` and preserves `output_config.format` when requested.
An omitted effort preserves the provider's default `high` effort and adaptive thinking behavior.
The adapter sends no explicit thinking control.

The response parser returns text blocks and excludes thinking blocks from public answers.
Output continuation retains the selected effort and appends visible assistant output followed by a user continuation instruction.

| Limit | Both models |
|---|---:|
| Input context | 1,000,000 tokens |
| Synchronous output | 128,000 tokens |
| Encoded request | 32,000,000 bytes |
| Encoded inline image | 10,000,000 bytes |
| Images per request | 600 |

The byte limits use decimal megabytes.
The current proxy attachment contract accepts JPEG, PNG, and WebP.
Provider GIF, PDF, batch, and tool capabilities require separate public capability work.

## Standard prices

Prices are USD per one million tokens, verified on 2026-09-05.

| Model | Input | Output | Cache read | 5-minute cache write | 1-hour cache write |
|---|---:|---:|---:|---:|---:|
| Fable 5.1 | 10 | 50 | 0.25 | 12.50 | 20 |
| Opus 5 | 5 | 25 | 0.50 | 6.25 | 10 |

The cache-write rates retain their durations in the catalog price conditions.
These prices describe the standard Claude API service tier.

## Provider retention

Fable 5.1 requires 30-day provider data retention unless Anthropic expressly authorizes a zero-retention arrangement.
The provider rejects a workspace without the required retention configuration.
The proxy uses the tenant's existing Anthropic key and workspace configuration.

## Qualification

The authenticated Models API returned both exact IDs, image support, structured output, limits, and all five effort levels.
Each disabled candidate passed key verification, default text, all five effort requests, and an image request through a disposable proxy.
The image test verifies that the model identifies the fixture color as `RED`.
The text test requires the exact answer `OK`.

Each model required two key verifications, six text requests, and one image request.
Evidence files are `/tmp/llm-proxy-f046-fable-live.log`, `/tmp/llm-proxy-f046-fable-image-live.log`,
`/tmp/llm-proxy-f046-opus-live.log`, and `/tmp/llm-proxy-f046-opus-image-live.log`.

For a disabled candidate, run:

```sh
LLM_PROXY_LIVE_REASONING_MATRIX=true make test-live-provider-candidate \
  LIVE_CANDIDATE_MODEL=anthropic/MODEL_ID LIVE_ENV_FILE=configs/.env
make test-live-provider-candidate-media \
  LIVE_CANDIDATE_MODEL=anthropic/MODEL_ID LIVE_ENV_FILE=configs/.env
```

For an enabled model, run the standard harness with its exact model override:

```sh
LLM_PROXY_LIVE_PROVIDERS=anthropic LLM_PROXY_LIVE_ANTHROPIC_MODEL=claude-opus-5 \
  LLM_PROXY_LIVE_REASONING_MATRIX=true make test-live-providers LIVE_ENV_FILE=configs/.env
LLM_PROXY_LIVE_PROVIDERS=anthropic LLM_PROXY_LIVE_ANTHROPIC_MODEL=claude-opus-5 \
  make test-live-provider-media LIVE_ENV_FILE=configs/.env
```

## Sources

- [Fable 5.1 specifications](https://platform.claude.com/docs/en/models/fable-5-1/overview)
- [Fable 5.1 migration and retention](https://platform.claude.com/docs/en/models/fable-5-1/migration-guide)
- [Opus 5 specifications](https://platform.claude.com/docs/en/models/opus-5/overview)
- [Opus 5 effort and thinking](https://platform.claude.com/docs/en/models/opus-5/whats-new-opus-5)
- [Vision limits](https://platform.claude.com/docs/en/build-with-claude/vision)
- [Standard prices](https://platform.claude.com/docs/en/about-claude/pricing)
- [Models API](https://platform.claude.com/docs/en/api/models/list)
