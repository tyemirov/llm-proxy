# Repository Terminology

This file contains approved technical nouns and technical verbs for repository documentation.

Use this file with ASD-STE100 Simplified Technical English, Issue 9.
Give each term one meaning. Use the same term for the same concept in all documents.

- `characterization test`: An integration test that records current public behavior before a refactor.
- `file permission mode`: A number or symbol that gives filesystem access bits.
- `GitHub Pages`: The GitHub service that hosts a static website from a repository branch.
- `integration test`: A test of real product logic and component interactions through a public entry point, with controlled dependencies when necessary.
- `inverted test pyramid`: The MPR Lab test strategy with integration tests as the primary layer and focused unit tests where useful.
- `production code`: Source code that implements repository behavior outside the test suite.
- `public entry point`: An interface through which a user or caller uses repository behavior.
- `static website`: A browser frontend that uses generated files without a website server runtime.
- `test-driven development`: A coding sequence that uses a failing integration test before a production code change.
- `unit test`: A test that isolates one code unit from its collaborators.
- `website hostname`: The hostname that identifies a public static website.

- `dependency injection`: A design that supplies a component's dependencies from outside that component.

## Repository Technical Nouns

- `emulator`: Software that reproduces a device runtime for application tests.
- `physical device`: Hardware used to run an application or do a device test.
- `simulator`: Software that models a device environment for application tests.

- `Gateway runtime`: The installed MPR Lab deployment executable, runtime assets, and dependencies.
- `operator root`: The directory that contains Gateway inventory and private operator inputs.

- `codec`: Code that serializes requests and parses responses for one provider protocol.
- `activation issue`: An issue that changes an external runtime to use completed development work.
- `client protocol adapter`: Code that translates one public client protocol to and from canonical request and result types.
- `development completion`: Completion of the specified repository changes and repository validation.
- `exact model`: One canonical model version that a client can select.
- `GPU lease`: An exclusive permit for one runtime to use one GPU during an operation.
- `idempotency tombstone`: A retained identity and request record that prevents duplicate work after artifact deletion.
- `inference node`: A host that runs local model containers and the node controller.
- `local offering`: A provider offering that runs on an inference node.
- `media operation`: Accepted tenant work with durable execution state and media input or output resources.
- `media operation adapter`: Code that validates and executes one catalog-selected media operation route.
- `model family`: A group of exact models from one model publisher.
- `model publisher`: An organization or community that creates or releases a model.
- `model residency`: The state in which a model uses GPU memory.
- `node controller`: A private service that controls request admission and runtime container lifecycle on one inference node.
- `weight access`: A model family classification of proprietary or open weights.
- `protocol adapter`: Code that translates canonical requests and responses for one reusable provider protocol.
- `protocol family`: A reusable request and response representation shared by provider offerings.
- `protocol variation`: A declared, typed difference within one protocol family.
- `provider catalog`: The canonical YAML file that defines all supported models, providers, provider offerings, controls, limits, and prices.
- `provider connection`: An account-owned named resource with credentials and settings for one provider definition.
- `provider definition`: One provider record in the provider catalog.
- `provider field`: One credential input or setting input in a provider definition.
- `provider gateway`: The service that authorizes tenant requests and owns shared provider access.
- `provider offering`: One exact model that one provider makes available as a route.
- `provider profile`: Tenant settings for one provider, such as the selected text model and system prompt.
- `provider staging`: Temporary storage that lets a provider read media for an accepted operation.
- `provider transport`: One provider route that defines an endpoint and selects request codec, response codec, authentication, and execution components.
- `request disposition`: A closed value that identifies a request as rejected, succeeded, or failed.
- `rejected request`: A request that cannot execute because it does not satisfy an input or tenant configuration requirement.
- `request intent`: The tenant-bound semantic inputs that one idempotency key identifies.
- `release decision validator`: A committed application program that validates the exact release decision that the gateway transaction uses.
- `repository release version`: The major-version-1 SemVer value from the stored Gix release decision.
- `resolved typed route`: An exact provider and model pair that passed route validation.
- `structured request`: A canonical text request that requires one caller JSON Schema for its output.
- `production acceptance`: Evidence that the production runtime satisfies the checks that an issue specifies.
- `route explorer`: The public interface that selects an exact model and a provider offering.
- `runtime profile`: A validated declaration for one model runtime, container image, private endpoint, resource limit, and idle policy.
- `usage dimension`: A canonical provider or model identity that groups managed usage events.
- `worker claim`: A durable record that assigns an accepted operation to one worker.
- `worker fencing`: Rejection of state changes or dispatch attempts from a worker whose claim is obsolete.

