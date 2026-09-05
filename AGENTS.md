# AGENTS.md

## Forward-Only Contract Discipline

This repository follows a forward-only, confident programming paradigm. This is a binding agent contract: no fallbacks, no backward compatibility, no legacy support, and no compatibility shims. Do not spend design or implementation effort on backward compatibility considerations except for explicit one-off data migrations into the current canonical contract.

Repeat for emphasis because this rule is binding: no fallbacks, no backward compatibility, no legacy compatibility. Delete or reject obsolete code paths, stale schemas, deprecated config, and old persisted shapes instead of preserving them through compatibility layers, dual reads/writes, aliases, or best-effort recovery.

One-off data migrations are allowed only when they move existing persisted data into the current schema in a bounded operation. After migration, remove the bridge and keep only the current contract.

## llm-proxy

llm-proxy repository managed through `.mprlab/ISSUES.md` workflow. See README.md for details

## Document Roles

- AGENTS.md: Read-only workflow + behavior playbook maintained by leads. Agents never edit it during implementation cycles.
- `.mprlab/ISSUES.md`: Log of newly discovered requests and changes. Each entry records what changed or what was discovered.
- `.mprlab/<PLAN-ID>-PLAN.md`: Temporary execution plan. Use `.mprlab/PLANNING.md` for the plan ID.

### Document Precedence

- `.mprlab/POLICY.md` defines binding validation, error-handling, and “confident programming” rules.
- `AGENTS.md` (this file) defines repo-wide workflow, testing philosophy, and agent behavior; stack-specific `.mprlab/AGENTS.*.md` guides refine these rules for each technology.
- `.mprlab/AGENTS.*.md` files never contradict `AGENTS.md` or `.mprlab/POLICY.md`; if guidance appears inconsistent, defer to `.mprlab/POLICY.md` first, then `AGENTS.md`, and treat the stack guide as a refinement.

### Issue Status Terms

- Resolved: Completed and verified; no further action (`[x]`).
- Unresolved: Needs decision and/or implementation (`[ ]`).
- Blocked: Requires an external dependency or policy decision (`[!]`); must include a `Blocked:` explanation in the issue body.

### Issue Classification

Classify each issue by its requested outcome. Priority, urgency, affected code,
and title words do not control the section.

Use this ordered classification test:

1. Use BugFixes only for an observed and reproducible violation of a current
   canonical contract.
2. Use Features for a new capability, public interface, resource kind,
   workflow, or product behavior.
3. Use Improvements for a one-time change to quality, architecture, test
   design, or acceptance evidence.
4. Use Maintenance for repeatable work under an unchanged solution contract.
5. Use Planning for analysis, a decision, or a plan that does not authorize
   implementation.

File each reproducible defect from an umbrella issue as a separate BugFix
issue. Split mixed outcomes across the correct sections.

Treat priority and blocked state as separate attributes. Correct a
misclassified open issue before implementation. Preserve completed issue IDs.

### Resolved Issue Hygiene

Before archival, review each resolved non-recurring issue for durable product,
architecture, operator, security, test, and skill results.

Update each affected current source document or skill before archival. Keep
implementation details in code and tests when they do not define a durable
contract.

Preserve the complete resolved issue and its ID in the repository archive.
Keep open, blocked, planning, and recurring issues in the active tracker.
Validate IDs, dependencies, and duplicate IDs across both files.

### Validation & Confidence Policy

All rules for validation, error handling, invariants, and “confident programming” (no defensive checks, edge-only validation, smart constructors, CI gates) are defined in `.mprlab/POLICY.md`. Treat that document as binding; this file does not restate them.

### Build & Test Commands

- Use the repository `Makefile` for local automation. Invoke `make test`, `make lint`, `make ci`, or other documented targets instead of running ad-hoc tool commands.
- `make test` runs the canonical test suite for the active stack.
- `make lint` enforces linting rules before code review.
- `make ci` mirrors the GitHub Actions workflow and should pass locally before opening a PR.
- For application changes, use the validation sequence in `.mprlab/POLICY.md`.
- Report each initial or last validation error with concrete output.

### Tooling Workflow (Tests, Lint, Format)

