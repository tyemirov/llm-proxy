package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"gopkg.in/yaml.v3"
)

const (
	// ProviderCatalogSchemaVersion is the only accepted providers.yml schema.
	ProviderCatalogSchemaVersion = 2

	CatalogProviderFieldKindCredential = "credential"
	CatalogProviderFieldKindSetting    = "setting"
	CatalogProviderFieldTypeOpaque     = "opaque"
	CatalogProviderFieldTypeURL        = "url"

	CatalogAuthenticationBearer           = "bearer"
	CatalogAuthenticationHeader           = "header"
	CatalogEndpointMethodPost             = "POST"
	CatalogProtocolOpenAIResponses        = "openai_responses"
	CatalogProtocolDashScopeResponses     = "dashscope_responses"
	CatalogProtocolXAIResponses           = "xai_responses"
	CatalogProtocolOpenAIChatCompletions  = "openai_chat_completions"
	CatalogProtocolAnthropicMessages      = "anthropic_messages"
	CatalogProtocolVertexGenerateContent  = "vertex_generate_content"
	CatalogProtocolGeminiInteractions     = "gemini_interactions"
	CatalogProtocolMultipartTranscription = "multipart_transcription"
	CatalogProtocolMetaTranscription      = "meta_transcription"

	CatalogProtocolVariationMaxTokens                 = "max_tokens"
	CatalogProtocolVariationMaxCompletionTokens       = "max_completion_tokens"
	CatalogProtocolVariationQianfanMaxTokens          = "qianfan_max_tokens"
	CatalogProtocolVariationTranscriptionModel        = "model"
	CatalogProtocolVariationTranscriptionModelOmitted = "model_omitted"

	providerCatalogResourceVisibilityMaxRetryIntervalMilliseconds = 60000
	providerCatalogResourceVisibilityMaxRetryLimit                = 100
	providerCatalogResourceVisibilityStatusCodeUpperBound         = 600
	CatalogProtocolXAIVideosGenerations                           = "xai_videos_generations"
	providerCatalogRevisionPrefix                                 = "sha256-"
)

var catalogEnvironmentNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// ProviderCatalogSchema is the strict persisted shape of configs/providers.yml.
type ProviderCatalogSchema struct {
	SchemaVersion   int                             `yaml:"schema_version"`
	ModelMigrations []ProviderCatalogModelMigration `yaml:"model_migrations,omitempty"`
	Operations      []ModelOperationKind            `yaml:"operations"`
	Publishers      []ModelPublisher                `yaml:"publishers"`
	Families        []ModelFamily                   `yaml:"families"`
	Models          []ProviderCatalogModel          `yaml:"models"`
	Providers       []ProviderCatalogProvider       `yaml:"providers"`
}

// ModelActivation is the explicit activation state of a persisted exact model.
type ModelActivation uint8

const (
	ModelDisabled ModelActivation = iota + 1
	ModelEnabled
)

// UnmarshalYAML accepts only a Boolean activation field.
func (activation *ModelActivation) UnmarshalYAML(value *yaml.Node) error {
	if value.Tag != "!!bool" {
		return fmt.Errorf("%w: field=enabled reason=boolean_required", ErrInvalidModelCatalog)
	}
	*activation = ModelDisabled
	if strings.EqualFold(value.Value, "true") {
		*activation = ModelEnabled
	}
	return nil
}

// MarshalYAML writes the activation state as a Boolean.
func (activation ModelActivation) MarshalYAML() (any, error) {
	return activation == ModelEnabled, nil
}

// ProviderCatalogModel adds private activation state to an exact model.
type ProviderCatalogModel struct {
	ExactModel `yaml:",inline"`
	Enabled    ModelActivation `yaml:"enabled"`
}

// ProviderCatalogModelMigration maps one persisted model identifier to the current catalog contract.
type ProviderCatalogModelMigration struct {
	TargetReasoningEffort string `yaml:"target_reasoning_effort,omitempty"`
	PreserveSourceUsage   bool   `yaml:"preserve_source_usage,omitempty"`
	ManagedSchemaVersion  int    `yaml:"managed_schema_version"`
	Provider              string `yaml:"provider"`
	Operation             string `yaml:"operation"`
	SourceModel           string `yaml:"source_model"`
	TargetModel           string `yaml:"target_model"`
}

// ProviderCatalogProvider defines one provider and all provider-owned routes.
type ProviderCatalogProvider struct {
	// Enabled defaults to enabled when omitted. Candidate providers set false explicitly.
	Enabled           ModelActivation            `yaml:"enabled,omitempty"`
	ID                string                     `yaml:"id"`
	Label             string                     `yaml:"label"`
	APIServiceLabel   string                     `yaml:"api_service_label"`
	KeyAcquisitionURL string                     `yaml:"key_acquisition_url"`
	Aliases           []string                   `yaml:"aliases,omitempty"`
	Fields            []ProviderCatalogField     `yaml:"fields"`
	Transports        []ProviderCatalogTransport `yaml:"transports"`
	Offerings         []ProviderCatalogOffering  `yaml:"offerings"`
}

// ProviderCatalogField defines one tenant connection input.
type ProviderCatalogField struct {
	ID          string                         `yaml:"id"`
	Label       string                         `yaml:"label"`
	Kind        string                         `yaml:"kind"`
	Type        string                         `yaml:"type"`
	Required    bool                           `yaml:"required"`
	Default     *string                        `yaml:"default"`
	Secret      bool                           `yaml:"secret"`
	Validation  ProviderCatalogFieldValidation `yaml:"validation"`
	Environment string                         `yaml:"environment"`
}

