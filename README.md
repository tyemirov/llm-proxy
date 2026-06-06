# LLM Proxy

LLM Proxy is a lightweight HTTP service that forwards user prompts to OpenAI's
Responses API, OpenAI-compatible chat providers, Google Gemini's native
generateContent API, and audio transcription APIs.
It exposes protected HTTP endpoints that require a shared secret and simplify
integrating provider capabilities without embedding API credentials in each
client.

## Features

- Minimal HTTP server that accepts:
  - `GET /?prompt=...&key=...[&provider=...]` for LLM responses
  - `POST /?key=...[&provider=...]` for large JSON prompt bodies
  - `POST /dictate?key=...[&provider=...]` for audio transcription
- Choose the provider per request via `provider=...` (default: `openai`)
- Choose the model per request via `model=...` (default: `gpt-4.1` for OpenAI)
- Choose the dictation model per request via `model=...` on `/dictate` (default: `gpt-4o-mini-transcribe`)
- Optional per-request OpenAI web search via `web_search=1|true|yes`
- Optional logging at `debug` or `info` levels
- Forwards requests using server-side provider API keys
- Supports plain text, JSON, XML, or CSV responses

## Configuration

The service reads service configuration from `config.yml`. The default path is
`config.yml` in the current working directory; use `--config /path/config.yml`
only to select a different file. Command-line flags and environment variables
are not service configuration sources.

Before parsing YAML, the loader expands `${NAME}` placeholders from process
environment variables and from an optional `.env` file in the same directory as
the selected config file. Process environment values override `.env` values.
Missing placeholders fail startup. The loader does not mutate process
environment, and all runtime code receives only the validated config value.

```yaml
server:
  service_secret: "${SERVICE_SECRET}"
  port: 8080
  log_level: info
  workers: 4
  queue_size: 100
  request_timeout_seconds: 180
  upstream_poll_timeout_seconds: 60
  max_prompt_bytes: 4194304
  max_input_audio_bytes: 26214400
defaults:
  provider: openai
  model: gpt-4.1
  dictation_provider: openai
  dictation_model: gpt-4o-mini-transcribe
  system_prompt: ""
providers:
  openai:
    api_key: "${OPENAI_API_KEY}"
  deepseek:
    api_key: ""
    base_url: ""
  dashscope:
    api_key: ""
    base_url: ""
  moonshot:
    api_key: ""
    base_url: ""
  siliconflow:
    api_key: ""
    base_url: ""
    transcriptions_url: ""
  zhipu:
    api_key: ""
    base_url: ""
  gemini:
    api_key: ""
    base_url: ""
```

Blank optional values use built-in provider defaults where applicable. Startup
validates `server.service_secret`, the credential for `defaults.provider`, and
the credential/endpoint support for `defaults.dictation_provider`. Credentials
for non-default providers may stay blank; requests selecting those providers
return `503 Service Unavailable` until the corresponding `api_key` is configured.
Unknown YAML keys fail startup.

Web search is per request and currently supported only on OpenAI models that
support the OpenAI web search tool.
Text output length is also per request: pass `max_tokens` when a client wants
to cap one generation. When omitted, the proxy does not send a provider
max-token field.
Provider-specific output-token limits are enforced at the request edge when
known. Gemini text models currently reject `max_tokens` above `65536` with
`400 Bad Request` before any upstream provider call.

## Running

Generate a secret:

```shell
openssl rand -hex 32
```

Run the service with OpenAI defaults:

```shell
./llm-proxy --config config.yml
```

Run the service with DeepSeek as the default text provider:

```yaml
defaults:
  provider: deepseek
providers:
  deepseek:
    api_key: "${DEEPSEEK_API_KEY}"
```

Run the service with Gemini as the default text provider:

```yaml
defaults:
  provider: gemini
providers:
  gemini:
    api_key: "${GEMINI_API_KEY}"
```

## Local Automation

This repository exposes the standard local targets used by MPR app repos:

