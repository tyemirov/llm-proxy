# Budget model qualification, November 16, 2026

This record distinguishes catalog declarations, local protocol qualification,
source activation, and exact-tenant live acceptance. Live operations belong to
LLM Proxy and the existing TellTale tenant
`managed-13f5fc612c3fac74f219d0bd483286be`. The TellTale client receives its tenant
secret and proxy origin. Credentials remain in the declared private input channel.

## Catalog decisions

| Provider | Exact model | Source state | Current evidence |
| --- | --- | --- | --- |
| OpenAI | `gpt-6-luna` | Enabled for authorized qualification | Official specifications and local protocol fixture; live qualification pending |
| Anthropic | `claude-haiku-5-5` | Enabled for authorized qualification | Official specifications and local protocol fixture; native numeric bounds rejected |
| Gemini Developer API | `gemini-3.5-flash-lite` | Enabled for authorized qualification | Existing synchronous transport and price declarations; exact-tenant Proxy qualification pending |
| Cerebras | `qwen3.8-27b` | Model and provider enabled | Retained TellTale native-body run completed an accepted 818-word Story in 36.589 seconds |
| Alibaba Cloud | `qwen3.8-27b` | Offering explicitly disabled | Cerebras qualification does not establish Alibaba qualification |

The Cerebras upstream ID is `qwen-3.8-27b`. Its text default is declared explicitly.
Source activation does not establish publication or production routing. Cerebras
live evidence is retained in TellTale's ignored benchmark archive under
`configs/credentials/benchmarks/2026-11-16-qwen-native-body`.
Read the [Cerebras contract](cerebras.md) for prior failures and the subsequent
successful native-body qualification.

## Controls and limits

