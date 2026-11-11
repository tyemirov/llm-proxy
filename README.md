# LLM Proxy

LLM Proxy is an HTTP service that centralizes AI provider access for applications and agents.
It keeps provider credentials on the server and gives each tenant its own client key, model defaults, prompts, and usage view.
It supports text generation, structured JSON output, transcription, and durable media operations through declared provider routes.

[Explore models](https://llm-proxy.mprlab.com/) · [API reference](https://llm-proxy.mprlab.com/docs/) · [Local setup](docs/local-setup.md) · [License](LICENSE)

## Why use it

- **Change models centrally.** Applications that omit provider and model selection use the tenant default. A saved default change takes effect without application redeployment.
- **Keep provider credentials in one service.** An account can assign one named provider connection to several tenants. Each tenant retains separate defaults and usage.
- **Use existing client interfaces.** Supported OpenAI clients, native Go and Python packages, the CLI, and MCP clients can access the service.
- **Track accepted media work.** Durable operations retain status and output assets after a client disconnect. Idempotency keys bind repeated submissions to the accepted operation.

The service fits applications that need several providers, centrally managed model choices, or durable image, audio, and video work.
Exact capabilities depend on the selected provider offering and client interface.
The repository uses [PolyForm Noncommercial](LICENSE), with a separate [commercial licensing notice](COMMERCIAL_LICENSE.md).

## First request

Use an existing LLM Proxy service, or start the [local browser stack](docs/local-setup.md).
For the public service, open [LLM Proxy](https://llm-proxy.mprlab.com/) and select **Log In**.

1. Create a provider connection with your provider credentials.
2. Select a tenant and attach the connection with **Connect**.
3. Select a model and save its text default.
4. Open **API access** in tenant details and create a client key.
5. Store the displayed key in `LLM_PROXY_DEFAULT_TENANT_KEY` for your application.

The new key appears once. Replacement invalidates the previous key immediately.
The tenant client key authorizes proxy calls. The provider key stays in the saved connection.
Connection verification can make a provider request and incur provider charges.

Send a native request that uses the saved text default:

```sh
export LLM_PROXY_BASE_URL='https://llm-proxy-api.mprlab.com'
# Set LLM_PROXY_DEFAULT_TENANT_KEY to the generated tenant client key.
curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"messages":[{"role":"user","content":"Reply with Hello."}]}' \
  "$LLM_PROXY_BASE_URL/v2?key=$LLM_PROXY_DEFAULT_TENANT_KEY"
```

A successful request returns the model's answer as plain text.
For local use, set `LLM_PROXY_BASE_URL` to `http://localhost:8080`.
See the [client guide](docs/client-guide.md) for provider selection, media inputs, response formats, installation, and Go and Python examples.

## Choose an interface

| Interface | Authentication | Use |
| --- | --- | --- |
| Native `/v2` | Tenant key in `key` query parameter | Messages, tenant defaults, declared image and audio inputs, provider web search, caller tools, and structured output. |
| OpenAI `/v1` | Tenant bearer key | Supported Chat Completions, Responses, model discovery, and transcription clients. Select an exact `provider/model`. |
| TypeSafe `/v1/systemone` | Tenant bearer key | The TypeSafe System One API for Jev decisions. Keep TypeSafe model names and client request syntax. |
| Media `/model/v1` | Tenant bearer key | Asset uploads, capability discovery, durable operations, cancellation requests, and result downloads. |
| MCP `/mcp` | TAuth OAuth token | Account tenant discovery, route discovery, text generation, and media operation tools. Each operation selects an explicit tenant. |
| Management `/api/management` | TAuth session | Accounts, tenants, connections, defaults, keys, and usage. |

The [client protocol guide](docs/client-protocols.md) defines the accepted OpenAI API subset.
Its event streams contain buffered results after completion, rather than incremental provider tokens.
The `/v1` text endpoints accept text message parts. Native `/v2` provides declared image and audio inputs and provider web search.
Caller tools require an eligible offering, and the client executes those tools.

The Go package defaults to native `/v2` and also provides a selected Responses protocol.
The Python package and prompt CLI use native `/v2`.
See [MCP setup](docs/mcp.md) for OAuth and client-specific evidence.
The [OpenAPI file](docs/openapi.yaml) owns the complete HTTP contract.

## Capabilities and limits

The [provider catalog](configs/providers.yml) defines exact models, provider offerings, controls, limits, and prices.
It covers OpenAI, Anthropic, Gemini, Vertex AI, DeepSeek, Alibaba Cloud, Moonshot, MiniMax, Meta, xAI, Baidu, SiliconFlow, Z.AI, Dictator, FAL, ElevenLabs, HeyGen, and TypeSafe.
Some entries remain disabled during qualification.
A catalog declaration does not establish live availability for an account.

| Capability | Scope |
| --- | --- |
| Text and reasoning | Exact model selection, saved defaults, and route-specific controls. |
| Structured JSON | Caller JSON Schema validation and durable reconciliation on supported routes. |
| Typed decisions | The [TypeSafe System One contract](docs/typesafe-system-one.md) with Choice, Score, Noul, Box, and image assets. |
| Image and audio input | Ordered native message attachments where the offering declares the input capability. |
| Transcription | File transcription through configured tenant routes. |
| Media operations | Implemented image, speech, audio-processing, and video routes with tenant assets and retained execution state. |

Use the [route explorer](https://llm-proxy.mprlab.com/) for catalog capabilities.
Use authenticated discovery for routes available to the selected tenant.
The [provider reference](docs/provider-catalog.md) and [media inventory](docs/media-provider-completeness.md) identify implemented contracts and remaining work.
MediaOps owns product workflows and local processing. LLM Proxy owns shared provider access.

Ordinary text requests wait for a final answer within the request budget.
Structured requests and media operations have separate reconciliation contracts.
An `uncertain` outcome requires reconciliation because another submission can duplicate provider work.
See [request execution](docs/request-execution.md) for budgets, continuation, cancellation, errors, and recovery.

## Credentials, data, and retention

Provider credentials use AES-GCM encryption at rest. The backend decrypts them for provider calls.
Tenant keys appear once. The database retains their digests.
Prompts and supplied media reach the selected provider under that provider's terms.

Request logs and operational usage exclude prompts, generated content, and credentials.
Durable structured results, media requests, and media assets have separate storage and retention rules.
OpenAI background Responses require provider storage.
Hosted recovery can retain private provider identifiers.

Operational usage uses a bounded process-local queue. Queue saturation, failed inserts, or process termination can lose uncommitted events.
Its token subtotals and coverage describe retained evidence, rather than complete financial history.
Hosted billing uses a separate durable journal and ledger.

Public pages load Google Analytics and LoopAware scripts.
See [Privacy](https://llm-proxy.mprlab.com/privacy/), [request storage](docs/request-execution.md), and [token evidence](docs/token-measurement-evidence.md).

## Project status and operation

F065 through F070 completed controlled development acceptance for prepaid hosted access, exact charges, funds, and payments.
The tracked configuration omits `hosted`.
Production activation, actual processor qualification under F087, and provider financial qualification remain separate.
The [billing runbook](docs/hosted-billing.md#development-acceptance) records acceptance and launch prerequisites.

The [integration register](docs/integrations.md) distinguishes configuration support, tested execution, upstream acceptance, and publication.
Goose configuration support is confirmed. Execution qualification remains open under I296.
OpenCode has recorded local acceptance. The [MCP client map](docs/mcp-clients.md) records results for individual client versions.

| Task | Guide |
| --- | --- |
| Run the local browser stack and configure the service | [Local setup](docs/local-setup.md) |
| Install clients and make requests | [Client guide](docs/client-guide.md) |
| Manage tenants, connections, and defaults | [Tenant connections](docs/tenant-connections.md) |
| Run tests, lint, and provider qualification | [Development](docs/development.md) |
| Prepare, publish, and deploy a release | [Deployment](docs/deployment.md) |
| Transfer a retained predecessor database | [Schema transition](docs/managed-schema-transition.md) |
| Use speech and media workflows | [Speech workflows](docs/speech-workflows.md) |

Local browser execution uses `make up` and `make down`.
Repository validation uses `make ci`. Live-provider checks remain separate and can incur charges.
Release, publication, and deployment use the installed MPR Lab Gateway and have distinct results.

## License

See [LICENSE](LICENSE) for PolyForm Noncommercial License 1.0.0.
The [commercial licensing notice](COMMERCIAL_LICENSE.md) describes separate written agreements and the contact `legal@mprlab.com`.
Third-party and separately licensed materials retain their own terms.
