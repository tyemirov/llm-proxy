# Dependency References

This document gives the repository syntax for cross-repository dependencies.
Use it with [ISSUES.md Format](issues-md-format.md).

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