| Command | Purpose |
|---------|---------|
| `make ci` | Run format checks, Go lint (`go vet`, `staticcheck`, `ineffassign`), and the 100% coverage-gated Go test suite. |
| `make test-live-gemini` | Generate a temporary config file and run the current binary against live Gemini using `GEMINI_API_KEY` and `SERVICE_SECRET` placeholders; use `LIVE_ENV_FILE=/path/to/env` to load interpolation values. |
| `make release` | Cut a `v*` release from `master`, update `CHANGELOG.md` when needed, and push the release tag. |
| `make publish` | Validate the release source and publish `ghcr.io/tyemirov/llm-proxy:<tag>` plus `:latest`. |
| `make deploy` | Verify the published image and deploy through the sibling `../mprlab-gateway` checkout. |

`llm-proxy` is a gateway-local service in `mprlab-gateway`, so `make deploy`
defaults to the gateway `deploy-gateway` target. Override the checkout or target
with `GATEWAY_DIR=/path/to/mprlab-gateway` or
`GATEWAY_DEPLOY_TARGET=<target>`.

## Usage

### Installable prompt client

Install the reusable JSON POST client:

```shell
go install github.com/tyemirov/llm-proxy/llm-proxy-client@latest
```

Use it with explicit flags:

```shell
llm-proxy-client \
  --base-url "http://localhost:8080/?provider=gemini" \
  --secret "$SERVICE_SECRET" \
  --model gemini-3.5-flash \
  --prompt "Summarize this"
```

Or read configuration and prompt text from environment/stdin:

```shell
export LLM_PROXY_BASE_URL="http://localhost:8080/"
export LLM_PROXY_SECRET="$SERVICE_SECRET"
printf 'large prompt...\n' | llm-proxy-client --model gpt-5.5 --max-tokens 4096
```

The client always uses `POST /?key=...` with a JSON body. It keeps non-payload
query parameters such as `provider`, strips body-owned query fields such as
`prompt` and `model`, and sends `prompt`, `model`, `web_search`,
`system_prompt`, and `max_tokens` in the body.

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

### Large text request

Use `POST /` with a JSON body when the prompt is too large for a URL query
parameter. Authentication still uses the `key` query parameter, which is the
configured `server.service_secret`. Provider selection also stays in the query parameter.
Do not send upstream provider secrets in the request body; the proxy reads them
from server-side configuration. The JSON body is capped by
`server.max_prompt_bytes`.

```shell
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"prompt":"large text...","model":"gpt-5.5","web_search":false,"system_prompt":"optional","max_tokens":4096}' \
  "http://localhost:8080/?key=mysecret"
```

JSON body fields:

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `prompt` | Yes | none | Full text to send to the LLM. Use this body field for large or non-ASCII prompts. |
| `model` | No | provider default | Model identifier from the selected provider's supported model list. |
| `web_search` | No | `false` | Enables OpenAI web search when the selected provider/model supports it. |
| `system_prompt` | No | configured `defaults.system_prompt` | Per-request system prompt override. |
| `max_tokens` | No | provider default | Positive integer output-token cap for this request. The proxy maps it to OpenAI `max_output_tokens`, OpenAI-compatible `max_tokens`, or Gemini `generationConfig.maxOutputTokens`. |

For `POST /`, `provider` remains a query parameter. Query `model` may override
the JSON body only when the body omits `model` or provides the same value;
conflicting values return `400 Bad Request`.
Gemini `max_tokens` values above `65536` return `400 Bad Request` before the
proxy calls Gemini.

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
  --data-urlencode "web_search=1" \
  "http://localhost:8080/"
```

You can enable web search with GPT-5 by specifying the model:

```shell
curl --get \
  --data-urlencode "prompt=Latest research on quantum gravity" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "model=gpt-5" \
  --data-urlencode "web_search=1" \
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
  "http://localhost:8080/dictate?key=mysecret&model=gpt-4o-mini-transcribe"
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
  "usage": {
    "request_tokens": 1,
    "response_tokens": 1,
    "total_tokens": 2
  }
}
```

## Endpoint

### LLM endpoint

```text
GET /
  ?prompt=STRING            # required
  &key=SERVICE_SECRET       # required
  &provider=PROVIDER        # optional; defaults to openai
  &model=MODEL_NAME         # optional; provider default
  &web_search=1|true|yes    # optional; OpenAI web_search tool
  &max_tokens=N             # optional positive integer per-request cap
  &format=CONTENT_TYPE      # optional; or use Accept header
