# Week-One Benchmark: LP-C01-S1

## Result and decision

The current proxy and a small direct-provider caller both support the synthetic structured-extraction workflow with local provider doubles.
The absent-versus-zero measurement issue is reproducible in retained BYOK telemetry. It does not prevent extraction or immediate unknown-usage handling.
The next decision is to retain the direct patch as the default baseline before further proxy engineering.
Require a concrete shared-routing or tenant-operation need before selecting a product change.
This benchmark does not establish commercial advantage or buyer demand.

## Scope and workflow

The user selected this synthetic workflow on October 6, 2026. The primary agent owns its evaluation execution.
No customer workflow, buyer commitment, approved expenditure, or external account was created.
The [charter](commercialization-evaluation.md) defines S1 as a bounded week-one subset of LP-C01.

The task extracts three fields from `Invoice from ACME for USD 42.50.`
The required output is:

```json
{"vendor":"ACME","amount":42.5,"currency":"USD"}
```

The schema requires a string vendor, numeric amount, and currency enum `USD`.
All fields are required. Additional properties are prohibited.
The caller applies the same JSON Schema validation to each alternative.
The doubles return fixed text. They do not run inference or prove extraction accuracy on real documents.

| Input or control | Selected contract |
| --- | --- |
| Proxy endpoint | `POST /v1/responses` with tenant bearer authentication |
| Routes | `openai/gpt-4.1` and `anthropic/claude-sonnet-5` |
| Required fields | `model`, text `input`, `text.format` with strict JSON Schema, `max_output_tokens: 64`, `stream: false` |
| Direct OpenAI route | Native Responses body with the same schema and model `gpt-4.1` |
| Direct Anthropic route | Native Messages body with model `claude-sonnet-5`, `output_config.format`, and `max_tokens: 64` |
| Deadline | Five seconds for both caller alternatives and proxy work |
| Token states | Known `10/3`, absent usage, and reported `0/0` |
| Data and network | Dummy fixture keys, temporary databases, and loopback destinations only |
| Sample scope | One sample per scenario, route, and executable alternative: 24 observations |

The six scenarios are known usage, missing usage, measured zero, HTTP 429, HTTP 503, and schema-invalid output.
S1 retains the proxy's existing retry behavior. The direct caller makes one attempt.
This difference is an observed behavior, rather than a matched performance experiment.
The captured proxy OpenAI body sets `background: true`, `store: true`, and `temperature: 0.7`.
The direct OpenAI body omits those fields. The doubles return completed output immediately.
S1 does not test live background polling, provider retention, or the effect of those defaults on extraction quality.
The full LP-C01 sample, latency, isolation, restart, and commercial gates remain incomplete.
This workflow requires no direct-caller account administration or retrospective token history.

## Executed evidence

The standalone harness uses Go `testing`, `httptest`, the existing proxy router, and existing repository test fixtures.
It imports the current source through a local module replacement. It changes no product source or repository test.
Its transport rejects every non-loopback destination. Module downloads are disabled.
Temporary managed accounts are fixture data, rather than external accounts.

The direct OpenAI call uses the fixture root `/`. The direct Anthropic call uses `/v1/messages`.
The provider doubles accept any method and path. They do not validate authentication headers.
S1 establishes payload and response compatibility. Native endpoint and authentication qualification remain open.

| Scenario | Direct caller, both routes | Proxy OpenAI route | Proxy Anthropic route |
| --- | --- | --- | --- |
| Known usage | HTTP 200, valid extraction, one attempt | Same, with normalized usage | Same, with normalized usage |
| Missing usage | HTTP 200, valid extraction, usage absent | HTTP 200, valid extraction, `usage: null` | HTTP 200, valid extraction, `usage: null` |
| Measured zero | HTTP 200, valid extraction, usage object with zero counts | Same | Same |
| Rate limit | HTTP 429, one attempt | Five attempts, caller deadline error, retained logical status 504 | HTTP 429, one attempt, sanitized rate-limit code |
| Provider failure | HTTP 503, one attempt | Five attempts, caller deadline error, retained logical status 504 | HTTP 502, one attempt, sanitized provider-error code |
| Invalid output | HTTP 200 from double, caller schema rejection | HTTP 502, one attempt, proxy schema rejection | HTTP 502, one attempt, proxy schema rejection |

All 12 successful extraction observations passed the required output assertion.
All four invalid-output observations were rejected before the caller accepted an extraction.
The test checked the first captured schema for each successful or invalid-output scenario.
Error statuses, retry counts, and persistence deltas were recorded observations. They were not expected-value assertions.
The proxy kept the private provider-body sentinel out of its returned errors.
Its management failure rows retained route identity and logical error outcomes.
All 12 proxy executions produced one persisted event each in this isolated run.
This event count does not establish complete telemetry under load or process failure.

The OpenAI attempt counts are specific to this run. The retry strategy includes variable backoff.
`internal/proxy/openai.go` retries HTTP 429 and server failures within the request context.
The caller observed a transport deadline error, rather than an HTTP 504 response.
The retained usage record reported logical status 504. Keep these two observations separate.
The Anthropic rate-limit response retained `Retry-After: 1` in this run.
No retry timing, throughput, or live-provider reliability conclusion follows from these fixtures.

