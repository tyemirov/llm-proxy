# Tenant connections

P001 defines the dashboard design. F063 implements that design.

## Ownership

An account owns tenants and named provider connections.
A connection contains one provider identity, its credentials, and its required settings.
Each tenant can assign one account connection or one hosted access grant per provider.
The same connection can serve multiple tenants within its account.
A tenant can also leave a provider unconfigured.

Tenant access keys, default routes, system prompts, and usage belong to the tenant.
Connection reuse does not combine tenant usage.
The dashboard calculates `Used by N tenants` from saved assignments.
This count is independent of recent requests.

## Dashboard

The dashboard presents tenants, connections, and models in that order.
The dashboard always keeps one tenant selected. It selects the `Default` tenant, or the first tenant when none is named `Default`.
The selected tenant also controls the usage view.
Account-wide usage remains available through the usage scope selector.
Usage summaries refresh automatically every 30 seconds while the user is authenticated.
Search filters the visible lists. Each column scrolls when its list exceeds the available space.

The authenticated dashboard has a maximum width of 560 px.
Node labels use 7 px text. Secondary text uses 6 px text.
Model rows have a minimum height of 18 px. Connection rows have a minimum height of 28 px.

Long names and controls can increase the row height.
The three columns and their connectors remain visible at a 320 px browser width.
Dialogs keep their existing field sizes. All configuration, usage, and payment actions remain available.

Usage status summaries label HTTP 402 as `Payment required` and HTTP 403 as `Forbidden`.
Failed request details retain these statuses for unfunded or suspended prepaid access.

The workspace header shows search and `Copy MCP URL` in that order.
The search field has a search icon on its right side. The copy action stays at the right of the header.
Connection detail actions stay together. The app footer supplies the Resources link without a duplicate card.

Tenant and provider system prompts use native disclosures. Each editor starts collapsed.
A tenant, connection, or model change removes the `open` attribute from both disclosures.
Keyboard and pointer actions show each editor. A failed save keeps expanded editors and their drafts available for retry.

1. Create a tenant with a descriptive name.
2. Select an existing connection, create a connection, or complete configuration later.
3. Select `Connect` on the required connection card.
4. Select a model from the connection's provider catalog.
5. Select the explicit default action to save the model for that tenant.

`Connected` and a solid teal line identify a saved assignment.
A dashed amber line identifies a model preview.
A star and a text label identify a saved default.
Model selection alone does not save a default.

The Task filter shows compact rectangular buttons for tasks in the selected connection inventory.
Each button shows one task icon. Hover over a button to read its task description.
The buttons sit in one row beside the Models title. The row scrolls when space is limited.

Text generates text. Vision interprets images and produces text. Images generates images. Editing changes images.

Transcribe converts audio to text. Speech generates audio. Video generates video. Video analysis interprets video when supported.

Specialized tasks include voice conversion, voice extraction, speaker identification, alignment, and subtitles.

The dashboard starts without a task selection and shows all models that the selected connection supports.
Each button toggles independently. A second activation removes its selection, including the final selection.
The available buttons depend on the complete connection inventory, independent of search text and selected tasks.
The model list shows only offerings that support every pressed task on the same offering.
An empty result keeps the pressed buttons for revision.

A connection change removes unavailable selections and preserves the remaining selections.
If no selection remains, the dashboard shows all models for the connection.
A connection or tenant change removes unsaved model previews.

The text, transcription, and speech save actions preserve the other saved defaults.
The selected model determines the default save action. A selected task determines the domain for models with multiple default domains.
Without a task selection, the first supported task determines the domain.
Vision uses the text default controls. Task filtering does not save a default.

Each model card shows an icon for every task that its provider offering supports.
Search and task filters do not change these icons.
Task icons use the following meanings:

