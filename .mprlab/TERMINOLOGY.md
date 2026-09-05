# Repository Terminology

This file contains approved technical nouns and technical verbs for repository documentation.

Use this file with ASD-STE100 Simplified Technical English, Issue 9.
Give each term one meaning. Use the same term for the same concept in all documents.

## Repository Technical Nouns

- `activation issue`: An issue that changes an external runtime to use completed development work.
- `client protocol adapter`: Code that translates one public client protocol to and from canonical request and result types.
- `development completion`: Completion of the specified repository changes and repository validation.
- `exact model`: One canonical model version that a client can select.
- `GPU lease`: An exclusive permit for one runtime to use one GPU during an operation.
- `idempotency tombstone`: A retained identity and request record that prevents duplicate work after artifact deletion.
- `inference node`: A host that runs local model containers and the node controller.
- `local offering`: A provider offering that runs on an inference node.
- `media operation`: Accepted tenant work with durable execution state and media input or output resources.
- `model family`: A group of exact models from one model publisher.
- `model publisher`: An organization or community that creates or releases a model.
- `model residency`: The state in which a model uses GPU memory.
- `node controller`: A private service that controls request admission and runtime container lifecycle on one inference node.
- `weight access`: A model family classification of proprietary or open weights.
- `protocol adapter`: Code that translates canonical requests and responses for one reusable provider protocol.
- `provider catalog`: The canonical YAML file that defines all supported models, providers, provider offerings, controls, limits, and prices.
- `provider connection`: Tenant values for the credential fields and setting fields in one provider definition.
- `provider definition`: One provider record in the provider catalog.
- `provider field`: One credential input or setting input in a provider definition.
- `provider gateway`: The service that authorizes tenant requests and owns shared provider access.
- `provider offering`: One exact model that one provider makes available as a route.
- `provider profile`: Tenant settings for one provider, such as the selected text model and system prompt.
- `provider staging`: Temporary storage that lets a provider read media for an accepted operation.
- `provider transport`: One provider route that defines an endpoint, authentication, protocol adapter, and lifecycle.
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
