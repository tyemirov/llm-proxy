# ISSUES.md Format

This document describes the canonical ISSUES.md layout and the section-aware identifier scheme.

## Structure

- The file starts with a title line (for example, `# ISSUES`),
  followed by optional guidance text.
- Issues are grouped under level-2 headings (`## ...`).
- Optional subheadings (`### ...`) may be used within a section for organization (for example, "Recurring"), but IDs must still match the parent section.
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

Format: `<SectionLetter><SequenceNumber>` with no repo prefix.

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

## Example

```text
# ISSUES

## BugFixes
- [!] [B042] (P0) Fix crash on startup
  Blocked: waiting on upstream API credentials.
  ```bash
  timeout -k 30s -s SIGKILL 30s make test
  ```
```