Luna uses the existing Responses reasoning/tools profile. It supports `none`,
`low`, `medium`, `high`, `xhigh`, and `max`; omission retains the documented
`medium` default. Its context is 1,050,000 tokens and output limit is 128,000.
It reuses the current OpenAI structured output policy.
See the [Luna specifications](https://developers.openai.com/api/docs/models/gpt-6-luna).

Haiku uses the existing Messages codec and Anthropic structured output policy.
It supports `low`, `medium`, `high`, `xhigh`, and `max`; omission retains adaptive
thinking and the documented `medium` default. Its context is 1,000,000 tokens and
output limit is 128,000. The proxy sends no explicit thinking control or sampling
override. See the [Haiku specifications](https://platform.claude.com/docs/en/models/haiku-5-5/overview)
and [effort controls](https://platform.claude.com/docs/en/build-with-claude/effort).

Anthropic's native schema subset excludes numeric `minimum` and `maximum`.
The proxy preserves this restriction and rejects those requests before dispatch.
TellTale now represents bounded integer scores as an integer enum containing
1 through 10. Its benchmark preflight also honors `array_minimum: zero_or_one`
by omitting array minima from the provider projection. The canonical validator
retains the same score values and exact candidate counts. Credential-free
preflight and local CLI workflows passed, including accepted, repaired, pending,
array-correction, and exhausted-correction cases. This consumer preparation does
not establish live Haiku access. See the [structured output constraints](https://platform.claude.com/docs/en/build-with-claude/structured-outputs).

Gemini Flash-Lite retains synchronous Interactions with `background: false` and
`store: false`. Its declared efforts are `minimal`, `low`, `medium`, and `high`;
context is 1,048,576 tokens and output is 65,536. Developer API qualification is
separate from the enabled Vertex offering. See the [candidate contract](gemini-current-models.md)
and [current model specifications](https://aistudio.google.com/docs/models/gemini-3.5-flash-lite).

Cerebras retains its portable 65,536-token context and 32,768-token output limit,
with `none`, `low`, `medium`, and `high` reasoning. Account-specific larger limits
remain outside the portable declaration. The existing generic Chat Completions
codec sends `max_completion_tokens` and the strict JSON schema unchanged.
See the [model specifications](https://inference-docs.cerebras.ai/models/qwen-3.8-27b.md).

## Standard list prices

All token rates below use USD per million tokens. Verification date is November
16, 2026 for the new Luna and Haiku records. Their rate snapshots begin on that
date; this records the import date and makes no claim about historical rates.
The exact catalog bytes determine the revision. Runtime catalog revisions must
be recorded with live benchmark evidence.

| Model and prompt size | Input | Output | Cache read | Cache write |
| --- | ---: | ---: | ---: | ---: |
| Luna, at most 272,000 input tokens | 0.10 | 0.50 | 0.01 | 0.125 |
| Luna, above 272,000 input tokens | 0.20 | 0.75 | 0.02 | 0.25 |
| Haiku, at most 100,000 input tokens | 0.10 | 0.50 | 0.01 | 0.125 for 5 minutes; 0.20 for 1 hour |
| Haiku, above 100,000 input tokens | 0.50 | 2.50 | 0.05 | 0.625 for 5 minutes; 1.00 for 1 hour |
| Gemini Developer Flash-Lite | 0.30 | 2.50 | 0.03 | Storage: 1.00 per million token-hours |
| Cerebras Qwen | 0.99 | 1.49 | Unspecified | Unspecified |

Luna long-context multipliers apply to the full request: input and cache prices
double, and output prices increase by 50%. Haiku's higher prompt tier applies to
both input and output. Half-open catalog ranges split at 272,001 and 100,001,
respectively. Rates use the standard service tier and billing conditions; regional
premiums, Batch, Flex, and Fast processing require their own selected conditions.
See [Luna prices](https://developers.openai.com/api/docs/models/gpt-6-luna),
[Haiku prices](https://platform.claude.com/docs/en/models/haiku-5-5/overview),
[Gemini prices](https://aistudio.google.com/docs/pricing#gemini-3.5-flash-lite),
and [Cerebras prices](https://inference-docs.cerebras.ai/models/qwen-3.8-27b.md).

Provider list-cost estimates remain separate from invoices, discounts, credits,
and Proxy customer charges. Output usage includes reasoning when the provider
reports it within completion tokens; count it once. Preserve missing usage as
unknown and count every paid attempt, evaluator, repair, and failure in the
matched Story comparison.

## Local acceptance and remaining live work

`internal/proxy/budget_model_qualification_xa1d_test.go` checks the public catalog
service and real HTTP entry points with local provider protocol fixtures. It
covers authorized route activation, Alibaba exclusion, tier boundaries
at 272,000/272,001 and 100,000/100,001, cache classes, exact wire model IDs,
omitted and explicit reasoning efforts, and Haiku schema rejection before dispatch.
The existing Cerebras official-client fixtures cover strict schema preservation,
reported usage, arbitrary provider identity, and connection verification.

These local checks establish repository behavior. They do not establish access
through the TellTale tenant's provider connections. Complete exact-tenant discovery,
schema admission, bounded paid acceptance, and the matched Story benchmark through
LLM Proxy before scoring the enabled qualification routes. Their source
activation follows the explicitly authorized existing-tenant qualification
sequence; activation does not establish live acceptance. These catalog flags
make routes available to other already-authorized tenants too. Existing tenant
assignments and saved defaults remain unchanged. Record the selected model,
reasoning controls, token budget, source and runtime catalog revisions, all attempts,
measured usage, and qualification result. The user explicitly selected Proxy
prerequisites, Proxy full CI and deployment, then live configuration and the
benchmark, followed by final TellTale CI. Each gate follows its completed
prerequisites; unchanged passing results are reused.

The focused XA1D catalog and candidate protocol checks passed. A broader run
initially found two stale inventory assertions: `TestModelActivationExcludesDisabledRoutesFromHTTP`
reported `private catalog must retain the disabled model while runtime discovery excludes it`,
and `TestPublicCapabilityCatalogProjectsValidatedRuntimeRegistry` rejected the
updated catalog counts. The activation test now compares the exact one-model
change against its own parsed baseline. The public projection test derives counts
from the validated runtime registry. Both retain their original contract checks.
The Haiku fixture also verifies that integer enums and fixed identity constants
reach the real Messages codec unchanged and produce admitted structured output.
