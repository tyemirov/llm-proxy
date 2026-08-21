package proxy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

const (
	// ModelOperationText identifies text generation through the proxy messages contract.
	ModelOperationText = "text"
	// ModelOperationDictation identifies audio transcription through the proxy dictation contract.
	ModelOperationDictation = "dictation"
	// ModelOperationVideoGeneration identifies provider-backed video generation.
	ModelOperationVideoGeneration = "video_generation"
	// CatalogCredentialAPIKey identifies one opaque provider API key.
	CatalogCredentialAPIKey = "api_key"
	// CatalogArtifactText identifies text input or output.
	CatalogArtifactText = "text"
	// CatalogArtifactImage identifies image input or output.
	CatalogArtifactImage = "image"
	// CatalogArtifactAudio identifies audio input or output.
	CatalogArtifactAudio = "audio"
	// CatalogArtifactVideo identifies video input or output.
	CatalogArtifactVideo = "video"
	// CatalogWireContractMultipartTranscription identifies normalized multipart dictation.
	CatalogWireContractMultipartTranscription = "multipart_transcription"
)

// ModelCatalog is the canonical normalized model and provider-offering registry.
type ModelCatalog struct {
	Revision   string                   `mapstructure:"revision"`
	Operations []ModelOperationKind     `mapstructure:"operations"`
	Providers  []CatalogProvider        `mapstructure:"providers"`
	Publishers []ModelPublisher         `mapstructure:"publishers"`
	Families   []ModelFamily            `mapstructure:"families"`
	Models     []ExactModel             `mapstructure:"models"`
	Offerings  []ProviderOffering       `mapstructure:"offerings"`
	Prices     []CatalogPriceDescriptor `mapstructure:"prices"`
}

// ModelOperationKind declares one operation and its possible artifact types.
type ModelOperationKind struct {
	ID              string   `json:"id" mapstructure:"id" yaml:"id"`
	InputArtifacts  []string `json:"input_artifacts" mapstructure:"input_artifacts" yaml:"input_artifacts"`
	OutputArtifacts []string `json:"output_artifacts" mapstructure:"output_artifacts" yaml:"output_artifacts"`
}

// CatalogProvider declares one provider that can own provider offerings.
type CatalogProvider struct {
	ID              string   `mapstructure:"id"`
	Label           string   `mapstructure:"label"`
	CredentialKinds []string `mapstructure:"credential_kinds"`
}

// ModelPublisher declares the organization or community that publishes models.
type ModelPublisher struct {
	ID    string `mapstructure:"id" yaml:"id"`
	Label string `mapstructure:"label" yaml:"label"`
}

// ModelFamily groups exact models from one publisher.
type ModelFamily struct {
	ID           string `mapstructure:"id" yaml:"id"`
	Publisher    string `mapstructure:"publisher" yaml:"publisher"`
	Label        string `mapstructure:"label" yaml:"label"`
	WeightAccess string `mapstructure:"weight_access" yaml:"weight_access"`
}

const (
	// ModelWeightAccessProprietary identifies families whose model weights are not published for independent deployment.
	ModelWeightAccessProprietary = "proprietary"
	// ModelWeightAccessOpenWeights identifies families with published weights for independent deployment.
	ModelWeightAccessOpenWeights = "open_weights"
)

// ExactModel declares provider-independent identity and model capabilities.
type ExactModel struct {
	ID          string   `mapstructure:"id" yaml:"id"`
	Publisher   string   `mapstructure:"publisher" yaml:"publisher"`
	Family      string   `mapstructure:"family" yaml:"family"`
	Version     string   `mapstructure:"version" yaml:"version"`
	Operations  []string `mapstructure:"operations" yaml:"operations"`
	MediaInputs []string `mapstructure:"media_inputs" yaml:"media_inputs"`
}

// ProviderOffering declares one provider route for one exact model.
type ProviderOffering struct {
	Provider           string                     `mapstructure:"provider"`
	Model              string                     `mapstructure:"model"`
	ProviderModel      string                     `mapstructure:"provider_model"`
	Transport          string                     `mapstructure:"transport"`
	Operations         []string                   `mapstructure:"operations"`
	DefaultOperations  []string                   `mapstructure:"default_operations"`
	WireContract       string                     `mapstructure:"wire_contract"`
	ExecutionLifecycle string                     `mapstructure:"execution_lifecycle"`
	RequestProfile     string                     `mapstructure:"request_profile"`
	WebSearch          bool                       `mapstructure:"web_search"`
	OutputTokenLimit   int                        `mapstructure:"output_token_limit"`
	ReasoningEffort    *ReasoningEffortCapability `mapstructure:"reasoning_effort"`
	MediaInputs        []string                   `mapstructure:"media_inputs"`
	MediaLimits        []CatalogMediaLimit        `mapstructure:"media_limits"`
	Controls           []CatalogControl           `mapstructure:"controls"`
	Limits             []CatalogLimit             `mapstructure:"limits"`
}

