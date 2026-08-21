package proxy

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

type providerRegistry struct {
	definitions map[providerID]providerDefinition
	aliases     map[string]providerID
}

type providerSummary struct {
	identifier            string
	label                 string
	aliases               []string
	textDefaultModel      string
	textModels            []textModelSummary
	supportsDictation     bool
	dictationDefaultModel string
	dictationModels       []string
}

type textModelSummary struct {
	identifier      string
	reasoningEffort *reasoningEffortCapability
}

func newProviderRegistry(configuration Configuration) *providerRegistry {
	definitions := make(map[providerID]providerDefinition, len(configuration.ProviderCatalog.schema.Providers))
	for _, provider := range configuration.ProviderCatalog.schema.Providers {
		identifier := providerID(provider.ID)
		definition := providerDefinition{
			identifier:          identifier,
			label:               provider.Label,
			aliases:             append([]string(nil), provider.Aliases...),
			fields:              make(map[string]ProviderCatalogField, len(provider.Fields)),
			fieldOrder:          make([]string, 0, len(provider.Fields)),
			connectionValues:    make(map[string]string, len(provider.Fields)),
			transports:          make(map[string]providerTransportDefinition, len(provider.Transports)),
			textModels:          map[string]textModelDefinition{},
			transcriptionModels: map[string]dictationModelDefinition{},
		}
		for _, field := range provider.Fields {
			definition.fields[field.ID] = field
			definition.fieldOrder = append(definition.fieldOrder, field.ID)
			definition.connectionValues[field.ID] = *field.Default
		}
		for fieldIdentifier, value := range configuration.ProviderConnectionValues[provider.ID] {
			definition.connectionValues[fieldIdentifier] = value
		}
		for _, transport := range provider.Transports {
			definition.transports[transport.ID] = providerTransportDefinition{
				identifier:         transport.ID,
				endpoint:           transport.Endpoint,
				authentication:     transport.Authentication,
				headers:            append([]ProviderCatalogHeader(nil), transport.Headers...),
				requestProtocol:    transport.RequestProtocol,
				responseProtocol:   transport.ResponseProtocol,
				usageMapping:       transport.UsageMapping,
				lifecycle:          textExecutionLifecycle(transport.Lifecycle),
				protocolParameters: transport.ProtocolParameters,
			}
		}
		for _, offering := range provider.Offerings {
			transport := definition.transports[offering.Transport]
			if slices.Contains(offering.Operations, ModelOperationText) {
				routeCapabilities := textRouteCapabilities{
					wireContract:       textWireContract(transport.requestProtocol),
					executionLifecycle: transport.lifecycle,
				}
				definition.textModels[strings.ToLower(offering.Model)] = textModelDefinition{
					identifier:          modelID(offering.Model),
					providerIdentifier:  modelID(offering.UpstreamModel),
					transportIdentifier: offering.Transport,
					wireContract:        routeCapabilities.wireContract,
					executionLifecycle:  routeCapabilities.executionLifecycle,
					routeAdapter:        textRouteAdapters[routeCapabilities],
					requestProfile:      modelRequestProfile(offering.RequestProfile),
					supportsWebSearch:   offering.WebSearch,
					outputTokenLimit:    offering.OutputTokenLimit,
					hasOutputTokenLimit: offering.OutputTokenLimit > 0,
					reasoningEffort:     configuredReasoningEffortCapability(offering.ReasoningEffort),
					mediaInputs:         configuredMediaInputSet(offering.MediaInputs),
					mediaLimits:         cloneCatalogMediaLimits(offering.MediaLimits),
				}
				if slices.Contains(offering.DefaultOperations, ModelOperationText) {
					definition.defaultTextModel = modelID(offering.Model)
				}
			}
			if slices.Contains(offering.Operations, ModelOperationDictation) {
				definition.transcriptionModels[strings.ToLower(offering.Model)] = dictationModelDefinition{
					identifier:          modelID(offering.Model),
					providerIdentifier:  modelID(offering.UpstreamModel),
					transportIdentifier: offering.Transport,
				}
				definition.supportsDictation = true
				if slices.Contains(offering.DefaultOperations, ModelOperationDictation) {
					definition.defaultTranscriptionModel = modelID(offering.Model)
				}
			}
		}
		definitions[identifier] = definition
	}
	applyDefaultEndpointOverrides(configuration.ProviderCatalog.schema, definitions, configuration.Endpoints)

	registry := &providerRegistry{
		definitions: definitions,
		aliases:     map[string]providerID{},
	}
	for identifier, definition := range definitions {
		registry.aliases[identifier.string()] = identifier
		for _, alias := range definition.aliases {
			normalizedAlias := strings.ToLower(strings.TrimSpace(alias))
			if normalizedAlias != constants.EmptyString {
				registry.aliases[normalizedAlias] = identifier
			}
		}
	}
	return registry
}