// ProviderCatalogFieldValidation selects one reusable edge validator.
type ProviderCatalogFieldValidation struct {
	MinimumLength  int      `yaml:"minimum_length,omitempty"`
	Pattern        string   `yaml:"pattern,omitempty"`
	AllowedSchemes []string `yaml:"allowed_schemes,omitempty"`
}

// ProviderCatalogTransport defines one reusable provider protocol route.
type ProviderCatalogTransport struct {
	ID                 string                            `yaml:"id"`
	Endpoint           ProviderCatalogEndpoint           `yaml:"endpoint"`
	Authentication     ProviderCatalogAuthentication     `yaml:"authentication"`
	Headers            []ProviderCatalogHeader           `yaml:"headers,omitempty"`
	Protocol           ProviderCatalogProtocolReference  `yaml:"protocol"`
	Lifecycle          string                            `yaml:"lifecycle"`
	ResourceVisibility ProviderCatalogResourceVisibility `yaml:"resource_visibility,omitempty"`
}

// ProviderCatalogProtocolReference selects one codec definition and an optional typed variation.
type ProviderCatalogProtocolReference struct {
	ID        string `yaml:"id"`
	Variation string `yaml:"variation,omitempty"`
}

// ProviderCatalogResourceVisibility defines bounded retries for a created resource that is not readable yet.
type ProviderCatalogResourceVisibility struct {
	RetryIntervalMilliseconds int   `yaml:"retry_interval_milliseconds"`
	RetryLimit                int   `yaml:"retry_limit"`
	RetryStatusCodes          []int `yaml:"retry_status_codes"`
}

// ProviderCatalogEndpoint defines a transport collection URL.
type ProviderCatalogEndpoint struct {
	Method         string `yaml:"method"`
	DefaultBaseURL string `yaml:"default_base_url,omitempty"`
	SettingField   string `yaml:"setting_field,omitempty"`
	Path           string `yaml:"path"`
}

// ProviderCatalogAuthentication defines how a connection credential is sent.
type ProviderCatalogAuthentication struct {
	Kind   string `yaml:"kind"`
	Field  string `yaml:"field"`
	Header string `yaml:"header"`
	Prefix string `yaml:"prefix"`
}

// ProviderCatalogHeader defines one nonsecret static transport header.
type ProviderCatalogHeader struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// ProviderCatalogOffering defines one exact model route inside its provider.
type ProviderCatalogOffering struct {
	Enabled     ModelActivation `yaml:"enabled,omitempty"`
	Created     int64           `yaml:"created"`
	CallerTools bool            `yaml:"caller_tools,omitempty"`

	Model             string                     `yaml:"model"`
	UpstreamModel     string                     `yaml:"upstream_model"`
	Transport         string                     `yaml:"transport"`
	Operations        []string                   `yaml:"operations"`
	DefaultOperations []string                   `yaml:"default_operations,omitempty"`
	RequestProfile    string                     `yaml:"request_profile,omitempty"`
	WebSearch         bool                       `yaml:"web_search,omitempty"`
	OutputTokenLimit  int                        `yaml:"output_token_limit,omitempty"`
	ReasoningEffort   *ReasoningEffortCapability `yaml:"reasoning_effort,omitempty"`
	MediaInputs       []string                   `yaml:"media_inputs,omitempty"`
	ImageMIMETypes    []string                   `yaml:"image_mime_types,omitempty"`
	MediaLimits       []CatalogMediaLimit        `yaml:"media_limits,omitempty"`
	Controls          []CatalogControl           `yaml:"controls,omitempty"`
	Limits            []CatalogLimit             `yaml:"limits,omitempty"`
	Prices            []ProviderCatalogPrice     `yaml:"prices"`
}

// ProviderCatalogPrice defines one operation price inside its provider offering.
type ProviderCatalogPrice struct {
	Operation         string                `yaml:"operation"`
	Available         bool                  `yaml:"available"`
	Rates             []CatalogPriceRate    `yaml:"rates,omitempty"`
	MinimumCharge     *CatalogMinimumCharge `yaml:"minimum_charge,omitempty"`
	Source            string                `yaml:"source"`
	LastVerified      string                `yaml:"last_verified"`
	UnavailableReason string                `yaml:"unavailable_reason,omitempty"`
}

// ProviderCatalog is one validated immutable catalog snapshot.
type ProviderCatalog struct {
	schema        ProviderCatalogSchema
	runtimeSchema ProviderCatalogSchema
	modelCatalog  ModelCatalog
}

// ParseProviderCatalog decodes and validates one strict providers.yml document.
func ParseProviderCatalog(document []byte) (*ProviderCatalog, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(document))
	decoder.KnownFields(true)
	var schema ProviderCatalogSchema
	if decodeError := decoder.Decode(&schema); decodeError != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidModelCatalog, decodeError)
	}
	var trailing any
	if decodeError := decoder.Decode(&trailing); decodeError != io.EOF {
		if decodeError == nil {
			return nil, fmt.Errorf("%w: multiple YAML documents", ErrInvalidModelCatalog)
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidModelCatalog, decodeError)
	}
	digest := sha256.Sum256(document)
	return newProviderCatalog(schema, providerCatalogRevisionPrefix+hex.EncodeToString(digest[:]))
}

// NewProviderCatalog validates an in-memory provider catalog schema.
func NewProviderCatalog(schema ProviderCatalogSchema) (*ProviderCatalog, error) {
	document, _ := yaml.Marshal(schema)
	digest := sha256.Sum256(document)
	return newProviderCatalog(schema, providerCatalogRevisionPrefix+hex.EncodeToString(digest[:]))
}

