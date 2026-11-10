# Request execution and errors

The [OpenAPI contract](openapi.yaml) defines the HTTP interface.
This guide explains execution, reconciliation, budgets, telemetry, and provider errors.

## REST Contract

llm-proxy exposes a blocking REST contract for text generation. Canonical
`POST /v2` also accepts optional media evidence and structured requests. An
ordinary request returns its final formatted answer in the same HTTP response.
The structured request contract also provides durable reconciliation.

The authenticated `/model/v1` contract provides durable media operations.
`GET /model/v1/capabilities` returns routes that have a catalog offering and a
configured media operation adapter. `POST /model/v1/operations` requires one
tenant-bound `Idempotency-Key`. The caller reads status, requests explicit
cancellation, and downloads result assets through separate authenticated
resources. A disconnected creation or status request does not cancel accepted
work.

The service stores the accepted request, selected route, catalog revision, and
private credential-version reference before dispatch. The public operation
states are `queued`, `running`, `succeeded`, `failed`, `cancelled`, and
`uncertain`. A repeated key with the same intent returns the retained operation.
A changed intent returns `409` before provider dispatch. Terminal detail and
output bytes expire together. A tenant-lifetime tombstone keeps the key bound
after detail expiry.

The [official Go media example](../examples/media-operations/main.go) exercises
capability discovery, operation creation, waiting, metadata reads, and verified
byte download against a running service. The canonical request and response
schemas remain in [OpenAPI](openapi.yaml).

OpenAI `gpt-image-2` supports `image.generate` and `image.edit` through the existing
tenant connection. Select the explicit `images` or `responses` surface.
Supply the model, prompt, quality, size, background, output
format, and output count. JPEG and WebP also require explicit compression from
`0` through `100`. PNG must omit compression. Read exact route limits from
`GET /model/v1/capabilities`.

Editing accepts ordered tenant `image_asset_ids`. The Images surface also accepts
a PNG `mask_asset_id`. The Responses surface requires a catalog `responses_model`
and produces one image. Use `previous_operation_id` for a follow-up request.
The parent must be a successful Responses operation from the same tenant,
connection version, route, image model, and text model. Native identifiers remain
private. Both surfaces support `stream` and up to three `partial_images`.
Operation reads expose verified previews in `partial_outputs`.

Local admission rejection before image submission produces `failed` with `media_operation_unavailable`.
The service releases input assets after this definite failure.
The same idempotency key identifies the failed operation without another provider call.
A transport error after provider dispatch can produce `uncertain` because provider execution is not known.

The [typed Go image example](../examples/image-generation/main.go) reads all image
choices from `IMAGE_GENERATION_INPUT_JSON`. Set `LLM_PROXY_BASE_URL`,
`LLM_PROXY_TENANT_KEY`, `MEDIA_OPERATION_IDEMPOTENCY_KEY`, and an existing
`IMAGE_OUTPUT_DIRECTORY`. The example prints the accepted operation ID and
saves each verified output. See the [image client examples](../examples/image-generation/README.md)
for surface selection, previews, and follow-up edits. Reuse the same key and intent to recover the
operation. A lost Images response produces `uncertain`. Stored Responses jobs
use retrieval after a stream interruption or worker restart. Their background
cancellation requires provider confirmation. The gateway does not submit accepted
work again. Source tests use local providers. Client
publication, deployment, and live provider acceptance require separate evidence.

The media worker settings are explicit under `server`:

| Setting | Default | Meaning |
| --- | ---: | --- |
| `media_operation_workers` | `2` | In-process workers that claim accepted cloud operations. |
| `dictator_media_operation_workers` | `1` | Isolated workers that claim accepted Dictator operations. |
| `media_operation_capacity` | `32` | Maximum active accepted operations for the service. |
| `tenant_media_operation_capacity` | `4` | Maximum active accepted operations for one tenant. |
| `media_operation_lifetime_seconds` | `900` | Execution authority from acceptance. |
| `media_operation_claim_seconds` | `60` | Worker claim lifetime. |
| `media_operation_claim_renewal_seconds` | `20` | Claim renewal and maintenance interval. |