- `caller tool`: A function that the client declares and executes after a model returns its call.
- `function call`: A typed model result with an identifier, a function name, and JSON argument text.
- `bearer key`: A tenant client key supplied in the HTTP Authorization header.
- `server-sent event`: One event in the HTTP event-stream representation of a result.

- `model activation`: An explicit catalog state that controls whether an exact model is available through runtime routing and discovery.

- **DashScope Responses codec**: The Alibaba-specific request and response
  adapter for synchronous Qwen generation.

- `dispatch intent`: A durable record that a worker can have started an external provider request.
- `operation artifact`: An output asset attached to a media operation.
- `cancellation observation`: The observed state of a cancellation request: pending, confirmed, unsupported, or unresolved.
- `usage delivery record`: A durable record that permits one usage event to be delivered after a process restart.

## Media Execution Technical Verbs

- `persist`: Record operation data in durable storage. Approved forms: persist, persists, persisted, persisted.
- `recover`: Restore an operation result from existing provider evidence. Approved forms: recover, recovers, recovered, recovered.
- `deduplicate`: Permit one record or effect for the same identifier. Approved forms: deduplicate, deduplicates, deduplicated, deduplicated.
- `reconcile`: Compare retained records with provider evidence and record the established outcome. Approved forms: reconcile, reconciles, reconciled, reconciled.

- `Google credential profile`: An operator declaration that binds a Google identity, credential file, project, and location to one tenant.

- `tenant assignment`: The saved relation between a tenant and one provider connection.

- `versionless schema`: A database contract whose current tables and records define acceptance without a numeric schema-version requirement.

- `access region`: The Alibaba region that receives API requests and stores request data.
- `service deployment scope`: The geographic area in which Alibaba can execute model inference.
- `model inventory`: A dated list of upstream model identifiers and their documented capabilities.
- `model inventory importer`: An operator tool that converts upstream model data into a proposed provider catalog change.

## Hosted Billing Technical Nouns

- `billing account`: The account resource that owns customer funds, charges, and payment references across its tenants.
- `hosted access grant`: An explicit authorization for a tenant to use specified provider offerings through platform credentials.
- `platform connection`: An operator-owned provider credential resource used for hosted customer requests.
- `billable attempt`: One recorded attempt to execute a customer request through an external provider.
- `usage journal`: The durable record of requests, attempts, measured quantities, and provider evidence used for billing.
- `price snapshot`: The immutable rates, conditions, effective time, and revision selected for one accepted request.
- `customer charge`: The monetary amount assigned to customer usage under the selected price snapshot.
- `provider cost`: The monetary amount attributed to upstream work, with its evidence and reconciliation state.
- `credit ledger`: The existing Ledger journal of monetary effects for one account, including funds, reservations, charges, and adjustments.
- `balance conservation`: The equality between an account balance and its recorded financial effects, with exact remainders and holds accounted for.
- `funds reservation`: A durable hold that reduces available customer funds before provider work starts.
- `payment receipt`: A retained record of a processor payment and its verified financial state.
- `payment inbox`: A durable collection of verified processor events awaiting application to financial records.
- `delivery outbox`: A durable collection of external delivery requests committed with the source transaction.
- `reconciliation case`: A durable record of a difference or uncertainty between local financial records and external evidence.
- `chargeback`: A payment reversal initiated through a card issuer or payment network.
- `rating`: The calculation of a customer charge or provider cost from measured quantities and an applicable price snapshot.

## Upstream Capacity Technical Nouns

- `admission allocation`: The bounded capacity for active and queued requests assigned to one upstream origin.
- `interactive reserve`: Capacity reserved for interactive requests within a shared limit.
- `upstream origin`: The normalized scheme, hostname, and optional port of an upstream HTTP endpoint.
- `work class`: The interactive, media submission, status, or transfer category of an upstream HTTP request.
- `admission telemetry`: Safe events that record admission decisions, capacity counts, and request or operation identifiers.