func newProviderCatalog(schema ProviderCatalogSchema, revision string) (*ProviderCatalog, error) {
	if structureError := validateProviderCatalogSchema(schema); structureError != nil {
		return nil, structureError
	}
	modelCatalog, compileError := compileProviderCatalogSchema(schema, revision)
	if compileError != nil {
		return nil, compileError
	}
	runtimeSchema, modelCatalog := enabledProviderCatalog(schema, modelCatalog)
	if defaultsError := validateProviderOperationDefaults(modelCatalog.Offerings); defaultsError != nil {
		return nil, defaultsError
	}
	if migrationError := validateProviderCatalogModelMigrations(schema.ModelMigrations, modelCatalog); migrationError != nil {
		return nil, migrationError
	}
	return &ProviderCatalog{schema: cloneProviderCatalogSchema(schema), runtimeSchema: runtimeSchema, modelCatalog: modelCatalog}, nil
}

func enabledProviderCatalog(schema ProviderCatalogSchema, catalog ModelCatalog) (ProviderCatalogSchema, ModelCatalog) {
	enabledOfferings := make(map[string]bool)
	enabledProviders := make(map[string]bool, len(schema.Providers))
	modelsWithEnabledOfferings := make(map[string]bool, len(schema.Models))
	for _, provider := range schema.Providers {
		enabledProviders[provider.ID] = provider.Enabled != ModelDisabled
		for _, offering := range provider.Offerings {
			enabledOfferings[provider.ID+"\x00"+offering.Model] = offering.Enabled != ModelDisabled
			if enabledProviders[provider.ID] && offering.Enabled != ModelDisabled {
				modelsWithEnabledOfferings[offering.Model] = true
			}
		}
	}
	enabledModels := make(map[string]bool, len(schema.Models))
	enabledFamilies := make(map[string]bool, len(schema.Families))
	for _, model := range schema.Models {
		enabledModels[model.ID] = model.Enabled == ModelEnabled && modelsWithEnabledOfferings[model.ID]
		if enabledModels[model.ID] {
			enabledFamilies[model.Family] = true
		}
	}
	runtimeSchema := cloneProviderCatalogSchema(schema)
	runtimeSchema.Providers = slices.DeleteFunc(runtimeSchema.Providers, func(provider ProviderCatalogProvider) bool { return !enabledProviders[provider.ID] })
	catalog.Providers = slices.DeleteFunc(catalog.Providers, func(provider CatalogProvider) bool { return !enabledProviders[provider.ID] })
	runtimeSchema.Families = slices.DeleteFunc(runtimeSchema.Families, func(family ModelFamily) bool { return !enabledFamilies[family.ID] })
	runtimeSchema.Models = slices.DeleteFunc(runtimeSchema.Models, func(model ProviderCatalogModel) bool { return !enabledModels[model.ID] })
	for providerIndex := range runtimeSchema.Providers {
		provider := &runtimeSchema.Providers[providerIndex]
		provider.Offerings = slices.DeleteFunc(provider.Offerings, func(offering ProviderCatalogOffering) bool {
			return !enabledModels[offering.Model] || offering.Enabled == ModelDisabled
		})
	}
	catalog.Families = slices.DeleteFunc(catalog.Families, func(family ModelFamily) bool { return !enabledFamilies[family.ID] })
	catalog.Models = slices.DeleteFunc(catalog.Models, func(model ExactModel) bool { return !enabledModels[model.ID] })
	catalog.Offerings = slices.DeleteFunc(catalog.Offerings, func(offering ProviderOffering) bool {
		return !enabledModels[offering.Model] || !enabledProviders[offering.Provider] || !enabledOfferings[offering.Provider+"\x00"+offering.Model]
	})
	catalog.Prices = slices.DeleteFunc(catalog.Prices, func(price CatalogPriceDescriptor) bool {
		return !enabledModels[price.Model] || !enabledProviders[price.Provider] || !enabledOfferings[price.Provider+"\x00"+price.Model]
	})
	return runtimeSchema, catalog
}

func validateProviderCatalogModelMigrations(migrations []ProviderCatalogModelMigration, catalog ModelCatalog) error {
	providers := make(map[string]struct{}, len(catalog.Providers))
	for _, provider := range catalog.Providers {
		providers[provider.ID] = struct{}{}
	}
	offerings := make(map[string]ProviderOffering, len(catalog.Offerings))
	for _, offering := range catalog.Offerings {
		for _, operation := range offering.Operations {
			offerings[offering.Provider+"\x00"+operation+"\x00"+offering.Model] = offering
		}
	}
	identities := make(map[string]struct{}, len(migrations))
	for migrationIndex, migration := range migrations {
		fieldPrefix := fmt.Sprintf("model_migrations[%d]", migrationIndex)
		if migration.ManagedSchemaVersion <= 0 || migration.ManagedSchemaVersion > managedTenantSchemaVersion {
			return fmt.Errorf("%w: field=%s.managed_schema_version value=%d", ErrInvalidModelCatalog, fieldPrefix, migration.ManagedSchemaVersion)
		}
		provider, providerError := canonicalCatalogIdentifier(migration.Provider, fieldPrefix+".provider")
		if providerError != nil {
			return providerError
		}
		if migration.Operation != ModelOperationText && migration.Operation != ModelOperationDictation {
			return fmt.Errorf("%w: field=%s.operation operation=%s", ErrInvalidModelCatalog, fieldPrefix, migration.Operation)
		}
		if strings.TrimSpace(migration.SourceModel) == constants.EmptyString || migration.SourceModel != strings.TrimSpace(migration.SourceModel) {
			return fmt.Errorf("%w: field=%s.source_model", ErrInvalidModelCatalog, fieldPrefix)
		}
		identity := fmt.Sprintf("%d\x00%s\x00%s\x00%s", migration.ManagedSchemaVersion, provider, migration.Operation, migration.SourceModel)
		if _, duplicate := identities[identity]; duplicate {
			return fmt.Errorf("%w: field=%s duplicate_model_migration=%s", ErrInvalidModelCatalog, fieldPrefix, migration.SourceModel)
		}
		identities[identity] = struct{}{}
		_, currentProvider := providers[provider]
		if migration.TargetReasoningEffort != "" && (migration.Operation != ModelOperationText || migration.TargetModel == "") {
			return fmt.Errorf("%w: field=%s.target_reasoning_effort reason=text_target_required", ErrInvalidModelCatalog, fieldPrefix)
		}
		if migration.TargetModel == constants.EmptyString {
			if currentProvider {
				return fmt.Errorf("%w: field=%s.target_model provider=%s reason=current_provider", ErrInvalidModelCatalog, fieldPrefix, provider)
			}
			continue
		}
		if migration.TargetModel != strings.TrimSpace(migration.TargetModel) || migration.TargetModel == migration.SourceModel {
			return fmt.Errorf("%w: field=%s.target_model", ErrInvalidModelCatalog, fieldPrefix)
		}
		if !currentProvider {
			return fmt.Errorf("%w: field=%s.target_model provider=%s reason=retired_provider", ErrInvalidModelCatalog, fieldPrefix, provider)
		}
		target, found := offerings[provider+"\x00"+migration.Operation+"\x00"+migration.TargetModel]
		if !found {
			return fmt.Errorf("%w: field=%s.target_model provider=%s operation=%s model=%s reason=dangling_reference", ErrInvalidModelCatalog, fieldPrefix, provider, migration.Operation, migration.TargetModel)
		}
		if migration.TargetReasoningEffort != "" && !configuredReasoningEffortCapability(target.ReasoningEffort).supports(migration.TargetReasoningEffort) {
			return fmt.Errorf("%w: field=%s.target_reasoning_effort reason=unsupported", ErrInvalidModelCatalog, fieldPrefix)
		}
	}
	return nil
}