```

```text
POST /
  ?key=SERVICE_SECRET       # required
  &provider=PROVIDER        # optional; defaults to openai
  &model=MODEL_NAME         # optional; overrides JSON body if absent or equal
  &format=CONTENT_TYPE      # optional; or use Accept header
Content-Type: application/json
{
  "prompt": "STRING",       # required
  "model": "MODEL_NAME",    # optional; provider default
  "web_search": false,      # optional; defaults to false
  "system_prompt": "STRING",# optional; defaults to configured system prompt
  "max_tokens": 512         # optional positive integer per-request cap
}
```

The POST JSON body carries only LLM request parameters. The client shared
secret remains in the `key` query parameter, and upstream provider API keys are
never accepted from client requests.

### Dictation endpoint

```text
POST /dictate
  ?key=SERVICE_SECRET       # required
  &provider=PROVIDER        # optional; defaults to openai
  &model=MODEL_NAME         # optional; provider default
Content-Type: multipart/form-data
  audio=<file>              # required (alias: file)
```

Success response:

```json
{ "text": "..." }
```

Supported LLM endpoint models are listed below. The `/dictate` endpoint defaults
to OpenAI's audio transcriptions API and supports SiliconFlow when
`provider=siliconflow`. Not all models support tools; use a model marked `Yes`
below for web search.

### Model capabilities

| Model | Provider | Web Search |
|-------|----------|------------|
| `gpt-4.1` | OpenAI | Yes |
| `gpt-4o` | OpenAI | Yes |
| `gpt-4o-mini` | OpenAI | No |
| `gpt-5` | OpenAI | Yes |
| `gpt-5-mini` | OpenAI | No |
| `gpt-5.5` | OpenAI | Yes |
| `gpt-5.5-pro` | OpenAI | Yes |
| `deepseek-v4-flash` | DeepSeek | No |
| `deepseek-v4-pro` | DeepSeek | No |
| `deepseek-chat` | DeepSeek | No |
| `deepseek-reasoner` | DeepSeek | No |
| `qwen-plus` | DashScope | No |
| `kimi-k2-0905-preview` | Moonshot/Kimi | No |
| `deepseek-ai/DeepSeek-R1` | SiliconFlow | No |
| `glm-5.1` | Zhipu/GLM | No |
| `gemini-3.5-flash` | Gemini | No |
| `gemini-3.1-flash-lite` | Gemini | No |
| `gemini-2.5-flash` | Gemini | No |
| `gemini-2.5-flash-lite` | Gemini | No |
| `gemini-2.5-pro` | Gemini | No |

### Status codes

* `200 OK` - success
* `400 Bad Request` - missing/invalid parameters, invalid multipart audio form, unknown provider/model, or unsupported provider capability
* `403 Forbidden` - missing or invalid `key`
* `413 Payload Too Large` - JSON prompt body exceeds `max_prompt_bytes`
* `429 Too Many Requests` - upstream provider rate limit
* `503 Service Unavailable` - selected provider is not configured server-side
* `504 Gateway Timeout` - upstream request timed out
* `502 Bad Gateway` - upstream provider API returned an error

## Security

* All requests must include the shared secret via `key=...`.
* Client requests must not include upstream provider API keys; configure them on the server.
* Do not expose this service to the public internet without appropriate network controls.

## Implementation Plans

Current scoped implementation plans are tracked under `docs/implementation/`.

## MPR Integration Verification

For Marco Polo Research Lab integration workflows, use the Codex
`mpr-integration` skill when a change needs contract/profile/task-based
black-box verification against an MPR app or fixture. Keep app-specific
hostnames, cookie names, ports, OAuth callbacks, and environment literals in
the selected integration profile or deployment docs, not in this README.

## Releasing

Use `make release` from a clean, up-to-date `master` branch. It runs `make ci`,
updates `CHANGELOG.md` if the selected version is missing, creates the release
commit when needed, and pushes the `v*` tag. Tags that begin with `v` trigger
the release workflow, which builds and publishes release artifacts and uses the
matching changelog section as release notes.

Use `make publish` only when you need to publish the release image manually.
Use `make deploy` after the release image is published and `:latest` points at
the same digest as the release tag.

## License

This project is licensed under the MIT License. See [LICENSE](MIT-LICENSE) for
details.
