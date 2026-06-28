# ISSUES.md Format

This document describes the canonical ISSUES.md layout and the section-aware identifier scheme.

## Structure

- The file starts with a title line (for example, `# ISSUES`),
  followed by optional guidance text.
- Issues are grouped under level-2 headings (`## ...`).
- Optional subheadings (`### ...`) may be used within a section for organization (for example, "Recurring"), but IDs must still match the parent section. Recurring semantics are canonically represented by the identifier suffix; parsers normalize entries under a `Recurring` subheading to that suffix.
- Sections are:
  - BugFixes
  - Improvements
  - Maintenance
  - Features
  - Planning

Section headings should not include numeric ranges; the section name alone is
the category.

## Issue entries

Each issue entry is a single list item with this shape:

```text
- [ ] [B042] (P1) {I007} Short title
```

Rules:

- `[ ]` means open (unresolved), `[!]` means blocked (unresolved), `[x]` means closed (resolved).
- The external ID is required.
- Priority and dependencies are optional and appear immediately after the ID.
- The title is required.
- Blocked issues (`[!]`) MUST include a short explanation in the body (at minimum one indented line starting with `Blocked:`).

## Identifiers

Format: `<SectionLetter><SequenceNumber>[R]` with no repo prefix.

Section letters:

- B = BugFixes
- I = Improvements
- M = Maintenance
- F = Features
- P = Planning

Identifiers must match the section they appear in. Numbers increment
independently per section. Use three digits (`001`-`999`) per section; after a
section reaches its max (example: after B999), the next auto-number wraps to
B001.
A capital `R` suffix inside the identifier marks the entry as recurring
(example: `[M001R]`). A separate `R` token after the identifier is invalid.
Parsers accept lowercase `r` while reading and render uppercase `R` in
canonical output.
Recurring entries represent standing or repeated work that should remain
visible during cleanup. Scheduling, timers, and job IDs are outside the
ISSUES.md format.
Legacy repo-prefixed identifiers (for example `IM-###`) are invalid.

## Priority and dependencies

- Priority uses `(P0)` through `(P2)` immediately after the ID.
- Dependencies use `{ID,ID}` with comma-separated IDs.

## Body text

- To attach a body on the same line, separate the title and body with a space,
  an em dash (U+2014), and a space.
- Additional body lines may follow on subsequent lines; indent by two spaces
  to keep them attached to the issue.
- Fenced code blocks are allowed in the body; indent them by two spaces as well.
- Structured issue bodies should use plain labels rather than Markdown
  headings. The canonical labels are `Goal:`, `Requirements:`,
  `Deliverables:`, `Validation:`, and `Blocked:`.
- `Goal:`, `Requirements:`, `Deliverables:`, and `Validation:` are recommended
  guidance for human and AI producers. Parsers recognize them but do not require
  every free-form issue body to contain all four labels.
- `Blocked:` is required only for blocked issues (`[!]`) and must include the
  concrete external dependency, missing input, or policy decision preventing
  progress.

## Example

```text
# ISSUES

## BugFixes
- [!] [B042] (P0) Fix crash on startup
  Goal:
  Prevent startup crashes during repository initialization.

  Requirements:
  Preserve the existing configuration loading contract.

  Deliverables:
  Patch the initialization path and document the failure mode.

  Validation:
  Reproduce the startup path with the affected configuration.

  Blocked: waiting on upstream API credentials.
  ```bash
  timeout -k 30s -s SIGKILL 30s make test
  ```
```