// SchemaVersion returns the exact persisted schema version.
func (catalog *ProviderCatalog) SchemaVersion() int {
	return catalog.schema.SchemaVersion
}

// ModelCatalog returns a detached normalized runtime projection.
func (catalog *ProviderCatalog) ModelCatalog() ModelCatalog {
	return cloneModelCatalog(catalog.modelCatalog)
}

// Schema returns a detached copy of the private catalog schema.
func (catalog *ProviderCatalog) Schema() ProviderCatalogSchema {
	return cloneProviderCatalogSchema(catalog.schema)
}

// ResolveEnvironmentBindings returns validated runtime connection values for configured catalog bindings.
func (catalog *ProviderCatalog) ResolveEnvironmentBindings(environment map[string]string) (map[string]map[string]string, error) {
	connectionValues := map[string]map[string]string{}
	for _, provider := range catalog.schema.Providers {
		for _, field := range provider.Fields {
			if field.Environment == constants.EmptyString {
				continue
			}
			rawValue, configured := environment[field.Environment]
			if !configured || rawValue == constants.EmptyString {
				continue
			}
			value, valueError := validatedProviderFieldValue(field, rawValue)
			if valueError != nil {
				return nil, fmt.Errorf("%w: provider=%s field=%s environment=%s", ErrInvalidModelCatalog, provider.ID, field.ID, field.Environment)
			}
			if connectionValues[provider.ID] == nil {
				connectionValues[provider.ID] = map[string]string{}
			}
			connectionValues[provider.ID][field.ID] = value
		}
	}
	return connectionValues, nil
}

func (catalog *ProviderCatalog) validatedConnectionValues(rawValues map[string]map[string]string) (map[string]map[string]string, error) {
	providerFields := make(map[string]map[string]ProviderCatalogField, len(catalog.schema.Providers))
	for _, provider := range catalog.schema.Providers {
		fields := make(map[string]ProviderCatalogField, len(provider.Fields))
		for _, field := range provider.Fields {
			fields[field.ID] = field
		}
		providerFields[provider.ID] = fields
	}
	connectionValues := make(map[string]map[string]string, len(rawValues))
	for providerIdentifier, rawProviderValues := range rawValues {
		fields, knownProvider := providerFields[providerIdentifier]
		if !knownProvider {
			return nil, fmt.Errorf("%w: provider=%s reason=unknown_provider_connection", ErrInvalidModelCatalog, providerIdentifier)
		}
		providerValues := make(map[string]string, len(rawProviderValues))
		for fieldIdentifier, rawValue := range rawProviderValues {
			field, knownField := fields[fieldIdentifier]
			if !knownField {
				return nil, fmt.Errorf("%w: provider=%s field=%s reason=unknown_provider_connection_field", ErrInvalidModelCatalog, providerIdentifier, fieldIdentifier)
			}
			value, valueError := validatedProviderFieldValue(field, rawValue)
			if valueError != nil {
				return nil, fmt.Errorf("%w: provider=%s field=%s reason=invalid_provider_connection", ErrInvalidModelCatalog, providerIdentifier, fieldIdentifier)
			}
			providerValues[fieldIdentifier] = value
		}
		connectionValues[providerIdentifier] = providerValues
	}
	return connectionValues, nil
}

