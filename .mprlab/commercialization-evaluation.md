# Six-Week LLM Proxy Evaluation

## Decision and authority

The evaluation starts on October 6, 2026. Its six-week review window ends on November 16, 2026.
The user owns the commercial decision. The primary agent owns synthetic evaluation execution.
The customer workflow owner remains unspecified.
P015 records the charter and baseline. P016 owns the comparison. P017 owns the final decision.

The candidate offer adds a second provider to one existing, compatible, non-streaming text workflow.
The offer includes tenant, provider, model, usage, and failure information with explicit limitations.
This offer is a hypothesis. Demand, willingness to pay, price, and support effort remain unknown.
Local tests cannot establish these commercial facts.

The supplied discovery research identifies two candidates. It does not identify ten qualified buyers.
Postiz has reported demand for alternative or configurable models. Its narrow candidate is asynchronous autopost with structured text.
A direct configuration patch or OpenRouter workaround could solve that need with less effort.
Parseur had a resolved provider-capacity incident in May. Plain-text extraction could fit, but quota increases could already solve the problem.
Neither candidate has verified purchasing intent or the same validated recurring problem.
These supplied notes are discovery evidence, rather than proof of buyer qualification. This task performed no prospect contact.
The commercial gate remains unmet.

The user plans no new platform services. The earlier proposal concerned MCP access for existing services.
This evaluation authorizes source inspection, planning records, and existing free local tests.
Product changes, new MCP interfaces, financial activation, prospect contact, purchases, and deployment require separate authorization.
Time and budget ceilings remain unapproved. This document approves no expenditure or engineering allocation.
The week labels define review order. They are not deadlines that require work beyond the authorized scope.

## Selected week-one workflow

The user selected synthetic, non-streaming structured extraction from plain text on October 6, 2026.
This choice establishes an evaluation workflow. It does not establish a customer commitment.
The input is `Invoice from ACME for USD 42.50.`
The required JSON contains `vendor: ACME`, `amount: 42.5`, and `currency: USD`.
All three fields are required. Additional properties are prohibited. The caller validates the output against the same schema.
The supported proxy path is `POST /v1/responses` with `text.format.type: json_schema` and `stream: false`.
The selected routes are `openai/gpt-4.1` and `anthropic/claude-sonnet-5`.
The workflow permits unknown token usage. It requires no financial journal, token history, or account administration in the direct caller.
The proxy service retains its existing tenant authorization requirements.

The user authorized LP-C01-S1 as a bounded compatibility experiment before approval of commercial thresholds.
S1 uses the same routes, five-second deadline, output limit, and local provider protocols as LP-C01.
It replaces the plain fixture answer with the specified extraction JSON and schema.
It runs one sample for each of six scenarios per route and executable alternative.
The scenarios are known usage, missing usage, measured zero, rate limit, provider failure, and invalid output.
S1 records actual built-in proxy retries and uses a direct caller without retries.
These differing retry policies prevent a comparative performance score. S1 measures no latency or timed setup advantage.
The established gateway comparison uses documented interfaces when executable access is unavailable.
S1 does not complete the full LP-C01 matrix or pass a commercial gate.
The [S1 benchmark record](commercialization-benchmark-LP-C01-S1.md) contains the results and next decision.

## Current contracts

| Contract | Evidence and limitation |
| --- | --- |
| Text interfaces | `docs/client-protocols.md` defines `/v1/chat/completions` and `/v1/responses`. Both reject unsupported fields before dispatch. |
| Native messages | `/v2` supports canonical messages and declared route controls. Select an exact provider/model pair. |
| Client contract | `/v1` rejects sampling controls and stateful Responses operations. Preserve the actual workflow requirements during assessment. |
| MCP | `docs/mcp.md` defines tenant discovery, text generation, route resources, and three media tools. No usage-inspection tool exists. |
| MCP ownership | OAuth identity selects owned tenants. Generation shares the text execution lifecycle and logical usage status. |
| Management visibility | Authenticated account and tenant APIs provide usage summaries, failures, and rejections. Detail queries have bounded pages and scoped cursors. |
| BYOK telemetry | `management_usage_writer.go` uses a bounded process-local queue. Queue saturation, failed inserts, and process termination can lose events. |
| Token evidence | B305 retains per-quantity measurement presence in new events and exposes scoped coverage. Historical null evidence remains unverified. Queue loss remains a separate limitation. |
| Route evidence | Unresolved routes have empty provider/model dimensions. Do not assign a provider to an unresolved request. |
| Hosted billing | `docs/hosted-billing.md` records separate durable financial evidence and controlled development acceptance. Activation and live qualification remain separate. |

Ordinary BYOK telemetry is operational information. It does not establish invoice amounts, complete request history, or customer funds.
Persisted rows survive database reopen. Queued rows do not establish crash durability.
F021 owns existing MCP acceptance. F065 through F070 own hosted development contracts.
P006 owns proposed service commitments. P012 owns Gemini customer access. Reuse these issues if the selected workflow requires them.

## Six-week sequence

