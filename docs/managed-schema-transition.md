# Managed schema transition

I271 owns removal of completed managed-database transfers.
The user selected a versionless current schema on September 13, 2026.
Historical schema numbers describe predecessor inputs only.
They do not control fresh creation or current account-connection validation.

## Read-only inventory

The inspection occurred on September 13, 2026, Pacific time.
The queries returned table names, column names, historical version records, and record counts.
They did not return credentials, prompts, access keys, or account identifiers.
No retained database changed.

| Database | Observed shape | Retained records | Remaining prerequisite |
| --- | --- | --- | --- |
| Production, host `tutosh`, volume `mprlab-nginx-gateway_llm-proxy-data`, file `llm-proxy-management.sqlite` | Tenant connection fields and provider profiles. Historical version records are 8 through 16. Current account-connection tables are absent. | One account, three tenants, ten connection-field records, ten profiles, and 4,684 usage records. | Complete the account-connection transfer and record its acceptance. |
| Local Compose, volume `llm-proxy-local_llm_proxy_local_data`, file `llm-proxy-management.sqlite` | Current account connections, tenant assignments, and media tables. Historical version records are 2 through 17. | One account, two tenants, two connections, two fields, two profiles, two assignments, and no usage records. | No predecessor connection transfer remains. Preserve current records and validate restart. |
| Local, ignored `configs/llm-proxy-management.sqlite` | Tenant `user_id`, provider-key records, and usage `success`. No version table. | One tenant with an unclaimed static owner, two provider-key records, one usage record, and one routing-transfer record. | Establish its retained owner through the F011 ownership prerequisite, or obtain an explicit disposal decision. |

The production volume contains one database file.
The inventory covers that volume, the local Compose data volume, and the ignored database in this checkout.
It does not establish the state of operator backups or undeclared database copies.

The production inspection used the installed Gateway host inventory and its existing private deployment input.
It used SSH and read-only SQLite connections through the host's existing privileged command path.
The application container was `mprlab-llm-proxy-runtime-llm-proxy-1`.
Its data mount resolved to `/volume1/@docker/volumes/mprlab-nginx-gateway_llm-proxy-data/_data`.
The local Compose inspection used an existing Python image with a read-only volume mount and no network.
The temporary inspection container was removed after the query.

## Source dependencies

The public router opens the managed store through `newGORMManagedTenantDatabase`.
That function calls `initializeManagedTenantSchema`.
Empty databases now create current records directly.
Databases with account connections enter `validateAccountConnectionSchema` without a version query after any required capability transfer.
The predecessor branch remains necessary for the inventoried data.

B236 corrects the capability transfer omitted from the transcription and speech change.
The exact input has `owner_user_id`, both `default_dictation_*` columns, and none of the four current transcription and speech columns.
The GORM migration API renames the dictation columns and adds empty speech defaults within the startup transaction.
The transfer keeps the other records and tenant timestamps unchanged.
Current validation occurs before commit, including the account-connection transfer when required.
Failure rolls back all changes in that transaction.
Mixed or incomplete capability columns remain invalid.
The current schema does not execute the transfer again.
Apply the bounded procedure below to each retained database before deployment.
Keep the B236 transfer until all retained inputs have completion receipts under I271.

