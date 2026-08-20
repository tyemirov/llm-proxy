# ISSUES.md Format

This document describes the canonical ISSUES.md layout and section-aware identifier scheme.

## Structure

- The file starts with a title line, for example `# ISSUES`.
- Issues are grouped under level-2 headings.
- Sections are `BugFixes`, `Improvements`, `Maintenance`, `Features`, and `Planning`.
- Optional subheadings can organize a section, but issue IDs must still match the parent section.

## Issue Classification

Classify each issue by its requested outcome. Priority, urgency, affected code, and title words do not control the section.

Use this ordered test:

1. Use `BugFixes` only for an observed and reproducible violation of a current canonical contract.
2. Use `Features` for a new user or operator capability, public interface, resource kind, workflow, or product behavior.
3. Use `Improvements` for a one-time change to an existing capability, architecture, test system, or acceptance boundary.
4. Use `Maintenance` for repeatable upkeep under an unchanged solution contract. The same activity must remain valid for a future run.
5. Use `Planning` for analysis, a decision, or a plan that does not authorize implementation.

File each reproducible defect from an acceptance or migration issue as a separate BugFix issue. Split mixed outcomes across their correct sections.

Use priority and blocked state as separate attributes. Correct a misclassified unresolved issue before implementation. Preserve completed issue IDs as historical references.

## Resolved Issue Hygiene

Before archival, review each resolved non-recurring issue for durable product,
architecture, operator, security, testing, and skill consequences. Update each
affected source-of-truth document or skill before you move the issue.

Preserve the complete resolved entry and its identifier in the repository
archive. Keep unresolved, blocked, planning, and recurring issues in the active
tracker. Validate identifiers, dependencies, and duplicate IDs across both
files.

## Issue Entries

Each issue entry is a single list item:

```text
- [ ] [B042] (P1) {I007} Short title
```

Rules:

- `[ ]` means open.
- `[-]` means taken.
- `[!]` means blocked and must include a `Blocked:` body line.
- `[x]` means closed.
- The external ID is necessary.
- Priority `(P0)` through `(P2)` is optional.
- Dependencies `{ID,ID}` are optional.
- The title is necessary.
- Write each new or changed title in ASD-STE100 Simplified Technical English.

## Identifiers

Format: `<SectionLetter><SequenceNumber>[R]`.

Section letters:

- `B` = BugFixes
- `I` = Improvements
- `M` = Maintenance
- `F` = Features
- `P` = Planning

Numbers increment independently per section and use three digits. A capital `R` suffix marks a recurring issue, for example `[M400R]`. A separate `R` token after the identifier is invalid.

Recurring entries represent standing or repeated work. Scheduling, timers, and job IDs are outside the ISSUES.md format.

Legacy repo-prefixed identifiers are invalid.

## Body Text

Indent additional body lines by two spaces. Structured issue bodies must use plain labels:

- `Goal:`
- `Requirements:`
- `Deliverables:`
- `Validation:`
- `Blocked:`

`Blocked:` is necessary only for blocked issues. It must identify the dependency, input, or policy decision that prevents progress.

Write each new or changed body in ASD-STE100. Use `.mprlab/AGENTS.DOCS.md` and `.mprlab/TERMINOLOGY.md`.
