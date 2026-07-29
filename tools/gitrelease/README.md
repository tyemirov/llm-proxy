# Repository Release Toolchain

This directory owns the immutable local release, publication, container, and
Pages artifact commands used by the root `Makefile`. Keep the scripts and their
black-box tests versioned together so a clean llm-proxy checkout contains the
complete release implementation.

Release versions use the repository's single canonical `vMAJOR.MINOR.PATCH`
SemVer contract, with optional SemVer prerelease identifiers.

The local manifest is the authoritative sealed release. New payloads are built
under `.git/mprlab-release.pending` and replace `.git/mprlab-release` only
after the changelog-only release commit, annotated tag, notes, and complete
payload inventory validate together. An exact retry at that release commit
returns the sealed version without CI, rebuild, Git, or artifact mutation.
Interrupted work after the release commit resumes from the pending payloads;
failed work before that commit leaves the previous sealed artifact intact.

Publication is convergent and immutable. Existing GitHub Release metadata and
assets, GHCR platform manifests, and the GHCR version index must exactly match
the prepared release before they are reused. Only confirmed-missing state is
published, registry inspection ambiguity fails closed, and an existing
conflict is never overwritten. The mutable `latest` tag changes only when its
digest does not identify the prepared final release.

Each prepared platform tag is one immutable OCI index containing exactly one
runnable Linux image manifest and its matching provenance attestation. Its
remote index digest must equal the prepared local image ID. Version and
`latest` indexes contain the exact union of those platform-index descriptors,
including the attestations.

Pages artifacts always contain an empty `.nojekyll` file and a schema-versioned
`.mprlab-release.json` marker. Deployment validates the archive contract and
matches the release tag to `release_commit` while matching artifact and public
marker provenance to the distinct `source_commit`. Container publication waits
for the exact OCI manifests to become readable through the standard Docker
client, with each inspection bounded by
`CONTAINER_REGISTRY_VERIFY_ATTEMPT_TIMEOUT_SECONDS`. Pages activation reuses an
exact branch and site configuration, selects the newest build for that branch
commit, waits when it is queued or building, and requests one replacement when
it is absent or failed. It then verifies a cache-distinct public marker, so a
branch push or an older successful build is never treated as public
availability.
