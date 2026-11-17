# Repository Policy Sections

## Selected Manifest Contract

Apply this section when the task changes or validates a selected application manifest.

- Keep the selected application manifest versionless.
- Keep `owner`, `release`, and `lifecycle` as the required baseline fields.
- Use the fixed `artifacts`, `publications`, and `targets` catalogs.
- Use explicit identity lists and ordered command lists.
- Declare CI and execution policy in the selected manifest's `lifecycle` field.
- After this cutover, accept each manifest that an earlier versionless gateway accepted.
- Keep each accepted field name, type, requirement, and function.
- A field added to an existing shape must be optional.
- A field added to an existing shape must have one canonical default.
- Normalize that default before manifest identity calculation.
- Add a declaration kind only with one closed shape.
- Reject unknown fields and `schema_version`.

## Static Website Hosting

Apply this section to deployment or publication work for a browser frontend.

- Use GitHub Pages as the production host for each deployable browser frontend.
- Declare the browser frontend with a `targets.github_pages.sites[]` declaration in `.mprlab/deploy/resources.yml`.
- Use `gh-pages` as the publication branch.
- The GitHub Pages repository can differ from the application repository.
- Keep API and service routes on hostnames that differ from the website hostname.
- Reserve the GitHub Pages domain and its `www` hostname for GitHub Pages.
- Treat a container as an artifact source only when its static output goes to GitHub Pages.
- Verify publication through the public website and `/.mprlab-release.json`.
- Run the Governor check after each selected manifest change and before each release, publish, or deploy operation.

## Validation

Full CI is the final validation gate before CI-dependent delivery or task completion.
Finish all other authorized task work and all non-CI results and documentation first.
This includes implementation, configuration, required account setup, focused tests,
review fixes, live acceptance, benchmarks, and their analysis when applicable.
Use focused checks while any work or required input remains.

- Use repository-native `make` targets.
- Do not run a pre-edit or per-issue `make ci` baseline.
- Preserve reproduction results and any expected failing regression test as implementation evidence.
- During the change, run the smallest public-entrypoint target that validates the changed contract.
- Run full CI once at the final checkpoint, after absolutely all other authorized task work is finished.
- Use successful full CI from the pull request when its workflow supplies all required checks.
- Open the pull request at the final checkpoint, after all other authorized task work and focused local checks are complete. Let its workflow supply the final full CI result.
- Wait for that result before merge or completion.
- When no pull request supplies full CI, run `make ci` locally only after all other authorized task work is complete.
- Use focused checks to correct failures.
- If final CI fails, make corrections with focused checks. Repeat full CI only after all corrections and other remaining work are complete.
- Reuse successful CI results while the validated code remains unchanged.
- Use equivalent CI results across local validation, pull requests, and release.
- Keep separate release checks for additional artifact, signing, or platform coverage.
- Record the selected CI command, its result, and any required checks outside its coverage.
- Run `make verify` only when the operator explicitly selects the complete qualification lane.
- When `make ci` includes `make fmt`, `make lint`, and `make test`, use its result for those targets.
- During the change or error diagnosis, run the necessary component target.
- Run a component target when `make ci` does not include the necessary check.
- For documentation-only work, run the applicable document and repository checks.
- For `.mprlab/`-only work, run the Governor check and `git diff --check`.
- These are the repository checks for `.mprlab/`-only work. Changed prose also requires the documentation-language review.
- For read-only work, use source facts and run only the necessary checks.
- For frontend behavior, verify through a browser test when the behavior is user-visible.
- For services and CLIs, verify through HTTP, CLI, or public API entry points.
