# Muse Spark 1.3 Candidate

F049 adds `muse-spark-1.3` as a disabled Meta offering in the existing Muse Spark family.
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
Live qualification must verify all six efforts before activation.

The reasoning guide uses `max_tokens` in its output-budget description.
The protocol guide identifies that field as deprecated and specifies `max_completion_tokens`.
The candidate retains the current protocol field used by the existing Meta route.

The provider documents image, video, PDF, and limited audio input for Muse Spark 1.3.
Audio quality can be degraded on this model.
These provider capabilities do not expand the proxy's text-only contract.

## Qualification

Local HTTP tests verify all three public text endpoints, each effort, omission, continuation, visible output, and usage totals.
They also verify rejection of unsupported structured output.
Catalog tests verify the context window, prices, and candidate exclusion from public discovery.
These tests use a local provider protocol implementation.
Final CI passes all 12 gates with 100.0% Go statement coverage.

`MODEL_API_KEY` is absent from the process and all six private repository environment files.
The disposable command stops with `required catalog environment is not set: MODEL_API_KEY`.
No live Muse Spark 1.3 request ran.
Live provider qualification requires that key.
After the operator configures the key, run:

```sh
LLM_PROXY_LIVE_REASONING_MATRIX=true make test-live-provider-candidate \
  LIVE_CANDIDATE_MODEL=meta/muse-spark-1.3 LIVE_ENV_FILE=configs/.env
```

The command verifies the key and every catalog effort in a disposable proxy.
Keep the model disabled until the command passes.
Production deployment belongs to the operator.

## Sources

- [Release announcement](https://research.meta.ai/blog/introducing-muse-spark-1-3)
- [Current models and tiers](https://dev.meta.ai/docs/models)
- [Reasoning contract](https://dev.meta.ai/docs/reasoning)
- [Chat Completions protocol](https://dev.meta.ai/docs/protocols/chat-completions)
- [Standard pricing](https://dev.meta.ai/docs/pricing-rate-limits)
