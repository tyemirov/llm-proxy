# Confident Programming

This core policy controls all agent work. Product and platform requirements apply through the conditional guides in root `AGENTS.md`.

## Repository Policy Sections

Governor generates this core policy and the product and deployment guides from shared templates and optional `.mprlab/POLICY.REPOSITORY.md` sections.
A local section replaces the shared section with the same level-2 heading in its owning guide.
Unknown local sections extend this core policy. The repository source requires unique headings and the title `# Repository Policy Sections`.

- Put repository-specific requirements in that source and run the normalizer after a source change.
- Read this core policy for every task. Read the product and deployment guides only for the conditions listed in root `AGENTS.md`.
- Keep deployment specification and execution policy in the selected application manifest.

## Temporary Task Files

- Put temporary diagnostic files in `_scratch/<task-id>/`.
- Use a unique task identifier for each directory.
- Keep `/_scratch/` in the repository `.gitignore`.
- Record the directory and its task in the task plan before use.
- Keep files from an interrupted task until that task resumes or its owner authorizes removal.
- Before task completion, preserve useful results in maintained tests, tools, or documentation.
- Remove only the temporary directory owned by the completed task.
- Verify that this directory is absent before reporting task completion.
- Preserve directories owned by other tasks.
- If removal fails, report the remaining path and keep task completion pending.

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

## Test-Driven Development

- Use integration tests as the primary layer for public behavior and component contracts.
- For a bug fix, first reproduce the failure through the relevant public entry point and confirm its cause.
- Prefer a failing regression test before the fix. When another reproducible check gives better evidence, record the reason and result.
- For a behavior change, add or update coverage that exercises the changed public contract. Choose the smallest meaningful check.
- For a refactor, run relevant existing coverage first. Add characterization coverage where changed behavior is otherwise unprotected.
- Use focused unit tests for algorithms, calculations, and isolated logic when they provide useful evidence.
- Documentation, static configuration, and other low-impact changes can use document, schema, build, or structural checks appropriate to their effect.
- Record a justified exception when the default test sequence adds no useful evidence. A required product acceptance check remains binding.
- Use deterministic local infrastructure for repository-owned databases, filesystems, queues, servers, and browsers.
- At an external provider boundary, use a provider sandbox or a local protocol implementation for routine integration tests.
- Keep live-provider acceptance as a separate qualification step.

## Dependency Injection In Integration Tests

- Use dependency injection for integration scenarios that are difficult to reproduce.
- Inject the dependency that creates the test condition.
- Use controlled clocks, dependency failures, or unusual responses when the scenario requires them.
- Keep the product logic under test and its related internal interactions real.
- Assert observable results through the public contract.
- Keep integration coverage with real dependency implementations.
- Injected scenarios prove behavior under the specified conditions. They do not prove actual provider connectivity.
- Qualify actual provider connectivity separately from injected scenarios.

## Prohibited Patterns

- Silent fallbacks, best-effort behavior, legacy aliases, and compatibility reads unless an explicit product requirement says the behavior is current.
- Duplicated validation inside core modules.
- Exporting invalid zero-values as usable domain objects.
- Swallowing errors.
- Increasing waits or timeouts as the primary fix for flakiness.
- Boolean parameters that switch unrelated behaviors.
- Hardcoded workflow, path, event, or message literals when a canonical constant or backend payload exists.

## Device Validation

- Never require a physical device or physical-device access for any task or gate, under any circumstances.
- Apply this prohibition to development, tests, validation, acceptance, issue closure, release, publication, and deployment.
- Never ask the user to provide, obtain, connect, or arrange access to physical devices or physical-device test services.
- Use simulators, emulators, and automated browsers for device validation.
- Accept these environments as sufficient for device validation and acceptance.
- Remove physical-device requirements and blockers from the selected task and its current acceptance records.
- Never replace a removed physical-device gate with another hardware prerequisite.
- Report the actual test environment and observed results without claiming physical-device execution.

## File Permission Boundary

- File permission modes are outside agent scope.
- Never examine, validate, compare, require, change, or record a file permission mode.
- Never use a file permission mode in acceptance, security, credential, execution, publication, deployment, or failure analysis.
- The values `0600` and `7777` have no governance meaning.
- This rule does not change service authorization or operation authority.

