# TypeSafe Operational Acceptance

## Record: 2026-11-11

Status: Pending live qualification.
Owner: LLM Proxy maintainer.
Related development: F074 is closed on completed implementation and deterministic CI evidence.

The final development run passed all 15 CI gates with 100.0 percent Go statement coverage and zero uncovered blocks.
The operational check establishes live TypeSafe execution through the implemented proxy endpoint.
This record tracks operational acceptance separately from development completion.

## Required inputs

- Confirm the TypeSafe account and organization for the live run.
- Supply the provider credential through its approved private input.
- Select the proxy runtime and tenant connection for the check.
- Obtain authorization for the live provider requests.

At development completion, the documented `TYPESAFE_API_KEY` environment input was absent.
This historical input check does not establish the current credential state.

## Pending acceptance

1. Verify the selected provider credential with authenticated `GET https://api.typesafe.ai/v1/models`.
2. Select an available exact model and record its identifier.
3. Run authorized live requests through `POST /v1/systemone` with the selected tenant connection.
4. Examine typed Choice, Score, Noul, and Box results against the TypeSafe contract.
5. Examine model identity, answer identifiers, usage, request identifiers, status codes, and provider errors.
6. Record the exact model, endpoint, usage, and sanitized request and response evidence.
7. Keep credentials and request content outside public evidence.
8. Record the result and execution date in this operational record.

Use the [integration guide](typesafe-system-one.md) for setup and wire behavior.
Qualify each future upstream provider separately before operational activation.

## Live evidence

| Field | Current evidence |
| --- | --- |
| Execution date | Pending |
| Account and organization | Pending confirmation |
| Proxy runtime and source revision | Pending selection |
| Provider endpoint and exact model | Pending execution |
| HTTP results and request identifiers | Pending execution |
| Usage | Pending execution |
| Sanitized request and response evidence | Pending execution |
| Acceptance result | Pending |