| Source family | Remaining caller and input | Removal condition |
| --- | --- | --- |
| `migrateManagedCapabilityDefaults` | The startup transaction reads the exact capability predecessor before account-connection initialization or validation. | Record successful transfers for all retained capability predecessors under I271. |
| `initializeManagedTenantSchemaRecords` and its historical version switch | The predecessor branch of `initializeManagedTenantSchema`. Both inventoried databases still have predecessor shapes. | Complete each retained transfer. |
| `migrateAccountConnections`, `migrateAccountConnectionRecords`, and `managedProviderConnectionRecord` | The production tenant-connection transfer. It reads fields and profiles, encrypts credentials for new connection identities, and writes assignments. | Verify the production transfer and resolve the local input. |
| `migrateLegacyManagedTenantSchema`, its preflight and verification functions, and `legacyManaged*Record` types | The local database still has tenant `user_id` and predecessor usage fields. | Resolve F011 ownership, complete the retained transfer, or record authorized disposal. |
| `migrateManagedUsageOutcomeSchema`, `migrateManagedResolvedUsageRoutes`, and `migrateManagedUsageDispositionSchema` | Historical usage conversion during predecessor initialization. | Complete each retained usage transfer. Keep current disposition validation and stored usage identities. |
| `migrateManagedKeyedRoutingDefaults` and `migrateManagedQwenCloudRetirement` | Predecessor default-route conversion and provider retirement. The local input has older defaults and provider-key records. | Complete local preflight and any required transfer. |
| `migrateManagedModelIdentity` and the `migrateManaged*ModelSelection*` functions | Model conversion selected by the predecessor version switch or local preflight. | Complete retained model transfers. Preserve historical usage identities. |
| `migrateManagedXAIProvider`, `migrateManagedDashScopeSettings`, and `migrateManagedZAIProvider` | Predecessor provider identity and settings conversion, with their preflight and verification functions. | Complete retained provider transfers. |
| `migrateManagedProviderConnections` and `migrateManagedProviderConnectionData` | Conversion from provider-key rows to tenant fields and profiles before account connections. | Complete the local predecessor transfer or record authorized disposal. |
| `managedProviderAPIKeyRecord`, predecessor datasets, projection helpers, and associated cipher helpers | The transfer families above still use these types and helpers. | Remove them with their last required caller. |
| `managedProviderSettingsFromRecordsForSchema`, `managedProviderBaseURL`, `validDashScopeWorkspaceID`, and the DashScope URL constants | Predecessor settings validation still uses the Singapore-only rule. Account connections use catalog field validation. | Remove the rule with its last required predecessor caller. |
| Historical version, temporary table, and index constants | Predecessor dispatch, input validation, table conversion, and transfer verification. | Complete retained transfers. Keep literals needed to reject obsolete shapes. |

No data-transfer family is declared complete by this inventory.
Fresh creation no longer enters these families or creates their intermediate tables.
The historical fresh-initialization branch in `initializeManagedTenantSchemaRecords` was removed.
Its only remaining creation dependencies were test fixtures, which now declare their predecessor tables explicitly.
Nonempty databases without a recognized tenant table fail before schema changes.
The tenant's predecessor connection association is excluded from current schema creation.
It remains available to the required transfer queries.

## Catalog dependencies

`ProviderCatalogSchema.ModelMigrations` feeds `providerRegistry.modelMigrations`.
`validateProviderCatalogModelMigrations` checks each declaration at the catalog boundary.
Predecessor transfers consume the selected source, target, operation, and reasoning setting.

`managedUsageRouteIsCurrent` accepts historical usage identities marked with `preserve_source_usage` during predecessor startup.
Its callers are `clearInvalidManagedUsageRoutes` and `validateManagedResolvedUsageRoutes` within the predecessor transfer paths.
Current account-connection validation does not call this helper.
Current usage summaries group stored `ProviderID` and `ModelID` values directly in `managedUsageAccumulator.apply`.
They do not require model-migration declarations.
After the retained transfers, remove the declarations and their exclusive catalog validation with the transfer code.
Preserve the stored usage identities and verify them through the public Usage API.
Keep canonical provider URL validation in the provider catalog.

## Bounded transfer procedure

The application operator owns the production transfer and its completion receipt.
The production input is the exact retained volume and file identified above.
The local input requires an ownership or disposal decision first.
I271 implementation does not authorize an application deployment or disposal of retained data.

