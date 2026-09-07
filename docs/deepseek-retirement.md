# DeepSeek Model Retirement

## Current contract

The direct DeepSeek provider exposes `deepseek-v4-flash` and `deepseek-v4-pro`.
Both models accept `none`, `low`, `high`, and `max` as reasoning efforts.
`none` disables thinking. The other values enable thinking and set the matching effort.
An omitted effort enables thinking with the provider default of `high`.

DeepSeek announced the retirement of `deepseek-chat` and `deepseek-reasoner` for July 24, 2026.
The [provider change log](https://api-docs.deepseek.com/updates/) identifies their replacement as V4 Flash with different thinking modes.
The [thinking guide](https://api-docs.deepseek.com/guides/thinking_mode/) defines the current controls.
These sources were checked on September 5, 2026.

The proxy rejects both retired direct routes before provider dispatch.
The separate SiliconFlow `deepseek-reasoner` route still uses `deepseek-ai/DeepSeek-R1`.

## Stored selections

Schema version 14 applies these exact changes:

| Provider | Source selection | Target selection | Tenant default effort |
|---|---|---|---|
| `deepseek` | `deepseek-chat` | `deepseek-v4-flash` | `none` |
| `deepseek` | `deepseek-reasoner` | `deepseek-v4-flash` | `high` |

The transaction changes matching provider profiles and tenant defaults.
It preserves credentials, prompts, unrelated settings, timestamps, and historical usage records.
Private migration records permit the retired identities in historical usage only.
They do not create selectable routes.
Repeated startup does not repeat the selection changes.

Provider profiles inherit the tenant default effort.
A profile can require a different effort from the tenant default after migration.
For that state, startup reports `provider_reasoning_decision_required` with the tenant and source model.
The transaction rolls back all changes.
The operator must decide the required profile behavior before activation.
A separate provider effort setting requires an explicit product contract.

## Operator procedure

1. List affected profiles and defaults in the selected management database.

```sql
SELECT provider_id, text_model, COUNT(*) AS profiles
FROM managed_provider_profile_records
WHERE provider_id = 'deepseek'
  AND text_model IN ('deepseek-chat', 'deepseek-reasoner')
GROUP BY provider_id, text_model;

SELECT tenant_id, default_provider, default_model, default_reasoning_effort
FROM managed_tenant_records
WHERE default_provider = 'deepseek';
```

2. Record the required behavior for each affected profile and tenant default.
3. Supply `DEEPSEEK_API_KEY` in the authorized environment file for live acceptance.
4. Run the replacement model and reasoning matrix against a disposable local proxy.

```bash
env -u DEEPSEEK_API_KEY \
  LIVE_ENV_FILE=configs/.env \
  LLM_PROXY_LIVE_PROVIDERS=deepseek \
  LLM_PROXY_LIVE_ALL_MODELS=true \
  LLM_PROXY_LIVE_REASONING_MATRIX=true \
  make test-live-providers
```

5. Verify the selected tenant database backup before production activation.
6. Start the approved release through the operator-owned deployment procedure.
7. If startup reports a profile conflict, resolve the stated behavior decision before another activation attempt.
8. Verify the migrated defaults and profile selections through Settings.
9. Verify public model discovery and one request with each affected tenant default.
10. Compare historical usage identities and counts with the recorded database state.

## Acceptance evidence

Repository tests exercise real HTTP handlers and SQLite startup.
They cover explicit reasoning controls, retired-route rejection, default dispatch, repeated startup, rollback, and SiliconFlow isolation.
Browser tests verify current model discovery and reasoning selection.

Live acceptance requires a provider credential.
On September 5, 2026, `DEEPSEEK_API_KEY` was absent from the process and the repository private environment files.
