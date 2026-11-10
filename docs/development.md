# Development and validation

Run commands from the repository root. Live-provider checks can incur provider charges.

## Local Automation

This repository exposes the standard local targets used by MPR app repos:

Hosted CI runs Go coverage, backend supporting checks, and frontend qualification in three independent jobs.
The coverage job has a 60-minute limit for setup, compilation, and tests.
Go coverage uses nine passes across every package: funds, payments, media, runtime, two other hosted groups, and three remaining groups.
Each pass retains the existing ten-minute Go test limit. The coverage gate combines all profiles and the executable probes.
The other qualification jobs have ten-minute limits.
These jobs run every portable gate from local `make ci`.
They do not require a private Gateway release or operator credentials.
Local and lifecycle CI also require the real installed Gateway qualification.
`make ci` runs this gate before the other stages.
The gate fails when the selected executable is absent.
It uses the current tracked manifest and local Git transport without production operations.
Playwright global setup builds the capability binary before browser test workers start.
The build does not use the 30-second test hook limit.
Global teardown removes the temporary binary after the browser tests.
Browser tests capture screenshots only when a test fails.
Successful tests must not save screenshots. Do not commit generated browser screenshots.
Frontend tests use `test-results/frontend/`. Authentication tests use `test-results/blackbox/`.
These ignored directories preserve failure screenshots and traces from both CI stages.
Playwright clears each output directory before the next run of that suite.

The required `Test / test` check passes only when all three jobs succeed.
A failed, cancelled, skipped, or missing job result prevents success.

| Command | Purpose |
|---------|---------|
| `make frontend-dependencies` | Install the pinned npm graph and Chromium into ignored project-local state. Focused frontend validation, `make lint`, `make test`, and `make ci` invoke this target automatically. |
| `make check-brand-icons` | Validate local SVG assets and all provider and family mappings. See [Provider and model icons](provider-model-icons.md). |
| `make test-brand-icons` | Run browser and build checks for management and public catalog icons. |
| `make ci-backend` | Run the portable Go suite and require 100% statement coverage. |
| `make test-installed-gateway` | Require an installed Gateway executable and run the real selected-manifest planner plus the portable facade test. |
| `make test-coverage-contract` | Verify all Go test groups, merged coverage counts, and executable probe inputs. |
| `make ci-backend-checks` | Run release checks, Go and Python analysis, protocol acceptance, admission race tests, Python tests, and local provider preflight. |
| `make ci-frontend` | Run frontend analysis, browser tests, the Pages artifact check, and the management authentication test. |
| `make up` | Require the ignored private `configs/.env.local`, then build and run the complete local browser orchestration: ghttp static UI and same-origin TAuth routes on `localhost:4179`, plus the API on `localhost:8080`. It waits for Compose startup before verifying the static/config/auth/API boundaries and reporting ready. |
| `make down` | Stop the exact local Compose project started by `make up`, including orphaned services and its project network, while retaining the named local TAuth and management data volumes. |
| `make ci` | Require the installed Gateway qualification and all portable gates. Require 100% Go statement coverage. Report each gate and the final `CI PASSED` receipt. |
| `make test-dictator` | Run speech protocol acceptance with local providers and the official client. |
| `make test-dictator-live` | Run all six speech capabilities against the selected live service. See [Dictator live acceptance](dictator-live-acceptance.md). |
| `make test-live-provider-harness` | Save a temporary managed OpenAI connection and route one request through a loopback provider. |
| `make test-live-providers` | Start a disposable managed tenant, verify every available provider key through the canonical management operation, and run that provider's live text smoke only after verification succeeds; use `LIVE_ENV_FILE=/path/to/env` to load key values. |
| `make test-live-provider-media` | Verify OpenAI, Anthropic, Gemini, Moonshot, and xAI keys, then send one paid canonical image request through each provider. |
| `make test-live-gemini` | Run direct acceptance for the exact Gemini 3.6 Flash and Gemini 3.7 Flash candidates, then run registered Gemini routes through `make test-live-providers`. |
| `make test-live-minimax-m3` | Qualify MiniMax M3 text and image input through a disposable catalog with M3 enabled. Requires `MINIMAX_API_KEY`. |
| `make test-live-provider-candidate` | Qualify one disabled model through a copied catalog. Set `LIVE_CANDIDATE_MODEL=provider/model` and `LIVE_ENV_FILE`. |
| `make test-live-provider-candidate-media` | Qualify image input for one disabled model through a copied catalog. Set `LIVE_CANDIDATE_MODEL=provider/model` and `LIVE_ENV_FILE`. |
| `make test-live-local-providers` | Build an isolated local Compose project, verify selected provider keys, and send paid `POST /v2` requests through its Dockerized API. |
| `make test-live-local-gemini` | Run every registered Gemini model and reasoning level through the isolated local Compose API. |
| `make live-test` | Send paid production `POST /v2` requests through the Default tenant using only `LLM_PROXY_DEFAULT_TENANT_KEY`: echo checks for OpenAI, Anthropic, Meta, Gemini, and Moonshot, plus large completion cases for OpenAI, Anthropic, Meta, and Gemini. |
| `make release` | Delegate the validated application checkout to the installed Gateway. The transaction validates its exact prepared or reused decision before CI. |
| `make publish` | Publish the exact sealed release through the installed Gateway. |
| `make deploy` | Converge the declared application resources through the installed Gateway. |