func applyDefaultEndpointOverrides(schema ProviderCatalogSchema, definitions map[providerID]providerDefinition, endpoints *Endpoints) {
	if endpoints == nil {
		return
	}
	operationEndpoints := map[string]string{
		ModelOperationText:      endpoints.GetResponsesURL(),
		ModelOperationDictation: endpoints.GetTranscriptionsURL(),
	}
	overriddenOperations := map[string]struct{}{}
	for _, provider := range schema.Providers {
		definition := definitions[providerID(provider.ID)]
		for transportIdentifier, transport := range definition.transports {
			if endpointURL := endpoints.providerTransportEndpoint(provider.ID, transportIdentifier, transport.endpoint); endpointURL != constants.EmptyString {
				transport.endpointURLOverride = endpointURL
				definition.transports[transportIdentifier] = transport
			}
		}
		for _, offering := range provider.Offerings {
			for _, operation := range offering.DefaultOperations {
				if _, overridden := overriddenOperations[operation]; overridden {
					continue
				}
				endpointURL := operationEndpoints[operation]
				if strings.TrimSpace(endpointURL) == constants.EmptyString {
					continue
				}
				transport := definition.transports[offering.Transport]
				if transport.endpoint.SettingField != constants.EmptyString {
					continue
				}
				if transport.endpointURLOverride == constants.EmptyString {
					transport.endpointURLOverride = endpointURL
					definition.transports[offering.Transport] = transport
				}
				overriddenOperations[operation] = struct{}{}
			}
		}
		definitions[definition.identifier] = definition
	}
}