1. Resolve the local retention and ownership decision.
2. Identify every retained backup that must remain usable after bridge removal.
3. Select the exact application artifact that contains the required transfer code.
4. Stop application writers before the production transfer.
5. Create an operator-owned SQLite backup with all committed records.
6. Apply the selected artifact to a disposable backup with the matching provider-key encryption key and canonical provider catalog.
7. Verify credentials through decryption without disclosure of their values.
8. Compare account ownership, tenant identifiers, access-key digests, defaults, prompts, timestamps, and usage with the input.
9. Verify one account connection and original-tenant assignment for each predecessor provider profile.
10. Verify separate connection identities for equal credentials from separate configurations.
11. Verify that failure leaves the input database unchanged.
12. After separate deployment authorization, run the same bounded transfer against the retained production database.
13. Verify current startup and public account, tenant, routing, and usage behavior.
14. Record the input digest, artifact identity, before-and-after counts, preserved-value comparisons, and public acceptance results.
15. Remove the completed transfer code after all retained inputs have completion receipts.

The existing startup transaction performs the predecessor transfer.
The procedure introduces no second importer or compatibility interface.
Production still requires this transaction before source removal.
No production transfer or deployment occurred during this investigation.

I244 independently owns the MediaOps operation-import bridge.

## Historical predecessor transfers

These procedures describe predecessor database shapes. Fresh databases create the current schema directly.
Current account-connection databases validate their records without a schema-version query.
Retained predecessor inputs still require the bounded transfer procedure above.
The historical version numbers below do not define the current schema.

Management startup requires every persisted routing field to be canonical and
catalog-valid and every nonempty provider default to have the tenant's saved
key. It never infers or repairs a provider, model, or reasoning effort at read
time. The bounded schema-version-3 migration performs the one-time
reconciliation of older managed defaults against saved provider keys. The
bounded schema-version-4 migration then removes retired `qwencloud` provider
settings and reconciles affected text defaults to the first remaining keyed
provider. The schema-version-5 migration converts affected provider-native
model values to canonical exact model identifiers. These migrations preserve
tenant timestamps and verify their result before recording the version.
Invalid keys, models, or routing data stop startup with
the owner, tenant, endpoint, provider, and model context.


F014 upgrades the previous one-tenant-per-user database to schema version 1
as one bounded startup transaction:

1. Drain every old llm-proxy instance and take an operator-owned database
   backup. Never run the old and new binaries against the same database during
   this migration.
2. Exercise the exact SQLite migration on a disposable database through the
   repository `make ci` gate.
3. Start one new instance. Preflight reads all legacy tenant, provider-key, and
   usage rows before opening the mutation transaction. It rejects missing
   tables, unclaimed `static-config:` owners, blank or duplicate owners and
   tenant ids, duplicate or malformed secret digests, orphan provider or
   usage rows, plaintext or corrupt provider keys, and non-canonical routing
   data.
4. The transaction renames the two colliding legacy GORM indexes and the three
   legacy tables, creates explicit user and tenant tables, preserves every
   opaque tenant id, moves secret digests and routing data, rebinds encrypted
   provider keys from the prior user id to the tenant id, copies usage rows,
   verifies counts and values including decryption, writes schema version 1,
   and removes the bounded legacy tables.
5. Verify account, tenant, provider, secret, routing, and usage behavior
   before adding capacity. A failed stage rolls the transaction back to the
   untouched legacy schema and prevents startup. Correct the source data or
   restore the backup; do not hand-edit a partially migrated shape.

The subsequent bounded schema-version-2 migration preflights every schema-1
usage row, maps successful rows to `success`, and maps historical `400`, `413`,
`429`, `499`, `502`, `503`, and `504` statuses to their exact canonical failure
codes. Caller cancellation `499` and proxy-budget expiry `504` both become
`request_timeout`. It rejects any other historical failure status before
mutation, then writes the non-null outcome field and the
tenant/success/time/id failure-page index in one transaction. Historical
diagnostics therefore contain normalized, status-derived codes, never
reconstructed raw error messages.