func validateProviderCatalogSchema(schema ProviderCatalogSchema) error {
	if schema.SchemaVersion != ProviderCatalogSchemaVersion {
		return fmt.Errorf("%w: field=schema_version value=%d", ErrInvalidModelCatalog, schema.SchemaVersion)
	}
	if len(schema.Providers) == 0 {
		return fmt.Errorf("%w: field=providers", ErrInvalidModelCatalog)
	}
	modelActivations := make(map[string]ModelActivation, len(schema.Models))
	for index, model := range schema.Models {
		if model.Enabled != ModelEnabled && model.Enabled != ModelDisabled {
			return fmt.Errorf("%w: field=models[%d].enabled reason=boolean_required", ErrInvalidModelCatalog, index)
		}
		modelActivations[model.ID] = model.Enabled
	}
	providerIdentifiers := map[string]struct{}{}
	providerAliases := map[string]string{}
	environmentBindings := map[string]string{}
	for providerIndex, provider := range schema.Providers {
		fieldPrefix := fmt.Sprintf("providers[%d]", providerIndex)
		if provider.Enabled != 0 && provider.Enabled != ModelEnabled && provider.Enabled != ModelDisabled {
			return fmt.Errorf("%w: field=%s.enabled reason=boolean_required", ErrInvalidModelCatalog, fieldPrefix)
		}
		identifier, identifierError := canonicalCatalogIdentifier(provider.ID, fieldPrefix+".id")
		if identifierError != nil {
			return identifierError
		}
		if _, duplicate := providerIdentifiers[identifier]; duplicate {
			return fmt.Errorf("%w: field=%s.id duplicate_identifier=%s", ErrInvalidModelCatalog, fieldPrefix, identifier)
		}
		if owner, collision := providerAliases[identifier]; collision {
			return fmt.Errorf("%w: field=%s.id alias_collision=%s owner=%s", ErrInvalidModelCatalog, fieldPrefix, identifier, owner)
		}
		providerIdentifiers[identifier] = struct{}{}
		providerAliases[identifier] = identifier
		if strings.TrimSpace(provider.Label) == constants.EmptyString || provider.Label != strings.TrimSpace(provider.Label) {
			return fmt.Errorf("%w: field=%s.label", ErrInvalidModelCatalog, fieldPrefix)
		}
		if strings.TrimSpace(provider.APIServiceLabel) == constants.EmptyString || provider.APIServiceLabel != strings.TrimSpace(provider.APIServiceLabel) {
			return fmt.Errorf("%w: field=%s.api_service_label", ErrInvalidModelCatalog, fieldPrefix)
		}
		if !validProviderKeyAcquisitionURL(provider.KeyAcquisitionURL) {
			return fmt.Errorf("%w: field=%s.key_acquisition_url", ErrInvalidModelCatalog, fieldPrefix)
		}
		for aliasIndex, rawAlias := range provider.Aliases {
			alias, aliasError := canonicalCatalogIdentifier(rawAlias, fmt.Sprintf("%s.aliases[%d]", fieldPrefix, aliasIndex))
			if aliasError != nil {
				return aliasError
			}
			if owner, collision := providerAliases[alias]; collision {
				return fmt.Errorf("%w: field=%s.aliases[%d] alias_collision=%s owner=%s", ErrInvalidModelCatalog, fieldPrefix, aliasIndex, alias, owner)
			}
			providerAliases[alias] = identifier
		}
		fields, fieldError := validateProviderCatalogFields(provider.Fields, fieldPrefix+".fields")
		if fieldError != nil {
			return fieldError
		}
		for fieldIdentifier, definition := range fields {
			if definition.Environment == constants.EmptyString {
				continue
			}
			if owner, duplicate := environmentBindings[definition.Environment]; duplicate {
				return fmt.Errorf("%w: field=%s.fields environment=%s duplicate_binding=%s", ErrInvalidModelCatalog, fieldPrefix, definition.Environment, owner)
			}
			environmentBindings[definition.Environment] = identifier + "." + fieldIdentifier
		}
		transports, transportError := validateProviderCatalogTransports(provider.Transports, fields, fieldPrefix+".transports")
		if transportError != nil {
			return transportError
		}
		if len(provider.Offerings) == 0 {
			return fmt.Errorf("%w: field=%s.offerings", ErrInvalidModelCatalog, fieldPrefix)
		}
		for offeringIndex, offering := range provider.Offerings {
			offeringField := fmt.Sprintf("%s.offerings[%d]", fieldPrefix, offeringIndex)
			if offering.Enabled != 0 && offering.Enabled != ModelEnabled && offering.Enabled != ModelDisabled {
				return fmt.Errorf("%w: field=%s.enabled", ErrInvalidModelCatalog, offeringField)
			}
			if (modelActivations[offering.Model] == ModelDisabled || offering.Enabled == ModelDisabled) && len(offering.DefaultOperations) != 0 {
				return fmt.Errorf("%w: field=%s.default_operations model=%s reason=disabled_default", ErrInvalidModelCatalog, offeringField, offering.Model)
			}
			if _, found := transports[offering.Transport]; !found {
				return fmt.Errorf("%w: field=%s.transport transport=%s reason=dangling_reference", ErrInvalidModelCatalog, offeringField, offering.Transport)
			}
			if offering.Created <= 0 {
				return fmt.Errorf("%w: field=%s.created", ErrInvalidModelCatalog, offeringField)
			}
			if offering.CallerTools {
				transport := transports[offering.Transport]
				supported := transport.Protocol.ID == CatalogProtocolXAIResponses || transport.Protocol.ID == CatalogProtocolOpenAIChatCompletions || (transport.Protocol.ID == CatalogProtocolOpenAIResponses && (offering.RequestProfile == string(requestProfileOpenAIResponsesReasoningTools) || offering.RequestProfile == string(requestProfileOpenAIResponsesTemperatureTools)))
				if !supported {
					return fmt.Errorf("%w: field=%s.caller_tools", ErrInvalidModelCatalog, offeringField)
				}
			}
			if len(offering.Prices) == 0 {
				return fmt.Errorf("%w: field=%s.prices", ErrInvalidModelCatalog, offeringField)
			}
		}
	}
	return nil
}

func validProviderKeyAcquisitionURL(value string) bool {
	parsedURL, parseError := url.Parse(value)
	return parseError == nil &&
		parsedURL.Scheme == "https" &&
		parsedURL.Host != constants.EmptyString &&
		parsedURL.User == nil &&
		parsedURL.RawQuery == constants.EmptyString &&
		parsedURL.Fragment == constants.EmptyString &&
		value == strings.TrimSpace(value)
}

