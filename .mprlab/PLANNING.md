# Planning

Use this file as the durable planning contract for the repository.

## Planning Rules

- Read `AGENTS.md`, `.mprlab/POLICY.md`, relevant `.mprlab/AGENTS.*.md` guides, and current issue context before editing.
- Plan one concrete change at a time.
- Keep plans forward-only: choose the current canonical contract instead of preserving legacy paths.
- Record blockers with exact missing input, failing command, or external dependency.
- Do not turn planning notes into implementation unless the user or active issue explicitly asks for implementation.

## Working Plan

Keep each temporary execution plan in `.mprlab/<PLAN-ID>-PLAN.md`.

When the execution is for one issue, use the same issue ID as `<PLAN-ID>`. For example, use `.mprlab/B012-PLAN.md` for issue `B012`.

When the execution is not for an issue, use `X` plus three random hexadecimal characters in uppercase as `<PLAN-ID>`. For example, use `.mprlab/X7AF-PLAN.md`.

Before you make the plan, make sure that no plan has the same path. When a plan has the same path, generate a new ID.

Keep the execution plan short, current, and untracked. After you complete the execution, remove its plan.

Use `/.mprlab/*-PLAN.md` as the canonical execution-plan rule in `.gitignore`.

Keep durable decisions and requirements in the issue tracker or a source-controlled document.

## Validation Plan

Apply the proportional test sequence in `POLICY.md`.
Start with the tests that express the requested behavior.
Record the changed contract, relevant public entry point, and chosen tests.
For a bug fix, reproduce the failure before the fix and retain the result.
For behavior changes or consequential refactors, protect the affected contract with appropriate coverage.
Record applicable independent reviews and CI selected by the user or repository policy.
Resolve review findings and finish all other authorized work before the selected final CI gate.
Treat production state and deployed versions as auxiliary context, separate from implementation acceptance.

A short task can use the host's plan or a concise checklist.
Use a temporary execution plan when multiple steps, an interruption, or a handoff needs durable local state.

## Completion

Use [Issue Closure](POLICY.md#issue-closure) to determine implementation issue completion.
Keep production operations in a separate operational record.

Complete a change only after you complete all requested edits and necessary documentation updates.

The applicable validation after the last change must pass. If validation cannot pass, record the concrete blocker.
