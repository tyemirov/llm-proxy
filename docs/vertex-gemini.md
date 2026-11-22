# Vertex Gemini

## Customer API-Key Contract

The user approved the F060 API-key revision on September 8, 2026.
The `vertex` provider accepts one customer authorization key in its secret `api_key` field.
The service stores the key in the encrypted tenant connection record.
The catalog binds local acceptance credentials to `VERTEX_API_KEY`.
The [customer assessment](gemini-customer-connections.md) records the qualification results and remaining activation gates.

The transport uses this global endpoint:

```text
POST https://aiplatform.googleapis.com/v1/publishers/google/models/{model}:generateContent
x-goog-api-key: CUSTOMER_VERTEX_API_KEY
```

The request uses the key to authorize the selected exact model.
The transport uses the catalog authentication contract and synchronous completion.
The current runtime rejects operator credential-profile fields and configuration declarations.
The earlier service-account results remain historical transport evidence in I252 and F060.

The projectless provider offers `gemini-3.5-flash`, `gemini-3.1-pro-preview`, `gemini-3.8-flash`, and `gemini-3.5-flash-lite` text routes.
The dictation route is `gemini-3.5-transcribe-preview`.
Each offering requires separate acceptance. The projectless offerings retain unavailable price records.

## Request contract

The transport maps messages to `contents` and ordered `parts`.
System messages become `systemInstruction`.
Assistant messages use the `model` role.
Media bytes use `inlineData` with their MIME type.
Reasoning uses `generationConfig.thinkingConfig.thinkingLevel`.
Structured output uses `generationConfig.responseFormat` with `text.mimeType: APPLICATION_JSON` and `text.schema`.

The service accepts one candidate with a `STOP` finish reason.
It excludes thought parts from the returned text.
Output usage includes candidate tokens and thought tokens.
`MAX_TOKENS` returns the public output-limit error.
Provider failures preserve the public error contract and keep provider response bodies private.
Client cancellation reaches the upstream request.

The local request limit is 20,000,000 encoded bytes.
The local response limit is 16 MiB.
The text offerings accept at most 3,000 images and one audio attachment per request.
Each inline image has a 7,000,000-byte limit.
F043 owns provider staging for larger assets.

Vertex dictation uses the exact model `gemini-3.5-transcribe-preview` and audio-only input parts.
Google documents this model as a preview.
The Developer API identifier `gemini-3.5-transcribe` returned HTTP 404 on Vertex.

## Gemini 4 project connections

Select **Vertex AI Project** (`vertex-project`) to use Gemini 4 Neon
(`gemini-4-neon`), Carbon (`gemini-4-carbon`), or Hydrogen (`gemini-4-hydrogen`).
Hydrogen is the connection verification model and the provider's text default.

Enter a Google Cloud API key authorized for Vertex and the global URL for
your billed Google Cloud project. Use this format with your project ID or number:

```text
https://aiplatform.googleapis.com/v1/projects/PROJECT_ID/locations/global
```

Connect the saved account connection to your tenant. The proxy sends the key
in `x-goog-api-key` and appends `/publishers/google/models/{model}:generateContent`
to the URL. Google bills the selected project. Each connection requires its own project
and key. Project-scoped and Express connections use distinct provider IDs and
tenant assignments. The server loads the project and key from the assigned
connection. Qualification environment variables are test inputs.

All three offerings support synchronous text generation, structured output,
and inline image and audio input. Each uses a combined input and output budget
of 1,048,576 tokens and the proxy's existing inline media limits.

| Model | Supported reasoning efforts | Google default | Synchronous output limit |
| --- | --- | --- | ---: |
| Gemini 4 Neon | `low`, `medium`, `high`, `xhigh` | `high` | 65,536 |
| Gemini 4 Carbon | `low`, `medium`, `high` | `medium` | 32,768 |
| Gemini 4 Hydrogen | `low`, `medium`, `high`, `xhigh` | `high` | 65,536 |