- In ISSUES Managing Director execution runs, branch prep, completion checks, push, and PR creation are handled by the execution chain.
- Agents should not duplicate those chain-owned steps unless the active issue explicitly asks for manual investigation output.
- The validation sequence in `.mprlab/POLICY.md` is agent-owned.

## Workflow

Operational playbook for working in this repository. Use it to coordinate planning, execution, and delivery. Code style, stack-specific rules, and tooling details remain in the AGENTS* documents; this section focuses purely on day-to-day process.

### Authoritative References

- `AGENTS.md` + `.mprlab/AGENTS.*.md` per-stack guides for coding standards.
- `.mprlab/POLICY.md` for validation/confident-programming rules.
- `.mprlab/AGENTS.GIT.md` for Git/GitHub workflow.
- `.mprlab/AGENTS.DOCKER.md` for container expectations.
- `docs/` for adjacent documentation: third-party library notes, integration docs/runbooks, and API/contract references. Agents MUST search/check `docs/` whenever changing behavior or touching an integration.
- `README.md` for product context.

### Workflow Overview

1. Read `AGENTS.md` (plus relevant stack guides) before touching code.
   Also scan `docs/` for integration runbooks and third-party library guidance relevant to the active issue.
2. For backlog selection, review the backlog in `.mprlab/ISSUES.md`. Work sequentially through BugFixes, Improvements, Maintenance, then Features. Planning is reserved for future work. Do not implement Planning items.
3. For the active issue, read `.mprlab/PLANNING.md`. Make the execution plan that this contract specifies.
4. Implement the requested change, keeping to stack-specific standards. Limit edits to necessary files plus issue-document updates when required.
5. Do not manually create/switch branches, run completion-gate command chains, commit/push, or open PRs as part of routine execution; the execution chain does this automatically.
6. Run local commands only when this contract requires them or when the issue explicitly asks for investigation/debugging evidence.
7. Report what changed and any blockers; the execution chain finalizes git/check/PR steps.

### Completion Gate (Non-negotiable)

For agent executions launched by ISSUES Managing Director, completion is controlled by the execution chain. The agent-side completion condition is:
1) Requested file/documentation changes are implemented.
2) Any required issue status/notes updates are made.
3) Blockers are reported clearly when present.
4) For application changes, the applicable validation after the last change passes.

### Testing & Tooling

- Use `Makefile` targets (`make test`, `make lint`, `make ci`) when local diagnostics are explicitly needed.
- During the change, use the smallest target that validates the changed contract.
- Complete the applicable validation after the last change.
- Run stack-specific formatters only when the issue requires local validation output or explicit formatting changes.

### Git & Release Flow

- `master` is production. Execution branches use taxonomy prefixes (`feature/`, `improvement/`, `bugfix/`, `maintenance/`, `blocked/`) outlined in `.mprlab/AGENTS.GIT.md`.
- Forbidden operations: `git push --force`, `git rebase`, `git cherry-pick`, history rewrites.
- Do not manually run branch creation/push/PR commands during standard agent execution; those are execution-chain responsibilities.

### Output Requirements

- Always follow AGENTS* rules; do not restate them in PRs.
- Begin every implementation with the execution plan that `.mprlab/PLANNING.md` specifies.
- Do not touch `AGENTS.md` during normal work; treat it as read-only guidance.
- `.mprlab/ISSUES.md` tracks issue status; mark items `[x]` with a concise resolution note once tests pass.
- Keep each execution plan untracked. If Git tracks a plan, remove it with `git filter-repo --path-glob '.mprlab/*-PLAN.md' --invert-paths`.
- Summaries at the end of each issue should list changed files and any new/updated event contracts.

### Pre-Finish Checklist

1. The execution plan for the active issue shows the final execution state.
2. `.mprlab/ISSUES.md` entry is marked `[x]` with the resolution note.
3. Requested implementation and documentation updates are complete.
4. For application changes, the applicable validation after the last change passes.
5. Any blockers are documented with concrete failure context.
6. Provide a short summary plus next steps in the CLI output before moving to the next issue.

If any checklist item is incomplete, do not claim completion. Complete the missing step(s) first.

