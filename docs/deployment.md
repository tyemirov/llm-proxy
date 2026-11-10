# Deployment and release

Run lifecycle commands from the repository root.
For local execution, use [local setup](local-setup.md).

## Split-origin setup

Production is split-origin:

| Hostname | Owner | Purpose |
|----------|-------|---------|
| `llm-proxy.mprlab.com` | GitHub Pages | Public landing at `/`; noindex self-service app at `/app/`. |
| `llm-proxy-api.mprlab.com` | MPR gateway/backend | llm-proxy API, management API, `/`, `/v2`, and `/dictate`. |
| `tauth-api.mprlab.com` | TAuth backend | Google login, nonce, logout, `/auth/session`, and session-cookie issuance. |

Add these DNS records:

1. `CNAME llm-proxy.mprlab.com -> tyemirov.github.io`
2. Point `llm-proxy-api.mprlab.com` at the MPR gateway public endpoint. Use a
   `CNAME` when the gateway has a hostname, or `A`/`AAAA` records when it is
   addressed by public IP.

Then configure GitHub Pages for this repository:

1. Use branch publishing from `gh-pages` at `/`.
2. Set the Pages custom domain to `llm-proxy.mprlab.com`.
3. Supply the private inputs declared by the selected manifest before lifecycle execution.
   See [Production lifecycle](#production-lifecycle) for the input names.
   The service declaration supplies `management.database_path` as `/data/llm-proxy-management.sqlite`.
4. Use `make deploy` for the complete lifecycle through the installed `mprlab-gateway` runtime.
   Gateway renders the declared Pages container and publishes its immutable artifact.
   It activates that artifact on `gh-pages` and verifies the matching Pages build and `/.mprlab-release.json` marker.
5. Do not store browser runtime config in the Pages branch. Production browser
   config is served only by `https://llm-proxy-api.mprlab.com/config-ui.yaml`
   from the running backend's loaded management config. The declared Pages
   container writes that canonical URL into `mpr-header[data-config-url]` and
   validates the declarative mpr-ui bundle marker.

Configure TAuth for tenant `llm-proxy` with:

- allowed tenant origin `https://llm-proxy.mprlab.com`
- browser-facing API origin `https://tauth-api.mprlab.com`
- session cookie name matching `management.session_cookie_name`
- cookie domain `.mprlab.com`
- HTTPS-only cookies
- JWT signing key matching `management.jwt_signing_key`

TAuth owns tenant configuration and session issuance.
LLM Proxy declares its TAuth URL, tenant ID, session paths, and signing input in the runtime environment.
Gateway reconciles the declared runtime and Pages resources.
It requires the declared terminal HTTP observations before deployment success.

The same boundary is executable locally without Google OAuth or deployed
services:

```bash
make test-management-auth-blackbox
```

The target builds the TAuth version pinned in `go.mod` and the current
llm-proxy binary, starts both on disposable local ports, and opens the real
static management app in Playwright. A Google Identity test adapter activates
the visible MPR UI control and maps its provider exchange to TAuth's seeded
password fixture via the same-origin frontend proxy. TAuth issues the configured
HttpOnly access and refresh cookies, and MPR UI drives the authenticated
lifecycle and documented redirect without an application-owned auth script or
manual auth event. The test proves the
public **Log In** control is owned by MPR UI, proves an anonymous direct
`/app/` visit returns to `/`, and proves the anonymous/authorized behavior of
`/api/management/account`. The browser makes no protected account or tenant
request before MPR UI authentication, restores the real TAuth session on
`/app/`, then loads the selected dashboard tenant and its usage. It
creates two tenants for one real TAuth subject, proves both secrets remain
independently routable, proves the default account-wide usage and safe
tenant-attributed failure page include both, and signs in a second real subject
to prove foreign tenant ids return `404` without disclosure. It waits for the
`mpr-ui@latest` shell plus the
dashboard to report the authenticated state, then proves an ordinary reload
stays authenticated, removes only the access cookie and proves `/auth/session`
recovers it from the refresh cookie without returning to the public page, and
uses the visible **Sign out** action to prove `/auth/logout` clears both cookies
and returns TAuth plus the management API to anonymous responses. The MPR UI
application assets are loaded through their literal `@latest` CDN contract;
MPR UI's shell and lifecycle, TAuth's session/refresh/logout routes, and the
management API routes are exercised as real boundaries. Only the external
Google provider exchange is replaced by the local seeded-credential adapter.

Normal navigation, page refreshes, and access-cookie expiration do not sign the
user out. MPR UI silently restores the TAuth session while its rotating refresh
cookie remains valid and reports the resulting lifecycle. Only the explicit
**Sign out** action asks MPR UI to clear the browser session; LLM Proxy does not
own a second session store or an automatic logout path.

Configure the gateway/backend route for `llm-proxy-api.mprlab.com` to the
llm-proxy service, and remove any backend route that still claims
`llm-proxy.mprlab.com`; that hostname is now owned by GitHub Pages. The backend
must run with `management.public_origin: "https://llm-proxy.mprlab.com"` so
`/config-ui.yaml` and `/api/management/*` return credentialed CORS headers only
to the static frontend.


## Production lifecycle

Run the complete production lifecycle with:

```shell
make deploy
```

Use `make release` to stop after preparation or `make publish` to stop after publication.

Each target resolves the application Git root and calls the installed `mprlab-gateway` command with `--app-root`.
Gateway v9 uses the commands `release`, `publish`, and `deploy` for these targets.
Use `MPRLAB_GATEWAY_EXECUTABLE` to select an explicit installed command.
The Gateway package supplies its own assets and dependencies.
Gateway development source is not an operational input.

Use the [Gateway installation guide](https://github.com/MarcoPoloResearchLab/mprlab-gateway/blob/master/docs/runtime-installation.md) for first installation and explicit upgrades.
Use `mprlab-gateway version --json` to inspect the selected runtime.
Gateway operator inputs use `MPRLAB_GATEWAY_OPERATOR_ROOT`, with `$HOME/.config/mprlab-gateway` as the default.

The complete deployment command prepares and publishes the selected release when those phases are incomplete.
Valid sealed preparation is reused without another CI run or build.
Publication and activation consume that exact release.
Retries reuse completed state and reject conflicting immutable state.
The selected app transaction reads no unrelated app repository.

This repository owns one production declaration:
`.mprlab/deploy/resources.yml`. It declares the container image and service,
retained data volume, HTTP endpoints, Caddy route, public health observations,
GitHub Pages publication, and private inputs.
Runtime readiness uses the existing `http` endpoint at `llm-proxy:8080/healthz`.
It requires HTTP 200 through the same alias that public API routing uses.
The API route keeps the resource ID `llm-proxy-public-api` for reconciliation of the existing shared configuration.
The route declares its Caddy deployment policy locally. Application-wide execution budgets remain independent.

The manifest selects the ignored `.mprlab/deploy/.env` file through `environment.source`.
This field is optional.
When the manifest has no `environment.source` field, Gateway uses only process values.
If the process and selected file supply the same input, Gateway uses the process value.
If Gateway cannot read the selected file, it rejects the manifest even when process values are available.

Gateway reads the manifest inputs one time at the manifest boundary.
It validates the configuration before CI, builds, host contact, or provider effects.
Gateway gives the validated configuration to all subsequent lifecycle operations.
A missing necessary input stops the lifecycle operation.
Gateway generates one service environment from the validated configuration.

The manifest has these five consumed private inputs:

- `LLM_PROXY_MANAGEMENT_ADMIN_EMAILS`
- `LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID`
- `LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY`
- `LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY`
- `DEPLOY_SUDO_PASSWORD_TUTOSH`

Gateway resolves the Pages origin and declared endpoint references.
The runtime environment explicitly supplies the TAuth client configuration.

The gateway owns schema validation, resource-output resolution, artifact
sealing and publication, Ansible reconciliation, inventory, and production
verification. Application private values do not use gateway inventory secret
maps. There is no application-owned production Ansible, Compose, Caddy,
release, publish, or deploy implementation.

The runtime retains the explicit cleanup declaration for `mprlab-nginx-gateway/llm-proxy`.
Gateway selects that cleanup through the exact Compose project and service labels.
The current runtime uses the separate project `mprlab-llm-proxy-runtime`.
The named data volume `mprlab-nginx-gateway_llm-proxy-data` retains the application data.

The API route keeps the resource ID `llm-proxy-public-api`.
Its local deployment policy matches the current shared Caddy configuration policy.
See the [complete manifest audit](gateway-manifest-audit.md) for declaration coverage and current runtime evidence.

The Pages declaration uses `docker/pages/Dockerfile`. A compiled Go backend
serves the secret-free `/api/public/capabilities` resource during the build.
The Node frontend renderer fetches that resource. It renders model families,
exact models, provider offerings, and route filters in the route explorer. It
keeps publishers in the capability matrix. It also renders
request limits and injects the browser configuration URL into
every generated auth-aware HTML page. The final Pages image contains only the
static artifact. Application runtime code has no Caddy deployment knowledge, and
its TAuth knowledge remains limited to the published client/session
integration.

## Releasing

Use `make deploy` from the selected application checkout for the complete lifecycle.
This command uses the installed Gateway runtime.
This repository owns its permanent major version `1` release policy.
Gateway selects the version from current remote release markers under that policy.
It uses the selected version for release artifacts, tags, and receipts.
An exact release retry reuses the stored decision.

The gateway runs the committed `scripts/validate-release-decision` program.
The validator checks the SemVer policy and major version `1`.
The validator accepts only `mprlab.version-decision/v3` from the current Gateway runtime.
Gateway selects each `v1.X.X` release version. Python package metadata consumes the resulting release tag.
The package build does not select or change a release version.

The [`setuptools-scm` configuration](https://setuptools-scm.readthedocs.io/en/stable/config/)
reads major-version-1 tags from the repository root.
Source distributions keep the selected version for later builds without Git metadata.
Untagged development commits produce development versions, not release versions.

CI fetches the complete Git history and tags before Python builds.
uv includes Git commits and tags in package cache keys.
A release does not require a separate version update command or a stored version declaration.

The generic gateway transaction validates this app, builds the declared
multi-platform container and frontend-rendered Pages artifact from committed source,
and seals the canonical SemVer release. Publication creates only missing remote
state from that exact release and rejects conflicts. Deployment uses the
gateway-owned Ansible inventory and transaction to reconcile only this app's
declared resources, then verifies its backend and Pages boundaries.
An unchanged retry reuses completed preparation and publication.
Missing preparation runs the CI and builds declared in the manifest.

The gateway does not own this application's version policy, version history,
artifact names, migration, cutover, or cleanup sequence. Keep each such rule in
this repository.