func (registry *providerRegistry) forTenant(requestTenant tenant) *providerRegistry {
	definitions := make(map[providerID]providerDefinition, len(registry.definitions))
	for identifier, definition := range registry.definitions {
		definition.connectionValues = cloneStringMap(definition.connectionValues)
		if providerSettings, configured := requestTenant.providerSettings[identifier]; configured {
			for fieldIdentifier, value := range providerSettings.connectionValues {
				definition.connectionValues[fieldIdentifier] = value
			}
		}
		definitions[identifier] = definition
	}
	return &providerRegistry{
		definitions: definitions,
		aliases:     registry.aliases,
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func (registry *providerRegistry) canonicalProviderID(rawProvider string) (providerID, error) {
	definition, providerError := registry.resolveProvider(rawProvider, constants.EmptyString)
	if providerError != nil {
		return providerID(""), providerError
	}
	return definition.identifier, nil
}

func (registry *providerRegistry) providerSummaries() []providerSummary {
	identifiers := make([]string, 0, len(registry.definitions))
	identifierLookup := map[string]providerID{}
	for identifier := range registry.definitions {
		identifierString := identifier.string()
		identifiers = append(identifiers, identifierString)
		identifierLookup[identifierString] = identifier
	}
	sort.Strings(identifiers)
	summaries := make([]providerSummary, 0, len(identifiers))
	for _, identifierString := range identifiers {
		identifier := identifierLookup[identifierString]
		definition := registry.definitions[identifier]
		aliases := append([]string(nil), definition.aliases...)
		sort.Strings(aliases)
		summaries = append(summaries, providerSummary{
			identifier:            definition.identifier.string(),
			label:                 definition.label,
			aliases:               aliases,
			textDefaultModel:      definition.defaultTextModel.string(),
			textModels:            sortedTextModelSummaries(definition.textModels),
			supportsDictation:     definition.supportsDictation,
			dictationDefaultModel: definition.defaultTranscriptionModel.string(),
			dictationModels:       sortedDictationModels(definition.transcriptionModels),
		})
	}
	return summaries
}

func (registry *providerRegistry) resolveProvider(rawProvider string, defaultProvider string) (providerDefinition, error) {
	providerCandidate := strings.TrimSpace(rawProvider)
	if providerCandidate == constants.EmptyString {
		providerCandidate = defaultProvider
	}
	normalizedProvider := newProviderID(providerCandidate)
	canonicalIdentifier, foundAlias := registry.aliases[normalizedProvider.string()]
	if !foundAlias {
		return providerDefinition{}, fmt.Errorf("%w: %s", ErrUnknownProvider, normalizedProvider.string())
	}
	return registry.definitions[canonicalIdentifier], nil
}

func validateReasoningEffortForResolvedTextRoute(provider providerDefinition, model textModelDefinition, effort string) error {
	if effort == constants.EmptyString {
		return nil
	}
	if !model.reasoningEffort.supports(effort) {
		return unsupportedReasoningEffortError(provider, model, effort)
	}
	return nil
}

func unsupportedReasoningEffortError(provider providerDefinition, model textModelDefinition, effort string) error {
	return fmt.Errorf("%w: provider=%s model=%s capability=reasoning_effort effort=%s", ErrUnsupportedCapability, provider.identifier.string(), model.string(), effort)
}

func (registry *providerRegistry) resolveTextRequest(rawProvider string, rawModel string, defaultProvider string, defaultModel string, webSearchEnabled bool) (providerDefinition, textModelDefinition, error) {
	definition, resolvedModel, resolutionError := registry.resolveTextModel(rawProvider, rawModel, defaultProvider, defaultModel, webSearchEnabled)
	if resolutionError != nil {
		return providerDefinition{}, textModelDefinition{}, resolutionError
	}
	if definition.credentialFor(endpointKindText) == constants.EmptyString || definition.textEndpointURL == constants.EmptyString {
		return providerDefinition{}, textModelDefinition{}, fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, definition.identifier.string(), endpointKindText)
	}
	return definition, resolvedModel, nil
}

func (registry *providerRegistry) resolveTextModel(rawProvider string, rawModel string, defaultProvider string, defaultModel string, webSearchEnabled bool) (providerDefinition, textModelDefinition, error) {
	definition, providerError := registry.resolveProvider(rawProvider, defaultProvider)
	if providerError != nil {
		return providerDefinition{}, textModelDefinition{}, providerError
	}
	modelIdentifier := strings.TrimSpace(rawModel)
	if modelIdentifier == constants.EmptyString {
		if strings.TrimSpace(defaultModel) != constants.EmptyString {
			modelIdentifier = defaultModel
		} else {
			modelIdentifier = definition.defaultTextModel.string()
		}
	}
	resolvedModel, modelError := resolveTextModelFromSet(definition.textModels, modelIdentifier)
	if modelError != nil {
		return providerDefinition{}, textModelDefinition{}, modelError
	}
	if webSearchEnabled && !resolvedModel.supportsWebSearch {
		return providerDefinition{}, textModelDefinition{}, fmt.Errorf("%w: provider=%s model=%s capability=web_search", ErrUnsupportedCapability, definition.identifier.string(), resolvedModel.string())
	}
	definition, _ = definition.resolvedTransport(resolvedModel.transportIdentifier)
	return definition, resolvedModel, nil
}

func (registry *providerRegistry) resolveDictationRequest(rawProvider string, rawModel string, defaultProvider string, defaultModel string) (providerDefinition, modelID, error) {
	definition, resolvedModel, resolutionError := registry.resolveDictationModel(rawProvider, rawModel, defaultProvider, defaultModel)
	if resolutionError != nil {
		return providerDefinition{}, modelID(""), resolutionError
	}
	if definition.credentialFor(endpointKindDictation) == constants.EmptyString || definition.transcriptionsURL == constants.EmptyString {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, definition.identifier.string(), endpointKindDictation)
	}
	return definition, resolvedModel, nil
}

