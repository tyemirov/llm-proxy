# Deployment And Native Preparation Policy

Read this guide for deployment manifests, release, publication, deployment, or native preparation.
Apply each section only when its stated condition applies. Preserve the selected product and platform contracts.

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

## Gateway Lifecycle Execution

Apply this section when the repository uses the selected-manifest lifecycle or owns Gateway infrastructure operations.

- Use the installed `mprlab-gateway` runtime for application and infrastructure operations.
- Use `make deploy` as the full application operator command.
- Delegate it to Gateway v9 `deploy --app-root` for CI, preparation, publication, and deployment.
- When optional `release` or `publish` targets exist, delegate each target to the Gateway command with the same name.
- Keep `app-release`, `app-publish`, and `app-deploy` as internal manifest policy identifiers.
- Pass the selected application Git root through `--app-root`.
- Use `MPRLAB_GATEWAY_EXECUTABLE` for an explicit installed command.
- Preserve application source validation, release policy, resource ownership, private inputs, and sealed receipts.
- Keep each operation and its child calls on one captured runtime package and dependency environment.
- Keep Gateway inventory and private config under `MPRLAB_GATEWAY_OPERATOR_ROOT`.
- Use `$HOME/.config/mprlab-gateway` as the default operator root.
- Select an explicit version with `mprlab-gateway install --version <version>` to change the active runtime.
- Use Gateway source for repository CI, release preparation, and Gateway publication.
- Keep installation, application operations, and operator-controlled production activation as separate gates.

## Manifest Environment Resolution

Use this contract for Gateway lifecycle operations that use a selected manifest.
Manifest processing is the only boundary that reads environment values for Gateway configuration.

- Let manifest values include environment variable references.
- Use the optional `environment.source` field for a relative or absolute environment file path specified in the manifest.
- Use the selected consuming repository as the root for relative paths.
- When the manifest has no `environment.source` field, use only process values.
- Reject a selected file that is missing or that you cannot read, even when process values are available.
- When the two sources have a value for a variable, use its process value.
- Use only the path specified by `environment.source` for an environment file.
- Do not use a default path or examine other paths for environment files.
- Do not make an environment file necessary because of a resource kind or lifecycle phase.
- Read environment values one time at the manifest boundary.
- During manifest processing, change environment variable references to their values.
- Validate the configuration before CI, builds, host contact, or provider effects.
- For each missing necessary reference value, reject the manifest with its variable name and manifest location.
- Keep private values out of error messages and public diagnostics.
- Give the validated configuration to subsequent operations.
- Do not read environment variables or environment files for configuration after manifest processing.
- Do not change environment references to values in downstream Go, Python, Ansible, or shell code.
- Supply deployment, publication, and signing credentials from the validated configuration.
- Keep private configuration values outside public artifacts, plans, receipts, state documents, and logs.
- Keep each selected application's validated configuration separate.
- Validate all selected manifests before application-set effects.
- Use only the selected resources and their specified dependency tree for readiness checks.

Environment files are optional inputs to manifest processing.
They are not a separate deployment specification or a necessary credential store.
Provider-native credential stores are provider inputs under their specified contracts.
They cannot supply Gateway configuration that the selected manifest does not include.

## Native Preparation Authority

The operator selected this contract on October 26, 2026.

- Let CocoaPods and Gradle own dependency resolution and native metadata.
- Use ordinary `pod install` for native dependency preparation.
- Keep preparation records limited to application identity, configuration, dependency versions, toolchain versions, and completed command results.
- Do not use file hashes, fingerprints, whole-file comparisons, or lock restoration as readiness gates.
- Verify required native files and their selected application configuration.
- Permit native metadata changes that retain the required configuration.
- Keep asset and plugin freshness outside configuration verification claims.
- Preserve native command failures, signing checks, application versions, build numbers, and provider results.

## Gateway Credential Inputs

Apply this section when Gateway credentials are necessary.

- Find credential references in the selected manifest and resolve them during manifest processing.
- When a reference has no value, examine only the selected input sources.
- Keep certificate and key artifact paths specified in the validated configuration.
- Apply the general Credential Discovery gate in `POLICY.md` before reporting a credential blocker.

## Single Manifest Authority

- Use `.mprlab/deploy/resources.yml` as the only tracked production deployment file for a deployable application.
- Keep local orchestration outside `.mprlab/deploy/` and outside production lifecycle inputs.
- Use only the environment source explicitly selected by the manifest for private file inputs.
- Preserve application source validation and the Gateway-owned lifecycle boundary.
