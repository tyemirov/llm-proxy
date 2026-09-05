# Management UI modules

`app.js` is the browser composition root. It registers the single
`llmProxyManagementApplication` Alpine component exported by
`managementApplication.js`.

The management application is composed from non-overlapping responsibilities:

| Module | Responsibility |
| --- | --- |
| `managementApplication.js` | Compose the Alpine application and reject duplicate property ownership. |
| `managementApplicationState.js` | Create the complete reactive state object. |
| `managementApplicationPresentation.js` | Derive cross-responsibility Settings readiness and disabled state. |
| `authenticationLifecycle.js` | Reconcile MPR UI authentication, hydrate the application, and clear authenticated state boundaries. |
| `tenantSettings.js` | Select, create, rename, delete, and switch the Settings tenant. |
| `settingsDialog.js` | Open, close, focus, and enforce the Settings modal. |
| `providerEditor.js` | Select a provider and own its browser-memory editor session. |
| `providerCredentials.js` | Enter, reveal, verify, and remove provider credentials. |
| `providerSettings.js` | Serialize and autosave provider setting changes. |
| `routingDefaults.js` | Edit and autosave tenant routing defaults. |
| `profileMutations.js` | Serialize whole-profile mutations and apply returned profiles. |
| `clientAccess.js` | Generate, replace, reveal, and copy one-time client keys. |
| `usageDashboard.js` | Load account or tenant usage, failed-request details, and rejected-request details. |
| `adminDashboard.js` | Load and present administrator usage. |
| `requestExamples.js` | Build and copy current-profile proxy request examples. |
| `notifications.js` | Publish and dismiss page or Settings notifications. |
| `usageFailurePresentation.js` | Validate and present bounded failure or rejection payloads. |
| `usagePresentation.js` | Transform usage summaries into metrics, rows, and chart points. |
| `dialogFocus.js` | Keep keyboard focus inside modal dialogs. |

All modules share one Alpine component instance. Cross-module calls therefore
remain explicit component contracts, while server effects continue to pass
through `core/backendClient.js` and complete profile writes continue to pass
through `profileMutations.js`.
