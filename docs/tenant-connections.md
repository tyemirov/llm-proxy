# Tenant connections

P001 defines the dashboard design. F063 implements that design.

## Ownership

An account owns tenants and named provider connections.
A connection contains one provider identity, its credentials, and its required settings.
Each tenant can assign one connection per provider.
The same connection can serve multiple tenants within its account.
A tenant can also leave a provider unconfigured.

Tenant access keys, default routes, system prompts, and usage belong to the tenant.
Connection reuse does not combine tenant usage.
The dashboard calculates `Used by N tenants` from saved assignments.
This count is independent of recent requests.

## Dashboard

The dashboard presents tenants, connections, and models in that order.
The selected tenant also controls the usage view.
Account usage remains available through an explicit selection.
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

## Detach and delete

A detach operation removes only the selected tenant's assignment.
The connection remains available to its other tenants.
When a default depends on that assignment, the operation requires explicit confirmation to clear the default.
The assignment removal and default updates occur in one database transaction.
The service rejects connection deletion while assignments remain.
The existing final-tenant deletion constraint remains in effect.

## Data migration

Schema version 17 creates one account connection for each existing provider configuration.
It attaches each connection to the configuration's current tenant.
Equal credential values remain in separate connections.
The migration encrypts each secret with its new connection identity as authenticated data.
It preserves tenant access keys, default routes, system prompts, and usage records.
The transaction removes the predecessor credential table after the transfer.
A restart uses the current schema and preserves the migrated connection identifiers.

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

1. Run `make test-account-connections` for connection ownership, assignment, and migration checks.
2. Run `make test-management-auth-blackbox BLACKBOX_TEST_ARGS='connection-dashboard.spec.js'` for the real browser flow.
3. Run the remaining repository checks required by the current validation policy.

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
