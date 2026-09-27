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

## Hosted Access

F065 adds hosted access to the dashboard.
The Hosted access section shows grants for the selected tenant and the customer's billing account.
Each grant shows its state and permitted models and operations.
An assigned grant remains visible after suspension or revocation.
The service does not expose platform credentials in customer responses.
Hosted execution remains disabled until the shared F070 acceptance requirements pass.

1. Select `Create billing account` to create the USD account.
2. Select `Refresh hosted access` after an operator provisions a grant.
3. Select `Use hosted access` on an active grant.
4. Select `API access` to create the tenant key.

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