Local CI runs Python client checks at the same time as Go integration and admission race checks.
Each check uses separate temporary state.
CI waits for all three checks before browser gates or removal of temporary files.
A failed check prevents a success receipt.

Live provider smoke tests are intentionally not part of `make ci`; they call
paid upstream APIs and depend on local or CI secret availability. The dynamic
target clears catalog provider fields before it loads `LIVE_ENV_FILE`. The
file is the only provider-value source when it is set. The target discovers
these provider keys after loading the file. It verifies
each key against the provider's configured default model, or the exact model
override below, before making that provider's smoke request. By default, the
subsequent smoke omits `model` and proves that the newly saved managed provider
default is operational; an override is included in both verification and smoke.
Set `LLM_PROXY_LIVE_ALL_MODELS=true` to discover every text model for each
selected provider. The harness uses the validated public catalog and runs one
verification and one explicit-model smoke request for each model.

The local Compose live test uses the current `docker-compose.local.yml` file.
It builds the API image from the current checkout. Each project name starts
with `llm-proxy-live-test-` and has a unique random suffix. The suffix prevents
reuse of resources from an earlier run. The test uses allocated loopback ports
and temporary management and TAuth volumes. The command verifies all local
readiness boundaries before it saves a provider
connection. Each text smoke uses canonical `POST /v2` through
the allocated API origin. The harness does not start a host proxy process for
this mode. Cleanup removes the test containers, network, volumes, and temporary
site files after success or failure. The command stops if `make up` is active.

Run selected providers through the isolated local Compose API:

```shell
LLM_PROXY_LIVE_PROVIDERS=openai,gemini \
  make test-live-local-providers LIVE_ENV_FILE=/path/to/provider.env
```

Run every registered Gemini text model and reasoning level through local
Compose:

```shell
make test-live-local-gemini LIVE_ENV_FILE=/path/to/provider.env
```

The Gemini wrapper first tests `gemini-3.6-flash` and `gemini-3.7-flash` directly through Interactions.
It sends one omitted thinking level and each explicit level that the exact model supports.
It then proves active retrieval, completion, cancellation, and deletion for stored background interactions.
Both models passed these checks and are registered through I207.
See [qualified Gemini models](gemini-qualified-models.md) for their route contract and acceptance evidence.
Set `LLM_PROXY_LIVE_GEMINI_CANDIDATES=false` when a run requires only the registered proxy routes.

Select one exact model for independent acceptance:

```shell
LLM_PROXY_LIVE_GEMINI_MODEL=gemini-3.1-pro-preview \
  make test-live-gemini-candidate LIVE_ENV_FILE=configs/.env
```