func validateProviderCatalogFields(rawFields []ProviderCatalogField, field string) (map[string]ProviderCatalogField, error) {
	if len(rawFields) == 0 {
		return nil, fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
	}
	fields := make(map[string]ProviderCatalogField, len(rawFields))
	credentialCount := 0
	for fieldIndex, definition := range rawFields {
		fieldPrefix := fmt.Sprintf("%s[%d]", field, fieldIndex)
		identifier, identifierError := canonicalCatalogIdentifier(definition.ID, fieldPrefix+".id")
		if identifierError != nil {
			return nil, identifierError
		}
		if _, duplicate := fields[identifier]; duplicate {
			return nil, fmt.Errorf("%w: field=%s.id duplicate_identifier=%s", ErrInvalidModelCatalog, fieldPrefix, identifier)
		}
		if strings.TrimSpace(definition.Label) == constants.EmptyString || definition.Label != strings.TrimSpace(definition.Label) || definition.Default == nil {
			return nil, fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, fieldPrefix)
		}
		switch definition.Kind {
		case CatalogProviderFieldKindCredential:
			credentialCount++
			if definition.Type != CatalogProviderFieldTypeOpaque || !definition.Secret || *definition.Default != constants.EmptyString || definition.Validation.MinimumLength <= 0 || definition.Validation.Pattern != constants.EmptyString || len(definition.Validation.AllowedSchemes) != 0 {
				return nil, fmt.Errorf("%w: field=%s reason=invalid_credential_field", ErrInvalidModelCatalog, fieldPrefix)
			}
		case CatalogProviderFieldKindSetting:
			if definition.Type != CatalogProviderFieldTypeURL || definition.Secret || definition.Validation.MinimumLength < 0 || len(definition.Validation.AllowedSchemes) == 0 {
				return nil, fmt.Errorf("%w: field=%s reason=invalid_setting_field", ErrInvalidModelCatalog, fieldPrefix)
			}
		default:
			return nil, fmt.Errorf("%w: field=%s.kind kind=%s", ErrInvalidModelCatalog, fieldPrefix, definition.Kind)
		}
		if definition.Environment != constants.EmptyString && !catalogEnvironmentNamePattern.MatchString(definition.Environment) {
			return nil, fmt.Errorf("%w: field=%s.environment", ErrInvalidModelCatalog, fieldPrefix)
		}
		if definition.Validation.Pattern != constants.EmptyString {
			if _, patternError := regexp.Compile(definition.Validation.Pattern); patternError != nil {
				return nil, fmt.Errorf("%w: field=%s.validation.pattern", ErrInvalidModelCatalog, fieldPrefix)
			}
		}
		if *definition.Default != constants.EmptyString {
			if _, valueError := validatedProviderFieldValue(definition, *definition.Default); valueError != nil {
				return nil, fmt.Errorf("%w: field=%s.default", ErrInvalidModelCatalog, fieldPrefix)
			}
		}
		fields[identifier] = definition
	}
	if credentialCount == 0 {
		return nil, fmt.Errorf("%w: field=%s reason=credential_missing", ErrInvalidModelCatalog, field)
	}
	return fields, nil
}

func validatedProviderFieldValue(definition ProviderCatalogField, rawValue string) (string, error) {
	value := strings.TrimSpace(rawValue)
	if value != rawValue || len(value) < definition.Validation.MinimumLength {
		return constants.EmptyString, fmt.Errorf("provider_field_invalid: field=%s", definition.ID)
	}
	if definition.Validation.Pattern != constants.EmptyString && !regexp.MustCompile(definition.Validation.Pattern).MatchString(value) {
		return constants.EmptyString, fmt.Errorf("provider_field_invalid: field=%s", definition.ID)
	}
	if definition.Type != CatalogProviderFieldTypeURL {
		return value, nil
	}
	parsedURL, parseError := url.Parse(value)
	if parseError != nil || parsedURL.Host == constants.EmptyString || parsedURL.User != nil || parsedURL.RawQuery != constants.EmptyString || parsedURL.Fragment != constants.EmptyString || strings.TrimRight(value, "/") != value {
		return constants.EmptyString, fmt.Errorf("provider_field_invalid: field=%s", definition.ID)
	}
	for _, allowedScheme := range definition.Validation.AllowedSchemes {
		if parsedURL.Scheme == allowedScheme {
			return value, nil
		}
	}
	return constants.EmptyString, fmt.Errorf("provider_field_invalid: field=%s", definition.ID)
}

