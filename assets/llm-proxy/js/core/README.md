# Management application core modules

Core modules contain browser-independent contracts or integration boundaries
used by the management UI:

| Module | Responsibility |
| --- | --- |
| `backendClient.js` | Send protected management requests through `MPRUI.authenticatedFetch` and decode boundary responses. |
| `managementProfile.js` | Validate account, tenant profile, provider catalog, and routing-default payloads and construct current profile projections. |
| `mprShell.js` | Integrate with the MPR UI authentication and user-menu contract. |
| `runtimeTransition.js` | Dispatch the management-ready transition. |

UI modules consume these contracts after boundary validation and do not call
`fetch` directly.

The header owns shared session recovery. The backend authorizes management
requests before domain work. The client declares
`mutationReplay: "authorization-before-domain-work"` for that contract.
Runtime config uses a public request before the header initializes.