| Task | Icon | Description |
| --- | --- | --- |
| Text | Text lines and a pen nib | Generate text |
| Vision | Eye | Understand images |
| Images | Picture frame and a sparkle | Generate images |
| Editing | Picture frame and a pencil | Edit images |
| Transcribe | Waveform and text lines | Transcribe audio |
| Speech | Speaker and sound waves | Generate speech |
| Video analysis | Film frame and a magnifying glass | Understand video |
| Video | Film frame and a sparkle | Generate video |

Hover text and keyboard focus show the task description. Accessible names use the same description.
Teal identifies selection and connection state.
Model details show input and output directions.
Select a model to read all its supported tasks and their required and optional inputs.
These details remain available by touch.

Vision inputs must include text and an image. Listen inputs must include text and audio.
Voice extraction inputs must include audio and a transcript.
The Text input filter in the public explorer includes voice extraction offerings.

An unattached connection has a `Not connected` label.
A connection without required credentials retains its assignments and shows `Credentials needed`.
The model list remains empty until its required credentials are configured.

Connection details show masked credentials and assigned tenants.
The connection header groups Edit connection, Detach, and Tenant details and API access.
Credential changes apply to each assigned tenant.
The edit form shows those tenants before the save action.
Updates use the current connection version to detect concurrent credential or assignment changes.
If the assigned tenants change, the user must open the edit form again before saving.
When creation succeeds but attachment fails, the dashboard shows the saved connection and a `Connect` action.
Connection creation requires one account-scoped `Idempotency-Key` header.
The dashboard retains this key while the creation form remains open.
A retry with the same key and request returns the original creation response.
A different request with that key returns a conflict.
Creation receipts remain after connection deletion, so a delayed retry cannot recreate the connection.
Read the connection resource for its current state.
The canonical [OpenAPI contract](openapi.yaml) defines each management operation and its request shape.

## Prepaid Provider Access

F065 adds prepaid provider access to the dashboard.
The Prepaid provider access section shows grants for the selected tenant and the customer's billing account.
Each grant shows its state and permitted models and operations.
An assigned grant remains visible after suspension or revocation.
The service does not expose platform credentials in customer responses.

F070 development acceptance passed. The tracked configuration omits `hosted`.
Production activation and actual processor qualification remain separate.

The section header puts Set up prepaid balance beside its title.
The account label replaces the creation action when the account exists. Descriptive text remains below the actions.

1. Select `Set up prepaid balance` to create the USD account.
2. Select `Use provider access` on an active grant.
3. Open tenant details. Select `API access` to get the tenant key.

Access, balance, payment history, and request history update every 30 seconds while visible.
They also update when the application receives focus or becomes visible.
The application has no Refresh action for these resources.
Updates stop when the component is removed.

An open dialog or a focused input delays updates until editing stops.
Updates preserve unsaved spending limits, routing selections, and the loaded history pages.

Charges load when requested and then update with request history.
A read failure shows an error and clears data that the application cannot verify.
The next successful update clears the read error.

This flow does not require customer provider credentials.
The assignment API requires `kind` and `resource_id`.
The permitted kinds are `account_connection` and `hosted_access_grant`.
The service rejects the obsolete `connection_id` request field.
`GET /api/management/tenants/{tenant_id}/connections` returns the saved selections.
The customer must detach an assignment before selecting another resource for that provider.
Provider profiles retain the tenant's model and prompt settings for either assignment kind.
See [hosted billing](hosted-billing.md) for pricing decisions, payment integration reuse, and the remaining work.

## Dictator speech connections

An account can connect its own Dictator server.
Select `Dictator Speech API` in the provider selector.
Enter the server address and bearer token. Select the required TLS setting.
Connection creation checks the token through voice discovery without a speech job.
The token remains encrypted in the backend credential store.
Assign the saved connection to the required tenant.
Select Transcribe to inspect `whisper-base`. Select Speech to inspect `qwen3-tts` and `silero-ru`.
Transcription and speech models use their own capability defaults.
Select Voice extraction to inspect voice extraction models.
Only synthesis models have a speech default save action.
Language, voice, sample rate, and output format remain operation request controls.
The default resource saves provider and model selections.