func validateProviderCatalogTransports(rawTransports []ProviderCatalogTransport, fields map[string]ProviderCatalogField, field string) (map[string]ProviderCatalogTransport, error) {
	if len(rawTransports) == 0 {
		return nil, fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
	}
	transports := make(map[string]ProviderCatalogTransport, len(rawTransports))
	for transportIndex, transport := range rawTransports {
		fieldPrefix := fmt.Sprintf("%s[%d]", field, transportIndex)
		identifier, identifierError := canonicalCatalogIdentifier(transport.ID, fieldPrefix+".id")
		if identifierError != nil {
			return nil, identifierError
		}
		if _, duplicate := transports[identifier]; duplicate {
			return nil, fmt.Errorf("%w: field=%s.id duplicate_identifier=%s", ErrInvalidModelCatalog, fieldPrefix, identifier)
		}
		if endpointError := validateProviderCatalogEndpoint(transport.Endpoint, fields, fieldPrefix+".endpoint"); endpointError != nil {
			return nil, endpointError
		}
		credentialField, found := fields[transport.Authentication.Field]
		if !found || credentialField.Kind != CatalogProviderFieldKindCredential || !credentialField.Required {
			return nil, fmt.Errorf("%w: field=%s.authentication.field field_id=%s reason=dangling_reference", ErrInvalidModelCatalog, fieldPrefix, transport.Authentication.Field)
		}
		if authenticationError := validateProviderCatalogAuthentication(transport.Authentication, fieldPrefix+".authentication"); authenticationError != nil {
			return nil, authenticationError
		}
		if headersError := validateProviderCatalogHeaders(transport.Headers, fieldPrefix+".headers"); headersError != nil {
			return nil, headersError
		}
		definition, definitionError := providerProtocolDefinitionFor(transport.Protocol, fieldPrefix+".protocol")
		if definitionError != nil {
			return nil, definitionError
		}
		if !slices.Contains(definition.allowedLifecycles, textExecutionLifecycle(transport.Lifecycle)) {
			return nil, fmt.Errorf("%w: field=%s.lifecycle lifecycle=%s", ErrInvalidModelCatalog, fieldPrefix, transport.Lifecycle)
		}
		if visibilityError := validateProviderCatalogResourceVisibility(transport, fieldPrefix+".resource_visibility"); visibilityError != nil {
			return nil, visibilityError
		}
		if adapterError := validateProviderCatalogAdapterContract(transport, fieldPrefix); adapterError != nil {
			return nil, adapterError
		}
		transports[identifier] = transport
	}
	return transports, nil
}

func validateProviderCatalogResourceVisibility(transport ProviderCatalogTransport, field string) error {
	sharedPollableLifecycle := transport.Lifecycle == string(textExecutionLifecyclePollableResource) &&
		(transport.Protocol.ID == CatalogProtocolOpenAIResponses || transport.Protocol.ID == CatalogProtocolGeminiInteractions)
	visibility := transport.ResourceVisibility
	if !sharedPollableLifecycle {
		if visibility.RetryIntervalMilliseconds != 0 || visibility.RetryLimit != 0 || len(visibility.RetryStatusCodes) != 0 {
			return fmt.Errorf("%w: field=%s reason=unexpected_resource_visibility", ErrInvalidModelCatalog, field)
		}
		return nil
	}
	if visibility.RetryIntervalMilliseconds <= 0 || visibility.RetryIntervalMilliseconds > providerCatalogResourceVisibilityMaxRetryIntervalMilliseconds ||
		visibility.RetryLimit <= 0 || visibility.RetryLimit > providerCatalogResourceVisibilityMaxRetryLimit || len(visibility.RetryStatusCodes) == 0 {
		return fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
	}
	seenStatusCodes := map[int]struct{}{}
	for statusIndex, statusCode := range visibility.RetryStatusCodes {
		if statusCode < http.StatusBadRequest || statusCode >= providerCatalogResourceVisibilityStatusCodeUpperBound {
			return fmt.Errorf("%w: field=%s.retry_status_codes[%d] status=%d", ErrInvalidModelCatalog, field, statusIndex, statusCode)
		}
		if _, duplicate := seenStatusCodes[statusCode]; duplicate {
			return fmt.Errorf("%w: field=%s.retry_status_codes[%d] duplicate=%d", ErrInvalidModelCatalog, field, statusIndex, statusCode)
		}
		seenStatusCodes[statusCode] = struct{}{}
	}
	return nil
}

func validateProviderCatalogEndpoint(endpoint ProviderCatalogEndpoint, fields map[string]ProviderCatalogField, field string) error {
	if endpoint.Method != CatalogEndpointMethodPost || !strings.HasPrefix(endpoint.Path, "/") || endpoint.Path != strings.TrimSpace(endpoint.Path) {
		return fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
	}
	if (endpoint.DefaultBaseURL == constants.EmptyString) == (endpoint.SettingField == constants.EmptyString) {
		return fmt.Errorf("%w: field=%s reason=endpoint_source_count", ErrInvalidModelCatalog, field)
	}
	if endpoint.SettingField != constants.EmptyString {
		definition, found := fields[endpoint.SettingField]
		if !found || definition.Kind != CatalogProviderFieldKindSetting {
			return fmt.Errorf("%w: field=%s.setting_field field_id=%s reason=dangling_reference", ErrInvalidModelCatalog, field, endpoint.SettingField)
		}
		return nil
	}
	parsedURL, parseError := url.Parse(endpoint.DefaultBaseURL)
	if parseError != nil {
		return fmt.Errorf("%w: field=%s.default_base_url", ErrInvalidModelCatalog, field)
	}
	validScheme := parsedURL.Scheme == "https" || (parsedURL.Scheme == "http" && providerCatalogLoopbackHost(parsedURL.Hostname()))
	if !validScheme || parsedURL.Host == constants.EmptyString || parsedURL.User != nil || parsedURL.RawQuery != constants.EmptyString || parsedURL.Fragment != constants.EmptyString || strings.TrimRight(endpoint.DefaultBaseURL, "/") != endpoint.DefaultBaseURL {
		return fmt.Errorf("%w: field=%s.default_base_url", ErrInvalidModelCatalog, field)
	}
	return nil
}

func providerCatalogLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func validateProviderCatalogAuthentication(authentication ProviderCatalogAuthentication, field string) error {
	switch authentication.Kind {
	case CatalogAuthenticationBearer:
		if authentication.Header != "Authorization" || authentication.Prefix != "Bearer " {
			return fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
		}
	case CatalogAuthenticationHeader:
		if strings.TrimSpace(authentication.Header) == constants.EmptyString || authentication.Header != strings.TrimSpace(authentication.Header) || authentication.Prefix != constants.EmptyString {
			return fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
		}
	default:
		return fmt.Errorf("%w: field=%s.kind kind=%s", ErrInvalidModelCatalog, field, authentication.Kind)
	}
	return nil
}