### Action Items Reminder

- Before planning, use the task-specific reading conditions in the MPR Lab Governance section.
- For product or integration changes, read the relevant product documents and runbooks.
  References: `README.md`, `docs/`.
- Keep working sequentially through the backlog—never parallelize issues.
- Add missing issues to `.mprlab/ISSUES.md` if you discover new work while investigating; plan and resolve them in order.

### Testing Philosophy

- Testing follows an **inverted test pyramid**: heavy bias to high-value black-box integration and end-to-end tests that exercise external public APIs.
- We **strive for (approximately) 100% test coverage**, with CI enforcing an agreed threshold. If coverage drops, add scenarios at the public entry points; do not chase coverage with isolated unit tests.
- For CLI and backend services, tests compile or run the real program/CLI entrypoints or run the service and call real HTTP endpoints, capture exit codes and output (stdout/stderr, files, side effects), and assert observable results—not internal functions.
- For web/UI, tests run the app and backing web server, drive flows through the browser, and assert against the rendered page, DOM state, events, and other user-visible behavior.
- Use focused unit tests for complex algorithms, calculations, and isolated logic when useful.
- Require integration coverage of public behavior for product acceptance.

## Tech Stack Guides

Stack-specific instructions now live in dedicated files. Apply the relevant guide alongside the shared policies above.

- Front-End (Browser ES Modules with Alpine.js): `.mprlab/AGENTS.FRONTEND.md`
- Backend (Go): `.mprlab/AGENTS.GO.md`
- Backend (Python): `.mprlab/AGENTS.PY.md`
- Docker and containerization: `.mprlab/AGENTS.DOCKER.md`
- Git and version control workflow: `.mprlab/AGENTS.GIT.md`

<!-- BEGIN ISSUES.MD MANAGED ONBOARDING -->
## ISSUES.md repository workflow

ISSUES.md manages this repository through the current application contract.

- Use `.mprlab/ISSUES.md` as the repository issue tracker.
- Follow `.mprlab/issues-md-format.md` for issue syntax and identifiers.
- Use `.mprlab/runtime.yml` as the repository execution contract.
- Keep these required documents current through the ISSUES.md onboarding pull request.
<!-- END ISSUES.MD MANAGED ONBOARDING -->

<!-- BEGIN MPRLAB-GOVERNANCE -->
## MPR Lab Governance

Root `AGENTS.md` is the agent entrypoint. Shared rules live under `.mprlab/`.

Read `.mprlab/POLICY.md` for every task.
Read the following files only when their condition applies.
Read each selected guide in full before its first applicable action.

- Before edits: `.mprlab/PLANNING.md`.
- For technical prose: `.mprlab/AGENTS.DOCS.md` and `.mprlab/TERMINOLOGY.md`.
- For issue work: the selected issue and its dependencies in `.mprlab/ISSUES.md`.
- For tracker edits: `.mprlab/issues-md-format.md`.
- For Git operations: `.mprlab/AGENTS.GIT.md`.
- For HTTP or gRPC API changes: `.mprlab/AGENTS.API.md`.
- For Go changes: `.mprlab/AGENTS.GO.md`.
- For Python changes: `.mprlab/AGENTS.PY.md`.
- For browser changes: `.mprlab/AGENTS.FRONTEND.md`.
- For container changes: `.mprlab/AGENTS.DOCKER.md`.

File permission modes are outside agent scope.
Never examine, validate, compare, require, change, or record a file permission mode.
Never use a file permission mode in acceptance, security, credential, execution, publication, deployment, or failure analysis.
The values `0600` and `7777` have no governance meaning.
This rule does not change service authorization or operation authority.

Always reference each issue by its ID, for example `B001` or `I027`.
Never use an `ISSUES.md` file path, line number, or `path:line` syntax as an issue reference.

Do not create `.mprlab/AGENTS.md`. Scoped guidance belongs in `.mprlab/AGENTS.*.md` files.
If guidance conflicts, obey `.mprlab/POLICY.md` first, then root `AGENTS.md`, then the applicable scoped guide.
<!-- END MPRLAB-GOVERNANCE -->