The defaults resource has separate provider and model pairs for text, transcription, and speech.
API requests use `transcription_provider`, `transcription_model`, `speech_provider`, and `speech_model` for the two audio domains.
The service rejects the obsolete `dictation_provider` and `dictation_model` fields.
A default update replaces the resource. The client includes the unchanged pairs to preserve other selections.
Each selected pair remains intact after a restart.

Operations, output assets, and voice identifiers belong to the tenant.
Execution and recovery require the accepted connection identity and version.
A connection change cannot redirect an accepted job to another server.

Assigned tenants read their retained operation counts through `GET /model/v1/provider-diagnostics/dictator`.
The response contains `scope: tenant` and counts for each operation state.
Counts exclude other tenants, including tenants with the same account connection.
The request does not call Dictator. Detachment removes access.
Terminal detail expiry removes its count. These counts are not lifetime usage or billing totals.

## Detach and delete

A detach operation removes only the selected tenant's assignment.
The connection remains available to its other tenants.
When a default depends on that assignment, the operation requires explicit confirmation to clear the default.
The assignment removal and default updates occur in one database transaction.
The transaction clears only defaults that use the removed connection.
The service rejects connection deletion while assignments remain.
The existing final-tenant deletion constraint remains in effect.
Tenant deletion also returns a conflict when hosted grant history exists, including revoked grants.
This restriction preserves the tenant identity and grant audit.

## Current database schema

The current schema is versionless.
Fresh databases create the current tables directly, without a migration-version table or intermediate provider-key tables.
Current account-connection databases validate their records on startup.
Historical version records do not control this validation and remain unchanged.
Production SQLite connections enforce foreign keys.
Hosted tables and assignment constraints are created together when no hosted schema exists.
Startup rejects partial hosted schemas and altered assignment constraints without repair.
Startup also rejects broken foreign keys, credential histories, grant audits, ownership relations, and hosted assignments.
Startup rejects predecessor credential and temporary transfer tables beside the current account connections.
It preserves the rejected records for operator action.

