# Claude Opus 4.1 Retirement

The direct Anthropic catalog excludes `claude-opus-4-1` and `claude-opus-4-1-20250805`.
New requests and management selections reject these IDs before provider dispatch.
Anthropic marks Opus 4.1 as retired from the direct Claude API in its [pricing documentation](https://platform.claude.com/docs/en/about-claude/pricing).
The authenticated Models API response on September 5, 2026, omitted both IDs.

## Stored selections

Schema version 15 applies this bounded migration:

| Provider | Source model | Target model | Tenant default effort |
|---|---|---|---|
| `anthropic` | `claude-opus-4-1` | `claude-opus-5` | `high` |
| `anthropic` | `claude-opus-4-1-20250805` | `claude-opus-5` | `high` |

The transaction updates matching provider profiles and tenant defaults.
It preserves credentials, prompts, timestamps, unrelated settings, and historical usage identities.
Historical Opus 4.1 usage remains available with its original model ID.
Repeated startup does not repeat the migration.
The Anthropic provider default remains `claude-sonnet-4-6`.

Opus 5 uses adaptive thinking and supports explicit effort controls.
The migration selects its default `high` effort.
Provider profiles inherit the tenant default effort.
An affected profile with a conflicting inherited effort causes `provider_reasoning_decision_required` and transaction rollback.
The operator must resolve that profile decision before activation.

The repository inventory found both IDs in the catalog, a routing test, and README model rows.
These active references are removed.
Private migration records and retirement tests retain the source IDs to identify persisted data.

## Operator procedure

1. Query the selected management database for affected profiles and defaults.

```sql
SELECT tenant_id, provider_id, text_model
FROM managed_provider_profile_records
WHERE provider_id = 'anthropic'
  AND text_model IN ('claude-opus-4-1', 'claude-opus-4-1-20250805');

SELECT tenant_id, default_provider, default_model, default_reasoning_effort
FROM managed_tenant_records
WHERE default_provider = 'anthropic';

SELECT model_id, COUNT(*) AS events, SUM(total_tokens) AS tokens
FROM managed_usage_event_records
WHERE provider_id = 'anthropic'
  AND model_id IN ('claude-opus-4-1', 'claude-opus-4-1-20250805')
GROUP BY model_id;
```

2. Record each affected profile's inherited effort and required behavior.
3. Replace retired IDs in external clients and operational scripts with `claude-opus-5`.
4. Verify the selected database backup before activation.
5. Use the operator-owned deployment procedure to start the approved release.
6. If startup reports a profile conflict, resolve the stated decision before another activation attempt.
7. Verify that Settings shows Opus 5 for each migrated selection.
8. Verify that public discovery excludes both retired IDs.
9. Run one request with each affected tenant default.
10. Compare historical usage counts, tokens, and model IDs with the recorded state.

A failed migration rolls back its database changes and prevents startup.
Restore the verified database backup with its matching release when an operator rollback is required.

## Acceptance evidence

F046 qualified Opus 5 through live key verification, text, all five efforts, and image requests.
See [current Claude models](claude-current-models.md) for the provider qualification receipts and current limits.

`make test-claude-retirement` exercises real HTTP handlers and SQLite startup.
The tests verify migration, repeated startup, transaction failures, inherited effort conflicts, older database upgrades, and preserved historical records.
They also verify retired request rejection, management selection rejection, and current discovery.
Repository acceptance is separate from production database migration and deployment.
