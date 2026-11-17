# Application Product Policy

Read this guide for application onboarding and changes to user-interface behavior.
Apply each section only to its named operation. These product decisions remain binding.

## Application Onboarding

These requirements apply to every owned application runtime, including native, browser, service, and container applications.
They are mandatory onboarding requirements.
Governance files alone do not complete application onboarding.

- Read root `AGENTS.md`, this policy, and every applicable guide before the corresponding action.
- Obey their requirements throughout implementation, tests, documentation, and delivery preparation.
- Require working root Make targets for `up`, `down`, and `deploy`.
- Make `up` prepare and start the current local application.
- Make `down` stop only that local application and its owned resources.
- Make an already stopped application a successful `down` result.
- Preserve user data during ordinary start and stop operations.
- Implement repository-owned deployment wiring through inline recipes or repository-owned scripts.
- Require `.mprlab/deploy/resources.yml` and the installed Gateway lifecycle contract.
- Keep local start and stop separate from release, publication, and deployment.
- Reject missing commands, empty targets, placeholder recipes, and deployment wiring that cannot be verified.
- Preserve command failures through each wrapper and public Make target.
- Verify start, stop, isolation, and lifecycle routing through headless integration tests.
- Inject external operation boundaries when actual operations require separate authorization.
- Keep visible applications, desktop input, and production operations outside onboarding validation.
- Run every applicable policy check and record the command and result.
- Resolve failed checks before reporting application onboarding as complete.
- Report remaining requirements as blockers, including their exact owning boundary.

The normalizer checks structural requirements without executing Makefiles or application scripts.
A structural pass does not prove command behavior, policy compliance, or deployment success.
Application ownership requires source review beyond broad language and stack profiles.
For libraries, documentation, or tools without an application runtime, record the source evidence for that scope decision.
Do not create placeholder runtime commands to satisfy an inapplicable application contract.

## Automatic Data Updates And Persistence

This is a binding product principle for every user interface.

- Load current data automatically when a view opens and when its relevant data changes.
- Update stale data automatically after reconnection and when the user returns to the view.
- Never require or expose refresh or reload controls for data synchronization.
- Persist valid user edits automatically. Never require or expose save controls for data persistence.
- Show pending, saved, validation, conflict, and failure states without requiring a manual refresh or save.
- Preserve unsaved edits during background updates, request failures, and concurrent changes.
- Serialize writes and retain newer edits when an earlier write completes.
- Keep explicit controls for user intent, such as creation, deletion, activation, and confirmation of consequential actions.
- Do not use an explicit action as a substitute for automatic data persistence.
- Verify automatic updates, persistence, error recovery, and account isolation through public interface tests.