| Week | Dates | Required evidence and decision |
| --- | --- | --- |
| 1 | Oct 6–12 | Record source contracts, local baseline, workflow owner, actual request fields, and two compatible routes. |
| 2 | Oct 13–19 | Prepare the local comparison protocol under P016. Approve decision thresholds before measurements. |
| 3 | Oct 20–26 | Compare direct integration, LLM Proxy, and LiteLLM with identical local fixtures. Record setup and second-provider effort. |
| 4 | Oct 27–Nov 2 | Compare usage coverage, failure diagnosis, isolation, and restart behavior. Identify one demonstrated gap if necessary. |
| 5 | Nov 3–9 | Assess the offer, support burden, and unresolved commercial facts from authorized evidence. Record any proposed pilot separately. |
| 6 | Nov 10–16 | Complete P017. Recommend stop, limited continuation, or a separately authorized pilot. |

## Local benchmark specification: LP-C01

LP-C01 measures local integration overhead and operational behavior. It does not measure model quality, live reliability, or invoice accuracy.
The full comparison harness remains proposed. LP-C01-S1 implements the authorized week-one subset.
These steps specify the full comparison inputs and outputs.

1. Record the host, OS, tool versions, source commit, source hashes, fixture hashes, and each configuration hash.
2. Use isolated temporary databases and loopback listeners. Use dummy keys and local authentication fixtures only.
3. Permit outbound traffic only to the selected loopback fixtures during execution.
4. Use the same workflow driver for all three alternatives. Keep required behavior in each alternative's effort count.
5. Compare the smallest direct provider/configuration patch, LLM Proxy, and the open-source LiteLLM gateway.
6. Select LiteLLM `v1.104.0` for the first comparison. Record its immutable artifact digest before execution.
7. Use exact routes `openai/gpt-4.1` and `anthropic/claude-sonnet-5` for the synthetic protocol test.
8. Serve OpenAI Responses and Anthropic Messages from separate local provider fixtures.
9. Require fixture output `fixture-answer`, input tokens `10`, and output tokens `3` for each successful request.
10. Use one user message, `Return fixture-answer.`, an output limit of `64`, and a non-streaming response.
11. Use a five-second deadline and a 25-millisecond fixture delay for successful requests.
12. Disable retries, automatic provider substitution, caches, and external callbacks. Record actual upstream attempt counts.
13. Create two owned tenants and one foreign account. Record equivalent ownership behavior in each alternative.
14. Start with one provider. Record setup steps, active minutes, source edits, and configuration edits until acceptance passes.
15. Add the second provider. Record incremental effort separately from initial setup.
16. Run 20 warm-up requests per route. Exclude these requests from latency calculations.
17. Run 100 successful requests per route at concurrency one, then concurrency four. Repeat the matrix three times.
18. Rotate alternative order between repetitions. Report p50 and p95 with the nearest-rank method, `ceil(p*n)`.
19. Run each failure scenario ten times per route and alternative. Keep these samples separate from successful latency samples.
20. Record the first failed assertion. Keep failures in the report rather than deleting their samples.

| Scenario | Fixture behavior | Observable acceptance |
| --- | --- | --- |
| Missing usage | Return successful text without usage fields. | Preserve unknown quantities. Distinguish missing quantities from missing events. |
| Rate limit | Return HTTP 429 and `Retry-After: 1`. | Expose the failure. Record one attempt and no provider substitution. |
| Provider failure | Return HTTP 503 with a private-body sentinel. | Expose the failure class. Keep the private body out of public output and logs. |
| Timeout | Wait six seconds against the five-second deadline. | Expose timeout and cancellation evidence. Keep the provider outcome unknown where necessary. |
| Invalid route or input | Select an unavailable model or unsupported field. | Reject before upstream dispatch. Keep unresolved dimensions explicit. |
| Ownership | Read foreign summaries, detail rows, and cursors. | Return no foreign data. Bind pagination to its account, tenant, interval, and scope. |
| Normal restart | Complete writes, stop gracefully, and reopen the database. | Retain committed rows and connection ownership. Record queued shutdown behavior separately. |
| Abrupt termination | Stop only the isolated test process with pending telemetry. | Compare fixture attempts with persisted rows. Report loss without a durability claim. |
| Queue saturation | Delay the isolated database writer and exceed its configured queue size. | Keep request results correct. Record dropped events and the available loss evidence. |

Use independent fixture request records as the denominator for event coverage.
Report accepted executions, rejected requests, provider attempts, persisted events, and absent token measurements separately.
Match event counts by tenant, provider, model, and scenario. Do not infer token coverage from aggregate zero values.
Record any missing request correlation as a limitation.
Use existing management APIs for LLM Proxy visibility. Charge each alternative for required diagnosis tooling and storage.
Start the direct baseline from the existing provider integration. Add only the smallest second-provider or configuration change.
Assess a supported direct configuration or OpenRouter workaround as a low-effort alternative when the actual workflow permits it.
Use the local fixtures for its transport measurements. Keep its external service terms and paid acceptance unverified.
Count any required structured-text or asynchronous caller behavior from the real workflow. Keep the synthetic text test separate.
Separate basic workflow effort from optional ownership tests. Count only the selected workflow ownership requirements in comparative effort.
Require isolation for each proposed tenant service. Do not add production services to make the comparison pass.

