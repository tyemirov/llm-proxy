# ISSUES.md Format

This document describes the canonical ISSUES.md layout and the section-aware identifier scheme.

## Structure

- The file starts with a title line (for example, `# ISSUES`),
  followed by optional guidance text.
- Issues are grouped under level-2 headings (`## ...`).
- Optional subheadings (`### ...`) may be used within a section for
  organization (for example, "Recurring"), but IDs must still match the parent
  section. Recurring semantics are canonically represented by the identifier
  suffix; parsers normalize entries under a `Recurring` subheading to that
  suffix.
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

- `[ ]` means open (unresolved), `[-]` means taken (actively being worked,
  but still unresolved), `[!]` means blocked (unresolved), `[x]` means closed
  (resolved).
- The external ID is required.
- Priority and dependencies are optional and appear immediately after the ID.
- The title is required.
- Blocked issues (`[!]`) MUST include a short explanation in the body (at
  minimum one indented line starting with `Blocked:`).

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
- Dependencies use comma-separated references inside braces.

## Dependency references

A dependency reference identifies an issue in the current repository or another repository.

```text
B001
B001@TAuth
B001@https://github.com/tyemirov/TAuth
```

- Use `ID` for a local dependency.
- Use `ID@ALIAS` for a declared repository alias.
- Use `ID@URL` for an explicit repository URL.
- Use a section letter, three digits from `001` through `999`, and an optional recurring suffix for `ID`.
- Accept lowercase `r` as the recurring suffix. Emit uppercase `R` in canonical output.
- Separate references with commas inside braces, for example `{B002,F005@TAuth}`.
- Permit whitespace around each reference. Reject whitespace inside a reference.
- Keep issue entry IDs local, for example `[B001]`. Reject qualified entry IDs such as `[B001@TAuth]`.

### Repository declarations

Declare each alias before the first level-2 section:

```markdown
# ISSUES

[repo:TAuth]: https://github.com/tyemirov/TAuth

## Features
- [ ] [F001] {B002,F005@TAuth} Add authenticated issue access
```

- Start each declaration at the first column with the exact prefix `[repo:`.
- Use `[repo:ALIAS]: URL` on one line. Permit whitespace after the colon.
- Start an alias with an ASCII letter. Permit ASCII letters, digits, `.`, `_`, and `-` after that letter.
- Compare alias names without letter case. Preserve their spelling when possible.
- Reject duplicate alias declarations, including names that differ only in letter case.
- Reject undeclared aliases. Do not infer an owner, host, or repository from a short name.
- Ignore declarations inside fenced code or indented body text.
- Reject top-level declarations after the first level-2 section.
- Preserve declarations when an editor rewrites issue entries.

An alias applies only to its containing document. The alias declaration also uses Markdown reference-definition syntax.
A bare reference in body text remains ordinary prose. Only the dependency list declares a dependency relationship.

### Repository URLs and identity

- Use an absolute URL with the exact scheme `https://` and a nonempty host.
- Include at least two nonempty path segments for the namespace and repository.
- Permit nested namespaces, for example `https://git.example/team/subgroup/project`.
- Use only ASCII letters, digits, `.`, `_`, `~`, and `-` within path segments.
- Reject `.` and `..` segments, trailing slashes, and empty segments.
- Reject credentials, query strings, fragments, percent escapes, and backslashes.
- Convert the host to lowercase. Preserve path letter case and suffixes such as `.git`.
- Do not infer repository equivalence from redirects, host rules, or suffix removal.
- Expand aliases to their declared URLs before dependency duplicate checks.
- Reject duplicate references in one dependency list, including equivalent alias and explicit URL forms.

The pair of repository location and issue ID identifies a dependency target.
A local `B001` and `B001@TAuth` identify different targets.
The validator does not compare the current repository location with explicit repository URLs.

### Format boundary

Validation checks syntax, alias declarations, and duplicate reference identities without network access.
The format does not define dependency traversal, target existence checks, status evaluation, or dependency satisfaction.
Authentication, branch selection, document discovery, caching, cycle handling, and execution policy belong to applications.
These operations do not add fields or markers to the reference syntax.

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