Current tenant records require separate transcription and speech columns.
B236 adds a bounded startup transfer for the previous capability defaults.
The transfer renames `default_dictation_provider` and `default_dictation_model` to their `default_transcription_*` columns through the GORM migration API.
It adds empty speech defaults and keeps the saved routes and tenant timestamps.
The same transaction validates the current records before commit.
An invalid route or transfer failure rolls back the schema and data changes.
Startup rejects incomplete or mixed column sets without data changes.
The current schema does not execute the transfer again.
Verify a disposable database copy before deployment, as specified in the [schema transition procedure](managed-schema-transition.md#bounded-transfer-procedure).

New product fields and tables extend the declared current schema.
Each change must define how existing records receive any required values.
A data transfer must have a bounded procedure and a completion receipt before its bridge is removed.

## Remaining data transfer

The predecessor transfer creates one account connection for each existing provider configuration.
It attaches each connection to the configuration's current tenant.
Equal credential values remain in separate connections.
The migration encrypts each secret with its new connection identity as authenticated data.
It preserves tenant access keys, default routes, system prompts, and usage records.
The transaction removes the predecessor credential table after the transfer.
A restart uses the current schema and preserves the migrated connection identifiers.
I271 retains this transfer until the production database has current account connections.
The [schema transition record](managed-schema-transition.md) contains the inventory, local ownership prerequisite, and completion requirements.

## Connection inventory

The connection list uses ascending connection ID order.
The `limit` query accepts 1 through 100 and defaults to 50.
Use `next_cursor` as the next request's `cursor` query value.
An empty `next_cursor` indicates the last page.
Deleting the cursor connection does not invalidate the next page.
The dashboard and provider qualification script read all pages.
Management responses use `Cache-Control: no-store`, including error responses.
Unexpected tenant store failures return `managed_tenant_store_persist_failed` without database details.
Connection failures return the documented connection error code.
Default-route validation and connection detachment use the same mutation lock.

## Acceptance

1. Run `make test-account-connections` for connection ownership, assignment, versionless startup, restart, and remaining transfer checks.
2. Run `make test-management-auth-blackbox BLACKBOX_TEST_ARGS='connection-dashboard.spec.js'` for the real browser flow.
3. Run `make test-hosted-access` for hosted account, grant, assignment, authority, and database checks.
4. Run `make test-management-auth-blackbox BLACKBOX_TEST_ARGS='hosted-access.spec.js'` for hosted browser setup.
5. Run the remaining repository checks required by the current validation policy.

The browser fixture uses local TAuth and the real LLM Proxy server.
Its controlled upstream responses prove the local interaction contract.
Live provider qualification and production deployment remain separate evidence.

## Tenant details

Detaching a connection preserves the tenant provider profile.
The tenant profile continues to show its saved provider prompt and model.
Connection fields remain with the account connection.

The dashboard uses the public capability catalog for model family logos.
The public catalog permits browser reads without credentials.
Tenant API access includes a copyable request example after a text default is saved.
Examples use the `<generated-secret>` placeholder.
The dashboard also provides the account MCP URL before provider setup.
Closing the access dialog clears the generated key and example from browser state.
The browser validates each tenant, connection, and API key response before use.
Failed saves preserve draft prompts and reasoning choices for retry.
Session loss cancels pending configuration and API key requests.

### Self-service management UI

The application document title is always `LLM Proxy`.

The mandatory management configuration exposes TAuth-protected management APIs
under `/api/management`. The browser site is static and lives in `site/`: `/` is the
anonymous-only product landing page and `/app/` is the only authenticated app
route. The landing page replaces itself with `/app/` whenever MPR UI reports
its documented authenticated lifecycle, including restored and refreshed TAuth
sessions. Every other public header owns the same declarative **Log In** action,
and successful interactive authentication redirects to `/app/`. An anonymous
direct visit to `/app/` returns to `/` after MPR UI reports its unauthenticated
lifecycle state.
There is no second signed-out application screen.
The site is declared as a `github_pages` resource in
`.mprlab/deploy/resources.yml`.
`make release`, `make publish`, and `make deploy` delegate that resource to the installed `mprlab-gateway` runtime.
Gateway owns Pages deployment.
The backend does not serve management HTML or assets.
`GET /`
remains a proxy endpoint and returns `403` without a tenant `key`. The backend
serves public `/config-ui.yaml` from the loaded management config and
`/api/public/capabilities` from the validated provider registry. The GitHub
Pages frontend consumes these REST resources from `llm-proxy-api`; neither
resource exposes provider credentials, tenant state, or management secrets.

The `/app/` UI uses the shared MPR shell through API-served `config-ui.yaml`,
literal `mpr-ui@latest` assets, `mpr-ui-config.js`,
`<mpr-header data-config-url="...">`, the `@latest` bundle marker, `<mpr-user>`,
and the shared compact `<mpr-footer>`. MPR UI owns browser authentication,
session restoration, refresh, logout, and all communication with TAuth;
application JavaScript does not load a second TAuth client, implement those
operations, or apply MPR UI config. The Pages artifact contains no static
`config-ui.yaml` or `llm-proxy-config.json`. The declared Pages build starts the
backend's public-capabilities-only REST surface, then the frontend-owned Node
renderer fetches `/api/public/capabilities`. The renderer writes
`https://llm-proxy-api.mprlab.com/config-ui.yaml` into every auth-aware header,
replaces the public landing's capability marker with the returned model-centric
catalog. It replaces the routing marker with a five-stage route from the
product through LLM Proxy, model family, exact model, and provider offering.
Every route item remains in semantic HTML without JavaScript. Browser
enhancement supplies a multi-choice weight-access filter and a single-choice
capability filter in the route title row. Both weight access groups and All capabilities are the default filters. Browser enhancement also supplies family, model, and provider
selection and the final route display. The capability catalog supplies
one all-characteristics search surface, disclosed match-all capability filters,
sortable table headers, a live result count, and reset. Node exists only in the
Pages build stage; the published artifact is static and has no runtime renderer
or environment-expansion path. A missing or invalid provider catalog, management config
attribute, or landing catalog marker fails the site build. That single
API-served YAML points browser management API
calls, generated usage examples, and MPR UI/TAuth at the configured origins.
Browser-facing values are projected from the already-loaded backend
`config.yml`.

Every public HTML route also uses the MPR component shell. `/`, `/docs/`,
`/resources/`, every generated resource article, `/privacy/`, and `/terms/`
render the same canonical `mpr-header` and compact sticky `mpr-footer` from
`scripts/public_site_shell.mjs`, in that exact order around one `main` region.
The authenticated app uses the same footer. The MPR footer keeps its host in
flow while its hydrated surface stays fixed, so the end of `main` remains
accessible. It contains crawlable Resources, Privacy, Terms, and GitHub links
plus the active **Built by Marco Polo Research Lab** project-catalog drop-up. A
semantic fallback inside the component keeps
those anchors and the native project drop-up usable when JavaScript is
unavailable; MPR UI replaces that fallback when the component hydrates. The
shell validator and every public-page generator reject a missing, duplicated,
misordered, or divergent header/main/footer contract, content outside `main`,
and any public anchor that bypasses authentication by linking directly to
`/app/`.

Release rendering also copies the exact committed `docs/openapi.yaml` bytes to
the Pages artifact root and verifies the SHA-256 provenance embedded in the
derived `site/docs/index.html`. `site/openapi.yaml` is intentionally forbidden:
there is no independently editable or generated schema copy in site source.

`scripts/generate_legal_pages.mjs` is the single maintained source for the
canonical `/privacy/` and `/terms/` pages. It owns their route metadata,
indexable sitemap entries, effective and updated dates, exact
`mpr-legal-document` section data, and semantic no-JavaScript fallback. The
current policy source dates both documents `2026-08-08`. Privacy statements are
grounded in the repository's MPR UI/TAuth session boundary, encrypted provider
credentials, digest-only client secrets, content-free usage records, and
Google Analytics plus LoopAware public-page telemetry. Terms describe the
current proxy/provider and user-responsibility boundaries without introducing
an unsupported payment or availability contract.

MPR UI owns the browser authentication presentation, lifecycle, and all browser
communication with TAuth. Public headers use MPR UI's documented sign-in label
and successful-authentication redirect attributes.
The app registers the documented `mpr-ui:auth:authenticated` and
`mpr-ui:auth:unauthenticated` lifecycle listeners, uses the header's documented
`data-mpr-auth-status` only to reconcile the current state after startup, and
does not request
`/api/management/account` until MPR UI reports `authenticated`. LLM Proxy does
not inspect TAuth cookies, storage, tokens, or claims and does not call TAuth
authentication endpoints. After MPR UI reports authentication, a management
API failure renders an explicit app error; it does not reinterpret the
MPR UI session as signed out.

The Go backend consumes TAuth's published `pkg/sessionvalidator` for the
configured session cookie. It does not maintain a second JWT parser or claims
schema; llm-proxy adds only its product-owned tenant, required-expiry, and
principal checks after TAuth validation. Authentication rejections are logged
only as stable categories such as `missing_cookie`, `expired`,
`invalid_issuer`, or `wrong_tenant`; session values and identity claims are
never logged.

After the shared `mpr-ui` shell reports authentication, the frontend loads the
account through `GET /api/management/account`. A new TAuth subject receives one
`Default` tenant. Each account may create, rename, select, and delete its own
tenants; deleting the final tenant returns `409 Conflict`. Every owned tenant is
operational at the same time: each generated secret independently selects that
tenant's credentials, defaults, and usage owner. The browser has no global
active-tenant state, activation flag, or tenant URL parameter.

The dashboard presents tenants, connections, and models in three columns.
Select **Create tenant** to add a tenant.
The next step can use an existing connection, create a connection, or defer configuration.
Select **Connect** on a connection card to attach it to the selected tenant.
Creating or attaching a connection leaves tenant defaults unset until the user saves them.
See [tenant connections](tenant-connections.md) for the resource and interaction contracts.

Connection forms use the provider catalog for credential fields, settings, and key acquisition links.
Saving a new connection verifies its credentials through the provider's catalog default text model.
Changing connection fields repeats verification before persistence.
The verifier uses a fixed probe and the selected route's declared lifecycle.
Synchronous routes make one provider request.
Pollable Gemini routes create, observe, and delete a stored interaction before accepting the credential.
Verification uses the shared worker, queue, rate limits, and request timeout.
It records no tenant usage.

A failed verification preserves the saved connection and its assignments.
The form retains the submitted draft for correction or retry.
Successful responses contain masked credentials.
Connection edits require the current version and show all affected tenants.
Assignment changes invalidate an earlier edit version.
A connection must have no tenant assignments before deletion.

Tenant details provide rename, deletion, and API access controls.
API-key creation requires an explicit action.
Replacement requires confirmation because the previous key stops working immediately.
The new value appears once, with a copy control.
Request examples use the `<generated-secret>` placeholder.
A client key cannot be deleted independently.
Deleting a non-final tenant without hosted grant history removes its access key and usage while preserving account connections.
Hosted grant history prevents tenant deletion, including after revocation.

The [persistence boundary](persistence-boundary.md) owns database-specific connections, constraints, and transaction coordination.

SQLite stores account connections, tenant assignments, provider profiles, default routes, secret digests, and usage.
Runtime authentication loads the tenant and its assigned connections from one database snapshot.
Provider secret fields use AES-GCM encryption bound to the connection identity, provider, and field.
Management responses keep these credentials masked.
Tenant API keys are returned once, and the database retains their SHA-256 digests.
Fresh databases create the current tables directly, without a schema number or intermediate migration tables.
Current account-connection databases validate their records without a version query or a repeated transfer.
The bounded connection transfer remains necessary for the retained production database.
The transfer preserves tenant assignments, access keys, defaults, prompts, timestamps, and usage.
See the [schema transition record](managed-schema-transition.md) for I271 inventory evidence and the remaining transfer requirements.

The packaged configuration requires every `LLM_PROXY_MANAGEMENT_*` placeholder to have a value.
Local `make up` projects these values from `configs/.env.local` into `configs/.env.api.local`.
Both files are ignored.
Runtime configuration files remain unchanged by tenant creation, connection edits, and usage recording.

Tenant defaults have independent text, transcription, and speech provider/model pairs.
The pairs can remain empty after a connection is attached.
Selecting a model previews the route.
The default action for the selected capability persists the selection.

Text defaults can include a route-supported reasoning effort and the tenant system prompt.
Provider prompts belong to each tenant's provider profile.
Editing a connection preserves these tenant choices.
Detaching a connection requires confirmation before dependent defaults are cleared atomically.
The provider profile remains available for later attachment.

`PUT /api/management/tenants/:tenant_id/defaults` accepts complete, eligible provider/model pairs.
The selected tenant must have the provider's connection.
A nonempty reasoning effort must belong to the exact model's declared list.
Invalid pairs or efforts return `400` before persistence.
Public requests can override the model and reasoning effort under the same route validation rules.
The backend rejects malformed persisted profiles instead of repairing them during reads.

Authenticated users land on the tenant dashboard and usage overview.
Selecting a tenant in the map also selects its usage.
**Account usage** shows totals across all owned tenants.
The usage selector can narrow the charts independently.
The interval defaults to `30 days` and retains the selected usage scope during automatic refresh.
Usage summaries refresh automatically every 30 seconds while the user is authenticated.
Execution metrics include succeeded and failed requests.
The separate `rejected_requests` count identifies requests rejected before provider dispatch. The
success-rate metric renders an **N failed requests** action only when the selected
snapshot contains failures. It opens a keyboard- and focus-managed dialog with
the current failed-execution status breakdown and newest-first safe failure metadata.
The dialog retains the active interval, paginates within one opaque snapshot,
and discards any response made stale by an interval or Usage tenant change. An
account-wide failure row includes the owning tenant's safe ID and current
display name; a tenant-scoped row retains the tenant-less safe shape. A
details error stays inside the dialog and never replaces aggregate dashboard
data. The
`Settings` menu item is inserted before `Sign out` through the shared
`<mpr-user>` menu contract.

The Requests card renders an **N rejected requests** action when the selected
snapshot contains rejections. It opens the same safe, scope-bound detail
surface without a failure status breakdown. Rejected rows remain visible for
diagnosis but do not enter totals, buckets, provider or model breakdowns,
status groups, token counts, latency, or success rate.

Provider usage and Model usage each have one icon-only chart toggle. Each card
starts with a `Bar graph` that ranks exact request counts against the largest
row. Its toggle changes only that card to a `Donut chart`. The donut presents
the same ordered request counts as shares with a count-and-percentage legend.
Rounded shares total 100 percent, and every category remains separate. Each
selection survives interval, automatic usage refresh, and Usage tenant changes. Authentication
reset or page reload resets both cards to bars. A toggle makes no request.
Requests and Tokens remain separate line charts. Their
visible X axes use the summary's UTC hour or date buckets. Their zero-based,
integer Y axes identify requests or tokens per hour or day. Exact bucket starts
and values remain available to assistive technology.

Usage charts show the existing request aggregates.
Connection configuration remains in the tenant, connection, and model map above the charts.
Connection cards show provider identities and assignment counts.
Model cards use their canonical model family logos.
The map distinguishes saved assignments, a selected connection, and an unsaved model preview.

Administrators are configured only through `management.admin_emails`; use the
plural `${LLM_PROXY_MANAGEMENT_ADMIN_EMAILS}` placeholder in public config files
and define the real value as a YAML flow sequence in the runtime environment or
ignored `configs/.env`. When the validated TAuth
session email matches that list, the account response includes
`user.is_admin: true`, the shared avatar menu gets an `Admin` item, and
`GET /api/management/admin/users` returns all managed users with tenant facts
and 30-day usage summaries. Admin responses never include provider API keys,
masked provider-key strings, generated tenant secrets, secret digests, prompts,
audio names, transcripts, or model responses. Authenticated non-admin users get
`403 Forbidden` from admin-only APIs.

`GET /api/management/usage?interval=all|30d|7d|1d` returns one summary across
every tenant owned by the authenticated TAuth subject.
`GET /api/management/tenants/:tenant_id/usage?interval=all|30d|7d|1d` returns
the same summary shape for one explicitly selected owned tenant. These are
distinct canonical scopes; neither is an alias or browser-side fan-out.
`interval` is required exactly once; a missing, repeated, or unknown value
returns `400`. Both responses carry `Cache-Control: no-store` and contain
the selected `interval`, its `bucket_unit`, `totals`, ordered generic `buckets`,
and provider, model, and status-code breakdowns; the user endpoint has no
`period_days` or `daily` fields. `1d` uses 24 hourly buckets, `7d` and `30d` use
7 and 30 daily buckets, and each finite interval is an exact trailing duration
ending at one captured server timestamp. `all` includes retained tenant events
through that timestamp in UTC daily buckets from the earliest event through
today, or an empty bucket list when the selected scope has no events. Account
totals and average latency are calculated from the complete owned event set,
not from per-tenant summaries. The administrator
endpoint remains a separate fixed 30-day daily contract.

```text
GET /api/management/usage?interval=30d
GET /api/management/tenants/:tenant_id/usage?interval=30d
```

`GET /api/management/usage/failures?interval=all|30d|7d|1d`
is the account-wide failure operation. It uses one stable newest-first snapshot
across all owned tenants and adds only `tenant_id` and `tenant_name` to each safe
row. Its cursor is bound to the account-wide scope.
`GET /api/management/tenants/:tenant_id/usage/failures?interval=all|30d|7d|1d`
is the corresponding operation for one explicitly selected tenant; its rows do
not repeat tenant identity. Both operations require exactly one `interval`,
accept one optional `limit` from 1 through 100 (default 25) and one optional
opaque `cursor`, and reject missing, repeated, malformed, or unknown query
fields with `400`. Missing and foreign tenant ids both return `404`. Pages are
newest first under a stable `(created_at, id)` position and an opaque snapshot
boundary; a cursor from one tenant or account-wide scope is rejected in every
other scope. Safe failure metadata is limited to `occurred_at`, `endpoint`,
`provider`, `model`, `status_code`, `outcome_code`, and `latency_ms`, plus the
account-wide tenant fields described above. Neither operation returns row or
user ids; prompts; responses; audio; transcripts; client secrets; provider
keys; raw upstream bodies; or free-form errors. The administrator surface
remains aggregate-only and cannot fetch another owner's rows.

`GET /api/management/usage/rejections?interval=all|30d|7d|1d`
and
`GET /api/management/tenants/:tenant_id/usage/rejections?interval=all|30d|7d|1d`
provide the corresponding account-wide and tenant-scoped rejection reports.
They use the same interval, limit, cursor, ordering, scope-binding, ownership,
cache, and safe-field contracts as the failure operations. Their outcome codes
are exactly `invalid_request`, `payload_too_large`, and
`provider_not_configured`. Provider and model are present only when the proxy
resolved an exact typed route before rejection.

Usage events are recorded only for managed tenants when they call the public
proxy endpoints with a generated secret. Account-wide usage queries apply the
authenticated owner and all owned tenant ids at the database boundary;
tenant-scoped queries additionally require the explicit tenant id. Every query
uses one captured time boundary. Because proxy responses enqueue usage
asynchronously, a summary or failure query can temporarily omit accepted work
that has not committed yet. Stored usage
metadata includes endpoint, provider, model, status code, request disposition,
one canonical outcome code, latency, and normalized request/response/total
token counts. Each event has exactly one `rejected`, `succeeded`, or `failed`
disposition. Rejected outcomes are `invalid_request`, `payload_too_large`, and
`provider_not_configured`. The succeeded outcome is `success`. Failed outcomes
are `rate_limited`, `service_unavailable`, `request_timeout`, `upstream_error`,
and `proxy_error`. They are selected at the request/error boundary; prompts,
audio, transcripts, responses, tenant secrets, provider API keys, raw upstream
bodies, and free-form error text are not stored in usage events.

After tenant authentication, an invalid request-timeout header produces an
`invalid_request` rejection for text, V2, and dictation. Unauthenticated
requests and asset operations do not create proxy usage events. A V2
asset-store read error produces a `proxy_error` failure. Invalid asset input
and a missing asset remain rejections.

The runtime never imports config tenants or global provider keys.
TAuth subjects own personal tenants directly. The contract has no shared
tenant, membership, role, invitation, or team tenancy.

Server settings and browser-facing MPR UI/TAuth bootstrap settings remain in
`config.yml`. Provider definitions and static endpoints remain in
`providers.yml`. Each managed Alibaba Cloud workspace URL belongs to an account connection and is
stored as a provider connection value. The selected model and system prompt use
a provider profile record. The GitHub Pages artifact is only the static shell.
API-served browser config
endpoints are projections of backend `config.yml`, not independent
configuration sources.