The bounded schema-version-3 migration preflights every managed provider key
and canonical routing pair. For each tenant it preserves currently eligible
defaults, otherwise selects the first keyed text provider and first keyed
dictation-capable provider by canonical provider id, and clears the corresponding
pair when no provider is eligible. It writes the reconciled defaults without
changing tenant timestamps, verifies every row against its decrypted saved
keys, and records schema version 3 in the same transaction. Reopening a
version-3 database validates this invariant and rejects drift instead of
repairing it at read time.

The bounded schema-version-4 migration removes every stored `qwencloud` key,
selected model, and provider system prompt. Affected text defaults move to the
first remaining keyed provider by canonical identifier. The default uses that
provider's stored text model. If no key remains, the migration clears the text
route and reasoning effort. Tenant timestamps and
historical usage provider/model identifiers remain unchanged. The transaction
verifies deleted settings, reconciled defaults, decrypted remaining keys,
timestamps, and usage rows before recording version 4. Current-version startup
rejects retired provider settings or routing defaults instead of repairing them.

The bounded schema-version-5 migration converts stored provider-native model
values to canonical exact model identifiers. It updates affected provider
settings and tenant defaults in one transaction. It preserves tenant timestamps
and all historical usage records. Current-version startup rejects an invalid
or dangling canonical route.

The bounded schema-version-6 migration replaces the retired `grok` provider
identity with `xai`. The bounded schema-version-7 migration adds the provider
base URL field to managed provider settings. Existing Alibaba Cloud records do not
contain a workspace URL. The migration removes those incomplete records and
reconciles affected text defaults. It preserves tenant timestamps and
historical usage. The owner must then save the complete Alibaba Cloud key and URL
pair. The bounded schema-version-8 migration replaces the retired `zhipu`
provider identity with `zai`. It preflights all provider settings and defaults,
decrypts each affected key with `zhipu` associated data, and re-encrypts it with
  `zai` associated data. The transaction updates keys and current routing
defaults, preserves tenant timestamps and historical usage, and rejects
conflicting, corrupt, or noncanonical stored values before mutation.

The bounded schema-version-9 migration replaces each provider-key row with
provider connection records and one provider profile record. It maps every
predecessor value through the current provider definition. It re-encrypts the
credential with the provider field identity as associated data. The transaction
verifies every value and timestamp before it removes the predecessor table.
Before current-catalog validation, startup atomically replaces each predecessor
model that a schema-version-10 catalog record identifies. It also replaces the
matching tenant default.
Current-schema reads use only provider connection records and provider profile
records.

The bounded schema-version-10 migration replaces each stored Gemini 2.5 text
model and same-provider tenant default with `gemini-3.5-flash`. It updates the
provider profile and current default in one transaction. It preserves tenant
and profile timestamps and all historical usage model identifiers. Current
startup rejects retired Gemini routes instead of accepting an alias or fallback.

The bounded schema-version-11 migration replaces stored
`gemini-3.1-flash-lite` and `gemini-3.1-pro-preview` selections with
`gemini-3.5-flash`. It updates the provider profile and same-provider tenant
default in one transaction. It preserves provider connections, system prompts,
timestamps, and historical usage model identifiers.

The bounded schema-version-13 migration replaces the historical success flag
with the required request disposition. It maps successful events to
`succeeded`. It maps input and payload-size outcomes to `rejected`. It maps
attempted provider or proxy outcomes to `failed`. A historical `service_unavailable`
event without a resolved route becomes `provider_not_configured`. A historical
server-side `invalid_request` becomes `proxy_error`. The transaction preserves
safe event metadata, copies records in fixed batches of 256, validates each
mapped row, records version 13, and removes the predecessor table. Current
startup validates each distinct disposition and outcome pair. It rejects the
obsolete success column, an invalid pair, a missing disposition index, or a
retained predecessor table. A report returns a store error if usage data
changes to an invalid pair after startup.