// ReasoningEffortCapability declares the configured upstream mapping for one
// exact provider offering.
type ReasoningEffortCapability struct {
	Adapter string   `mapstructure:"adapter" yaml:"adapter"`
	Efforts []string `mapstructure:"efforts" yaml:"efforts"`
}

type validatedModelCatalog struct {
	revision   string
	operations map[string]ModelOperationKind
	providers  map[string]CatalogProvider
	publishers map[string]ModelPublisher
	families   map[string]ModelFamily
	models     map[string]ExactModel
	offerings  map[string]ProviderOffering
	prices     map[string]CatalogPriceDescriptor
}

func validateModelCatalog(catalog ModelCatalog) (validatedModelCatalog, error) {
	validated := validatedModelCatalog{
		revision:   catalog.Revision,
		operations: map[string]ModelOperationKind{},
		providers:  map[string]CatalogProvider{},
		publishers: map[string]ModelPublisher{},
		families:   map[string]ModelFamily{},
		models:     map[string]ExactModel{},
		offerings:  map[string]ProviderOffering{},
		prices:     map[string]CatalogPriceDescriptor{},
	}
	if revisionError := validateCatalogRevision(catalog.Revision); revisionError != nil {
		return validatedModelCatalog{}, revisionError
	}
	if operationError := validateModelOperationKinds(catalog.Operations, validated.operations); operationError != nil {
		return validatedModelCatalog{}, operationError
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
	if catalogError := validateCatalogPrices(catalog.Prices, validated); catalogError != nil {
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
		if credentialError := validateCredentialKinds(provider.CredentialKinds, fmt.Sprintf("catalog.providers[%d].credential_kinds", index)); credentialError != nil {
			return credentialError
		}
		validated[identifier] = provider
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
		if family.WeightAccess != ModelWeightAccessProprietary && family.WeightAccess != ModelWeightAccessOpenWeights {
			return fmt.Errorf("%w: field=catalog.families[%d].weight_access value=%s", ErrInvalidModelCatalog, index, family.WeightAccess)
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
	referencedPublishers := make(map[string]struct{}, len(validated))
	for _, model := range validated {
		referencedPublishers[model.Publisher] = struct{}{}
	}
	publisherIdentifiers := make([]string, 0, len(publishers))
	for publisherIdentifier := range publishers {
		publisherIdentifiers = append(publisherIdentifiers, publisherIdentifier)
	}
	sort.Strings(publisherIdentifiers)
	for _, publisherIdentifier := range publisherIdentifiers {
		if _, referenced := referencedPublishers[publisherIdentifier]; !referenced {
			return fmt.Errorf("%w: field=catalog.publishers publisher=%s reason=missing_exact_model", ErrInvalidModelCatalog, publisherIdentifier)
		}
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
		if controlError := validateCatalogControls(offering.Controls, fieldPrefix+".controls"); controlError != nil {
			return controlError
		}
		if limitError := validateCatalogLimits(offering.Limits, fieldPrefix+".limits"); limitError != nil {
			return limitError
		}
		for _, operation := range offering.Operations {
			var routeError error
			switch operation {
			case ModelOperationText:
				routeError = validateTextOffering(offering, fieldPrefix)
			case ModelOperationVideoGeneration:
				routeError = validateVideoOffering(offering, fieldPrefix)
			case ModelOperationDictation:
				routeError = validateDictationOffering(offering, fieldPrefix)
			}
			if routeError != nil {
				return routeError
			}
		}
		modelMediaInputs, _ := validatedExactModelMediaInputs(exactModel.MediaInputs, "catalog.models.media_inputs")
		offeringMediaInputs, mediaError := validatedMediaInputSet(offering, fieldPrefix+".media_inputs")
		if mediaError != nil {
			return mediaError
		}
		for mediaInput := range offeringMediaInputs {
			if _, supported := modelMediaInputs[mediaInput]; !supported {
				return fmt.Errorf("%w: field=%s.media_inputs media_input=%s reason=unsupported_by_model", ErrInvalidModelCatalog, fieldPrefix, mediaInput)
			}
		}
		routeCapabilities := textRouteCapabilities{
			wireContract:       textWireContract(offering.WireContract),
			executionLifecycle: textExecutionLifecycle(offering.ExecutionLifecycle),
		}
		if mediaLimitError := validateCatalogMediaLimits(offering.MediaLimits, offering.MediaInputs, routeCapabilities, fieldPrefix+".media_limits"); mediaLimitError != nil {
			return mediaLimitError
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

func validateDictationOffering(offering ProviderOffering, fieldPrefix string) error {
	if offering.WireContract != CatalogWireContractMultipartTranscription || offering.ExecutionLifecycle != string(textExecutionLifecycleSynchronousCompletion) {
		return fmt.Errorf("%w: field=%s reason=unsupported_dictation_route", ErrInvalidModelCatalog, fieldPrefix)
	}
	if offering.RequestProfile != constants.EmptyString || offering.WebSearch || offering.OutputTokenLimit != 0 || offering.ReasoningEffort != nil || len(offering.MediaInputs) != 0 || len(offering.MediaLimits) != 0 || len(offering.Controls) != 0 || len(offering.Limits) != 0 {
		return fmt.Errorf("%w: field=%s reason=text_capabilities_on_dictation_route", ErrInvalidModelCatalog, fieldPrefix)
	}
	return nil
}

func validateTextOffering(offering ProviderOffering, fieldPrefix string) error {
	capabilities, capabilityError := validatedTextRouteCapabilities(offering.Provider, offering, fieldPrefix)
	if capabilityError != nil {
		return capabilityError
	}
	if _, allowed := textRouteAdapters[capabilities]; !allowed {
		return fmt.Errorf("%w: provider=%s model=%s wire_contract=%s execution_lifecycle=%s", ErrInvalidModelCatalog, offering.Provider, offering.Model, offering.WireContract, offering.ExecutionLifecycle)
	}
	if offering.WebSearch && capabilities.wireContract != textWireContractOpenAIResponses {
		return fmt.Errorf("%w: field=%s.web_search provider=%s", ErrInvalidModelCatalog, fieldPrefix, offering.Provider)
	}
	if capabilities.wireContract == textWireContractAnthropicMessages && offering.OutputTokenLimit <= 0 {
		return fmt.Errorf("%w: field=%s.output_token_limit provider=%s", ErrInvalidModelCatalog, fieldPrefix, offering.Provider)
	}
	if profileError := validateOfferingRequestProfile(offering); profileError != nil {
		return fmt.Errorf("%w: field=%s.request_profile", profileError, fieldPrefix)
	}
	reasoningEffort, reasoningError := validatedReasoningEffortCapability(offering.ReasoningEffort, fieldPrefix+".reasoning_effort")
	if reasoningError != nil {
		return reasoningError
	}
	if reasoningEffort != nil {
		switch reasoningEffort.adapter {
		case reasoningEffortAdapterOpenAIResponses:
			if capabilities.wireContract != textWireContractOpenAIResponses || modelRequestProfile(offering.RequestProfile) != requestProfileOpenAIResponsesReasoningTools {
				return fmt.Errorf("%w: field=%s.reasoning_effort adapter=%s", ErrInvalidModelCatalog, fieldPrefix, reasoningEffort.adapter)
			}
		case reasoningEffortAdapterOpenAIChatCompletions:
			if capabilities != openAIChatCompletionsSynchronousRouteCapabilities || offering.RequestProfile != constants.EmptyString {
				return fmt.Errorf("%w: field=%s.reasoning_effort adapter=%s", ErrInvalidModelCatalog, fieldPrefix, reasoningEffort.adapter)
			}
		}
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
		if operation != ModelOperationText && operation != ModelOperationDictation && operation != ModelOperationVideoGeneration {
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
	if requestProfile == constants.EmptyString {
		return nil
	}
	if textWireContract(offering.WireContract) != textWireContractOpenAIResponses || !knownModelRequestProfile(modelRequestProfile(requestProfile)) {
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

func validatedMediaInputSet(offering ProviderOffering, fieldPrefix string) (map[messageMediaType]struct{}, error) {
	routeCapabilities := textRouteCapabilities{
		wireContract:       textWireContract(offering.WireContract),
		executionLifecycle: textExecutionLifecycle(offering.ExecutionLifecycle),
	}
	mediaInputs := make(map[messageMediaType]struct{}, len(offering.MediaInputs))
	for mediaInputIndex, rawMediaInput := range offering.MediaInputs {
		mediaInput := messageMediaType(rawMediaInput)
		if rawMediaInput == constants.EmptyString || rawMediaInput != strings.TrimSpace(rawMediaInput) || !textRouteSupportsMessageMedia(routeCapabilities, mediaInput) {
			return nil, fmt.Errorf("%w: field=%s[%d] provider=%s wire_contract=%s execution_lifecycle=%s media_input=%s", ErrInvalidModelCatalog, fieldPrefix, mediaInputIndex, offering.Provider, offering.WireContract, offering.ExecutionLifecycle, rawMediaInput)
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

func (requestProfile modelRequestProfile) string() string {
	return string(requestProfile)
}