### Canonical OpenAPI ownership

[`docs/openapi.yaml`](openapi.yaml) is the sole hand-maintained HTTP wire
contract for every llm-proxy-owned proxy, browser-configuration, and management
operation. It defines exact paths, methods, authentication, parameters, bodies,
multipart parts, response headers, media types, schemas, and intentional status
codes. TAuth-owned operations are deliberately excluded.

Every server or bundled-client wire change must update that artifact in the
same change. CI compares the real router inventory bidirectionally with it and
validates representative real-handler, Go package, Python package, and Go CLI
exchanges against it. Prose and command examples in this guide are explanatory,
not a second contract.

The Pages release copies those committed bytes to
[`/openapi.yaml`](https://llm-proxy.mprlab.com/openapi.yaml) and publishes a
human-readable reference derived from them at
[`/docs/`](https://llm-proxy.mprlab.com/docs/). The contract names
`https://llm-proxy-api.mprlab.com` as its API server; the API origin does not
serve another schema location. The public OpenAPI navigation opens the schema
actions on `/docs/`: visitors can view the complete YAML in the generated
reference or download the same canonical bytes as `llm-proxy-openapi.yaml`.

### `web_search` boolean migration

Direct `GET /` callers must send the optional query parameter as exact
`web_search=true` or `web_search=false`. Omission means false. Former spellings
such as `1`, `0`, `t`, `f`, `y`, `n`, `yes`, and `no`, along with blank,
uppercase, or whitespace-padded values, now return HTTP 400 instead of being
coerced or silently treated as false.

JSON callers continue to send a native boolean body field on `POST /` or
`POST /v2`. The official Go package exposes `MessagesRequestInput.WebSearch`,
the Go CLI exposes the boolean `--web-search` flag, and the Python package
exposes `ClientMessagesRequest.web_search: bool`. All bundled clients use
`POST /v2`, remove any `web_search` query value inherited from a configured
base URL, and serialize the request body as `"web_search": true` or
`"web_search": false`.

An ordinary request does not stream tokens, poll a job endpoint, or use a
resume token.

OpenAI Responses and Gemini 3-series Interactions use pollable upstream
lifecycles. The proxy sends stored background requests and holds the client
connection. It polls a nonblank resource id through `status=queued` and
`status=in_progress`. The shared `pollable_resource` lifecycle makes the first
resource read immediately. The selected transport declares the retry statuses,
interval, and limit for a resource that is not visible yet. OpenAI permits one
retry after two seconds for `403` or `404`. Gemini permits six retries after
five-second intervals for `400`, `403`, or `404`. The lifecycle releases the
upstream worker during each wait. The request context bounds each wait.
For customer-owned credentials, provider resource ids remain in the active request lifecycle.
For hosted work, the journal keeps private provider resource identifiers for recovery.
The proxy does not expose these provider identifiers.

Every current text route uses one provider-neutral completion coordinator.
The coordinator starts a new request only when the selected transport declares
continuation actions. OpenAI Responses `status=incomplete` with
`reason=max_output_tokens`, Chat Completions `finish_reason=length`, and
Anthropic `stop_reason=max_tokens` use those actions. The coordinator retains
the original messages and appends the visible output and one missing-suffix
instruction. Gemini declares no continuation actions. A Gemini
`status=incomplete` result causes a provider error after interaction cleanup.
Safety filters, refusals, tool/intermediate states, malformed responses, and
missing or unknown signals remain `502` failures and never trigger this loop.

A `504 Gateway Timeout` means the proxy request budget expired before a final
answer. For hosted or structured text, reconcile the idempotency key before a new
submission.

### Structured JSON output and reconciliation

Canonical `POST /v2` accepts an optional `structured_output.schema` object.
The object contains the caller-owned JSON Schema. A structured request requires
exactly one valid `Idempotency-Key` header.
For customer-owned provider credentials, the proxy rejects either input when the other input is absent.
The hosted text contract requires the key for all requests, including requests without a schema.

F070 development acceptance passed. Production activation and actual processor qualification remain separate.
See [hosted text identity](hosted-billing.md#hosted-text-identity) for replay and result expiry.

The proxy compiles the schema and validates the selected provider subset before
provider dispatch. It maps the schema to OpenAI Responses `text.format`, Gemini
Interactions `response_format`, or Anthropic Messages `output_config.format`.
The proxy validates the final JSON against the same schema before durable
success. Other provider protocols reject structured requests before provider
work.

A structured request does not use output continuation or a repair inference.
Invalid provider JSON causes one terminal provider failure. This rule prevents
a malformed response from causing a second paid inference.

The idempotency key binds one request intent to one authenticated tenant. The
intent contains the resolved provider, model, and canonical request body. A
different intent with the same key returns `409 Conflict`. Request-timeout and
transport settings are not part of the request intent.

The durable record has these states:

- `not_dispatched`: The proxy saved the request intent before provider work.
- `dispatched`: The proxy started provider work.
- `succeeded`: The proxy saved the validated JSON result.
- `failed`: The proxy saved a known provider failure.
- `uncertain`: The proxy cannot prove the result after provider dispatch.

Use authenticated `GET /v2/requests` with the same `Idempotency-Key` header.
A successful record returns the exact saved JSON with status `200`. An active
record returns safe timestamps and elapsed time with status `202`. A failed
record returns its safe failure with the recorded status. An uncertain record
returns `409 structured_request_outcome_unknown`. Every reconciliation response
sets `Cache-Control: no-store` because its content depends on the tenant and key.

An identical `POST /v2` submission reuses a succeeded or active record.
For customer-owned credentials, a failed record permits an explicit new attempt with the same request intent.
Hosted failed records retain their execution identity and do not start another provider call.
An uncertain record never starts provider work again. A process restart changes
each interrupted `dispatched` record to `uncertain`.

The proxy stores private records under
`server.asset_store_path/structured-requests`. A record contains tenant, key, and intent hashes. It
also contains routing, timing, state, safe failure, and successful result data.
It does not contain prompts, schemas, credentials, or raw provider bodies.

Terminal records use `server.asset_retention_seconds`. Uncertain records remain
available because automatic removal would erase the no-resubmit decision. A
structured record does not contain a provider resource id and cannot restart
uncertain provider work.

See [structured persistence concurrency](structured-persistence-concurrency.md) for identity locks, durability boundaries, and measured limits.

### Request work budgets

Every authenticated operation that can run long work accepts one optional
header:

```text
X-LLM-Proxy-Request-Timeout-Seconds: N
```

`N` is the positive whole-number wall-clock budget, in seconds, for that
request. The budget begins before body parsing and covers validation, asset
upload, queue admission, every provider call, OpenAI background polling, and
response construction for `GET /`, `POST /`, `POST /v2`,
`POST /model/v1/assets`, and `POST /dictate`.

If the header is omitted, `server.request_timeout_seconds` is the effective
budget. A supplied value must be in the inclusive range
`1..server.max_request_timeout_seconds`; the proxy never rounds, clamps, or
replaces it. A blank, repeated, signed, fractional, nonnumeric, zero, negative,
or over-limit value returns this exact `400 application/json` response before
queue admission:

```json
{"error":{"code":"invalid_request_timeout","max_request_timeout_seconds":3600}}
```

Every accepted response, including errors, echoes the effective value in
`X-LLM-Proxy-Request-Timeout-Seconds`. If that budget expires, the proxy cancels
the remaining queued, provider, or polling work and returns:

```json
{"error":{"code":"request_timeout","request_timeout_seconds":900}}
```

with `504 application/json`. The value shown is the effective budget for that
request.

This server work budget is not a client transport timeout. A caller may still
cancel sooner with a Go context, a process signal, or an explicitly configured
HTTP transport policy, and no response is guaranteed after caller cancellation.
The bundled clients impose no separate total-response deadline by default.

Accepted upstream requests emit safe terminal evidence with the effective
budget and one of `validation_failure`, `success`, `proxy_timeout`,
`proxy_overload`, `provider_failure`, or `caller_cancelled`. Queue-capacity
rejection is proxy overload, not a provider failure.

Every routed proxy request also emits one `proxy request phase summary`
structured event keyed by the same `request_id` returned in
`X-LLM-Proxy-Request-ID`. It carries the query-free `endpoint`, canonical
`provider` and `model`, `request_timeout_seconds`, terminal `status` and
`outcome`, `total_latency_ms`, and these explicit phase totals:

- `authentication_ms`: static or managed tenant authentication.
- `upstream_admission_ms`: bounded queue admission and worker acquisition.
- `upstream_rate_limit_wait_ms`: time waiting for configured origin windows.
- `provider_http_ms`: observed provider HTTP work through response-body close.
- `provider_poll_wait_ms`: proxy-owned sleeps between provider resource polls.
- `continuation_wait_ms`: proxy-owned sleeps between missing-suffix attempts.
- `response_formatting_ms`: construction and writing of the selected proxy response.
- `managed_usage_enqueue_ms`: synchronous construction and non-blocking enqueue
  of a managed usage event; detached persistence is outside the request.

An unused phase is present with zero. The totals use one request-scoped
monotonic accumulator. They describe proxy-observed boundaries, not provider
execution time or billing, and unclassified orchestration time must not be
derived by subtracting phase totals from total latency.

OpenAI create and poll observations and every provider-neutral continuation
attempt emit `proxy provider progress` under the same request ID. These events
contain canonical provider and model values, `progress_kind`, a count,
`completion_signal`, and `elapsed_ms`. OpenAI progress also contains
`provider_state`, `current_output_bytes`, and `accumulated_output_bytes`.
Gemini create, poll, cancel, and delete operations use the same event. Gemini
progress contains `provider_state` and validated `provider_error_codes` for a
terminal fault. It does not contain a provider message or interaction ID. The
event contains no prompt, message, generated text, provider body, credential,
cookie, or tenant secret. Phase fields stay in structured logs. They do not
extend responses, OpenAPI, bundled clients, or managed usage persistence.

For managed tenants, the proxy writes and flushes the selected response before
it attempts a non-blocking usage enqueue. Each management runtime has one FIFO
channel bounded by `management.usage_queue_size` and starts one writer goroutine
when the first event is accepted. The writer attempts each accepted insert once
under a detached five-second budget, in acceptance order, without per-event
goroutines or retries. A dedicated database-write gate sequences usage inserts
with management mutations without acquiring the management mutation mutex.
Authentication bypasses both, so database latency neither retains the request
handler nor serializes authentication for another request, and it cannot change
an already selected response.

When the channel is full, the newest event is dropped, previously accepted work
stays queued, and the proxy emits one `managed_usage_queue_full` warning with
safe request metadata. Accepted events are process-local, at-most-once work
until their database insert commits. Queue contents are not crash-durable, and
an insert failure or process termination can lose an uncommitted event. Managed
usage is operational telemetry, not a billing, accounting, or provider-job
ledger; summaries may briefly lag completed proxy responses.

Each summary aggregate includes per-quantity `token_coverage` for measured, partial, unknown, and historical execution events.
Numeric quantities remain retained subtotals. Coverage does not establish invoice completeness.
The dashboard distinguishes measured zero from unknown usage and labels incomplete subtotals.
See the [operational token evidence contract](token-measurement-evidence.md) for persistence, completion, and recovery rules.

`server.upstream_capacity` bounds upstream HTTP requests by origin, tenant, and provider account.
An active request retains its permit until the caller closes the response body.
The scheduler releases active capacity between remote-job polls.
Network admission does not store provider jobs or supply durable recovery.

## Canonical endpoint reference

Use the derived [API reference](https://llm-proxy.mprlab.com/docs/) for the
complete operation inventory and the exact query, header, JSON, multipart,
authentication, media-type, response-header, and status contracts. Use the
[committed OpenAPI artifact](openapi.yaml) for tooling and review.

The request examples in [Usage](client-guide.md) demonstrate common calls without
duplicating that inventory. In particular, dictation has one canonical incoming
multipart file part, `audio`; the obsolete `file` alias is rejected.

### Status codes

* `200 OK` - success
* `400 Bad Request` - missing/invalid parameters, invalid request timeout, invalid multipart audio form, unknown provider/model, or unsupported provider capability. Invalid timeout headers return `{"error":{"code":"invalid_request_timeout","max_request_timeout_seconds":M}}`.
* `403 Forbidden` - missing or invalid `key`
* `413 Payload Too Large` - compatibility JSON exceeds `max_prompt_bytes`, a
  `/v2` JSON envelope exceeds its service or catalog-derived ingress limit,
  dictation audio exceeds `max_input_audio_bytes`, an asset upload exceeds
  `max_asset_bytes`, or media exceeds the selected provider offering's
  transport limit. Provider media failures use
  `provider_media_limit_exceeded`.
* `429 Too Many Requests` - upstream provider rate limit; returns the sanitized `provider_rate_limited` JSON contract
* `409 Conflict` - selected provider credential is not configured. The proxy
  returns `provider_not_configured` before provider dispatch.
* `503 Service Unavailable` - the shared upstream operation queue is full
* `504 Gateway Timeout` - the accepted proxy work budget expired; the response is `{"error":{"code":"request_timeout","request_timeout_seconds":N}}`
* `502 Bad Gateway` - upstream provider API or response-protocol failure; returns the sanitized `provider_error` JSON contract

Provider failures use one stable response shape:

```json
{
  "error": {
    "code": "provider_error",
    "provider": "gemini",
    "upstream_status": 503,
    "retryable": true,
    "request_id": "PROXY_GENERATED_REQUEST_ID",
    "retry_after": "120"
  }
}
```

All six fields are present. `upstream_status` is the exact provider HTTP status,
not the proxy status. It is `null` when no usable unsuccessful upstream HTTP
response exists. The proxy preserves upstream `429` as public `429` and maps
other provider failures to public `502`. `retryable` is true for upstream HTTP
`408`, `425`, `429`, `500`, `502`, `503`, and `504`. It is also true for a
failed Gemini Interaction when every URI code has a retryable final path
segment. An unknown final segment sets `retryable` to false. The value
classifies the provider condition. It does not make an LLM request idempotent
or remove duplicate-work and billing risk. `retry_after` is `null` unless the
provider supplied a valid delta-seconds or HTTP-date value. The proxy also
returns that normalized value in the standard `Retry-After` header.

`request_id` is generated by the proxy, returned in
`X-LLM-Proxy-Request-ID`, and recorded in structured proxy logs. Provider
failure responses and logs never include the provider's raw response body or
error message. For a failed Gemini Interaction, structured logs can include
validated `provider_error_codes`. Public responses do not include these codes.

Local structured output failures add fixed codes to the existing `provider_error_codes` log field.
The codes are `structured_output.not_json`, `structured_output.multiple_json_values`, and `structured_output.schema_violation`.
Schema failures can also include up to eight sorted `json_schema.<keyword>` codes.
The diagnostic processes at most 64 validation nodes with a maximum depth of 16.
Only standard schema keywords enter this metadata.

Locations, property names, schema definitions, provider output, prompts, media, and values remain excluded.
These codes do not change public errors, strict validation, retained failure state, or explicit retries.

## Security

* Native text and dictation use a tenant secret in `key`. OpenAI client and media routes use a tenant bearer key.
* Management uses the TAuth session. MCP uses an account OAuth token and an explicit tenant identifier.
* Client requests must not include upstream provider API keys; public proxy endpoints reject provider-key-like query, JSON, and multipart form fields.
* Request logs record only the query-free path plus method, status, latency, client IP, proxy request ID, and tenant metadata; they do not record query strings, request bodies, cookies, or authorization headers.
* Provider credentials are accepted through TAuth-protected connection operations. Management responses show masked values.
* Public static pages load Google Analytics and LoopAware page-view scripts. The canonical `/privacy/` policy discloses those integrations without making unsupported collection, retention, consent, or opt-out claims. Do not put tenant secrets or other sensitive values in public-page URLs.
* Do not expose this service to the public internet without appropriate network controls.
