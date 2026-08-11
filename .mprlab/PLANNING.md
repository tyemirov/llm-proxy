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

Do not make `.mprlab/PLAN.md`. Keep the execution plan short, current, and untracked. After you complete the execution, remove its plan.

Use `/.mprlab/*-PLAN.md` as the canonical execution-plan rule in `.gitignore`.

Keep durable decisions and requirements in the issue tracker or a source-controlled document.

Suggested shape:

```text
- [ ] Read repo guidance and target issue.
- [ ] Inspect the current implementation and tests.
- [ ] Use the initial validation result for application changes.
- [ ] Make the scoped change.
- [ ] Run the smallest applicable target during the change.
- [ ] Complete the applicable validation after the last change.
- [ ] Update issue notes or docs.
```

## Completion

Complete a change only after you complete all requested edits and necessary documentation updates.

The applicable validation after the last change must pass. If validation cannot pass, record the concrete blocker.
