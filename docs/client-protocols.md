# Client protocols

LLM Proxy accepts native messages and OpenAI client protocols. Each client protocol adapter uses the same completion coordinator.
Provider credentials remain on the server. The client executes each caller tool.

`docs/openapi.yaml` defines all HTTP fields and responses. This document explains the supported subset.

## Connect an OpenAI client

1. Set the base URL to the API origin followed by `/v1`.
2. Supply the tenant client key as the SDK API key.
3. Select an exact `provider/model` identifier from `GET /v1/models`.

```javascript
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://llm-proxy-api.mprlab.com/v1",
  apiKey: process.env.LLM_PROXY_CLIENT_KEY,
});
const result = await client.chat.completions.create({
  model: "openai/gpt-5.6",
  messages: [{ role: "user", content: "Explain this function." }],
});
console.log(result.choices[0].message.content);
```

Each `/v1` endpoint requires `Authorization: Bearer <tenant-client-key>`.
The server rejects query parameters and credentials in request bodies.
The bearer key selects a tenant. It never becomes a provider credential.

## Supported fields

The server rejects unsupported fields before provider dispatch.
Each selected offering must support the requested controls.

| Endpoint | Supported inputs |
| --- | --- |
| `POST /v1/chat/completions` | `model`, text `messages`, `max_tokens` or `max_completion_tokens`, `reasoning_effort`, function `tools`, `tool_choice`, `parallel_tool_calls`, `response_format`, `stream`, `stream_options.include_usage`, `store: false`. |
| `POST /v1/responses` | `model`, text or item-array `input`, `instructions`, `max_output_tokens`, `reasoning.effort`, function `tools`, `tool_choice`, `parallel_tool_calls`, `text.format`, `stream`, `store: false`. |
| `GET /v1/models` | Tenant bearer authentication. No query parameters or request body. |
| `POST /v1/audio/transcriptions` | Multipart `model`, one `file`, and optional `response_format: json`. |

Chat messages support `system`, `developer`, `user`, `assistant`, and `tool` roles.
Text content can be a string or an array of text parts.
Assistant history can contain function calls and `refusal: null`.
A tool result requires the exact identifier of an earlier assistant call.

Responses input supports text messages, `function_call`, and `function_call_output` items.
The client supplies complete history on each turn.
`store: true`, `previous_response_id`, resource retrieval, and encrypted reasoning state are unsupported.
An empty `include` array selects no extra output fields.
Null `prompt_cache_key`, `reasoning.summary`, and `text.verbosity` values select no control.

Both text endpoints support named function selection, `auto`, `none`, and `required`.
An offering must declare `caller_tools` in the public capability catalog.
The committed catalog enables caller tools on the declared OpenAI function-capable offerings.
Other offerings reject caller tools before dispatch.
The provider protocol adapter supports explicit Chat Completions tool capabilities in a selected catalog.

JSON Schema output requires `response_format.type: json_schema` or `text.format.type: json_schema`.
The selected provider must support the canonical structured request schema.
Caller tools and structured output cannot be combined in one request.
Sampling controls, provider web tools, images, and audio message parts are unsupported on `/v1`.
Use `/v2` for canonical media inputs and supported provider web search.

## OpenCode examples

The examples use standard provider packages. They require no custom provider code.

1. Set `LLM_PROXY_CLIENT_KEY` in the process environment.
2. Copy one example to your project as `opencode.json`.
3. Set `baseURL` to the selected proxy runtime.
4. Start OpenCode in that project.

| Example | Package | Model | Acceptance |
| --- | --- | --- | --- |
| [Chat Completions](../examples/opencode/chat-completions.json) | `@ai-sdk/openai-compatible` | `openai/gpt-5.6` | Text and a local file read. |
| [Responses](../examples/opencode/responses.json) | `@ai-sdk/openai` | `openai/gpt-4.1` | Text and a local file read. |

OpenCode `1.18.28` adds encrypted reasoning controls to its GPT-5 Responses requests.
Those controls are outside this API subset. Use the Chat Completions example for that OpenCode model.
The Responses example disables provider cache-key injection with `setCacheKey: false`.
Its token limits constrain the example session. They do not declare catalog limits.

## Results and errors

Non-streaming calls return standard Chat Completions or Responses objects.
All response and output-item resource identifiers belong to the proxy.
Function call identifiers, names, and JSON argument text remain exact.
The server never executes a caller function.

`stream: true` returns a buffered server-sent event sequence after the coordinator completes.
This behavior is not provider token streaming.
Chat emits a role chunk, content or function chunks, a finish chunk, optional usage, and `[DONE]`.
Responses emits creation, progress, item, content or function-argument, item-completion, and response-completion events.
Each Responses event has a sequential `sequence_number`.
Caller cancellation and the request timeout stop active provider work.

