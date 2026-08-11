package proxy

import (
	"fmt"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

const (
	// ModelOperationText identifies text generation through the proxy messages contract.
	ModelOperationText = "text"
	// ModelOperationDictation identifies audio transcription through the proxy dictation contract.
	ModelOperationDictation = "dictation"
)

// ModelCatalog is the canonical normalized model and provider-offering registry.
type ModelCatalog struct {
	Providers  []CatalogProvider
	Publishers []ModelPublisher
	Families   []ModelFamily
	Models     []ExactModel
	Offerings  []ProviderOffering
}

// CatalogProvider declares one provider that can own provider offerings.
type CatalogProvider struct {
	ID    string
	Label string
}

// ModelPublisher declares the organization or community that publishes models.
type ModelPublisher struct {
	ID    string
	Label string
}

// ModelFamily groups exact models from one publisher.
type ModelFamily struct {
	ID        string
	Publisher string
	Label     string
}

// ExactModel declares provider-independent identity and model capabilities.
type ExactModel struct {
	ID          string
	Publisher   string
	Family      string
	Version     string
	Operations  []string
	MediaInputs []string
}

// ProviderOffering declares one provider route for one exact model.
type ProviderOffering struct {
	Provider           string
	Model              string
	ProviderModel      string
	Operations         []string
	DefaultOperations  []string
	WireContract       string
	ExecutionLifecycle string
	RequestProfile     string
	WebSearch          bool
	OutputTokenLimit   int
	ReasoningEffort    *ReasoningEffortCapability
	MediaInputs        []string
}

// ReasoningEffortCapability declares the configured upstream mapping for one
// exact provider offering.
type ReasoningEffortCapability struct {
	Adapter string
	Efforts []string
}

type validatedModelCatalog struct {
	providers  map[string]CatalogProvider
	publishers map[string]ModelPublisher
	families   map[string]ModelFamily
	models     map[string]ExactModel
	offerings  map[string]ProviderOffering
}

var providerTextRouteCapabilities = map[string]map[textRouteCapabilities]struct{}{
	ProviderNameOpenAI:      {openAIResponsesPollableRouteCapabilities: {}},
	ProviderNameDeepSeek:    {openAIChatCompletionsSynchronousRouteCapabilities: {}},
	ProviderNameDashScope:   {openAIChatCompletionsSynchronousRouteCapabilities: {}},
	ProviderNameMoonshot:    {openAIChatCompletionsSynchronousRouteCapabilities: {}},
	ProviderNameMiniMax:     {openAIChatCompletionsSynchronousRouteCapabilities: {}},
	ProviderNameSiliconFlow: {openAIChatCompletionsSynchronousRouteCapabilities: {}},
	ProviderNameZhipu:       {openAIChatCompletionsSynchronousRouteCapabilities: {}},
	ProviderNameGemini: {
		geminiInteractionsPollableRouteCapabilities:    {},
		geminiInteractionsSynchronousRouteCapabilities: {},
	},
	ProviderNameAnthropic: {anthropicMessagesSynchronousRouteCapabilities: {}},
	ProviderNameMeta:      {openAIChatCompletionsSynchronousRouteCapabilities: {}},
	ProviderNameGrok:      {openAIChatCompletionsSynchronousRouteCapabilities: {}},
}

func validateModelCatalog(catalog ModelCatalog) (validatedModelCatalog, error) {
	validated := validatedModelCatalog{
		providers:  map[string]CatalogProvider{},
		publishers: map[string]ModelPublisher{},
		families:   map[string]ModelFamily{},
		models:     map[string]ExactModel{},
		offerings:  map[string]ProviderOffering{},
	}
	if catalogError := validateCatalogProviders(catalog.Providers, validated.providers); catalogError != nil {
		return validatedModelCatalog{}, catalogError
	}
	if catalogError := validateModelPublishers(catalog.Publishers, validated.publishers); catalogError != nil {
		return validatedModelCatalog{}, catalogError
	}
	if catalogError := validateModelFamilies(catalog.Families, validated.publishers, validated.families); catalogError != nil {
		return validatedModelCatalog{}, catalogError
	}
	if catalogError := validateExactModels(catalog.Models, validated.publishers, validated.families, validated.models); catalogError != nil {
		return validatedModelCatalog{}, catalogError
	}
	if catalogError := validateProviderOfferings(catalog.Offerings, validated); catalogError != nil {
		return validatedModelCatalog{}, catalogError
	}
	return validated, nil
}

func validateCatalogProviders(providers []CatalogProvider, validated map[string]CatalogProvider) error {
	if len(providers) == 0 {
		return fmt.Errorf("%w: field=catalog.providers", ErrInvalidModelCatalog)
	}
	for index, provider := range providers {
		identifier, identifierError := canonicalCatalogIdentifier(provider.ID, fmt.Sprintf("catalog.providers[%d].id", index))
		if identifierError != nil {
			return identifierError
		}
		if _, duplicate := validated[identifier]; duplicate {
			return fmt.Errorf("%w: field=catalog.providers[%d].id duplicate_identifier=%s", ErrInvalidModelCatalog, index, identifier)
		}
		if strings.TrimSpace(provider.Label) == constants.EmptyString || provider.Label != strings.TrimSpace(provider.Label) {
			return fmt.Errorf("%w: field=catalog.providers[%d].label", ErrInvalidModelCatalog, index)
		}
		if _, supported := providerTextRouteCapabilities[identifier]; !supported {
			return fmt.Errorf("%w: field=catalog.providers[%d].id provider=%s reason=unknown", ErrInvalidModelCatalog, index, identifier)
		}
		validated[identifier] = provider
	}
	if len(validated) != len(providerTextRouteCapabilities) {
		for identifier := range providerTextRouteCapabilities {
			if _, found := validated[identifier]; !found {
				return fmt.Errorf("%w: field=catalog.providers provider=%s reason=missing", ErrInvalidModelCatalog, identifier)
			}
		}
	}
	return nil
}

func validateModelPublishers(publishers []ModelPublisher, validated map[string]ModelPublisher) error {
	if len(publishers) == 0 {
		return fmt.Errorf("%w: field=catalog.publishers", ErrInvalidModelCatalog)
	}
	for index, publisher := range publishers {
		identifier, identifierError := canonicalCatalogIdentifier(publisher.ID, fmt.Sprintf("catalog.publishers[%d].id", index))
		if identifierError != nil {
			return identifierError
		}
		if _, duplicate := validated[identifier]; duplicate {
			return fmt.Errorf("%w: field=catalog.publishers[%d].id duplicate_identifier=%s", ErrInvalidModelCatalog, index, identifier)
		}
		if strings.TrimSpace(publisher.Label) == constants.EmptyString || publisher.Label != strings.TrimSpace(publisher.Label) {
			return fmt.Errorf("%w: field=catalog.publishers[%d].label", ErrInvalidModelCatalog, index)
		}
		validated[identifier] = publisher
	}
	return nil
}

func validateModelFamilies(families []ModelFamily, publishers map[string]ModelPublisher, validated map[string]ModelFamily) error {
	if len(families) == 0 {
		return fmt.Errorf("%w: field=catalog.families", ErrInvalidModelCatalog)
	}
	for index, family := range families {
		identifier, identifierError := canonicalCatalogIdentifier(family.ID, fmt.Sprintf("catalog.families[%d].id", index))
		if identifierError != nil {
			return identifierError
		}
		if _, duplicate := validated[identifier]; duplicate {
			return fmt.Errorf("%w: field=catalog.families[%d].id duplicate_identifier=%s", ErrInvalidModelCatalog, index, identifier)
		}
		if _, found := publishers[family.Publisher]; !found {
			return fmt.Errorf("%w: field=catalog.families[%d].publisher publisher=%s reason=dangling_reference", ErrInvalidModelCatalog, index, family.Publisher)
		}
		if strings.TrimSpace(family.Label) == constants.EmptyString || family.Label != strings.TrimSpace(family.Label) {
			return fmt.Errorf("%w: field=catalog.families[%d].label", ErrInvalidModelCatalog, index)
		}
		validated[identifier] = family
	}
	return nil
}

func validateExactModels(models []ExactModel, publishers map[string]ModelPublisher, families map[string]ModelFamily, validated map[string]ExactModel) error {
	if len(models) == 0 {
		return fmt.Errorf("%w: field=catalog.models", ErrInvalidModelCatalog)
	}
	for index, model := range models {
		identifier, identifierError := canonicalCatalogIdentifier(model.ID, fmt.Sprintf("catalog.models[%d].id", index))
		if identifierError != nil {
			return identifierError
		}
		if _, duplicate := validated[identifier]; duplicate {
			return fmt.Errorf("%w: field=catalog.models[%d].id duplicate_identifier=%s", ErrInvalidModelCatalog, index, identifier)
		}
		if _, found := publishers[model.Publisher]; !found {
			return fmt.Errorf("%w: field=catalog.models[%d].publisher publisher=%s reason=dangling_reference", ErrInvalidModelCatalog, index, model.Publisher)
		}
		family, found := families[model.Family]
		if !found || family.Publisher != model.Publisher {
			return fmt.Errorf("%w: field=catalog.models[%d].family family=%s publisher=%s reason=dangling_reference", ErrInvalidModelCatalog, index, model.Family, model.Publisher)
		}
		if strings.TrimSpace(model.Version) == constants.EmptyString || model.Version != strings.TrimSpace(model.Version) {
			return fmt.Errorf("%w: field=catalog.models[%d].version", ErrInvalidModelCatalog, index)
		}
		if _, operationError := validatedOperationSet(model.Operations, fmt.Sprintf("catalog.models[%d].operations", index)); operationError != nil {
			return operationError
		}
		if _, mediaError := validatedExactModelMediaInputs(model.MediaInputs, fmt.Sprintf("catalog.models[%d].media_inputs", index)); mediaError != nil {
			return mediaError
		}
		validated[identifier] = model
	}
	return nil
}

func validateProviderOfferings(offerings []ProviderOffering, catalog validatedModelCatalog) error {
	if len(offerings) == 0 {
		return fmt.Errorf("%w: field=catalog.offerings", ErrInvalidModelCatalog)
	}
	providerOperationDefaults := map[string]int{}
	providerOperations := map[string]map[string]struct{}{}
	modelOperations := map[string]map[string]struct{}{}
	providerNativeModels := map[string]struct{}{}
	for index, offering := range offerings {
		fieldPrefix := fmt.Sprintf("catalog.offerings[%d]", index)
		if _, found := catalog.providers[offering.Provider]; !found {
			return fmt.Errorf("%w: field=%s.provider provider=%s reason=dangling_reference", ErrInvalidModelCatalog, fieldPrefix, offering.Provider)
		}
		exactModel, found := catalog.models[offering.Model]
		if !found {
			return fmt.Errorf("%w: field=%s.model model=%s reason=dangling_reference", ErrInvalidModelCatalog, fieldPrefix, offering.Model)
		}
		providerModel := strings.TrimSpace(offering.ProviderModel)
		if providerModel == constants.EmptyString || providerModel != offering.ProviderModel {
			return fmt.Errorf("%w: field=%s.provider_model", ErrInvalidModelCatalog, fieldPrefix)
		}
		offeringIdentifier := providerOfferingIdentifier(offering.Provider, offering.Model)
		if _, duplicate := catalog.offerings[offeringIdentifier]; duplicate {
			return fmt.Errorf("%w: field=%s route_conflict=%s", ErrInvalidModelCatalog, fieldPrefix, offeringIdentifier)
		}
		nativeIdentifier := strings.ToLower(offering.Provider + "\x00" + providerModel)
		if _, duplicate := providerNativeModels[nativeIdentifier]; duplicate {
			return fmt.Errorf("%w: field=%s provider_native_model_conflict=%s", ErrInvalidModelCatalog, fieldPrefix, providerModel)
		}
		providerNativeModels[nativeIdentifier] = struct{}{}
		offeredOperations, operationError := validatedOperationSet(offering.Operations, fieldPrefix+".operations")
		if operationError != nil {
			return operationError
		}
		modelOperationSet, _ := validatedOperationSet(exactModel.Operations, "catalog.models.operations")
		for operation := range offeredOperations {
			if _, supported := modelOperationSet[operation]; !supported {
				return fmt.Errorf("%w: field=%s.operations operation=%s reason=unsupported_by_model", ErrInvalidModelCatalog, fieldPrefix, operation)
			}
			if providerOperations[offering.Provider] == nil {
				providerOperations[offering.Provider] = map[string]struct{}{}
			}
			providerOperations[offering.Provider][operation] = struct{}{}
			if modelOperations[offering.Model] == nil {
				modelOperations[offering.Model] = map[string]struct{}{}
			}
			modelOperations[offering.Model][operation] = struct{}{}
		}
		defaultOperations, defaultError := validatedOperationSet(offering.DefaultOperations, fieldPrefix+".default_operations")
		if defaultError != nil && len(offering.DefaultOperations) != 0 {
			return defaultError
		}
		for operation := range defaultOperations {
			if _, offered := offeredOperations[operation]; !offered {
				return fmt.Errorf("%w: field=%s.default_operations operation=%s reason=unsupported_by_offering", ErrInvalidModelCatalog, fieldPrefix, operation)
			}
			providerOperationDefaults[offering.Provider+"\x00"+operation]++
		}
		if offering.OutputTokenLimit < 0 {
			return fmt.Errorf("%w: field=%s.output_token_limit", ErrInvalidModelCatalog, fieldPrefix)
		}
		if _, supportsText := offeredOperations[ModelOperationText]; supportsText {
			if routeError := validateTextOffering(offering, fieldPrefix); routeError != nil {
				return routeError
			}
		} else if offering.WireContract != constants.EmptyString || offering.ExecutionLifecycle != constants.EmptyString || offering.RequestProfile != constants.EmptyString || offering.WebSearch || offering.OutputTokenLimit != 0 || offering.ReasoningEffort != nil || len(offering.MediaInputs) != 0 {
			return fmt.Errorf("%w: field=%s reason=text_capabilities_without_text_operation", ErrInvalidModelCatalog, fieldPrefix)
		}
		modelMediaInputs, _ := validatedExactModelMediaInputs(exactModel.MediaInputs, "catalog.models.media_inputs")
		offeringMediaInputs, mediaError := validatedMediaInputSet(offering.Provider, offering.MediaInputs, fieldPrefix+".media_inputs")
		if mediaError != nil {
			return mediaError
		}
		for mediaInput := range offeringMediaInputs {
			if _, supported := modelMediaInputs[mediaInput]; !supported {
				return fmt.Errorf("%w: field=%s.media_inputs media_input=%s reason=unsupported_by_model", ErrInvalidModelCatalog, fieldPrefix, mediaInput)
			}
		}
		catalog.offerings[offeringIdentifier] = offering
	}
	for provider, operations := range providerOperations {
		for operation := range operations {
			if providerOperationDefaults[provider+"\x00"+operation] != 1 {
				return fmt.Errorf("%w: provider=%s operation=%s default_count=%d", ErrInvalidModelCatalog, provider, operation, providerOperationDefaults[provider+"\x00"+operation])
			}
		}
	}
	for modelIdentifier, exactModel := range catalog.models {
		exactOperations, _ := validatedOperationSet(exactModel.Operations, "catalog.models.operations")
		for operation := range exactOperations {
			if _, found := modelOperations[modelIdentifier][operation]; !found {
				return fmt.Errorf("%w: model=%s operation=%s reason=missing_provider_offering", ErrInvalidModelCatalog, modelIdentifier, operation)
			}
		}
	}
	return nil
}

func validateTextOffering(offering ProviderOffering, fieldPrefix string) error {
	capabilities, capabilityError := validatedTextRouteCapabilities(offering.Provider, offering, fieldPrefix)
	if capabilityError != nil {
		return capabilityError
	}
	if _, allowed := providerTextRouteCapabilities[offering.Provider][capabilities]; !allowed {
		return fmt.Errorf("%w: provider=%s model=%s wire_contract=%s execution_lifecycle=%s", ErrInvalidModelCatalog, offering.Provider, offering.Model, offering.WireContract, offering.ExecutionLifecycle)
	}
	if offering.WebSearch && offering.Provider != ProviderNameOpenAI {
		return fmt.Errorf("%w: field=%s.web_search provider=%s", ErrInvalidModelCatalog, fieldPrefix, offering.Provider)
	}
	if offering.Provider == ProviderNameAnthropic && offering.OutputTokenLimit <= 0 {
		return fmt.Errorf("%w: field=%s.output_token_limit provider=%s", ErrInvalidModelCatalog, fieldPrefix, offering.Provider)
	}
	if profileError := validateOfferingRequestProfile(offering); profileError != nil {
		return fmt.Errorf("%w: field=%s.request_profile", profileError, fieldPrefix)
	}
	reasoningEffort, reasoningError := validatedReasoningEffortCapability(offering.ReasoningEffort, fieldPrefix+".reasoning_effort")
	if reasoningError != nil {
		return reasoningError
	}
	if reasoningEffort != nil && (reasoningEffort.adapter != reasoningEffortAdapterOpenAIResponses || offering.Provider != ProviderNameOpenAI || modelRequestProfile(offering.RequestProfile) != requestProfileOpenAIResponsesReasoningTools) {
		return fmt.Errorf("%w: field=%s.reasoning_effort adapter=%s", ErrInvalidModelCatalog, fieldPrefix, reasoningEffort.adapter)
	}
	return nil
}

func validatedTextRouteCapabilities(providerName string, offering ProviderOffering, fieldPrefix string) (textRouteCapabilities, error) {
	if offering.WireContract == constants.EmptyString || offering.WireContract != strings.TrimSpace(offering.WireContract) {
		return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s field=%s.wire_contract", ErrInvalidModelCatalog, providerName, fieldPrefix)
	}
	if offering.ExecutionLifecycle == constants.EmptyString || offering.ExecutionLifecycle != strings.TrimSpace(offering.ExecutionLifecycle) {
		return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s field=%s.execution_lifecycle", ErrInvalidModelCatalog, providerName, fieldPrefix)
	}
	capabilities := textRouteCapabilities{
		wireContract:       textWireContract(offering.WireContract),
		executionLifecycle: textExecutionLifecycle(offering.ExecutionLifecycle),
	}
	if !knownTextWireContract(capabilities.wireContract) {
		return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s field=%s.wire_contract wire_contract=%s", ErrInvalidModelCatalog, providerName, fieldPrefix, offering.WireContract)
	}
	if !knownTextExecutionLifecycle(capabilities.executionLifecycle) {
		return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s field=%s.execution_lifecycle execution_lifecycle=%s", ErrInvalidModelCatalog, providerName, fieldPrefix, offering.ExecutionLifecycle)
	}
	return capabilities, nil
}

func canonicalCatalogIdentifier(rawIdentifier string, field string) (string, error) {
	identifier := strings.TrimSpace(rawIdentifier)
	if identifier == constants.EmptyString || identifier != rawIdentifier || identifier != strings.ToLower(identifier) {
		return constants.EmptyString, fmt.Errorf("%w: field=%s identifier=%s reason=not_canonical", ErrInvalidModelCatalog, field, rawIdentifier)
	}
	return identifier, nil
}

func providerOfferingIdentifier(providerIdentifier string, modelIdentifier string) string {
	return providerIdentifier + ":" + modelIdentifier
}

func validatedOperationSet(rawOperations []string, field string) (map[string]struct{}, error) {
	if len(rawOperations) == 0 {
		return nil, fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
	}
	operations := make(map[string]struct{}, len(rawOperations))
	for index, operation := range rawOperations {
		if operation != ModelOperationText && operation != ModelOperationDictation {
			return nil, fmt.Errorf("%w: field=%s[%d] operation=%s", ErrInvalidModelCatalog, field, index, operation)
		}
		if _, duplicate := operations[operation]; duplicate {
			return nil, fmt.Errorf("%w: field=%s[%d] duplicate=%s", ErrInvalidModelCatalog, field, index, operation)
		}
		operations[operation] = struct{}{}
	}
	return operations, nil
}

func validatedExactModelMediaInputs(rawMediaInputs []string, field string) (map[messageMediaType]struct{}, error) {
	mediaInputs := make(map[messageMediaType]struct{}, len(rawMediaInputs))
	for index, rawMediaInput := range rawMediaInputs {
		mediaInput := messageMediaType(rawMediaInput)
		if mediaInput != messageMediaTypeImage && mediaInput != messageMediaTypeAudio {
			return nil, fmt.Errorf("%w: field=%s[%d] media_input=%s", ErrInvalidModelCatalog, field, index, rawMediaInput)
		}
		if _, duplicate := mediaInputs[mediaInput]; duplicate {
			return nil, fmt.Errorf("%w: field=%s[%d] duplicate=%s", ErrInvalidModelCatalog, field, index, rawMediaInput)
		}
		mediaInputs[mediaInput] = struct{}{}
	}
	return mediaInputs, nil
}

func knownTextWireContract(wireContract textWireContract) bool {
	switch wireContract {
	case textWireContractOpenAIResponses, textWireContractOpenAIChatCompletions, textWireContractGeminiInteractions, textWireContractAnthropicMessages:
		return true
	default:
		return false
	}
}

func knownTextExecutionLifecycle(executionLifecycle textExecutionLifecycle) bool {
	switch executionLifecycle {
	case textExecutionLifecycleSynchronousCompletion, textExecutionLifecyclePollableResource:
		return true
	default:
		return false
	}
}

func validatedReasoningEffortCapability(rawCapability *ReasoningEffortCapability, fieldPrefix string) (*reasoningEffortCapability, error) {
	if rawCapability == nil {
		return nil, nil
	}
	adapter := reasoningEffortAdapter(strings.TrimSpace(rawCapability.Adapter))
	if adapter == reasoningEffortAdapterNone {
		return nil, fmt.Errorf("%w: field=%s.adapter", ErrInvalidModelCatalog, fieldPrefix)
	}
	if !knownReasoningEffortAdapter(adapter) {
		return nil, fmt.Errorf("%w: field=%s.adapter adapter=%s", ErrInvalidModelCatalog, fieldPrefix, adapter)
	}
	if len(rawCapability.Efforts) == 0 {
		return nil, fmt.Errorf("%w: field=%s.efforts", ErrInvalidModelCatalog, fieldPrefix)
	}
	efforts := make([]string, 0, len(rawCapability.Efforts))
	seenEfforts := map[string]struct{}{}
	for effortIndex, effort := range rawCapability.Efforts {
		if effort == constants.EmptyString || effort != strings.TrimSpace(effort) || !reasoningEffortAdapterSupports(adapter, effort) {
			return nil, fmt.Errorf("%w: field=%s.efforts[%d] effort=%s", ErrInvalidModelCatalog, fieldPrefix, effortIndex, effort)
		}
		if _, duplicate := seenEfforts[effort]; duplicate {
			return nil, fmt.Errorf("%w: field=%s.efforts[%d] effort=%s", ErrInvalidModelCatalog, fieldPrefix, effortIndex, effort)
		}
		seenEfforts[effort] = struct{}{}
		efforts = append(efforts, effort)
	}
	return &reasoningEffortCapability{adapter: adapter, efforts: efforts}, nil
}

func validateOfferingRequestProfile(offering ProviderOffering) error {
	requestProfile := strings.TrimSpace(offering.RequestProfile)
	if offering.Provider != ProviderNameOpenAI {
		if requestProfile != constants.EmptyString {
			return fmt.Errorf("%w: provider=%s profile=%s", ErrInvalidModelCatalog, offering.Provider, requestProfile)
		}
		return nil
	}
	if requestProfile == constants.EmptyString || !knownModelRequestProfile(modelRequestProfile(requestProfile)) {
		return fmt.Errorf("%w: provider=%s profile=%s", ErrInvalidModelCatalog, offering.Provider, requestProfile)
	}
	return nil
}

func knownModelRequestProfile(requestProfile modelRequestProfile) bool {
	switch requestProfile {
	case requestProfileOpenAIResponsesTemperature, requestProfileOpenAIResponsesTemperatureTools, requestProfileOpenAIResponsesReasoningTools:
		return true
	default:
		return false
	}
}

func validatedMediaInputSet(providerName string, rawMediaInputs []string, fieldPrefix string) (map[messageMediaType]struct{}, error) {
	mediaInputs := make(map[messageMediaType]struct{}, len(rawMediaInputs))
	for mediaInputIndex, rawMediaInput := range rawMediaInputs {
		mediaInput := messageMediaType(rawMediaInput)
		if rawMediaInput == constants.EmptyString || rawMediaInput != strings.TrimSpace(rawMediaInput) || !providerSupportsMessageMedia(providerName, mediaInput) {
			return nil, fmt.Errorf("%w: field=%s[%d] provider=%s media_input=%s", ErrInvalidModelCatalog, fieldPrefix, mediaInputIndex, providerName, rawMediaInput)
		}
		if _, duplicate := mediaInputs[mediaInput]; duplicate {
			return nil, fmt.Errorf("%w: field=%s[%d] duplicate=%s", ErrInvalidModelCatalog, fieldPrefix, mediaInputIndex, rawMediaInput)
		}
		mediaInputs[mediaInput] = struct{}{}
	}
	return mediaInputs, nil
}

func configuredMediaInputSet(rawMediaInputs []string) map[messageMediaType]struct{} {
	mediaInputs := make(map[messageMediaType]struct{}, len(rawMediaInputs))
	for _, rawMediaInput := range rawMediaInputs {
		mediaInputs[messageMediaType(rawMediaInput)] = struct{}{}
	}
	return mediaInputs
}

func configuredReasoningEffortCapability(configuration *ReasoningEffortCapability) *reasoningEffortCapability {
	if configuration == nil {
		return nil
	}
	return &reasoningEffortCapability{adapter: reasoningEffortAdapter(strings.TrimSpace(configuration.Adapter)), efforts: append([]string(nil), configuration.Efforts...)}
}

func offeringSupportsOperation(offering ProviderOffering, operation string) bool {
	for _, configuredOperation := range offering.Operations {
		if configuredOperation == operation {
			return true
		}
	}
	return false
}

func offeringDefaultsOperation(offering ProviderOffering, operation string) bool {
	for _, defaultOperation := range offering.DefaultOperations {
		if defaultOperation == operation {
			return true
		}
	}
	return false
}

func (requestProfile modelRequestProfile) string() string {
	return string(requestProfile)
}
