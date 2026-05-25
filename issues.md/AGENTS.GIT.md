# AGENTS.GIT.md

## Scope

Git and GitHub conventions for this repository. Use these rules whenever you create branches, commit, or open pull requests. Process-oriented steps (planning, test sequencing, etc.) live in `AGENTS.md` (Workflow section); this document focuses purely on version control hygiene.

In ISSUES Managing Director execution runs, branch preparation, commit, checks, push, and PR creation are handled by the execution chain. Agents should not duplicate those operations unless an issue explicitly requests manual git intervention.

## Branch Strategy

- `master` is the only production branch; there is no `main`.
- Work proceeds as a forward-only chain: branch from the latest issue branch rather than repeatedly branching off `master`, so history forms a linear sequence of completed issues.
- Only fast-forward or merge commits that advance history are allowed. Never rewrite or reset existing commits.

## Branch Naming

- For manual workflows outside the execution chain, create a branch before editing files. Use taxonomy prefixes `feature/`, `improvement/`, `bugfix/`, `maintenance/`, or `blocked/`.
- Follow the pattern `<prefix>/<ISSUE-ID>-short-description`, e.g., `bugfix/B042-editor-duplicate-preview`.
- Keep names concise enough for GitHub limits while remaining descriptive.
- Reserve `blocked/<issue-id>` branches for work that cannot progress; the blocking reason must be documented in `ISSUES.md`.

## Commits & History

- Each issue typically concludes with a single commit capturing the tests and implementation for that issue. If additional fixes are needed, append new commits—do not amend or reorder history.
- Never use `git push --force`, `git rebase`, or `git cherry-pick`. History is append-only.
- Do not use `git revert` to rewrite history; address mistakes by authoring a new forward commit.
- If a mistake occurs, fix it with a new commit on top of the existing branch.
- Commit messages must be descriptive (e.g., “Fix B042 editor preview duplication”) so reviewers understand the change at a glance.
- Do not commit or push changes unless the relevant tests, linters, and formatters have been run and are passing on the current branch, as required by `AGENTS.md` (Workflow section) (blocked work must be explicitly documented in `ISSUES.md`).

## Tracking & Remotes

- Push only to the `origin` remote.
- First push for a branch must establish upstream tracking (see Command Examples).
- Do not rename remote branches or create alternate remotes for the same work.
- Keep local branches aligned with their tracked remote counterpart; fetch/rebase equivalents are prohibited, so merge the tracked branch if needed to resolve conflicts.

## Pull Requests & GitHub

- For manual workflows outside the execution chain, open pull requests via the GitHub CLI (see Command Examples). Reserve the GitHub UI only for viewing existing PRs or reviews.
- PRs are chained: target the previously opened issue branch so reviewers can follow the sequential history. Only the first PR in a sequence targets `master`.
- PR descriptions should reference the issue ID, summarize the change, and include PLAN.md content if required by process docs.
- Keep PRs scoped to a single issue branch. If multiple issues are in flight, each must have its own branch and PR.
- Continuous Integration (CI) runs on GitHub Actions; rely on it for acceptance tests and release validation. Local checks remain mandatory, but CI is the authoritative gate before merge.
- Releases and deployment artifacts originate from GitHub workflows; keep branch histories clean to ensure deterministic CI results.

## File Tracking & Ignore Rules

- `PLAN.md` is intentionally ignored in `.gitignore`; ensure it never appears in commits. If it does, remove it with `timeout -k 350s -s SIGKILL 350s git filter-repo --path PLAN.md --invert-paths` before proceeding.
- Treat generated artifacts (build output, coverage reports, etc.) as untracked unless explicitly added to `.gitignore`.

## Conflict Resolution

- Resolve merge conflicts locally by editing files and committing the resolution; never use force pushes or rebases to “fix” conflicts.
- If conflicts arise because an earlier branch landed upstream, merge the updated upstream branch into your branch and continue forward.

## Blocking Branches

- After three documented attempts on a branch without resolution, convert the branch to `blocked/<issue-id>`, commit the current state, push it, and document the status in `ISSUES.md`.
- Future work resumes from the last successful (non-blocked) branch tip.

## Command Examples

Use these standard commands when working with Git and GitHub in this repo:

```sh
# Start from the latest branch tip
timeout -k 30s -s SIGKILL 30s git checkout bugfix/B041-editor-spam
timeout -k 30s -s SIGKILL 30s git checkout -b bugfix/B042-editor-preview

# Stage and commit work
timeout -k 30s -s SIGKILL 30s git add path/to/files
timeout -k 30s -s SIGKILL 30s git commit -m "Fix B042 editor preview duplication"

# Push and open a PR
timeout -k 30s -s SIGKILL 30s git push -u origin bugfix/B042-editor-preview
timeout -k 30s -s SIGKILL 30s gh pr create --base master --head bugfix/B042-editor-preview --fill
```

CI builds and release artifacts are produced by GitHub Actions workflows in `.github/workflows/`. Refer to those YAML files for additional build examples and replicate the same steps locally when needed.
