# Cerebras Qwen 3.8 27B

F097 adds a Cerebras provider using the existing OpenAI-compatible Chat Completions
codec, bearer authentication, and synchronous execution. Provider identity, endpoint,
model mapping, controls, limits, prices, and the schema policy live in
[`configs/providers.yml`](../configs/providers.yml). There is no Cerebras-specific
request profile or transport adapter.

## Offering

The canonical model is `qwen3.8-27b`; Cerebras receives `qwen-3.8-27b`.
The canonical model and Cerebras provider are enabled in the source catalog
after the retained native-body qualification. Publication and existing-tenant
activation remain separate operations.
An isolated qualification catalog enables the Cerebras provider and exact model,
assigns its text default, and disables the unqualified Alibaba offering for this model.
This is a different model from Alibaba's hosted `qwen3.8-flash`.

Requests use `https://api.cerebras.ai/v1/chat/completions`.
`max_tokens` maps to `max_completion_tokens`, including reasoning and visible output.
The portable catalog limits are 65,536 context tokens and 32,768 output tokens.
Cerebras documents 131,072 context and 40,960 output tokens for paid tiers and trial
customers; these account-dependent limits are outside the initial offering.

The accepted reasoning efforts are `none`, `low`, `medium`, and `high`, transmitted
unchanged. Cerebras maps `high` to Qwen-native `xhigh`; effort labels do not establish
identical compute across models. Provider omission retains its documented default.

The initial operation is text generation. The upstream model's image and tool
capabilities require separate qualification and are not advertised by this offering.

## Structured output

The generic Chat Completions codec serializes `response_format` with type
`json_schema`, `strict: true`, a schema name, and the admitted schema.
The catalog policy requires a root object and closed objects, permits optional
properties and local nonrecursive references, and permits the declared numeric
constraints and primitive enum/const values. The initial subset omits tuple schemas.
Unsupported string patterns, formats, and array length constraints fail before
provider work. Applications can project provider constraints while retaining their
full canonical output validator.

The policy bounds nesting at 10 levels, object properties at 500, enum values at
500, and counted schema text at 5,000 characters. It declares the documented
large-enum threshold and string bound separately.
The proxy validates returned JSON, retains reported usage on failure, and never
repairs structured output. Generic unstructured output continuation remains a
separate source of attempts and charges.

## Credentials and connection verification

The declared private input is `CEREBRAS_API_KEY`. Keep its value in the operator's
selected private environment or an encrypted provider connection.
Connection verification uses authenticated `GET /v1/models` through the shared
read-only JSON resource transport. It does not generate model output.
Before qualification, confirm the key's account, organization, and project when
the console exposes them. Record observed limits separately from the advertised
tier. The new benchmark account exposes a Default Project and higher context
limits than the portable offering; these observations do not establish its billing
plan or credit balance.

## Prices and usage

The November 16, 2026 list prices are USD 0.99 per million input tokens and USD 1.49
per million output tokens. These are estimates from token usage, separate from
provider invoices, discounts, credits, and proxy customer charges.
Completion tokens already include reasoning. Keep the optional reasoning detail
as a subset and charge completion tokens once. Missing usage remains unknown.
Count rejected generations, evaluators, repairs, and continuation attempts when
comparing total Story cost.

## Qualification

Local acceptance uses the real HTTP service, the released official Go client, and a
local provider protocol implementation. It covers capability discovery, unchanged
wire identifiers, strict schemas, validation before dispatch, returned schema
violations, idempotency, reasoning controls, and truthful usage.
A second arbitrary provider identity must work with the same codec and policy;
provider-specific production code is not part of this contract.

Run the focused Cerebras tests and `make test-protocol-acceptance`, then final
`make ci`. Local fixtures establish implementation behavior; live connectivity and
the matched Story benchmark require the selected private credential.
Keep publication and production activation separate from these results.

On November 16, 2026, the matched TellTale pilot and one separately declared
diagnostic follow-up both reached Story drafting and failed before producing a
Story. The follow-up returned HTTP 200 with `finish_reason: stop`, reasoning, and
missing or null answer content. Usage remained available for the failed operation.
The catalog offering stayed disabled after those failed runs. Read-only connection
verification and successful planning requests established connectivity; they did
not establish completion of the Story workflow.

Subsequent exact body-only diagnostics returned valid answers, establishing
intermittent behavior. Local wire tests found identical upstream requests through
the Native and Responses protocols. TellTale then enabled native enforcement of
its existing Story-body schema without changing prompts, reasoning, or budgets.
The revised Cerebras pipeline completed an accepted 818-word Story in 36.589
seconds, including planning and evaluation. This qualifies the observed workflow;
the internal reason for the earlier reasoning-only termination remains unknown.
Source catalog enablement, publication, and production routing remain separate.

## Sources

- [Model limits and prices](https://inference-docs.cerebras.ai/models/qwen-3.8-27b)
- [Reasoning controls](https://inference-docs.cerebras.ai/capabilities/reasoning)
- [Structured output subset](https://inference-docs.cerebras.ai/capabilities/structured-outputs)
- [Chat Completions API](https://inference-docs.cerebras.ai/api-reference/chat-completions)
- [API keys and projects](https://inference-docs.cerebras.ai/console/api-keys)
