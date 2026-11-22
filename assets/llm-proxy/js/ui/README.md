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

`../modelTasks.js` defines the shared task labels, input/output directions, and offering eligibility.
The dashboard and public route explorer use these definitions.
Tasks match exact offering capabilities. Image input alone does not imply image generation.

The dashboard and public explorer show task icons on filter buttons and model cards.
The filter buttons occupy one row that scrolls when space is limited.
Cards show all supported task icons, independent of search filters.
Each icon has a task description on hover and keyboard focus. Model details show input/output directions.

The dashboard starts without a task selection and shows all models for the connection.
Each dashboard button toggles independently. The final selection can be removed.
Only tasks in the complete connection inventory have filter buttons.
A connection change removes unavailable selections and keeps the remaining selections.

The public explorer initially selects Text, or the first available task, and keeps at least one selection.
Selections use AND matching on the same offering.
Model details list all supported tasks, including required and optional inputs.

Task selection changes the view. Model defaults still require an explicit save.
The selected model and task determine the default domain. Without a task selection, the first supported task determines the domain.
The public explorer matches input and output filters within one task on one offering.

Tenant access controls show each generated API key once.
A tenant with a text default receives a copyable request example.
Closing the dialog clears the key and example from browser state.

After shared session recovery, `authenticationLifecycle.js` dispatches
`llm-proxy:management-ready` when the application is already authenticated.
This completion event clears the shared transition.
It preserves application state and tenant selection.
