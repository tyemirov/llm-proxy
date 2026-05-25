# LLM Proxy

LLM Proxy is a lightweight HTTP service that forwards user prompts to OpenAI's
Responses API, OpenAI-compatible chat providers, and audio transcription APIs.
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

The service is configured entirely through command-line flags or environment
variables:

| Flag / Env | Description |
|------------|-------------|
| `--service_secret` / `SERVICE_SECRET` | Shared secret required in the `key` query parameter |
| `--openai_api_key` / `OPENAI_API_KEY` | OpenAI API key used for requests |
| `--deepseek_api_key` / `DEEPSEEK_API_KEY` | DeepSeek API key used when `provider=deepseek` |
| `--dashscope_api_key` / `DASHSCOPE_API_KEY` | DashScope API key used when `provider=dashscope` or `provider=qwen` |
| `--moonshot_api_key` / `MOONSHOT_API_KEY` | Moonshot/Kimi API key used when `provider=moonshot` or `provider=kimi` |
| `--siliconflow_api_key` / `SILICONFLOW_API_KEY` | SiliconFlow API key used when `provider=siliconflow` |
| `--zhipu_api_key` / `ZHIPU_API_KEY` | Zhipu/GLM API key used when `provider=zhipu` or `provider=glm` |
| `--default_provider` / `LLM_PROXY_DEFAULT_PROVIDER` | Default text provider when `provider` is omitted (default `openai`) |
| `--default_model` / `LLM_PROXY_DEFAULT_MODEL` | Default text model when `model` is omitted for OpenAI (default `gpt-4.1`) |
| `--default_dictation_provider` / `LLM_PROXY_DEFAULT_DICTATION_PROVIDER` | Default `/dictate` provider when `provider` is omitted (default `openai`) |
| `--deepseek_base_url` / `DEEPSEEK_BASE_URL` | DeepSeek OpenAI-compatible base URL |
| `--dashscope_base_url` / `DASHSCOPE_BASE_URL` | DashScope OpenAI-compatible base URL |
| `--moonshot_base_url` / `MOONSHOT_BASE_URL` | Moonshot OpenAI-compatible base URL |
| `--siliconflow_base_url` / `SILICONFLOW_BASE_URL` | SiliconFlow OpenAI-compatible base URL |
| `--siliconflow_transcriptions_url` / `SILICONFLOW_TRANSCRIPTIONS_URL` | SiliconFlow audio transcription URL |
| `--zhipu_base_url` / `ZHIPU_BASE_URL` | Zhipu OpenAI-compatible base URL |
| `--port` / `HTTP_PORT` | Port for the HTTP server (default `8080`) |
| `--log_level` / `LOG_LEVEL` | `debug` or `info` (default `info`) |
| `--system_prompt` / `SYSTEM_PROMPT` | Optional system prompt text |
| `--workers` / `LLM_PROXY_WORKERS` | Number of worker goroutines (default `4`) |
| `--queue_size` / `LLM_PROXY_QUEUE_SIZE` | Request queue size (default `100`) |
| `--max_output_tokens` / `LLM_PROXY_MAX_OUTPUT_TOKENS` | Max output token budget forwarded to text models (default `8192`) |
| `--max_prompt_bytes` / `LLM_PROXY_MAX_PROMPT_BYTES` | Max accepted JSON body size for `POST /` prompts (default `4194304`) |
| `--dictation_model` / `LLM_PROXY_DICTATION_MODEL` | Default model for `/dictate` when query model is not provided (default `gpt-4o-mini-transcribe`) |
| `--max_input_audio_bytes` / `LLM_PROXY_MAX_INPUT_AUDIO_BYTES` | Max accepted upload size for `/dictate` (default `26214400`) |

Web search is per request and currently supported only on OpenAI models that
support the OpenAI web search tool.

## Running

Generate a secret:

```shell
openssl rand -hex 32
```

Run the service with OpenAI defaults:

```shell
SERVICE_SECRET=mysecret OPENAI_API_KEY=sk-xxxxx \
  ./llm-proxy --port=8080 --log_level=info
```

Run the service with DeepSeek as the default text provider:

```shell
SERVICE_SECRET=mysecret DEEPSEEK_API_KEY=sk-xxxxx LLM_PROXY_DEFAULT_PROVIDER=deepseek \
  ./llm-proxy --port=8080 --log_level=info
```

## Local Automation

This repository exposes the standard local targets used by MPR app repos:

| Command | Purpose |
|---------|---------|
| `make ci` | Run format checks, Go lint (`go vet`, `staticcheck`, `ineffassign`), and the 100% coverage-gated Go test suite. |
| `make release` | Cut a `v*` release from `master`, update `CHANGELOG.md` when needed, and push the release tag. |
| `make publish` | Validate the release source and publish `ghcr.io/tyemirov/llm-proxy:<tag>` plus `:latest`. |
| `make deploy` | Verify the published image and deploy through the sibling `../mprlab-gateway` checkout. |

`llm-proxy` is a gateway-local service in `mprlab-gateway`, so `make deploy`
defaults to the gateway `deploy-gateway` target. Override the checkout or target
with `GATEWAY_DIR=/path/to/mprlab-gateway` or
`GATEWAY_DEPLOY_TARGET=<target>`.

## Usage

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

### Large text request

Use `POST /` with a JSON body when the prompt is too large for a URL query
parameter. Authentication still uses the `key` query parameter, which is the
proxy `SERVICE_SECRET`. Provider selection also stays in the query parameter.
Do not send upstream provider secrets in the request body; the proxy reads them
from server-side configuration. The JSON body is capped by `--max_prompt_bytes`
/ `LLM_PROXY_MAX_PROMPT_BYTES`.

```shell
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"prompt":"large text...","model":"gpt-5.5","web_search":false,"system_prompt":"optional"}' \
  "http://localhost:8080/?key=mysecret"
```

JSON body fields:

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `prompt` | Yes | none | Full text to send to the LLM. Use this body field for large or non-ASCII prompts. |
| `model` | No | provider default | Model identifier from the selected provider's supported model list. |
| `web_search` | No | `false` | Enables OpenAI web search when the selected provider/model supports it. |
| `system_prompt` | No | configured `SYSTEM_PROMPT` | Per-request system prompt override. |

For `POST /`, `provider` remains a query parameter. Query `model` may override
the JSON body only when the body omits `model` or provides the same value;
conflicting values return `400 Bad Request`.

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
* `application/json` - JSON object containing `request` and `response` fields
* `application/xml` - XML document `<response request="...">...</response>`

If no supported value is provided, `text/plain` is returned.

## Endpoint

### LLM endpoint

```text
GET /
  ?prompt=STRING            # required
  &key=SERVICE_SECRET       # required
  &provider=PROVIDER        # optional; defaults to openai
  &model=MODEL_NAME         # optional; provider default
  &web_search=1|true|yes    # optional; OpenAI web_search tool
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
  "system_prompt": "STRING" # optional; defaults to configured system prompt
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
