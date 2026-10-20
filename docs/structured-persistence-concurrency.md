# Structured Persistence Concurrency

## Observed Delay

NameSignal requested 32 complete trademark evidence assessments through LLM Proxy v1.12.2.
All assessments passed the protocol checks. The cohort took 58.139 seconds.
The median provider progress interval remained close to the earlier matched samples.

| Observed median interval | Earlier calls | Concurrent cohort |
| --- | ---: | ---: |
| Request start to provider creation | 1.8385 seconds | 24.128 seconds |
| Provider creation to observed completion | 11.1665 seconds | 11.6975 seconds |
| Observed completion to final summary | 0.4735 seconds | 9.5855 seconds |

The first upstream admission event occurred after a median 23.446 seconds in the concurrent cohort.
These boundaries locate the additional delay before upstream admission and after provider completion.
They do not measure individual filesystem operations or model computation.
The deployed source is `8247092fc36547f17456bf2b002aab0a62906ed6`.
The later source at `a049a40b49b437dfc9e27162444dfdff3f400dbe` retains the same persistence paths.

## Correction Scope

I311 replaces the shared request mutex with an identity lock for each tenant/key pair.
The lock registry counts holders and waiters before they wait for the identity lock.
The registry removes each entry after its last holder or waiter leaves.
Registry operations do not hold locks during filesystem work or identity-lock waits.

All online record operations use the same identity lock.
The lifecycle gate retains exclusive whole-store recovery before normal service use.
Hosted settlement retains its database transaction order before the lifecycle gate and identity lock.
The correction does not remove that database serialization.

Record encoding, temporary writes, file synchronization, atomic rename, and directory synchronization remain unchanged.
The proxy publishes validated success before it returns a successful response.
Duplicate dispatch protection, conflicts, tenant isolation, expiration, and uncertain outcomes retain their current behavior.
The HTTP schema, saved record schema, and log event contract remain unchanged.

## Validation Status

Six public HTTP regressions failed before the correction.
The tests block a record write in `not_dispatched`, `dispatched`, or `succeeded`.
Each state tests a different key or a different tenant with the same key.
An unrelated request must complete and reconcile before the test releases the blocked write.
The tests use the real router, managed tenants, a local provider, and real record persistence.

The focused public race suite passed in 18.036 seconds.
It includes duplicate requests during blocked publication, conflicting intent, tenant isolation, restart, write failures, and hosted settlement.
The registry tests verify waiter handoff, concurrent reuse, entry retirement, and recovery exclusion.
Independent review cleared the frozen source and tests. Repository CI passed all 15 gates in 801 seconds.
Go statement coverage is 100%. All 172 frontend cases and 11 TAuth browser cases passed.
At this initial validation, live load qualification and the complete NameSignal search remained pending.
This source correction does not establish a one-minute search guarantee or provider activation.

The subsequent [capacity qualification](openai-admission-capacity-evidence.json) passed all 32 live local requests in 19.604 seconds.
Final CI passed all 15 gates in 697 seconds, with 100% Go statement coverage and 13 TAuth browser cases.
The complete NameSignal search target remains unverified. Production activation remains separate.

## Local Comparison

The diagnostic used 128 existing records, an instant HTTP provider, and the real local persistence adapter.
The race detector was active. Each cohort has one sample per implementation.
An exact production source overlay supplied the baseline without a checkout change.

| Cohort measurement | Baseline | Correction |
| --- | ---: | ---: |
| One request, complete cohort | 25.930 milliseconds | 26.947 milliseconds |
| 32 requests, complete cohort | 818.706 milliseconds | 474.058 milliseconds |
| 32 requests, median request | 680.257 milliseconds | 398.391 milliseconds |

These samples show a local improvement under the stated conditions.
They do not reproduce deployed storage or establish a latency distribution.
The [evidence record](structured-persistence-concurrency-evidence.json) preserves source hashes, commands, and results.
