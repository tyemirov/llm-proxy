# Client guide

Use the [README first request](../README.md#first-request) for account and tenant setup.
Use [client protocols](client-protocols.md) for OpenAI clients and the selected Go Responses protocol.

## Usage

### Client authentication and configuration boundary

The installable `llm-proxy-client` command does not discover a user-level or
system-level YAML configuration file. It accepts `--base-url` and `--secret`,
with `LLM_PROXY_BASE_URL` and `LLM_PROXY_DEFAULT_TENANT_KEY` as their environment
counterparts; the Go and Python libraries accept the same values through
application-supplied configuration. The only optional file-based client
configuration input is an application-owned JSON model profile for per-user
provider/model selection. Service `config.yml` remains server-side operator
configuration and is never loaded by a bundled client.

Native text and dictation calls authenticate with the tenant secret in `key=...`.
Asset and media resources require a tenant bearer key.
OpenAI client protocols also use tenant Bearer authentication.
MCP uses account OAuth authorization.

The MPR UI/TAuth session instead authorizes management actions such as
creating a client key or saving a provider key. It does not authenticate a
direct `POST /v2` request. Upstream provider API keys stay only in
authenticated tenant management storage. A client must never send them.

For an end-to-end first request and the boundary between these credentials, see
the [client authentication guide](https://llm-proxy.mprlab.com/resources/llm-proxy-client-authentication/).

### Installable prompt client

Install the reusable JSON POST client:

```shell
go install github.com/tyemirov/llm-proxy/llm-proxy-client@latest
```

Use it with explicit flags:

```shell
llm-proxy-client \
  --base-url "http://localhost:8080/?provider=gemini" \
  --secret "$LLM_PROXY_DEFAULT_TENANT_KEY" \
  --prompt "Summarize this"
```

Or read configuration and prompt text from environment/stdin:

```shell
export LLM_PROXY_BASE_URL="http://localhost:8080/"
export LLM_PROXY_DEFAULT_TENANT_KEY="..."
printf 'large prompt...\n' | llm-proxy-client --max-tokens 4096
```

For a route whose catalog declares the value, override the tenant default for
one request with `--reasoning-effort`:

```shell
llm-proxy-client \
  --base-url "http://localhost:8080/?provider=openai" \
  --secret "$LLM_PROXY_DEFAULT_TENANT_KEY" \
  --model gpt-5.5 \
  --reasoning-effort high \
  --request-timeout-seconds 900 \
  --prompt "Summarize this"
```

The client always uses canonical `POST /v2?key=...` with a JSON body. It keeps
non-payload query parameters such as `provider`, strips body-owned query fields
such as `prompt` and `model`, and sends the prompt as a v2 `user` message.
`--system-prompt` becomes a v2 `system` message. Optional `model`,
`web_search`, `max_tokens`, and `reasoning_effort` values remain body fields.
When `--model` is omitted, the body omits `model` so llm-proxy uses the selected
provider's configured default model. `--request-timeout-seconds` is instead
serialized as `X-LLM-Proxy-Request-Timeout-Seconds`; omitting the flag omits the
header and selects the server default. The obsolete `--timeout` flag is not an
alias and is rejected.

The Go package under `pkg/llmproxyclient` uses native `/v2` by default.
It also provides the selected [OpenAI Responses protocol](client-protocols.md#go-client-protocol-selection).
For native messages, construct a
`MessagesRequest` with `NewMessagesRequest` and send it with
`Client.PostMessages`. `MessagesRequestInput.ReasoningEffort` is an optional
nonblank request override; the proxy validates it against the exact resolved
provider/model capability before it calls upstream.

Use `Client.GetPublicCapabilities` to read provider offerings and media limits.
The client owns the public resource path, response boundary, and typed offering
contract. Applications must not reproduce that HTTP operation.

Set `MessagesRequestInput.RequestTimeoutSeconds` when one request needs a
specific proxy work budget:

```go
requestTimeoutSeconds := 900
request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
    Messages: []llmproxyclient.MessageInput{
        {Role: "user", Content: "Summarize this"},
    },
    RequestTimeoutSeconds: &requestTimeoutSeconds,
})
if err != nil {
    return err
}
text, err := client.PostMessages(ctx, request)
```

For a route whose model catalog declares image input, construct media through
the official client and attach it to a user message:

```go
frameBytes, err := os.ReadFile("frame.png")
if err != nil {
    return err
}
frame, err := llmproxyclient.NewImageAttachment(llmproxyclient.ImageAttachmentInput{
    MIMEType: "image/png",
    Data:     frameBytes,
})
if err != nil {
    return err
}
request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
    Messages: []llmproxyclient.MessageInput{{
        Role:        "user",
        Content:     "Inspect this exact frame.",
        Attachments: []llmproxyclient.MessageAttachment{frame},
    }},
    Model: "gemini-3.5-flash",
})
if err != nil {
    return err
}
text, err := client.PostMessages(ctx, request)
```

`NewImageAttachment` accepts `image/jpeg`, `image/png`, or `image/webp`.
`NewAudioAttachment` accepts `audio/m4a`, `audio/mpeg`, or `audio/wav`.
Both constructors copy the supplied nonempty bytes and serialize canonical
base64. Attachment values cannot be constructed directly. The proxy decodes
the bytes at the HTTP edge and preserves attachment order. It rejects media on
non-user messages or unsupported model routes before upstream work. It never
echoes media bytes in response metadata.

Use `UploadAsset` when the application needs a reusable tenant asset. The
returned record contains the opaque asset id, MIME type, byte count, state,
and expiry. Construct the attachment from that exact record:

```go
asset, err := client.UploadAsset(ctx, llmproxyclient.AssetUploadInput{
    MIMEType: "image/png",
    Data:     frameBytes,
})
if err != nil {
    return err
}
frame, err := llmproxyclient.NewImageAssetAttachment(
    llmproxyclient.ImageAssetAttachmentInput{
        AssetID:  asset.AssetID,
        MIMEType: asset.MIMEType,
    },
)
```

`NewAudioAssetAttachment` provides the corresponding audio constructor. Both
asset constructors serialize `asset_id` instead of `data` and preserve the
same attachment union.

The request value controls only the proxy budget header. `ctx` remains the Go
caller's independent cancellation authority, and the injected `HTTPDoer` may
have its own explicitly selected transport policy. The package does not add a
total-response timeout.

Every completed non-2xx response returns an `*llmproxyclient.HTTPFailure`.
Use `errors.As` to read its `StatusCode()` and recognized stable
`ProxyErrorCode()` values; `errors.Is(err,
llmproxyclient.ErrClientHTTPFailure)` also remains true. Raw response bodies
are never included in or exposed by the returned error. Transport and response
read failures preserve `ErrClientHTTPFailure` without fabricating a completed
HTTP status.

## Media client cutover

This media contract change requires one coordinated server and client update.
The prior server rejects current media clients. The current server rejects prior
media clients.

For text-only applications, this media change does not require a source change.
Inline media helpers keep their function signatures, but their serialized form
changes. Complete these steps before you resume media traffic:

1. Stop media requests to the prior server.
2. Update each Go application to `llmproxyclient@latest`.
3. Remove `SHA256` from `ImageAssetAttachmentInput` and
   `AudioAssetAttachmentInput` values.
4. Stop reading `Asset.SHA256` from asset upload results.
5. Update each Python application to this repository release.
6. Remove the third argument from `image_asset_attachment` and
   `audio_asset_attachment` calls.
7. Stop reading `ClientAsset.sha256` from asset upload results.
8. For direct HTTP integrations, remove `sha256` from each attachment.
9. Remove `X-LLM-Proxy-Asset-SHA256` from each direct asset upload.
10. Remove `sha256` from each expected asset response.
11. Update fake server fixtures to use the exact current request and response
    shapes.
12. Run each application's fake server tests against the updated official
    client.
13. Stop the prior server.
14. Move root-level `ast_*.json` and `ast_*.data` files from
    `server.asset_store_path` to a private backup directory.
15. Keep the `structured-requests` directory unchanged.
16. Start the current server and the updated applications.
17. Upload retained source media again and replace each stored prior `asset_id`.
18. Resume media traffic after the server and all applications pass acceptance
    checks.

Do not deploy a current media client against the prior server. Do not deploy
the current server while a prior media client remains active. Do not add a
dual-shape client or server bridge.

Applications that use `GetPublicCapabilities` must update their fake responses.
Each response must contain nonempty offerings with `wire_contract`,
`execution_lifecycle`, and all required media limit records. The client returns
`ErrClientHTTPFailure` for an incomplete successful response.

To upgrade the Go package and CLI:

```shell
go get github.com/tyemirov/llm-proxy/pkg/llmproxyclient@latest
go install github.com/tyemirov/llm-proxy/llm-proxy-client@latest
```

Remove `ConfigInput.Timeout` from existing Go integrations and move the desired
proxy budget to `MessagesRequestInput.RequestTimeoutSeconds`. Use a caller
context only when the application intentionally needs an independent,
potentially shorter cancellation deadline.

#### Model selection without application redeployment

Every bundled client deliberately leaves `model` out of a request when the
caller does not set it. This is the correct integration when LLM Proxy owns
model selection: a managed-tenant owner can change that tenant's routing
default in the LLM Proxy dashboard, and the next model-omitting request uses
the saved default without an application code or deployment change. An explicit
`--model` or request `model` pins that one request and does not follow a tenant
default.

Changing an offering's `default_operations` in service configuration affects
only requests that resolve through that provider default. It does not rewrite
a managed tenant's saved routing default or saved provider setting.

#### Application-user model profiles

For application-owned, per-user selection, configure one client instance with
that user's JSON model-profile path. The document contains exactly these two
nonblank string fields and never contains credentials or TAuth material:

```json
{
  "provider": "gemini",
  "model": "gemini-3.5-flash"
}
```

The client reads this file for every outbound v2 request. An application can
write a replacement in the same filesystem and atomically rename it onto the
user's profile path; the next request from the existing client instance then
uses the new provider/model pair. The application continues to own the user
identity, authorization, storage, and atomic publication of that file.

Use the profile directly from the installable CLI:

```shell
llm-proxy-client \
  --base-url "http://localhost:8080/" \
  --secret "$LLM_PROXY_DEFAULT_TENANT_KEY" \
  --model-profile "/var/lib/my-app/users/42/model.json" \
  --prompt "Summarize this"
```

For the Go package, inject the application's file reader when creating the
validated config once:

```go
config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
    BaseURL:            "http://localhost:8080/",
    Secret:             serviceSecret,
    ModelProfilePath:   userModelProfilePath,
    ModelProfileReader: os.ReadFile,
})
if err != nil {
    return err
}
client, err := llmproxyclient.NewClient(config, http.DefaultClient)
if err != nil {
    return err
}
```

`Config.MessagesPostURL` also resolves the current profile and therefore returns
`(string, error)` in profile-capable client versions.

The profile is the sole provider/model source in this mode. Do not combine it
with `--model`, a request `model`, `--provider`, `ConfigInput.Provider`, or a
`provider` or `model` query parameter on the base URL. The clients reject those
competing inputs; they never choose a winner or retain a previous parsed
profile. A missing, unreadable, malformed, incomplete, or unsupported profile
also fails that request before HTTP with `ErrInvalidModelProfile` (Go) or
`LLMProxyModelProfileError` (Python). The proxy remains responsible for
validating whether the resulting provider/model pair is supported.

Without a profile path, the model-omitting tenant/provider-default path above
remains the separate normal contract.

### Python client package

The same transport contract is available as an importable Python package:

```shell
uv pip install --upgrade "llm-proxy-client @ git+https://github.com/tyemirov/llm-proxy.git@<release-tag>#subdirectory=python"
```

Replace `<release-tag>` with the exact published repository tag for the selected application build. The default
branch contains development source for the next release.
The Python package version comes from the Gix-selected `v1.X.X` release tag.
Python builds use `setuptools-scm` to read this tag.
Wheel and source distribution metadata keep the version without the `v` prefix.
Use the Git checkout or a Python source distribution as the package build input.

```python
from llm_proxy_client import (
    Client,
    ClientConfig,
    ClientMessage,
    ClientMessagesRequest,
    image_asset_attachment,
)

client = Client(
    ClientConfig(
        base_url="http://localhost:8080/?provider=gemini",
        secret="mysecret",
    )
)

text = client.post_messages(
    ClientMessagesRequest(
        messages=(ClientMessage(role="user", content="Summarize this"),),
        max_tokens=512,
        request_timeout_seconds=900,
    )
)
```

The Python client has the same asset contract:

```python
asset = client.upload_asset(frame_bytes, "image/png")
frame = image_asset_attachment(asset.asset_id, asset.mime_type)
text = client.post_messages(
    ClientMessagesRequest(
        messages=(
            ClientMessage(
                role="user",
                content="Inspect this exact frame.",
                attachments=(frame,),
            ),
        ),
        model="gemini-3.5-flash",
    )
)
```

`ClientMessagesRequest.reasoning_effort` is the same optional per-request
override. Supply a nonblank value only for a resolved provider/model route that
declares it; omit the field to retain the tenant default.
`ClientMessagesRequest.request_timeout_seconds` serializes the canonical proxy
work-budget header. Omit it to use the server default.

The current Python client does not accept `ClientConfig.timeout_seconds`. Move that value to
each `ClientMessagesRequest` that needs it. The default urllib transport is
called without a total-response timeout; applications that intentionally need
an independent transport deadline can continue to inject an opener that owns
that policy.

The Python package is v2-only. For chat-transcript callers, send the same
`post_messages` request with multiple messages:

```python
chat_text = client.post_messages(
    ClientMessagesRequest(
        messages=(
            ClientMessage(role="user", content="Summarize this", order=2),
            ClientMessage(role="system", content="Be concise.", order=1),
        ),
        model="deepseek-v4-flash",
    )
)
```

Pass `model` only when an application intentionally pins one request instead
of using the tenant or selected-provider default.

To give one application user a reloadable model choice, configure that user's
profile path and reader once. The client does not cache its parsed contents:

```python
from pathlib import Path


def read_model_profile(path: str) -> str:
    return Path(path).read_text(encoding="utf-8")


user_client = Client(
    ClientConfig(
        base_url="http://localhost:8080/",
        secret="mysecret",
        model_profile_path="/var/lib/my-app/users/42/model.json",
        model_profile_reader=read_model_profile,
    )
)
```

Publish a complete replacement JSON document atomically at that path after the
user selects a new provider/model pair. Do not set `provider` on the config or
base URL, or `model` on the request, when this profile is configured.

The optional `order` field is for callers that do not want to rely on array
position. When any message includes `order`, every submitted message must
include a unique non-negative integer `order`; the proxy sorts ascending before
provider routing and echoes provided order values in JSON responses.

For local development from the repository root, target the canonical `python/`
package project explicitly:

```shell
uv pip install -e ./python
make python-test
make python-lint
```

### Basic request (default provider and model, no web search)

```shell
curl --get \
  --data-urlencode "prompt=Hello, how are you?" \
  --data-urlencode "key=mysecret" \
  "http://localhost:8080/"
```

### Choose a provider

```shell
curl --get \
  --data-urlencode "prompt=Summarize this cheaply" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=deepseek" \
  --data-urlencode "model=deepseek-v4-flash" \
  "http://localhost:8080/"
```

Current Qwen text generation through the tenant's saved Alibaba Cloud workspace:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Qwen" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=dashscope" \
  --data-urlencode "model=qwen3.7-plus" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

Gemini text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Gemini" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=gemini" \
  --data-urlencode "model=gemini-3.5-flash" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

Anthropic Claude text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Claude" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=anthropic" \
  --data-urlencode "model=claude-sonnet-4-6" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

xAI Grok text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Grok" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=xai" \
  --data-urlencode "model=grok-4.3" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

Meta Muse Spark 1.2 text generation:

```shell
curl --get \
  --data-urlencode "prompt=Summarize this with Muse Spark" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "provider=meta" \
  --data-urlencode "model=muse-spark-1.2" \
  --data-urlencode "max_tokens=512" \
  "http://localhost:8080/"
```

### Large text request

Use `POST /` with a JSON body when the prompt is too large for a URL query
parameter or when the caller already has a chat transcript. Authentication
still uses the `key` query parameter, which is the configured tenant secret.
Provider selection also stays in the query parameter. Do not send upstream
provider secrets in the request body; the proxy reads them from server-side
configuration. The JSON body is capped by `server.max_prompt_bytes`.

```shell
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"prompt":"large text...","model":"gpt-5.5","web_search":false,"system_prompt":"optional","max_tokens":4096,"reasoning_effort":"high"}' \
  "http://localhost:8080/?key=mysecret"
```

Chat transcript on `POST /`:

```shell
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"messages":[{"role":"user","content":"Summarize this","order":2},{"role":"system","content":"Be concise.","order":1}],"model":"deepseek-v4-flash","max_tokens":4096}' \
  "http://localhost:8080/?key=mysecret&provider=deepseek"
```

Canonical v2 chat transcript:

```shell
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"messages":[{"role":"user","content":"Summarize this","order":2},{"role":"system","content":"Be concise.","order":1}],"model":"deepseek-v4-flash","max_tokens":4096}' \
  "http://localhost:8080/v2?key=mysecret&provider=deepseek"
```

The authoritative body-field list, required/optional distinction, nested message
shape, and response schemas are rendered directly from OpenAPI in the
[`POST /` API reference](https://llm-proxy.mprlab.com/docs/#operation-postText)
and
[`POST /v2` API reference](https://llm-proxy.mprlab.com/docs/#operation-postV2Messages).
The examples above are intentionally illustrative rather than a separately
maintained field inventory.

For `POST /`, `provider` remains a query parameter. Query `model` may override
the JSON body only when the body omits `model` or provides the same value;
conflicting values return `400 Bad Request`.
Bodies that provide both `prompt` and `messages`, empty `messages`, unsupported
message roles, empty message content, partially specified `order`, duplicate
or negative `order`, or both `system_prompt` and a system message return
`400 Bad Request` before any upstream call.
The proxy returns `400 Bad Request` before upstream dispatch for MiniMax M2
values above `204800` and current Qwen or Gemini values above `65536`. The same
response applies to values above the configured Claude model output limit.

`POST /v2` is the canonical chat endpoint. Its exact accepted fields come from
the OpenAPI schema, including the omission-versus-explicit-value contract for
`reasoning_effort`. It rejects `prompt` and body `system_prompt`; send a
`system` role message instead. The tenant default system prompt is still
prepended when the submitted messages do not include a system message.
Only `POST /v2` user messages may include `attachments`; compatibility
`POST /`, `GET /`, and `/dictate` do not accept that field. Each attachment
is one exact union variant. An inline attachment contains `type`, `mime_type`,
and canonical padded base64 `data`. An asset attachment contains `type`,
`asset_id`, and `mime_type`. The proxy validates tenant ownership, asset state,
expiry, MIME type, byte count, and stored-byte integrity before provider
dispatch. The proxy calculates private checksums for integrity validation.
These checksums are not public asset identity or response fields.
`server.max_prompt_bytes` applies to compatibility `POST /`. Canonical
`POST /v2` accepts an encoded JSON envelope of 8 MiB or less. A smaller
catalog-derived limit applies when its configured text and inline media limits
have a smaller sum. The proxy applies the selected provider offering's
published media limits and transport rules.
Upload larger media through `POST /model/v1/assets` and send its asset
reference through `/v2`.

Upload an asset with the exact media content type:

```shell
curl -X POST \
  -H "Content-Type: image/png" \
  --data-binary @./frame.png \
  "http://localhost:8080/model/v1/assets" \
  -H "Authorization: Bearer $LLM_PROXY_DEFAULT_TENANT_KEY"
```

Use the returned asset record in the canonical message:

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Inspect this exact frame.",
      "attachments": [
        {
          "type": "image",
          "asset_id": "ast_0123456789abcdef0123456789abcdef",
          "mime_type": "image/png"
        }
      ]
    }
  ],
  "model": "gemini-3.5-flash"
}
```

`DELETE /model/v1/assets/{asset_id}` with tenant Bearer authentication deletes the tenant asset. The
default store retains an available asset for 48 hours.

### Choose an OpenAI model

```shell
curl --get \
  --data-urlencode "prompt=Summarize quantum error correction" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "model=gpt-4o" \
  "http://localhost:8080/"
```

### Enable web search

```shell
curl --get \
  --data-urlencode "prompt=What changed in the 2025 child tax credit?" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "web_search=true" \
  "http://localhost:8080/"
```

You can enable web search with GPT-5 by specifying the model:

```shell
curl --get \
  --data-urlencode "prompt=Latest research on quantum gravity" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "model=gpt-5" \
  --data-urlencode "web_search=true" \
  "http://localhost:8080/"
```

### Dictation request

```shell
curl -X POST \
  -F "audio=@./recording.webm" \
  "http://localhost:8080/dictate?key=mysecret"
```

SiliconFlow dictation:

```shell
curl -X POST \
  -F "audio=@./recording.webm" \
  "http://localhost:8080/dictate?key=mysecret&provider=siliconflow"
```

Optional model override:

```shell
curl -X POST \
  -F "audio=@./recording.webm" \
  "http://localhost:8080/dictate?key=mysecret&model=gpt-transcribe"
```

### Response formats

You can request alternative formats using either the `format` query parameter or
the `Accept` header. Supported values are:

* `text/csv` - the reply as a single CSV cell with internal quotes doubled
  and a trailing newline
* `application/json` - JSON object containing `request` and `response` fields,
  plus `usage` when upstream token usage is available
* `application/xml` - XML document `<response request="...">...</response>`

If no supported value is provided, `text/plain` is returned.

When upstream text providers return token usage, the proxy also sets these
response headers without changing the plain text, XML, or CSV response bodies:

| Header | Description |
|--------|-------------|
| `X-LLM-Proxy-Request-Tokens` | Normalized request/input token count |
| `X-LLM-Proxy-Response-Tokens` | Normalized response/output token count |
| `X-LLM-Proxy-Total-Tokens` | Normalized total token count |

JSON-format LLM responses include the same normalized counts:

```json
{
  "request": "Hello",
  "response": "Hi",
  "object": "chat.completion",
  "model": "gpt-4.1",
  "choices": [
    {
      "index": 0,
      "finish_reason": "stop",
      "message": {
        "role": "assistant",
        "content": "Hi"
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Hello"
    }
  ],
  "usage": {
    "request_tokens": 1,
    "response_tokens": 1,
    "total_tokens": 2
  }
}
```

The response `messages` field echoes only caller-visible request messages.
Server-injected tenant default system prompts are sent upstream when applicable,
but are not returned in response metadata.

## Output budgets and web search

Web search is per request and currently supported only on OpenAI models that
support the OpenAI web search tool.
Text output length is per upstream attempt: pass `max_tokens` to set the
initial attempt's budget, which is reused for missing-suffix attempts. If an
output-budget stop contains no visible progress and the model has a configured
output limit, the coordinator increases the next attempt's budget toward that
limit. When omitted, the proxy does not send a provider max-token field, except
Anthropic Messages where `max_tokens` is required upstream and the proxy sends
the selected model's configured output limit.
Provider-specific output-token limits are enforced at the request edge when
known. MiniMax M2 routes reject `max_tokens` above `204800`. Gemini text models
currently reject values above `65536`. Claude models reject values above the
configured synchronous Messages output limit. Those errors return `400 Bad
Request` before any upstream provider call.