### Diagnosis exercise

Give the operator the same authorized tenant identity, request identifier, UTC interval, and failure symptom for each alternative.
Keep the fixture answer record separate from the operator inputs.
Start the timer when the operator receives these inputs and the permitted diagnostic surfaces.
Stop the timer when the operator records the correct tenant, route, failure class, dispatch state, and token measurement state.
Check the answer against the independent fixture record. Record incorrect answers and their correction time.
Run ten exercises per alternative. Use identical scenario assignments and rotate the alternative order between repetitions.
Record active minutes, elapsed minutes, incorrect answers, and unavailable diagnostic fields separately.

The named routes are synthetic candidates. P016 must confirm the real workflow and its exact routes before a pilot conclusion.
If required workflow fields fail, record incompatibility. Do not remove those fields to manufacture a successful comparison.

## Comparison rubric and acceptance

| Dimension | Measure | Decision rule |
| --- | --- | --- |
| Workflow correctness | Required output, controls, exact routes, and attempt counts | Require all deterministic contract scenarios to pass. |
| Ownership and privacy | Foreign reads, scoped cursors, safe failures, and private sentinels | Require zero data disclosures and unauthorized dispatches. |
| Integration effort | Initial setup and incremental second-provider minutes and edits | Proposed threshold: reduce incremental active minutes by at least 25% against both alternatives. |
| Diagnosis effort | Time to identify tenant, route, failure class, and missing measurements | Proposed threshold: reduce active diagnosis minutes by at least 25% against both alternatives. |
| Local overhead | Successful p50/p95 at both concurrency levels | Proposed threshold: add at most 10 milliseconds to direct p95 latency. |
| Operational evidence | Persisted-event coverage, token coverage, and observed losses | Disclose all observed gaps. Require the workflow owner to accept bounded telemetry. |
| Support burden | Setup instructions, recurring tasks, dependencies, and known restrictions | Record the owner and estimated recurring effort. Keep unsupported service promises outside the offer. |
| Commercial evidence | Buyer need, willingness to pay, price, and delivery cost | Keep unknown results explicit. Local fixtures cannot pass this dimension. |

The effort and latency thresholds are proposals. The user must accept or revise them before P016 measurement.
Use raw values for the decision. Mark unmeasured dimensions `unknown` rather than assigning a favorable score.
Record repeated-operator effects and existing product familiarity as effort-measurement limitations.

## Continue and stop gates

- Continue local comparison only after the owner, actual workflow, two routes, source state, and thresholds are recorded.
- If no compatible workflow exists, stop this offer and record the failed requirement.
- If the proxy has no material advantage over either alternative, stop new product investment for this offer.
- If isolation or privacy fails, stop pilot preparation until a separately authorized correction passes acceptance.
- If the workflow requires complete telemetry or invoice amounts, reject ordinary BYOK telemetry for that requirement.
- If the offer requires new platform services or broad wrappers, stop and request a new scope decision.
- Recommend a pilot only after technical advantage, accepted limitations, ownership, and commercial evidence are recorded.
- Obtain explicit approval for pilot scope, time, budget, and external operations before execution.
- At week six, record a stop or continuation decision. Do not extend the evaluation by default.

## Smallest conditional engineering candidate

B305 completed measurement presence in new BYOK events and coverage in existing summaries.
Public integration tests distinguish absent usage from measured zero before and after a database reopen.
New media usage rows record unknown evidence. Historical rows remain unverified.
Queue loss remains a separate limitation. This change does not establish durable metering or invoice accuracy.
The combined source passes acceptance CI and native release CI.
P018 blocks release preparation before artifact assembly. Publication and deployment did not start.
The [combined execution record](combined-release-2026-10-06.md) identifies the source, tests, and release blocker.

Complete the LP-C01 comparison before selection of another engineering candidate.
An MCP usage tool remains conditional on a demonstrated MCP workflow need after existing management access is evaluated.

## Sources and execution record

Repository sources: `README.md`, `docs/client-protocols.md`, `docs/mcp.md`, `docs/hosted-billing.md`, and `configs/providers.yml`.
Usage sources: `internal/proxy/management_api.go`, `management_store.go`, `management_usage.go`, and `management_usage_writer.go`.
The [baseline record](commercialization-baseline-2026-10-06.md) separates executed checks from source and document claims.

LiteLLM is the nominated existing gateway. Its performance remains unmeasured in this evaluation.
The official [configuration guide](https://docs.litellm.ai/docs/proxy/configs) defines provider configurations and endpoint overrides.
The official [release notes](https://docs.litellm.ai/release_notes/) list `v1.104.0` on October 3, 2026.
These sources were read on October 6, 2026. They do not prove local comparator acceptance.
