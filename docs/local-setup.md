# Local setup and configuration

Run commands in this guide from the repository root.
The browser stack requires Docker Compose v2, Python 3, curl, and OpenSSL.
It includes the static frontend, backend API, and TAuth.
For an existing service, use the [client guide](client-guide.md) instead.

## Running

Before the first run, create the ignored private `configs/.env.local` file.
Populate the file with real local values.
The tracked `configs/.env.local.example` file documents local fields.
The tracked `configs/.env.sample` file documents direct runtime fields.

Both files contain deliberately unrealistic values. Never copy or source them
as runtime configuration. `make up` fails before contacting Docker when the
private file is absent. When the real file uses the explicit
`__GENERATE_ON_FIRST_MAKE_UP__` marker for its local TAuth signing key or
provider-key encryption key, `make up` generates that value once. It then
writes ignored, service-scoped environment projections for ghttp, llm-proxy,
and TAuth.

Run the local services:

```sh
make up
```

Open `http://localhost:4179/` after the command reports readiness.
Stop the stack with `Ctrl-C`, or run this command from another terminal:

```sh
make down
```

ghttp receives only its `GHTTP_*` inputs. TAuth receives only its server and
tenant inputs, including the signing key it shares with the API. Only llm-proxy
receives the provider-key encryption configuration; aggregate dotenv files and
live provider smoke-test credentials are not injected into auxiliary
containers. The API image is built from the current source and runs the
canonical `configs/config.yml` configuration.

Local and production orchestration do not bind an Alibaba Cloud URL. Each Alibaba Cloud connection
stores its Singapore Model Studio workspace URL and API key.
The stack has these explicit browser-facing endpoints:

- Public landing: `http://localhost:4179/`, served by ghttp from the rendered
  local site artifact.
- Management UI: `http://localhost:4179/app/`, served from the same static artifact.
- API reference: `http://localhost:4179/docs/`, generated from the canonical
  schema with explicit raw-view and exact-YAML download actions.
- OpenAPI schema: `http://localhost:4179/openapi.yaml`, served through the
  frontend from the canonical read-only `docs/openapi.yaml` mount.
- Backend API: `http://localhost:8080/`, including the proxy and
  `/api/management/*` endpoints.

`make up` starts the API, verifies its public capability resource, and runs a
one-shot frontend site-builder. That builder fetches
`/api/public/capabilities`, renders the validated provider registry into the
public routing tree and capability matrix, and writes an isolated static
artifact for ghttp to serve read-only. Startup rejects a failed API resource or
site build, and shutdown removes the temporary artifact.

ghttp proxies `http://localhost:4179/config-ui.yaml` to the API and the
same-origin `/auth/*`, `/me`, `/oauth/*`, and OAuth discovery routes to the internal TAuth service. The
browser receives `http://localhost:4179` as its TAuth origin and the direct API
origin from that one runtime configuration. Production keeps its explicit
split-origin topology; local authentication stays on the front door so another
host process cannot intercept a TAuth port through a different `localhost`
address family. Use the `localhost` UI URL rather than `127.0.0.1`: TAuth's
insecure local HTTP cookie profile is intentionally scoped to the single
`localhost` host. The local ghttp front door sends `Cache-Control: no-store` so
an ordinary reload reads one current set of mounted HTML, CSS, and ES modules
instead of combining files cached from different working-tree states.

Compose first completes image pulls/builds and reports all four services
running through `docker compose up --wait`; only then does the bounded HTTP
readiness budget begin. Readiness proves static content (`200`), the canonical
OpenAPI mount (`200`), the ghttp-served runtime config (`200`), the
unauthenticated API boundary (`403`), the same-origin TAuth session (`204`) and
nonce (`200`) boundaries, and the unauthenticated management API boundary
(`401`). It does not call a paid provider. After readiness, Compose logs remain
attached in the foreground. Use `Ctrl-C` there or run `make down` from another
terminal to stop the same local containers, project network, and orphaned
services. Both shutdown paths retain the named local data volumes so local
TAuth and management state remain available for the next run.

Browser startup additionally loads the pinned Alpine 3.13.5 module from
`https://cdn.jsdelivr.net`. `make up` cannot override a Chrome extension,
privacy filter, or browser policy that blocks that client-side request. If the
page reports **Unable to open LLM Proxy**, allow `cdn.jsdelivr.net` for
`http://localhost:4179` in the blocking browser control and select **Reload LLM
Proxy**. The same failure screen replaces an incoherent or rejected first-party
module graph. LLM Proxy does not try another CDN or a bundled fallback; the
failure screen completes the shared MPR transition without making a protected
management request.

Select **Create connection** on the dashboard to save provider credentials.
Select a tenant and use **Connect** to attach an existing connection.
Select a model and save its text, transcription, or speech default.
Supported text routes also expose reasoning effort controls.
A supported request value overrides the saved effort for that request.

Use **API access** in tenant details to create or replace the tenant client key.
Applications store this value as `LLM_PROXY_DEFAULT_TENANT_KEY`.

## Configuration

The service provides `debug` and `info` logging levels.

The service reads service configuration from `config.yml`. The default path is
`config.yml` in the current working directory. Use `--config /path/config.yml`
to select a different file.

The loader also reads `providers.yml` from the selected configuration
directory. That file is the only source for supported providers, exact models,
provider offerings, operations, controls, limits, and prices.

Before parsing YAML, the loader expands `${NAME}` placeholders from process
environment variables and from an optional `.env` file in the same directory as
the selected config file. Process environment values override `.env` values.
Missing placeholders in `config.yml` fail startup. The loader does not mutate
the process environment. Runtime code receives only validated configuration.

