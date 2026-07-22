# Repository Release Toolchain

This directory owns the immutable local release, publication, container, and
Pages artifact commands used by the root `Makefile`. Keep the scripts and their
black-box tests versioned together so a clean llm-proxy checkout contains the
complete release implementation.

Release versions use the repository's single canonical `vMAJOR.MINOR.PATCH`
SemVer contract, with optional SemVer prerelease identifiers.

Pages artifacts always contain an empty `.nojekyll` file and a schema-versioned
`.mprlab-release.json` marker. Deployment validates the archive contract and
matches the release tag to `release_commit` while matching artifact and public
marker provenance to the distinct `source_commit`. Container publication waits
for the exact OCI manifests to become readable through the standard Docker
client, with each inspection bounded by
`CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS`. Pages activation selects
the newest GitHub Pages build for the pushed `gh-pages` commit before verifying
a cache-distinct public marker, so a branch push or an older successful build
is never treated as public availability.