## Credential Discovery

Use this gate when credentials are necessary for the selected task.

- Examine only credential sources specified in the active command documentation and only the specified variable names.
- Use tracked example and sample environment files as documentation only.
- Do not show, record, or copy secret values during diagnosis.
- Use each provider's non-mutating authentication command when available.
- Request replacement credentials only after verification rejects the selected input. Report the exact missing or incorrect input.
- For Gateway operations, also read the credential and selected-input rules in `.mprlab/AGENTS.DEPLOYMENT.md`.
- Preserve the user's account boundaries and global sign-in preparation rules.

## Issue Closure

- Close an issue when its required implementation, documentation, and code validation are completed.
- Keep release, publication, deployment, production DNS/TLS, and live acceptance outside implementation issue acceptance criteria.
- Do not keep an implementation issue open or blocked because production operations are not completed.
- Record pending production operations in a runbook or separate operational record.
- Validate deployment tooling and infrastructure configuration through the required isolated integration tests.

Failed or missing required code validation prevents closure.
Production operations outside the task do not remove code validation requirements.

## Validation

Full CI is the final validation gate before CI-dependent delivery or task completion.
Finish all other authorized task work and all non-CI results and documentation first.
This includes implementation, configuration, required account setup, focused tests,
review fixes, live acceptance, benchmarks, and their analysis when applicable.
Use focused checks while any work or required input remains.

- Use repository-native `make` targets.
- Do not run a pre-edit or per-issue `make ci` baseline.
- Preserve reproduction results and any expected failing regression test as implementation evidence.
- During the change, run the smallest public-entrypoint target that validates the changed contract.
- Run full CI once at the final checkpoint, after absolutely all other authorized task work is finished.
- Use successful full CI from the pull request when its workflow supplies all required checks.
- Open the pull request at the final checkpoint, after all other authorized task work and focused local checks are complete. Let its workflow supply the final full CI result.
- Wait for that result before merge or completion.
- When no pull request supplies full CI, run `make ci` locally only after all other authorized task work is complete.
- Use focused checks to correct failures.
- If final CI fails, make corrections with focused checks. Repeat full CI only after all corrections and other remaining work are complete.
- Reuse successful CI results while the validated code remains unchanged.
- Use equivalent CI results across local validation, pull requests, and release.
- Keep separate release checks for additional artifact, signing, or platform coverage.
- Record the selected CI command, its result, and any required checks outside its coverage.
- Run `make verify` only when the operator explicitly selects the complete qualification lane.
- When `make ci` includes `make fmt`, `make lint`, and `make test`, use its result for those targets.
- During the change or error diagnosis, run the necessary component target.
- Run a component target when `make ci` does not include the necessary check.
- For documentation-only work, run the applicable document and repository checks.
- For `.mprlab/`-only work, run the Governor check and `git diff --check`.
- These are the repository checks for `.mprlab/`-only work. Changed prose also requires the documentation-language review.
- For read-only work, use source facts and run only the necessary checks.
- For frontend behavior, verify through a browser test when the behavior is user-visible.
- For services and CLIs, verify through HTTP, CLI, or public API entry points.

## Documentation Language

- Use concise, consistent American English for technical prose.
- Preserve facts, product decisions, requirements, conditions, source literals, and ownership boundaries.
- Read `.mprlab/AGENTS.DOCS.md` and repository terminology before technical prose changes.
- Use formal ASD-STE100 compliance only when the user or an applicable document contract explicitly requires it.
- For that selected scope, read `.mprlab/AGENTS.STE.md` and complete its reference and review gates.
- Report the scope and limits of each review. Mechanical checks alone do not prove factual accuracy or formal language compliance.

## JavaScript Modules

Apply these language rules to JavaScript modules, including Node services and command-line tools.

- Put `// @ts-check` at the top of new or edited JavaScript modules when the repo uses checked JS.
- Use JSDoc typedefs for domain objects and payload contracts.

## Self-Check

Before claiming completion:

- External inputs are validated once at the edge.
- Core modules consume validated domain values.
- Error paths include operation and subject context.
- Reusable literals are centralized.
- Public behavior is covered through public entry points.
- Repo-native validation was run or a concrete blocker is documented.