The complete service configuration is in
[configs/config.yml](../configs/config.yml). The complete provider catalog is in
[configs/providers.yml](../configs/providers.yml). See the
[provider catalog reference](provider-catalog.md) for its field mapping
and provider procedure.

`server.upstream_capacity` is required. All limits are explicit positive integers.
An `active` limit bounds requests that hold an upstream response body.
An `admitted` limit includes active and queued requests.
Each origin declares its own `active` and `queued` limits.

| Field | Capacity ownership |
| --- | --- |
| `global` | All upstream HTTP requests in the process. |
| `tenant` | One authenticated tenant within one origin. |
| `account` | One saved provider connection within one origin, including its assigned tenants. |
| `media` | Media submission requests across all origins. |
| `status` | Provider status requests across all origins. |
| `transfer` | Provider artifact transfers across all origins. |
| `interactive_reserve` | Active and admitted capacity unavailable to media, status, and transfer requests within each shared limit. |
| `origins` | Exact origin allocations. Each entry requires `origin`, `active`, and `queued`. |

The sum of origin allocations must not exceed `global.admitted`.
Each allocation contains the origin's active and queued limits.
Thus, one origin cannot consume another origin's admission allocation.

The canonical OpenAI allocation has eight active slots and 24 queued slots.
It supports the 32-request tenant and account admission limits. Global admitted capacity is 652.
Ready origins and tenants receive capacity in turn.
An exhausted allocation returns `503 request queue full`.
Accepted media operations retain their separate durable-job limits.

Startup rejects missing, duplicate, unknown, and contradictory origin rules.
Every saved HTTP connection must resolve to declared origins at startup.
It also rejects the removed `workers` and `queue_size` fields.
The checked-in configuration declares the catalog's configured HTTP origins.
Before you configure a connection URL, add its exact origin to `origins`.
For a connection-specific origin, also set `provider` to its catalog provider identifier.
The provider must declare an HTTP endpoint supplied by a connection field.
For example, a DashScope workspace rule uses `provider: dashscope` and its exact HTTPS origin.
Restart the service after you change these allocations.

An active request retains capacity until its response body closes.
A media worker releases HTTP capacity between remote-job polls.
Accepted jobs use `media_operation_workers` and the durable operation limits.
HTTP admission does not replace those job limits.

The `upstream HTTP admission` event records each decision and work class.
It includes the origin, global active and admitted counts, and origin active and queued counts.
HTTP requests carry a `request_id`. Media workers carry an `operation_id`.
The event excludes prompts, credentials, connection fields, and provider error bodies.

`server.upstream_rate_limits` applies rolling-window call limits to the shared HTTP layer.
Rules match an exact normalized origin (`scheme://host[:port]`).
Providers that use the same origin share one rate budget.
Different origins have independent budgets.

A delayed call retains its origin's bounded admission but does not occupy active capacity.
Every upstream attempt, including each retry, consumes one call.
The scheduler reserves a rate slot only when both active capacity and the rate window permit dispatch.
An absent or empty list disables rate limits.
Invalid and duplicate rules fail startup.

```yaml
server:
  upstream_rate_limits:
    - origin: "https://api.openai.com"
      max_requests: 60
      interval: "1m"
```

`origin` accepts an exact `http` or `https` origin without user info, path, query, or fragment.
`max_requests` must be positive.
`interval` must be a positive Go duration such as `500ms`, `1s`, or `1m`.
The delay event records the origin, limit, interval, and wait duration.
The event also includes the proxy request identifier when the request has telemetry.
Context cancellation emits a warning and preserves the request-timeout error mapping.

## Management configuration

Required hosted values are profile-specific:

| Field | Purpose |
|-------|---------|
| `management.public_origin` | Static frontend origin allowed for credentialed management CORS, for example `https://llm-proxy.mprlab.com`. |
| `management.ui_description` | Browser-facing MPR UI environment description. |
| `management.ui_origins` | Browser-facing MPR UI allowed origins served from `/config-ui.yaml`. |
| `management.admin_emails` | Exact administrator email addresses. In public config, populate this from `${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}` as a YAML flow sequence such as `["admin@example.invalid","ops@example.invalid"]` so personal admin addresses stay out of the repository. |
| `management.tauth_url` | Browser-facing TAuth API origin served from `/config-ui.yaml`. |
| `management.tauth_tenant_id` | TAuth tenant id that issues accepted sessions. |
| `management.google_client_id` | Browser-facing Google OAuth web client id for the `llm-proxy` TAuth tenant. |
| `management.login_path` | Browser-facing TAuth Google login path. |
| `management.logout_path` | Browser-facing TAuth logout path. |
| `management.nonce_path` | Browser-facing TAuth nonce path. |
| `management.session_path` | Browser-facing TAuth session restore path, normally `/auth/session`. |
| `management.jwt_signing_key` | Internal signing key used to validate the TAuth session cookie. |
| `management.jwt_issuer` | JWT issuer, normally `tauth`. |
| `management.session_cookie_name` | Exact app/environment TAuth session cookie name. |
| `management.database_path` | Required SQLite location for provider connections, provider profiles, defaults, generated-secret digests, and usage events. |
| `management.usage_queue_size` | Positive capacity of the process-local FIFO for asynchronous managed usage persistence. Defaults to `1024`. This queue is independent from upstream network admission. |
| `management.provider_key_encryption_key` | Required base64-encoded 32-byte key used for AES-GCM encryption of account connection credentials at rest. Generate with `openssl rand -base64 32` and store it with backend deployment secrets. |
| `management.management_api_origin` | Browser-facing management API origin served from `/config-ui.yaml` under `llmProxy.managementApiOrigin`. |
| `management.proxy_origin` | Browser-facing public proxy origin served from `/config-ui.yaml` under `llmProxy.proxyOrigin` for generated examples. |
