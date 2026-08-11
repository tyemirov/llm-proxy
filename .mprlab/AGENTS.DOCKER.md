# AGENTS.DOCKER.md

## Scope

Docker and container guidance for this repository. Use this guide only when Dockerfiles, Compose files, container images, or container deployment manifests are present.

## Rules

- Containers run as `root` unless the repo has a current explicit requirement otherwise.
- Prefer multi-stage Dockerfiles.
- Copy only necessary runtime artifacts into final images.
- Keep environment configuration centralized in documented env files or deployment manifests.
- Do not bake secrets into images.
- Development images must exercise current unmerged source when used for validation.
- Production images must document their base image and publish path.
- For Go builds in Docker, set `CGO_ENABLED=0` unless CGO is necessary and documented.

## Orchestration Boundaries

- Treat local orchestration and production orchestration as different contracts.
- MPR Lab has no current plan to put these contracts together.
- Keep app-owned local orchestration during a migration to `schema_version: 3`.
- A local topology can be different from the production topology.
- Keep local orchestration files outside `.mprlab/deploy/`.
- Do not use local orchestration files as production lifecycle inputs.
- For a deployable application, use `.mprlab/deploy/resources.yml` as the only
  tracked production deployment file.

## Validation

- Use `.mprlab/POLICY.md` for validation.
- During the change, run the smallest container target that validates the changed contract.
- Build the image locally when Dockerfile changes affect runtime behavior.
- Run the service or container smoke test when available.
- Confirm exposed ports, health checks, volumes, and env variables match the documented deployment contract.