Usage uses the canonical token counts. Unavailable usage is null.
One accepted request produces one managed usage event, regardless of its number of output items or events.
Errors use an OpenAI error object with a proxy request identifier.
Provider errors, credentials, prompts, function arguments, and generated content remain outside logs.

Model discovery lists only usable tenant routes in identifier order.
`owned_by` comes from the model publisher in the catalog.
`created` records catalog entry creation from repository history. It does not claim a model release date.

## Native clients

The Go package, Python package, and CLI continue to use `/v2`.
Native messages retain media attachments, tenant defaults, provider web search, and durable structured request reconciliation.
The new native fields are `tools`, `tool_choice`, `parallel_tool_calls`, assistant `tool_calls`, and `tool_call_id`.
A native tool result returns JSON with `type: tool_calls`, `text`, `tool_calls`, and `usage`.
A normal text result retains its current format.
The Go and Python clients expose these request fields. Their text methods return the response body.

## Go client protocol selection

`ConfigInput.Protocol` selects the message completion protocol.
`ProtocolNative` is the default and uses the native `/v2` text contract.
`ProtocolOpenAIResponses` uses `POST /v1/responses` with Bearer authentication.
An unknown protocol causes a configuration error. Protocol selection is fixed for each client.

For OpenAI Responses, set these client fields:

```go
llmproxyclient.ConfigInput{
    Protocol: llmproxyclient.ProtocolOpenAIResponses,
    BaseURL:  "https://llm-proxy.example.com",
    Secret:   tenantKey,
    Provider: "openai",
}
```

Use a service-root BaseURL without credentials, query parameters, or a fragment.
Select the exact model in `MessagesRequestInput.Model` or the current model profile.
The client sends `provider/model` in the standard `model` field.
It sends ordered messages in `input`, the output budget in `max_output_tokens`, and effort in `reasoning.effort`.
It sets `store: false` and uses the existing proxy timeout header for the work budget.

`PostMessages` returns text through the selected protocol.
`PostMessagesCompletion` also returns optional model and usage data for OpenAI Responses.
Native completion results contain text with unknown measurements.
The Responses client supports synchronous text completion. It rejects media, caller tools, web search, and durable structured requests before dispatch.
Use the documented client contracts for those requests.

## Go completion measurements

The Responses client reads completed `output` messages and their `output_text` content.
It reads `model` and the standard `usage.input_tokens`, `usage.output_tokens`, and `usage.total_tokens` fields.
This client enhancement requires no new proxy response header or server deployment.

Read `Text()` after success. Read `ResolvedModel()` and `Usage()` when available.
An empty model means unknown identity. A nil usage value means unknown counts.
A measured zero remains zero. Missing metadata does not change text completion success.
The proxy response identifies the selected catalog route. It does not identify a provider-reported revision.
Keep the requested model in a different field.

`MetadataError()` reports invalid optional metadata. This diagnostic does not change completion success.
Valid text remains available. The diagnostic excludes response content.
Invalid usage remains unknown. Other valid metadata remains available.
HTTP failures, incomplete responses, refusals, and unsupported output return a completion error.
The proxy keeps its existing failure accounting.

## Validation

Run `make test-protocol-acceptance` for the shared local provider qualification.
The target starts the real service and uses the official Go client.
It also runs the OpenAI SDK, native client, OpenCode, connection, persistence, media, lifecycle, and usage scenarios.
The tests use fake provider servers and isolated local data.
OpenAI SDK `7.10.0` and OpenCode `1.18.28` are pinned in test dependencies.
Run `make ci` for repository validation after the final change.

## Qualify another provider

Use this procedure when a provider can use a current protocol adapter.

1. Add a disposable provider definition to the test catalog.
2. Select the protocol, lifecycle, operation, request profile, controls, and media inputs in the test case.
3. Supply test connection values through `ManagedTenantTestConfiguration`.
4. Use the real HTTP service and the official Go client for the request.
5. Verify public discovery, connection fields, secret masking, routing, tenant isolation, restart behavior, and usage.
6. Verify that unsupported controls and invalid credentials cause no provider request.
7. Verify wrong endpoints, malformed results, and incorrect usage totals with the negative controls.
8. Run `make test-protocol-acceptance`.
9. Add the provider definition to `configs/providers.yml` only after the local qualification passes.
10. Run `make ci` after the catalog change.

Do not add a production provider constant for the disposable definition.
Keep live-provider qualification in the live-provider harness.

The protocol references are the [OpenAI Responses API](https://developers.openai.com/api/reference/typescript/resources/responses/methods/create)
and [OpenCode provider configuration](https://opencode.ai/docs/providers/).
