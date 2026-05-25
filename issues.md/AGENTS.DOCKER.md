# AGENTS.DOCKER.md

## Scope

Docker build standards for the repository. Follow AGENTS.md for shared documentation policies, validation rules, and workflow expectations.

## Docker Guidelines

### Base User

- Containers must run as `root`. Do not create or switch to non-root users inside Docker images.

### Build Strategy

- Always use multi-stage Dockerfiles: compile or package artifacts in an initial stage, then copy only the required outputs into a lean runtime stage.
- Avoid elaborate build steps when an existing base image that already contains the needed tooling or dependencies can be used. Prefer composing from purpose-built images over scripting complex setups inside Dockerfiles.
- Provide curated Dockerfile and docker-compose YAML samples (development and production variants) for each service so contributors consistently follow approved patterns.
- Development Dockerfiles must build artifacts directly so they can exercise uncommitted or unmerged changes before they land on main.
- Production Dockerfiles must always pull the newest base image before layering repo artifacts to ensure dependencies stay current.

### Go Builds In Containers

- When building Go binaries in Docker, set `CGO_ENABLED=0`. Enable CGO only when absolutely required and document the justification in the Dockerfile.

### Environment Configuration

- All environment configuration for a service must reside in a single `.env.<service>` file shared by both development and production Docker workflows. Do not scatter env variables across multiple files or inline them inside Dockerfiles/compose YAML.