The selector accepts `gemini-3.1-pro-preview`, `gemini-3.6-flash`, `gemini-3.7-flash`, `gemini-3.8-flash`, and `gemini-3.5-flash-lite`.
The harness does not register a model in the provider catalog.
The preview model uses omitted, `low`, `medium`, and `high` thinking levels.
Its September 5, 2026 acceptance request returned HTTP 429 before the lifecycle tests.
I234 retains the restoration gate.
Google documents the [preview model](https://ai.google.dev/gemini-api/docs/models/gemini-3.1-pro-preview),
[thinking levels](https://ai.google.dev/gemini-api/docs/thinking), and
[background lifecycle](https://ai.google.dev/gemini-api/docs/background-execution).

`make ci` runs all declared gates through one runner.
The admission race tests and full Go tests run concurrently.
The runner requires both results before it continues to the remaining gates.
Each invocation writes a new coverage artifact and verifies it after the final test gate.
An incomplete run returns a nonzero status and identifies the active stage.
An earlier coverage artifact cannot satisfy the completion check.

| Provider | Key variable | Model override |
|----------|--------------|----------------|
| OpenAI | `OPENAI_API_KEY` | `LLM_PROXY_LIVE_OPENAI_MODEL` |
| Baidu Qianfan | `BAIDU_API_KEY` | `LLM_PROXY_LIVE_BAIDU_MODEL` |
| Meta Muse Spark | `MUSE_API_KEY` | `LLM_PROXY_LIVE_META_MODEL` |
| DeepSeek | `DEEPSEEK_API_KEY` | `LLM_PROXY_LIVE_DEEPSEEK_MODEL` |
| Alibaba Cloud/Qwen | `DASHSCOPE_API_KEY` | `LLM_PROXY_LIVE_DASHSCOPE_MODEL` |
| Moonshot/Kimi | `MOONSHOT_API_KEY` | `LLM_PROXY_LIVE_MOONSHOT_MODEL` |
| MiniMax | `MINIMAX_API_KEY` | `LLM_PROXY_LIVE_MINIMAX_MODEL` |
| SiliconFlow | `SILICONFLOW_API_KEY` | `LLM_PROXY_LIVE_SILICONFLOW_MODEL` |
| Z.AI | `ZAI_API_KEY` | `LLM_PROXY_LIVE_ZAI_MODEL` |
| Gemini | `GEMINI_API_KEY` | `LLM_PROXY_LIVE_GEMINI_MODEL` |
| Anthropic/Claude | `ANTHROPIC_API_KEY` | `LLM_PROXY_LIVE_ANTHROPIC_MODEL` |
| xAI/Grok | `XAI_API_KEY` | `LLM_PROXY_LIVE_XAI_MODEL` |

Run every provider with an available key:

```shell
make test-live-providers LIVE_ENV_FILE=configs/.env
```

Run only selected providers. When `LLM_PROXY_LIVE_PROVIDERS` is set, every
listed provider must have its key:

```shell
LLM_PROXY_LIVE_PROVIDERS=openai,gemini \
  make test-live-providers LIVE_ENV_FILE=configs/.env
```

Run the complete current Qwen model matrix through the saved Singapore
workspace URL:

```shell
LLM_PROXY_LIVE_PROVIDERS=dashscope \
  LLM_PROXY_LIVE_ALL_MODELS=true \
  make test-live-providers LIVE_ENV_FILE=configs/.env
```

Run the complete MiniMax M2 model matrix:

```shell
LLM_PROXY_LIVE_PROVIDERS=minimax \
  LLM_PROXY_LIVE_ALL_MODELS=true \
  make test-live-providers LIVE_ENV_FILE=configs/.env
```

The live harness parses `LIVE_ENV_FILE` as dotenv data without executing it as
shell code. A paid run creates a disposable management database, encryption
key, signed local session, tenant, and client secret under its private temporary
directory. It submits each candidate once to
`PUT /api/management/tenants/:tenant_id/provider-connections/:provider`, requires the
safe `200` tenant-profile result, and only then sends that provider's smoke
request. Candidate payloads, session material, provider responses, and proxy
responses are never printed, and the temporary state is removed at exit.

The paid image matrix uses OpenAI, Anthropic, Gemini, Moonshot, and xAI by
default. It requires all five provider keys. Set `LLM_PROXY_LIVE_PROVIDERS` to
run a selected subset. The harness selects each image model from the validated
public provider catalog. It uses the catalog default when that model supports
image input. Otherwise, it requires one exact image model for that provider.
The key verification uses the selected image model.

Set `LLM_PROXY_LIVE_ALL_MODELS=true` with media mode to verify and call every
image route for each selected provider. This command runs the complete current
Moonshot image matrix:

```shell
LLM_PROXY_LIVE_PROVIDERS=moonshot \
  LLM_PROXY_LIVE_ALL_MODELS=true \
  make test-live-provider-media LIVE_ENV_FILE=configs/.env
```

Each image case creates the same deterministic 256 by 256 red PNG. The request
sends canonical padded base64 data through `POST /v2`.
The case requires HTTP `200`, an exact `RED` response, and a valid proxy request
identifier. The harness does not print the image, response body, or credentials.

Run the complete paid image matrix:

```shell
make test-live-provider-media LIVE_ENV_FILE=configs/.env
```

Run a selected paid image subset:

```shell
LLM_PROXY_LIVE_PROVIDERS=openai,gemini \
  make test-live-provider-media LIVE_ENV_FILE=configs/.env
```

The non-paid `--preflight` mode creates an isolated managed user, tenant, and
client key. It saves a generated OpenAI key through the management API. It then
reloads the encrypted provider connection and routes one prompt through a loopback
Responses server. The `--write-config` mode writes the managed-only service
configuration without building or starting the proxy. Inspect that config with
`./scripts/test_live_providers.sh --write-config
/tmp/llm-proxy-live.yml`. Unless `LLM_PROXY_LIVE_PORT` explicitly selects a
port, each harness run allocates a fresh loopback port. Cleanup removes only the
temporary proxy and provider children that it starts.
The preflight divides the OpenAI capacity allocation between its native origin and the loopback origin.
This division preserves the total origin allocation and the global capacity budget.
The production configuration does not change.

### Production Default-tenant live test

`make live-test` is a separate paid production check. It calls only
`https://llm-proxy-api.mprlab.com` and requires exactly one local credential:
`LLM_PROXY_DEFAULT_TENANT_KEY`, the Default tenant's generated client secret. It neither
loads a dotenv file nor reads, accepts, or sends a local upstream-provider key.
The saved provider credentials and per-provider default models remain entirely
on the production tenant.

Before any paid provider call, the command sends an intentionally incomplete
`POST /v2` request with `LLM_PROXY_DEFAULT_TENANT_KEY`. A valid client key must reach normal
request validation and return `400` with a proxy request ID. A missing,
rejected, or unreachable client key stops the command before the provider
matrix starts.

The command then sends canonical `POST /v2` requests with an explicit provider.
Its echo cases omit `model`, so they exercise each saved Default-tenant provider
model. It runs echo markers for OpenAI, Anthropic, Meta, Gemini, and Moonshot.
It sends the same deterministic request larger than 16 KiB through OpenAI,
Anthropic, Meta, and Gemini. The long Gemini case explicitly selects
`gemini-3.5-flash`. This selection proves the default background Interactions
path. The long request requires normalized output for every portfolio record before its
final marker and uses a 900-second request budget. OpenAI and Gemini 3.5 keep
the blocking caller request open while their resource adapters own background
polling. Anthropic and Meta use their canonical
synchronous completion paths, including shared output-continuation work when
needed. The test client never polls a provider or llm-proxy itself.
Each case verifies HTTP `200`, the echoed request budget, a validated proxy
request ID, and a completion marker. Its result line prints that request ID for
log correlation without printing the response body or tenant secret. A
transport failure with no response prints no request ID. The command runs all
nine cases before returning nonzero for any failed case.

Set the Default-tenant client secret and its exact tenant identifier before
you invoke the target:

```shell
export LLM_PROXY_DEFAULT_TENANT_KEY='...'
export LLM_PROXY_EXPECTED_TENANT_ID='managed-...'
make live-test
```

The target first calls authenticated `GET /v2/identity`. The server returns
the tenant identifier that it resolved from the client secret. The target
compares that value to `LLM_PROXY_EXPECTED_TENANT_ID`. It stops before any paid
provider request if authentication fails or the tenant does not match. It does
not store the client secret in a temporary file.

This target is intentionally outside `make ci`: it has a real production cost
and is expected to fail honestly for a disabled, rate-limited, or failing
provider.


## Implementation Plans

Current scoped implementation plans are tracked under `docs/implementation/`.

## MPR Integration Verification

For Marco Polo Research Lab integration workflows, use the Codex
`mpr-integration` skill when a change needs contract/profile/task-based
black-box verification against an MPR app or fixture. Keep app-specific
hostnames, cookie names, ports, OAuth callbacks, and environment literals in
the selected integration profile or deployment docs, not in this guide.
