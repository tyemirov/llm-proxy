# Management UI modules

`app.js` is the browser composition root.
It registers the `llmProxyManagementApplication` Alpine component and the `connection-dashboard` custom element.

| Module | Responsibility |
| --- | --- |
| `managementApplication.js` | Compose the Alpine application and reject duplicate property ownership. |
| `managementApplicationState.js` | Create state for authentication, account context, usage, and notices. |
| `authenticationLifecycle.js` | Load the authenticated account and clear state after sign out. |
| `connectionDashboard.js` | Show tenants, connections, and models with explicit configuration controls. |
| `connectionContext.js` | Apply the selected tenant to the usage dashboard. |
| `usageDashboard.js` | Load account or tenant usage and request details. |
| `adminDashboard.js` | Load and show administrator usage. |
| `notifications.js` | Show and dismiss page notices. |
| `usageFailurePresentation.js` | Validate and show failure or rejection records. |
| `usagePresentation.js` | Convert usage summaries into metrics, rows, and chart points. |
| `dialogFocus.js` | Keep keyboard focus inside usage dialogs. |

`connectionDashboard.js` owns tenant selection and connection selection.
It saves configuration through `core/backendClient.js`.
It emits `llm-proxy:connection-context` with the selected tenant and its current profile.
`connectionContext.js` applies this context to the Alpine usage state.

Model cards use family identities from the public capability catalog.
Provider cards use provider identities from the connection inventory.
Both use the existing `brand-icon` element and asset manifest.

Tenant access controls show each generated API key once.
A tenant with a text default receives a copyable request example.
Closing the dialog clears the key and example from browser state.

After shared session recovery, `authenticationLifecycle.js` dispatches
`llm-proxy:management-ready` when the application is already authenticated.
This completion event clears the shared transition.
It preserves application state and tenant selection.
