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
- PLAN.md: Working plan for one concrete change/issue; ephemeral and replaced per change.

### Document Precedence

- `.mprlab/POLICY.md` defines binding validation, error-handling, and “confident programming” rules.
- `AGENTS.md` (this file) defines repo-wide workflow, testing philosophy, and agent behavior; stack-specific `.mprlab/AGENTS.*.md` guides refine these rules for each technology.
- `.mprlab/AGENTS.*.md` files never contradict `AGENTS.md` or `.mprlab/POLICY.md`; if guidance appears inconsistent, defer to `.mprlab/POLICY.md` first, then `AGENTS.md`, and treat the stack guide as a refinement.

### Issue Status Terms

- Resolved: Completed and verified; no further action (`[x]`).
- Unresolved: Needs decision and/or implementation (`[ ]`).
- Blocked: Requires an external dependency or policy decision (`[!]`); must include a `Blocked:` explanation in the issue body.

### Validation & Confidence Policy

All rules for validation, error handling, invariants, and “confident programming” (no defensive checks, edge-only validation, smart constructors, CI gates) are defined in `.mprlab/POLICY.md`. Treat that document as binding; this file does not restate them.

### Build & Test Commands

- Use the repository `Makefile` for local automation. Invoke `make test`, `make lint`, `make ci`, or other documented targets instead of running ad-hoc tool commands.
- `make test` runs the canonical test suite for the active stack.
- `make lint` enforces linting rules before code review.
- `make ci` mirrors the GitHub Actions workflow and should pass locally before opening a PR.

### Tooling Workflow (Tests, Lint, Format)

- In ISSUES Managing Director execution runs, branch prep, completion checks, push, and PR creation are handled by the execution chain.
- Agents should not duplicate those chain-owned steps unless the active issue explicitly asks for manual investigation output.

## Workflow

Operational playbook for working in this repository. Use it to coordinate planning, execution, and delivery. Code style, stack-specific rules, and tooling details remain in the AGENTS* documents; this section focuses purely on day-to-day process.

### Authoritative References

- `AGENTS.md` + `.mprlab/AGENTS.*.md` per-stack guides for coding standards.
- `.mprlab/POLICY.md` for validation/confident-programming rules.
- `.mprlab/AGENTS.GIT.md` for Git/GitHub workflow.
- `.mprlab/AGENTS.DOCKER.md` for container expectations.
- `docs/` for adjacent documentation: third-party library notes, integration docs/runbooks, and API/contract references. Agents MUST search/check `docs/` whenever changing behavior or touching an integration.
- `README.md`, `PRD.md`, and `ARCHITECTURE.md` for product context.

### Workflow Overview

1. Read `AGENTS.md` (plus relevant stack guides) before touching code.
   Also scan `docs/` for integration runbooks and third-party library guidance relevant to the active issue.
2. Review the backlog in `.mprlab/ISSUES.md`; work sequentially through BugFixes, Improvements, Maintenance, then Features. Planning is reserved for future work; do not implement Planning items.
3. For the active issue, read `.mprlab/PLANNING.md` and create `PLAN.md` (ignored by git) with bullet steps. Keep it updated and delete/rewrite it for the next issue.
4. Implement the requested change, keeping to stack-specific standards. Limit edits to necessary files plus issue-document updates when required.
5. Do not manually create/switch branches, run completion-gate command chains, commit/push, or open PRs as part of routine execution; the execution chain does this automatically.
6. Run local commands only when the issue explicitly asks for investigation/debugging evidence.
7. Report what changed and any blockers; the execution chain finalizes git/check/PR steps.

### Completion Gate (Non-negotiable)

For agent executions launched by ISSUES Managing Director, completion is controlled by the execution chain. The agent-side completion condition is:
1) Requested file/documentation changes are implemented.
2) Any required issue status/notes updates are made.
3) Blockers are reported clearly when present.

### Testing & Tooling

- Use `Makefile` targets (`make test`, `make lint`, `make ci`) when local diagnostics are explicitly needed.
- Do not run full completion-gate suites as routine finish steps; the execution chain runs completion checks automatically.
- Run stack-specific formatters only when the issue requires local validation output or explicit formatting changes.

### Git & Release Flow

- `master` is production. Execution branches use taxonomy prefixes (`feature/`, `improvement/`, `bugfix/`, `maintenance/`, `blocked/`) outlined in `.mprlab/AGENTS.GIT.md`.
- Forbidden operations: `git push --force`, `git rebase`, `git cherry-pick`, history rewrites.
- Do not manually run branch creation/push/PR commands during standard agent execution; those are execution-chain responsibilities.

### Output Requirements

- Always follow AGENTS* rules; do not restate them in PRs.
- Begin every implementation with an up-to-date `PLAN.md`.
- Do not touch `AGENTS.md` during normal work; treat it as read-only guidance.
- `.mprlab/ISSUES.md` tracks issue status; mark items `[x]` with a concise resolution note once tests pass.
- `PLAN.md` must remain untracked. If it enters git history, remove it via `timeout -k 350s -s SIGKILL 350s git filter-repo --path PLAN.md --invert-paths` before continuing.
- Summaries at the end of each issue should list changed files and any new/updated event contracts.

### Pre-Finish Checklist

1. `PLAN.md` reflects the final state for the active issue.
2. `.mprlab/ISSUES.md` entry is marked `[x]` with the resolution note.
3. Requested implementation and documentation updates are complete.
4. Any blockers are documented with concrete failure context.
5. Provide a short summary plus next steps in the CLI output before moving to the next issue.

If any checklist item is incomplete, do not claim completion. Complete the missing step(s) first.

### Action Items Reminder

- Read guiding docs (`README.md`, `PRD.md`, `docs/`, `.mprlab/AGENTS*`, `AGENTS.md`, `ARCHITECTURE.md`) before planning.
- Keep working sequentially through the backlog—never parallelize issues.
- Add missing issues to `.mprlab/ISSUES.md` if you discover new work while investigating; plan and resolve them in order.

### Testing Philosophy

- Testing follows an **inverted test pyramid**: heavy bias to high-value black-box integration and end-to-end tests that exercise external public APIs.
- We **strive for (approximately) 100% test coverage**, with CI enforcing an agreed threshold. If coverage drops, add scenarios at the public entry points; do not chase coverage with isolated unit tests.
- For CLI and backend services, tests compile or run the real program/CLI entrypoints or run the service and call real HTTP endpoints, capture exit codes and output (stdout/stderr, files, side effects), and assert observable results—not internal functions.
- For web/UI, tests run the app and backing web server, drive flows through the browser, and assert against the rendered page, DOM state, events, and other user-visible behavior.
- Unit tests are generally discouraged and may be prohibited by your stack guide. Only use unit tests when the relevant stack guide explicitly allows them (for example, `.mprlab/AGENTS.PY.md`), keep them as narrow guardrails for pure, deterministic helpers, and never use them as a substitute for black-box coverage or to pad coverage numbers.

## Tech Stack Guides

Stack-specific instructions now live in dedicated files. Apply the relevant guide alongside the shared policies above.

- Front-End (Browser ES Modules with Alpine.js): `.mprlab/AGENTS.FRONTEND.md`
- Backend (Go): `.mprlab/AGENTS.GO.md`
- Backend (Python): `.mprlab/AGENTS.PY.md`
- Docker and containerization: `.mprlab/AGENTS.DOCKER.md`
- Git and version control workflow: `.mprlab/AGENTS.GIT.md`
