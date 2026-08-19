# Repository Terminology

This file contains approved technical nouns and technical verbs for repository documentation.

Use this file with ASD-STE100 Simplified Technical English, Issue 9.
Give each term one meaning. Use the same term for the same concept in all documents.

## Repository Technical Nouns

- `activation issue`: An issue that changes an external runtime to use completed development work.
- `development completion`: Completion of the specified repository changes and repository validation.
- `exact model`: One canonical model version that a client can select.
- `GPU lease`: An exclusive permit for one runtime to use one GPU during an operation.
- `inference node`: A host that runs local model containers and the node controller.
- `local offering`: A provider offering that runs on an inference node.
- `model family`: A group of exact models from one model publisher.
- `model publisher`: An organization or community that creates or releases a model.
- `model residency`: The state in which a model uses GPU memory.
- `node controller`: A private service that controls request admission and runtime container lifecycle on one inference node.
- `weight access`: A model family classification of proprietary or open weights.
- `protocol adapter`: Code that translates canonical requests and responses for one reusable provider protocol.
- `provider catalog`: The canonical YAML file that defines all supported models, providers, provider offerings, controls, limits, and prices.
- `provider connection`: Tenant values for the credential fields and setting fields in one provider definition.
- `provider definition`: One provider record in the provider catalog.
- `provider offering`: One exact model that one provider makes available as a route.
- `request intent`: The tenant-bound semantic inputs that one idempotency key identifies.
- `structured request`: A canonical text request that requires one caller JSON Schema for its output.
- `production acceptance`: Evidence that the production runtime satisfies the checks that an issue specifies.
- `route explorer`: The public interface that selects an exact model and a provider offering.
- `runtime profile`: A validated declaration for one model runtime, container image, private endpoint, resource limit, and idle policy.
