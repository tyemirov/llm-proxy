# Management application core modules

Core modules contain browser-independent contracts or integration boundaries
used by the management UI:

| Module | Responsibility |
| --- | --- |
| `backendClient.js` | Perform browser-to-management API requests and decode boundary responses. |
| `managementProfile.js` | Validate account, tenant profile, provider catalog, and routing-default payloads and construct current profile projections. |
| `mprShell.js` | Integrate with the MPR UI authentication and user-menu contract. |
| `runtimeTransition.js` | Dispatch the management-ready transition. |

UI modules consume these contracts after boundary validation and do not call
`fetch` directly.
