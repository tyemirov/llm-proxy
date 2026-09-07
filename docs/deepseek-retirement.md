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

## Production inventory on September 6, 2026

The production database was inspected at `2026-09-07T06:30:22Z` with a read-only SQLite connection and one read transaction.
This timestamp is September 6 in America/Los_Angeles.
The live container on `tutosh` uses `/data/llm-proxy-management.sqlite` in volume `mprlab-nginx-gateway_llm-proxy-data`.
The inspection selected model identities and counts. It did not retrieve credential values or prompts.

| Item | Result |
| --- | --- |
| Latest database schema version | 13 |
| Tenant records | 2 |
| Provider profiles across all providers | 9 |
| DeepSeek profiles, including retired models | 0 |
| DeepSeek text or dictation defaults | 0 |
| Stored DeepSeek keys | 0 |
| Historical direct DeepSeek events | 3, all for `deepseek-v4-flash` |
| Historical SiliconFlow R1 events | 3, all for `deepseek-reasoner` |
| Total usage events in this snapshot | 3,287 |
| SQLite `quick_check` | `ok` |

No current production selection requires a DeepSeek reasoning decision.
The schema-14 selection migration has zero matching production profiles or defaults in this snapshot.
Historical usage remains subject to the preservation requirements above.
The active service can add records after the snapshot.
Repeat the inventory before activation if tenant settings change.

The inspected source commits were:

- Application: `406aac97925e3dffcaec5763a965ab9bdd08becf`.
- Gateway: `65508424bfe989d704f56a152b0c2a3d936f326c`.

The live container uses image `ghcr.io/tyemirov/llm-proxy@sha256:a06659999dc9c5e390009794d31217e0ca2e3991bbf1295b2f2cdbf464c1891e`.
The local deployment selection records version `v1.5.1` from application commit `2da5d87b6b536817bf8100391feb48a3d32208a7`.
Its release receipt digest is `sha256:ac5eb001298b3464a88acd7e8e399e8a29f8675360ff88f7f91239ce40c2d22d`.
Its publication receipt digest is `sha256:44946ee959450c0a5b40936795f4fdb2370485d54e26a45450868e1126c7e13b`.
This inventory did not audit deployment fences or establish release readiness.

At inventory time, live replacement-model acceptance required a DeepSeek key.
The process, all six application environment files, and the canonical gateway environment had no configured `DEEPSEEK_API_KEY`.
The inventory clears the production selection decision requirement for this snapshot.
Database backup verification and production activation remain operator steps after provider acceptance.

## Live qualification on September 7, 2026

The authorized `DEEPSEEK_API_KEY` in `configs/.env` passed the standard live-test loader.
This location is correct when the command selects `LIVE_ENV_FILE=configs/.env`.
The command above cleared the inherited key and used that file directly.

| Model | Key verification | Text requests |
| --- | --- | --- |
| `deepseek-v4-flash` | HTTP 200 | Omitted, `none`, `low`, `high`, and `max`: HTTP 200 each. |
| `deepseek-v4-pro` | HTTP 200 | Omitted, `none`, `low`, `high`, and `max`: HTTP 200 each. |

`make test-live-providers` exited successfully after all twelve checks.
Evidence: `/tmp/llm-proxy-i245-live-20260907.log`.
I245 development, production selection inventory, and live provider qualification are completed.
The test used a disposable local proxy and database.
Production backup verification, deployment, and migration acceptance remain separate operator steps.