## Minimal integration and code surface

| Alternative | Initial caller integration | Second-provider change |
| --- | --- | --- |
| Direct provider | Set the backend endpoint and credential. Use native Responses with the shared schema and caller validation. | Add native Messages payload, authentication, and response-text mappings. |
| Existing LLM Proxy | Configure a provider connection, tenant assignment, and tenant key. Point the caller at `/v1/responses`. | Configure the second connection and assignment. Change the qualified model selector. Keep the caller schema and decoder. |
| LiteLLM | Documented provider configuration, proxy authentication, and structured-output request | Add a model configuration and change its selector. Exact route execution remains unverified. |

The proxy's single Responses caller contract worked across the two native provider protocols.
The direct patch required three provider-specific mappings. It did not require infrastructure work or a new platform service.
The existing gateway must already be available for the proxy setup comparison.
Gateway deployment, customer setup effort, and ongoing operations remain unmeasured.

The formatted harness contains these nonblank source lines:

| Function | Lines | Scope |
| --- | --- | --- |
| `responsesPayload` | 6 | Shared Responses request body |
| `directPayload` | 9 | Two-provider native request selection |
| `extractText` | 26 | Responses and Anthropic text decoding |

These counts describe this harness. They exclude shared HTTP execution, schema validation, fixture setup, and existing SDK or server code.
They are not product patch estimates or measured engineering savings.
Active setup minutes, diagnosis minutes, and recurring support effort were not measured.
The small direct surface prevents a claim that a proxy must be cheaper for this workflow.

## Measurement-presence reproduction

For each route, missing usage and reported zero both produced valid extraction with HTTP 200.
Their immediate response differed: missing usage was null or absent, while measured zero had a usage object.
Each corresponding proxy summary delta contained one success and zero input, output, and total tokens.
The isolated database contained identical zero token quantities for the missing and measured-zero rows.
Its usage table had no measurement-presence column.
The [results](evidence/commercialization-LP-C01-S1/results.json) retain the wire values, summary deltas, database columns, and safe rows.

This reproduces an information loss after persistence. Aggregate zero values cannot establish whether the provider measured token usage.
The issue does not block this workflow because extraction acceptance requires schema-valid output and permits unknown usage.
It does block a claim that retained summaries can distinguish complete token measurement from unknown quantities.
A caller can preserve the immediate null state in its own request record if historical coverage becomes a requirement.

That caller record was not implemented here. Queue loss remains a separate limitation.
Thus, the benchmark does not justify a measurement-presence product feature for extraction alone.

## Established gateway comparison

LiteLLM executable access was unavailable in the selected shell and Python environment.
No package, account, or gateway service was installed for this comparison.
The selected `v1.104.0` artifact was not obtained or executed. Its exact implementation remains unqualified here.

The official [structured-output guide](https://docs.litellm.ai/docs/completion/json_mode) documents JSON Schema requests through `/v1/chat/completions`.
It lists OpenAI and Anthropic API models and provides model-support checks.
It also documents optional response validation through `enable_json_schema_validation`.
The official [configuration guide](https://docs.litellm.ai/docs/proxy/configs) defines `model_list`, provider endpoint overrides, and proxy authentication.
These interfaces offer a documented counterpart to the proxy's shared caller contract.
They do not establish acceptance for either exact route in this experiment.
Missing-usage semantics, retry attempts, retained coverage, and operational effort remain unmeasured for LiteLLM.
No superiority claim over LiteLLM is supported.

## Reproduction and source preservation

The [harness](evidence/commercialization-LP-C01-S1/harness/extraction_test.go) and its Makefile are retained with the report.
The module uses the existing checkout at `/Users/tyemirov/Development/llm-proxy`.
Run this command from its retained harness directory:

```bash
make benchmark
```

The Makefile selects the single benchmark test with the local Go cache and module network access disabled.
The test needs temporary loopback listeners. The sandbox-approved run supplied that access.
The [test summary](evidence/commercialization-LP-C01-S1/benchmark-summary.txt) records the final pass and scenario observations.
The [manifest](evidence/commercialization-LP-C01-S1/manifest.json) contains source, fixture, module, and result hashes.
The before and after hashes of 669 repository source files matched.
The source commit remained `91ba581ba5763ce99c38a31f42a1148a06e8966f`. Concurrent working changes were preserved.

The first harness compile failed because `proxy.LogLevelError` does not exist. The fixture then used the literal `error`.
The first execution stopped at a caller deadline because the harness treated every transport error as fatal.
The corrected harness retains that failure as an observation and verifies the extraction scenarios independently.
The final run passed in 10.484 seconds of Go package test time.
That duration is test execution evidence. It is not a service latency measurement.
Complete repository CI was not rerun. Governor differences remained outside this task.

## Remaining gates

P015 retains customer workflow ownership and approved commercial criteria as open decisions.
P016 records this bounded compatibility evidence. The full comparison remains open.
P017 retains the six-week decision and pilot authority.
Time and budget ceilings remain unapproved. Demand, willingness to pay, live quality, uptime, and cost savings remain unknown.