| Model | Input | Output including reasoning | Cached input |
| --- | ---: | ---: | ---: |
| Gemini 4 Neon | $3.00 | $15.00 | $0.15 |
| Gemini 4 Carbon through November 30, 2026 | $2.00 | $10.00 | $0.10 |
| Gemini 4 Carbon from December 1, 2026 | $4.00 | $20.00 | $0.20 |
| Gemini 4 Hydrogen | $1.50 | $7.50 | $0.075 |

Rates are USD per million tokens. Google's Carbon promotion applies as a
50% credit on eligible net spend. The catalog changes to list pricing at
midnight UTC on December 1, 2026. [Google's global Standard PayGo pricing](https://cloud.google.com/model-garden/pricing)

### November 19 proxy qualification

The projectless `v1 generateContent` endpoint returned HTTP 404 on all eight
cases for each Gemini 4 model. A direct Gemini 3.5 Flash control returned HTTP 200
with the same key and endpoint. Google's Express inventory omits both Gemini 4
models.

The project-scoped run used the existing `llm-proxy-499919` project and verified
the same key through the normal connection creation and tenant assignment APIs
on a disposable proxy. The test project and credential were supplied only for
this run.

| Exact model | Passed | Generation latency |
| --- | ---: | ---: |
| `vertex-project/gemini-4-neon` | 8/8 | 1.534–3.614 seconds |
| `vertex-project/gemini-4-hydrogen` | 8/8 | 0.702–2.150 seconds |

Each model passed omitted effort, all four explicit efforts, structured text,
PNG recognition, and WAV transcription. Every response included a request ID
and positive input and output usage that summed to the reported total.
The table describes one sequential qualification run.

### November 21 Carbon qualification

`vertex-project/gemini-4-carbon` passed seven cases through a disposable
proxy with the existing project connection. The cases covered omitted effort,
all three supported efforts, structured text, PNG recognition, and WAV
transcription. The requests took 2.478–5.419 seconds and returned request IDs
and measured token usage. Local HTTP tests verify the synchronous output cap
and reject `xhigh` before dispatch.

### Repeat Gemini 4 qualification

For repeat qualification, supply `VERTEX_PROJECT_API_KEY` and
`VERTEX_PROJECT_BASE_URL` in the test process environment and run:

```bash
LLM_PROXY_LIVE_VERTEX_GEMINI4=true \
LLM_PROXY_LIVE_VERTEX_AUDIO=/absolute/path/acceptance.wav \
GOFLAGS=-v make test-gemini-current
```

Use the audio phrase specified in [Repeat Acceptance](#repeat-acceptance).

See the [Neon specification](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-neon),
[Carbon specification](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-carbon),
[Hydrogen specification](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-hydrogen),
[Gemini 4 quickstart](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/start/get-started-with-gemini-4), and
[Express model inventory](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/start/express-mode/overview#available-models-and-rate-limits-in-express-mode).

## Get A Vertex API Key

The connection form opens [Google Cloud API Keys](https://console.cloud.google.com/agent-platform/studio/settings/api-keys).
Google labels this service Agent Platform in the console.
The link uses the customer account and project selection.

1. Open the API Keys page.
2. Select your Google Cloud project.
3. If the page shows **Enable APIs**, complete that setup first.
4. To get an existing key, select **Show key** for a key restricted to **Agent Platform API**.
5. To make a new key, select **Create API Key**.
6. Complete any setup that Google requires.
7. Enter the key in the **API key** field of the Vertex connection form.

The API service is `aiplatform.googleapis.com`.
Google can require billing setup or additional account permissions before key creation.
Use [Google's setup guide](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/start/api-keys) for these requirements.

## Tenant Connection

1. Obtain a Vertex authorization key that permits `aiplatform.googleapis.com` requests.
2. Open the selected tenant settings with its authorized management session.
3. Save the Vertex key, exact text model, and system prompt.
4. Select the accepted Vertex offering as the tenant text default.
5. Verify the required generation capabilities through the public proxy.
6. To disconnect, delete the Vertex connection through the management operation.
7. To revoke the provider key, delete it through the Google credential interface.

The connection operation is `PUT /api/management/tenants/{tenant_id}/provider-connections/vertex`.
Its request body has this shape:

```json
{
  "fields": {"api_key": "CUSTOMER_VERTEX_API_KEY"},
  "text_model": "gemini-3.8-flash",
  "system_prompt": ""
}
```

The service verifies the key with Vertex before it saves the connection.
Failed verification preserves the previous connection.
A successful replacement changes only the selected tenant connection.
The default operation is `PUT /api/management/tenants/{tenant_id}/defaults`.
Submit the required fields from the [OpenAPI contract](openapi.yaml).
The `gemini` and `vertex` provider identities have separate credentials, model selection, and usage records.

## Existing Operator-Profile Transition

An operator-profile reference cannot become a customer API key.
For an existing installation, complete one explicit transition:

1. Disconnect each Vertex connection through the previous runtime's management API.
2. Remove the operator profile declarations from its service configuration.
3. Update the runtime through the approved release procedure.
4. Reconnect each tenant with its own Vertex API key.
5. Complete production acceptance for each selected offering.

The current runtime rejects obsolete connection fields and configuration shapes.
This implementation does not change production configuration or deploy the service.
F043 separately owns the credential requirements for provider media staging.

## Repeat Acceptance

The live test uses a disposable proxy and tenant store with the customer API-key transport.
Supply `VERTEX_API_KEY` through the selected environment before the live command.
Use a WAV file with the spoken phrase `The quick brown fox jumps over the lazy dog`.

```bash
LLM_PROXY_LIVE_VERTEX=true \
LLM_PROXY_LIVE_VERTEX_AUDIO=/absolute/path/acceptance.wav \
GOFLAGS=-v make test-gemini-current
```

The ordinary CI run skips paid requests.
A failed required case blocks qualification. A later successful diagnostic does not replace that failure.
Local proxy acceptance, independent customer setup, and production acceptance remain separate results.

## September 8 API-Key Acceptance

Flash 3.8 passed 70/70 public generation requests at concurrency 10.
Cases covered four reasoning selections, structured output, an image, and an audio clip.
Observed p95 latency was 5.451 seconds. The maximum was 6.486 seconds.
The server and client request deadlines were 45 seconds.
All 15 connection and tenant checks passed with the live key.
All 28 additional offering checks passed, including both public dictation protocols.
The [API-key evidence](evidence/vertex-api-key-acceptance-2026-09-08-summary.json) records the counts, scope, and remaining gates.

The first diagnostic inherited the repository asset path and failed ten structured requests with `structured_request_store_error`.
The corrected diagnostic used its own temporary asset store. It passed the complete matrix.
Both trials remain in the evidence. The initial failure does not establish a provider failure.

These checks used the existing billed account and synthetic local management users.
Independent customer setup, live provider-key replacement, Google revocation, sustained request rate, and production acceptance remain unverified.
The earlier Pro and Flash-Lite reliability failures remain in P012.

## Sources

- [Vertex API keys](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/start/api-keys)
- [Vertex Express endpoint](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/start/express-mode/vertex-ai-express-mode-api-reference)
- [Gemini 3.8 Flash](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-8-flash)
- [Gemini 3.1 Pro](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-1-pro)
- [Gemini 3.5 Flash-Lite](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-5-flash-lite)
- [Gemini 3.5 Transcribe](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-5-transcribe)

Final `make ci` passed all 12 gates in 284 seconds with 100.0 percent Go statement coverage.
The Governor check, changed-prose review, issue-reference check, and `git diff --check` passed.
The tracker retains 79 language findings outside the changed scope.
