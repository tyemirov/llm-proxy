# Gateway manifest audit

Date: November 10, 2026.
Selected application: LLM Proxy.
Contract authority: Installed Gateway v9.2.1 and its packaged Ansible tasks.
The audit covers the complete `.mprlab/deploy/resources.yml` declaration.

## Declaration coverage

| Boundary | Reviewed result |
| --- | --- |
| Owner and release | `llm-proxy`, SemVer, and fixed major `1` match repository release authority. |
| Schema and defaults | The v1.12.10 invocation parsed and admitted the runtime and target declarations. The manifest and installed Gateway integration tests passed. |
| Container artifact | `Dockerfile` and the declared AMD64/ARM64 build inputs agree. Its `/app` working directory matches configuration paths. |
| Pages artifact | `docker/pages/Dockerfile` has the selected `pages` target and exports the static site. |
| Image publication | `llm-proxy-image` connects the build artifact, GHCR repository, and runtime image reference. |
| Pages publication | Artifact, publication, and target use `website`. Repository, `gh-pages`, domain, marker, and public URL agree. |
| Private inputs | Four management inputs feed runtime environment entries. The fifth input supplies elevation for `tutosh`. |
| Host access | The selected host matches inventory placement and the elevation input reference. |
| Runtime identity | Project `runtime` contains service `llm-proxy` on the declared gateway host. |
| Network endpoints | Same-host HTTP uses alias `llm-proxy` and port 8080. Private LAN HTTP uses host port 18083, mapped to container port 8080. |
| Readiness | The service references its existing `http` endpoint and requires HTTP 200 at `/healthz`. |
| Configuration assets | `/app/config.yml` and `/app/providers.yml` match the image working directory and configuration loader. |
| Runtime environment | Every management configuration variable has a declared value, private input, or supported public target reference. |
| Persistent data | `/data` contains the management database and runtime assets. The exact named Docker volume has `retention: retain`. |
| Explicit cleanup | The declared retired project differs from the running project. Its cleanup declaration remains unchanged during the pending generation. |
| Caddy identity | `llm-proxy-public-api` preserves the current route identity. Its hostname differs from the Pages domain. |
| Caddy provider | Current deployed state declares the shared `caddy` service on `tutosh`. The route uses that schema default. |
| Caddy policy | The route-local timeouts and polling match the current provider policy. Application-wide budgets remain unchanged. |
| HTTP access | Route access, browser origin, TLS, upstream capability, and application origins agree. |
| Health observations | API and website observations use their respective origins and `/healthz`. The Pages build includes the static health resource. |
| Lifecycle policy | All timeout kinds and selected lifecycle polling scopes are present. CI remains enabled in the manifest. |
| Current transition | Generation 32 selects v1.12.10. Current fences remain pending; generation 31 observations do not verify the new release. |

## Corrections

B319 preserves the API route identity.
B320 supplies the route-local Caddy execution policy.
This audit removes two unused TAuth provisioning input declarations.
Neither declaration has a current manifest consumer or application reader.
The ignored private file remains unchanged.

Repository assertions now require every declared private input to have a consumer.
The installed Gateway fixture supplies only the five current inputs.
The deployment guide assigns tenant configuration to TAuth and describes the complete `make deploy` pipeline.
It prepares private inputs before execution and qualifies reuse of valid sealed preparation.
It identifies Gateway as the version selector under repository policy.

## Native failure evidence

The failed v1.12.10 invocation completed graph admission, Caddy preparation, and private configuration.
It then failed in `verify-deployment-ghcr-credentials.yml`, at `Resolve operation GHCR identity`.
The delegated controller command `gh api user --jq .login` returned HTTP 401.
This failure occurred before remote Docker authentication and Compose activation.
The running container still reports image v1.12.4.

Later identity requests succeeded as `tyemirov` in both the operator Terminal and the audit process.
Requests also succeeded with `GH_TOKEN` and `GITHUB_TOKEN` removed from the process environment.
A read-only Ansible probe used the installed v9.2.1 toolchain and the declared inventory.
Ordinary and explicitly unprivileged localhost delegation both ran as `tyemirov` and authenticated successfully.
The probe reported zero changed tasks.

The earlier HTTP 401 is established, but its credential cause is not reproduced by current evidence.
No manifest change or Gateway credential change is justified by that failed request alone.

## Verification limits

Independent source review covers the declarations and corrections listed above.
The operator subsequently authorized validation and deployment.
The manifest contract and installed Gateway integration tests passed.
Focused Gateway regressions passed for route identity and contributor policy.

Full application CI passed all 15 gates in 737 seconds with 100.0% Go statement coverage.
Governor found eight existing managed-guide differences. Application structure passed.

Publication and deployment remain pending at the conclusion of this source audit.

The read-only credential probe does not establish GHCR pull acceptance or successful application activation.