func (registry *providerRegistry) resolveDictationModel(rawProvider string, rawModel string, defaultProvider string, defaultModel string) (providerDefinition, modelID, error) {
	definition, providerError := registry.resolveProvider(rawProvider, defaultProvider)
	if providerError != nil {
		return providerDefinition{}, modelID(""), providerError
	}
	if !definition.supportsDictation {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, definition.identifier.string(), endpointKindDictation)
	}
	modelIdentifier := strings.TrimSpace(rawModel)
	if modelIdentifier == constants.EmptyString {
		if strings.TrimSpace(rawProvider) == constants.EmptyString && strings.TrimSpace(defaultModel) != constants.EmptyString {
			modelIdentifier = defaultModel
		} else {
			modelIdentifier = definition.defaultTranscriptionModel.string()
		}
	}
	resolvedModel, modelError := resolveModelFromSet(definition.transcriptionModels, modelIdentifier)
	if modelError != nil {
		return providerDefinition{}, modelID(""), modelError
	}
	definition, _ = definition.resolvedTransport(definition.transcriptionModels[strings.ToLower(resolvedModel.string())].transportIdentifier)
	return definition, resolvedModel, nil
}

func resolveModelFromSet(modelIdentifiers map[string]dictationModelDefinition, rawModel string) (modelID, error) {
	resolvedModel := newModelID(rawModel)
	if modelDefinition, known := modelIdentifiers[strings.ToLower(resolvedModel.string())]; known {
		return modelDefinition.identifier, nil
	}
	return modelID(""), fmt.Errorf("%w: %s", ErrUnknownModel, resolvedModel.string())
}

func resolveTextModelFromSet(modelIdentifiers map[string]textModelDefinition, rawModel string) (textModelDefinition, error) {
	resolvedModel := newModelID(rawModel)
	if modelIdentifier, known := modelIdentifiers[strings.ToLower(resolvedModel.string())]; known {
		return modelIdentifier, nil
	}
	return textModelDefinition{}, fmt.Errorf("%w: %s", ErrUnknownModel, resolvedModel.string())
}

func sortedTextModelSummaries(modelIdentifiers map[string]textModelDefinition) []textModelSummary {
	modelsByIdentifier := map[string]textModelSummary{}
	modelIdentifiersByName := make([]string, 0, len(modelIdentifiers))
	for _, modelDefinition := range modelIdentifiers {
		modelIdentifier := modelDefinition.string()
		if _, seen := modelsByIdentifier[modelIdentifier]; seen {
			continue
		}
		modelsByIdentifier[modelIdentifier] = textModelSummary{
			identifier:      modelIdentifier,
			reasoningEffort: modelDefinition.reasoningEffort,
		}
		modelIdentifiersByName = append(modelIdentifiersByName, modelIdentifier)
	}
	sort.Strings(modelIdentifiersByName)
	models := make([]textModelSummary, 0, len(modelIdentifiersByName))
	for _, modelIdentifier := range modelIdentifiersByName {
		models = append(models, modelsByIdentifier[modelIdentifier])
	}
	return models
}

func sortedDictationModels(modelIdentifiers map[string]dictationModelDefinition) []string {
	models := make([]string, 0, len(modelIdentifiers))
	seenModels := map[string]struct{}{}
	for _, modelDefinition := range modelIdentifiers {
		modelIdentifierString := modelDefinition.identifier.string()
		if _, seen := seenModels[modelIdentifierString]; seen {
			continue
		}
		seenModels[modelIdentifierString] = struct{}{}
		models = append(models, modelIdentifierString)
	}
	sort.Strings(models)
	return models
}
