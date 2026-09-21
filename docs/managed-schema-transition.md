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
