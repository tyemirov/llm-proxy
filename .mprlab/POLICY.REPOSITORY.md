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

## Repository CI Requirements

Implementation changes require final full CI. Use equivalent passing pull-request CI for the same validated state or run `make ci` locally at the selected final checkpoint.

For read-only work, inspect source facts. For documentation-only work, run the applicable document and repository checks. For changes confined to `.mprlab/`, run the Governor check and `git diff --check`, and review changed prose.

When `make ci` covers format, lint, and test targets, reuse that result. Run additional focused targets only for required coverage outside that gate. Keep artifact, signing, and platform release checks for their additional coverage.