func validateProviderCatalogHeaders(headers []ProviderCatalogHeader, field string) error {
	seen := map[string]struct{}{}
	for headerIndex, header := range headers {
		if strings.TrimSpace(header.Name) == constants.EmptyString || header.Name != strings.TrimSpace(header.Name) || strings.TrimSpace(header.Value) == constants.EmptyString || header.Value != strings.TrimSpace(header.Value) {
			return fmt.Errorf("%w: field=%s[%d]", ErrInvalidModelCatalog, field, headerIndex)
		}
		normalizedName := strings.ToLower(header.Name)
		if _, duplicate := seen[normalizedName]; duplicate {
			return fmt.Errorf("%w: field=%s[%d].name duplicate=%s", ErrInvalidModelCatalog, field, headerIndex, header.Name)
		}
		seen[normalizedName] = struct{}{}
	}
	return nil
}

func validateProviderCatalogAdapterContract(transport ProviderCatalogTransport, field string) error {
	definition, definitionError := providerProtocolDefinitionFor(transport.Protocol, field+".protocol")
	if definitionError != nil {
		return definitionError
	}
	expectedAuthentication := definition.authentication
	expectedAuthentication.Field = transport.Authentication.Field
	if transport.Authentication != expectedAuthentication || !providerCatalogHeadersEqual(transport.Headers, definition.headers) {
		return providerCatalogAdapterContractError(field, transport.Protocol.ID)
	}
	return nil
}

func providerCatalogAdapterContractError(field string, protocol string) error {
	return fmt.Errorf("%w: field=%s reason=adapter_contract_mismatch protocol=%s", ErrInvalidModelCatalog, field, protocol)
}

func providerCatalogHeadersEqual(actual []ProviderCatalogHeader, expected []ProviderCatalogHeader) bool {
	return slices.EqualFunc(actual, expected, func(actualHeader ProviderCatalogHeader, expectedHeader ProviderCatalogHeader) bool {
		return actualHeader == expectedHeader
	})
}

func compileProviderCatalogSchema(schema ProviderCatalogSchema, revision string) (ModelCatalog, error) {
	modelCatalog := ModelCatalog{
		Revision:   revision,
		Operations: append([]ModelOperationKind(nil), schema.Operations...),
		Publishers: append([]ModelPublisher(nil), schema.Publishers...),
		Families:   append([]ModelFamily(nil), schema.Families...),
	}
	for _, model := range schema.Models {
		modelCatalog.Models = append(modelCatalog.Models, model.ExactModel)
	}
	for _, provider := range schema.Providers {
		modelCatalog.Providers = append(modelCatalog.Providers, CatalogProvider{
			ID: provider.ID, Label: provider.Label, CredentialKinds: []string{CatalogCredentialAPIKey},
		})
		transports := make(map[string]ProviderCatalogTransport, len(provider.Transports))
		for _, transport := range provider.Transports {
			transports[transport.ID] = transport
		}
		for _, rawOffering := range provider.Offerings {
			transport := transports[rawOffering.Transport]
			protocolDefinition, _ := providerProtocolDefinitionFor(transport.Protocol, "")
			offering := ProviderOffering{
				Provider:                provider.ID,
				Model:                   rawOffering.Model,
				ProviderModel:           rawOffering.UpstreamModel,
				Transport:               rawOffering.Transport,
				Operations:              append([]string(nil), rawOffering.Operations...),
				DefaultOperations:       append([]string(nil), rawOffering.DefaultOperations...),
				WireContract:            transport.Protocol.ID,
				ExecutionLifecycle:      transport.Lifecycle,
				MediaExecutionLifecycle: protocolDefinition.parameters.MediaExecutionLifecycle,
				RequestProfile:          rawOffering.RequestProfile,
				WebSearch:               rawOffering.WebSearch, CallerTools: rawOffering.CallerTools, Created: rawOffering.Created,
				OutputTokenLimit: rawOffering.OutputTokenLimit,
				ReasoningEffort:  rawOffering.ReasoningEffort,
				MediaInputs:      append([]string(nil), rawOffering.MediaInputs...),
				ImageMIMETypes:   append([]string(nil), rawOffering.ImageMIMETypes...),
				MediaLimits:      cloneCatalogMediaLimits(rawOffering.MediaLimits),
				Controls:         append([]CatalogControl(nil), rawOffering.Controls...),
				Limits:           append([]CatalogLimit(nil), rawOffering.Limits...),
			}
			modelCatalog.Offerings = append(modelCatalog.Offerings, offering)
			for _, rawPrice := range rawOffering.Prices {
				modelCatalog.Prices = append(modelCatalog.Prices, CatalogPriceDescriptor{
					Provider: provider.ID, Model: rawOffering.Model, Operation: rawPrice.Operation,
					Available: rawPrice.Available, Rates: append([]CatalogPriceRate(nil), rawPrice.Rates...),
					MinimumCharge: rawPrice.MinimumCharge, Source: rawPrice.Source,
					LastVerified: rawPrice.LastVerified, UnavailableReason: rawPrice.UnavailableReason,
				})
			}
		}
	}
	if _, validationError := validateModelCatalogStructure(modelCatalog); validationError != nil {
		return ModelCatalog{}, validationError
	}
	return cloneModelCatalog(modelCatalog), nil
}

func cloneProviderCatalogSchema(schema ProviderCatalogSchema) ProviderCatalogSchema {
	document, _ := yaml.Marshal(schema)
	decoder := yaml.NewDecoder(bytes.NewReader(document))
	decoder.KnownFields(true)
	var cloned ProviderCatalogSchema
	_ = decoder.Decode(&cloned)
	return cloned
}
