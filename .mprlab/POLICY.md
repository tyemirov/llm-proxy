# Confident Programming

This policy controls all agent work in this repository.

## Operator Rules

- Validate only at edges: I/O, HTTP, CLI, DB adapters, browser bootstrap, imported files, and other external boundaries.
- Design HTTP APIs as resource-oriented REST APIs. Use standard HTTP methods, status codes, and semantics.
- For gRPC APIs, obey protobuf service and RPC conventions. REST constraints do not apply.
- Make illegal states unrepresentable with domain types, smart constructors, dataclasses, enums, or closed action objects.
- Fail fast on impossible states.
- Wrap boundary errors with operation and subject context.
- After boundary validation, do not repeat validation in core modules.
- Keep interfaces narrow. Prefer domain types instead of loose strings, maps, booleans, or `any` values.
- Centralize reusable literals: paths, operation names, event names, config keys, status values, and shared messages.
- Tests target public contracts and invariants, not defensive branches.
- Prefer black-box integration and end-to-end tests through real entry points.

## Prohibited Patterns

- Silent fallbacks, best-effort behavior, legacy aliases, and compatibility reads unless an explicit product requirement says the behavior is current.
- Duplicated validation inside core modules.
- Exporting invalid zero-values as usable domain objects.
- Swallowing errors.
- Increasing waits or timeouts as the primary fix for flakiness.
- Boolean parameters that switch unrelated behaviors.
- Hardcoded workflow, path, event, or message literals when a canonical constant or backend payload exists.
- Unit tests as a substitute for public contract coverage.

## Validation

- Use repository-native `make` targets when available.
- Use a satisfactory CI result only when the source code, tests, config, dependencies, and build files stay the same.
- If there is no applicable satisfactory result, run `make ci` once before you change files.
- During the change, run the smallest repository target that validates the changed contract.
- After the last source, test, config, dependency, or build change, run `make ci` once.
- If this run reports an error, run the target that reports the error during the correction.
- After the last correction, run `make ci` once.
- When `make ci` includes `make fmt`, `make lint`, and `make test`, use its result for those targets.
- During the change or error diagnosis, run the necessary component target.
- Run a component target when `make ci` does not include the necessary check.
- For documentation-only work, run the applicable document and repository checks.
- For `.mprlab/`-only work, run the Governor check and `git diff --check`.
- These checks are the full validation for `.mprlab/`-only work.
- For read-only work, use source facts and run only the necessary checks.
- For frontend behavior, verify through a browser test when the behavior is user-visible.
- For services and CLIs, verify through HTTP, CLI, or public API entry points.

## Documentation Language

- Write new or changed English technical prose in ASD-STE100 Simplified Technical English, Issue 9.
- Read `.mprlab/AGENTS.DOCS.md` and `.mprlab/TERMINOLOGY.md` before you write technical prose.
- Apply this rule to PRDs, architecture documents, issues, plans, policies, ADRs, READMEs, runbooks, and API documents.
- Do not change technical meaning to make the language simpler.
- Run the skill `prepare-ste-reference` script to retrieve and verify the official Issue 9 PDF.
- Run the skill `check-ste` script on each technical document that you change.
- The producing agent must review Part 1 writing rules and the Part 2 dictionary.
- Do not assign the reference retrieval or language review to the end user.
- If the official reference is not available, report a blocker and do not claim compliance.

## Language Rules

### Go

- Use smart constructors returning `(Type, error)` when a type has invariants.
- Do not export invalid zero-values.
- Wrap errors with `%w`.
- Prefer integration tests through real HTTP, CLI, or package entry points.
- `make lint` must include `go vet`, `staticcheck`, and `ineffassign` when those tools are part of the repo contract.

### Python

- Use `@dataclass(frozen=True)` or Pydantic when already in use.
- Validate in constructors or edge adapters.
- Use type hints throughout.
- Prefer pytest scenarios through public entry points.
- Unit tests are allowed only as narrow guardrails for pure deterministic helpers.

### JavaScript And Frontend

- Put `// @ts-check` at the top of new or edited JavaScript modules when the repo uses checked JS.
- Use JSDoc typedefs for domain objects and payload contracts.
- Components render validated state and emit intent.
- Backend clients own request construction and response validation.
- User-visible behavior belongs in browser or integration coverage.

## Self-Check

Before claiming completion:

- External inputs are validated once at the edge.
- Core modules consume validated domain values.
- Error paths include operation and subject context.
- Reusable literals are centralized.
- Public behavior is covered through public entry points.
- Repo-native validation was run or a concrete blocker is documented.
