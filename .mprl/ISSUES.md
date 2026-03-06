# ISSUES

Working backlog for this repository. Keep it current and small. Use @issues-md-format.md for the canonical format.

- Status markers: `[ ]` open, `[!]` blocked (must include a `Blocked:` line), `[x]` closed.
- Hygiene: once a closed issue's consequences are reflected in code/tests and in user-facing docs, remove the entry from this file. Git history remains the record. (Recurring runbooks below are the exception: keep them open.)

## BugFixes

## Improvements

## Maintenance

### Recurring (runbooks; keep open)

These entries are always-available procedures. Keep them `[ ]` so they remain runnable; when you run one, update a short `Last run:` line in the body (and optionally link the PR/commit).

- [ ] [M400] (P2) Backlog housekeeping
  1. Validate `ISSUES.md` matches `issues-md-format.md`.
  2. Confirm user-facing consequences of recently closed work are documented (README/ARCHITECTURE/PRD).
  3. Prune closed entries once documented.
  4. Merge duplicates and delete irrelevant issues.

- [ ] [M401] (P2) Polish open issues
  1. For each open issue, add missing context (dependencies, repro steps, acceptance criteria, and test expectations).
  2. Ensure each issue has a clear priority and concrete deliverables.

- [ ] [M402] (P2) Architecture and policy review
  1. Review the codebase against POLICY.md and stack guides.
  2. Record findings as new Maintenance issues (or close as "no action" if already covered).

## Features

## Planning
