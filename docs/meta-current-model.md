# Muse Spark 1.3

F049 adds the enabled `muse-spark-1.3` Meta offering in the existing Muse Spark family.
The provider default remains `muse-spark-1.1`.
Meta released Muse Spark 1.3 on September 2, 2026.

## Request contract

The Standard-tier route uses synchronous `POST /v1/chat/completions`.
Public `max_tokens` maps to upstream `max_completion_tokens`.
The adapter sends explicit effort as the top-level `reasoning_effort` field.
Supported efforts are `minimal`, `low`, `medium`, `high`, `xhigh`, and `max`.
Omission preserves the provider-selected reasoning level.
The proxy rejects `none` before provider dispatch.
Reasoning tokens consume the output budget and count toward billed completion tokens.

This addition exposes text input and text output.
Media, structured output, caller tools, and Meta Responses require separate adapter contracts.
The current route rejects structured output before provider dispatch.
The context window is 1,048,576 tokens.
Input and requested output must fit within that shared window.
The reviewed model guide does not specify a separate maximum output size.
The catalog leaves that output limit unspecified.

## Standard prices

Prices are USD per million tokens, verified on September 5, 2026.

| Component | Price |
|---|---:|
| Input | 1.25 |
| Output | 4.25 |
| Cache read | 0.15 |

The Standard tier excludes prompts and completions from model training.
These prices have no long-context premium.
The separate Contributor tier permits training with prompts and completions.
F049 registers only the Standard-tier identifier.

## Source details

The authenticated model and reasoning guides confirm `max` for Standard-tier Muse Spark 1.3.
The September 2 release announcement described `max` as pending.
The Chat Completions parameter table still lists efforts only through `xhigh`.
The current model-specific guides define the candidate contract.
Live qualification verified all six efforts before activation.

The reasoning guide uses `max_tokens` in its output-budget description.
The protocol guide identifies that field as deprecated and specifies `max_completion_tokens`.
The candidate retains the current protocol field used by the existing Meta route.

The provider documents image, video, PDF, and limited audio input for Muse Spark 1.3.
Audio quality can be degraded on this model.
These provider capabilities do not expand the proxy's text-only contract.

## Qualification

Local HTTP tests verify all three public text endpoints, each effort, omission, continuation, visible output, and usage totals.
They also verify rejection of unsupported structured output.
Catalog tests verify the context window, prices, and public discovery.
These tests use a local provider protocol implementation.
Initial implementation CI passed all 12 gates with 100.0% Go statement coverage.

On September 7, 2026, the existing Meta key passed verification through the disposable proxy.
Text requests passed with omitted effort and all six explicit efforts.
Each check returned HTTP 200.
The user-selected entry in `configs/.env` now uses the canonical name `MUSE_API_KEY`.
All live Meta tests read this catalog-defined environment binding, including tests for later Muse Spark versions.
Its key value is unchanged.
These results permit model activation.
Use the enabled-model live command for later checks:

```sh
LLM_PROXY_LIVE_REASONING_MATRIX=true LLM_PROXY_LIVE_META_MODEL=muse-spark-1.3 \
  make test-live-providers LLM_PROXY_LIVE_PROVIDERS=meta LIVE_ENV_FILE=configs/.env
```

The command verifies the key and every catalog effort in a disposable proxy.
Production deployment belongs to the operator.

## Sources

- [Release announcement](https://research.meta.ai/blog/introducing-muse-spark-1-3)
- [Current models and tiers](https://dev.meta.ai/docs/models)
- [Reasoning contract](https://dev.meta.ai/docs/reasoning)
- [Chat Completions protocol](https://dev.meta.ai/docs/protocols/chat-completions)
- [Standard pricing](https://dev.meta.ai/docs/pricing-rate-limits)
