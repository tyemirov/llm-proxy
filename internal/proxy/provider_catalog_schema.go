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
	ProviderCatalogSchemaVersion = 1

	CatalogProviderFieldKindCredential = "credential"
	CatalogProviderFieldKindSetting    = "setting"
	CatalogProviderFieldTypeOpaque     = "opaque"
	CatalogProviderFieldTypeURL        = "url"

	CatalogAuthenticationBearer           = "bearer"
	CatalogAuthenticationHeader           = "header"
	CatalogEndpointMethodPost             = "POST"
	CatalogProtocolOpenAIResponses        = "openai_responses"
	CatalogProtocolOpenAIChatCompletions  = "openai_chat_completions"
	CatalogProtocolAnthropicMessages      = "anthropic_messages"
	CatalogProtocolGeminiInteractions     = "gemini_interactions"
	CatalogProtocolMultipartTranscription = "multipart_transcription"

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
	Models          []ExactModel                    `yaml:"models"`
	Providers       []ProviderCatalogProvider       `yaml:"providers"`
}

// ProviderCatalogModelMigration maps one persisted model identifier to the current catalog contract.
type ProviderCatalogModelMigration struct {
	ManagedSchemaVersion int    `yaml:"managed_schema_version"`
	Provider             string `yaml:"provider"`
	Operation            string `yaml:"operation"`
	SourceModel          string `yaml:"source_model"`
	TargetModel          string `yaml:"target_model"`
}

// ProviderCatalogProvider defines one provider and all provider-owned routes.
type ProviderCatalogProvider struct {
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
	RequestProtocol    string                            `yaml:"request_protocol"`
	ResponseProtocol   string                            `yaml:"response_protocol"`
	UsageMapping       string                            `yaml:"usage_mapping"`
	Lifecycle          string                            `yaml:"lifecycle"`
	ResourceVisibility ProviderCatalogResourceVisibility `yaml:"resource_visibility,omitempty"`
	ProtocolParameters ProviderCatalogProtocolParameters `yaml:"protocol_parameters"`
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

// ProviderCatalogProtocolParameters declares the adapter-owned wire fields and outcomes.
type ProviderCatalogProtocolParameters struct {
	ModelField              string                     `yaml:"model_field"`
	TokenField              string                     `yaml:"token_field"`
	MediaExecutionLifecycle string                     `yaml:"media_execution_lifecycle,omitempty"`
	OutputFields            []string                   `yaml:"output_fields"`
	FinishRules             ProviderCatalogFinishRules `yaml:"finish_rules"`
	ContinuationRules       []string                   `yaml:"continuation_rules"`
	ErrorRules              []string                   `yaml:"error_rules"`
	UsageFields             ProviderCatalogUsageFields `yaml:"usage_fields"`
}

// ProviderCatalogFinishRules declares exact complete and incomplete signals.
type ProviderCatalogFinishRules struct {
	Complete []string `yaml:"complete"`
	Continue []string `yaml:"continue"`
}

// ProviderCatalogUsageFields maps provider usage values to canonical token counts.
type ProviderCatalogUsageFields struct {
	Input  string `yaml:"input"`
	Output string `yaml:"output"`
	Total  string `yaml:"total"`
}

// ProviderCatalogOffering defines one exact model route inside its provider.
type ProviderCatalogOffering struct {
	Created     int64 `yaml:"created"`
	CallerTools bool  `yaml:"caller_tools,omitempty"`

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
	schema       ProviderCatalogSchema
	modelCatalog ModelCatalog
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
	if migrationError := validateProviderCatalogModelMigrations(schema.ModelMigrations, modelCatalog); migrationError != nil {
		return nil, migrationError
	}
	return &ProviderCatalog{schema: cloneProviderCatalogSchema(schema), modelCatalog: modelCatalog}, nil
}

func validateProviderCatalogModelMigrations(migrations []ProviderCatalogModelMigration, catalog ModelCatalog) error {
	providers := make(map[string]struct{}, len(catalog.Providers))
	for _, provider := range catalog.Providers {
		providers[provider.ID] = struct{}{}
	}
	offerings := make(map[string]struct{}, len(catalog.Offerings))
	for _, offering := range catalog.Offerings {
		for _, operation := range offering.Operations {
			offerings[offering.Provider+"\x00"+operation+"\x00"+offering.Model] = struct{}{}
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
		if _, found := offerings[provider+"\x00"+migration.Operation+"\x00"+migration.TargetModel]; !found {
			return fmt.Errorf("%w: field=%s.target_model provider=%s operation=%s model=%s reason=dangling_reference", ErrInvalidModelCatalog, fieldPrefix, provider, migration.Operation, migration.TargetModel)
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
	providerIdentifiers := map[string]struct{}{}
	providerAliases := map[string]string{}
	environmentBindings := map[string]string{}
	for providerIndex, provider := range schema.Providers {
		fieldPrefix := fmt.Sprintf("providers[%d]", providerIndex)
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
			if _, found := transports[offering.Transport]; !found {
				return fmt.Errorf("%w: field=%s.transport transport=%s reason=dangling_reference", ErrInvalidModelCatalog, offeringField, offering.Transport)
			}
			if offering.Created <= 0 {
				return fmt.Errorf("%w: field=%s.created", ErrInvalidModelCatalog, offeringField)
			}
			if offering.CallerTools {
				transport := transports[offering.Transport]
				supported := transport.RequestProtocol == CatalogProtocolOpenAIChatCompletions || (transport.RequestProtocol == CatalogProtocolOpenAIResponses && (offering.RequestProfile == string(requestProfileOpenAIResponsesReasoningTools) || offering.RequestProfile == string(requestProfileOpenAIResponsesTemperatureTools)))
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
		if !knownProviderCatalogProtocol(transport.RequestProtocol) || !knownProviderCatalogProtocol(transport.ResponseProtocol) || !knownProviderCatalogProtocol(transport.UsageMapping) {
			return nil, fmt.Errorf("%w: field=%s reason=unsupported_protocol", ErrInvalidModelCatalog, fieldPrefix)
		}
		if transport.RequestProtocol != transport.ResponseProtocol || transport.RequestProtocol != transport.UsageMapping {
			return nil, fmt.Errorf("%w: field=%s reason=protocol_mismatch", ErrInvalidModelCatalog, fieldPrefix)
		}
		if !knownTextExecutionLifecycle(textExecutionLifecycle(transport.Lifecycle)) {
			return nil, fmt.Errorf("%w: field=%s.lifecycle lifecycle=%s", ErrInvalidModelCatalog, fieldPrefix, transport.Lifecycle)
		}
		if visibilityError := validateProviderCatalogResourceVisibility(transport, fieldPrefix+".resource_visibility"); visibilityError != nil {
			return nil, visibilityError
		}
		if parametersError := validateProviderCatalogProtocolParameters(transport.ProtocolParameters, fieldPrefix+".protocol_parameters"); parametersError != nil {
			return nil, parametersError
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
		(transport.RequestProtocol == CatalogProtocolOpenAIResponses || transport.RequestProtocol == CatalogProtocolGeminiInteractions)
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

func validateProviderCatalogProtocolParameters(parameters ProviderCatalogProtocolParameters, field string) error {
	if strings.TrimSpace(parameters.ModelField) != parameters.ModelField || len(parameters.OutputFields) == 0 {
		return fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
	}
	for _, values := range [][]string{parameters.OutputFields, parameters.FinishRules.Complete, parameters.FinishRules.Continue, parameters.ContinuationRules, parameters.ErrorRules} {
		seen := map[string]struct{}{}
		for _, value := range values {
			if strings.TrimSpace(value) == constants.EmptyString || value != strings.TrimSpace(value) {
				return fmt.Errorf("%w: field=%s reason=invalid_protocol_value", ErrInvalidModelCatalog, field)
			}
			if _, duplicate := seen[value]; duplicate {
				return fmt.Errorf("%w: field=%s duplicate=%s", ErrInvalidModelCatalog, field, value)
			}
			seen[value] = struct{}{}
		}
	}
	return nil
}

func validateProviderCatalogAdapterContract(transport ProviderCatalogTransport, field string) error {
	var parameters ProviderCatalogProtocolParameters
	expectedAuthentication := ProviderCatalogAuthentication{
		Kind: CatalogAuthenticationBearer, Field: transport.Authentication.Field,
		Header: "Authorization", Prefix: "Bearer ",
	}
	expectedHeaders := []ProviderCatalogHeader(nil)
	var allowedLifecycles []string

	switch transport.RequestProtocol {
	case CatalogProtocolOpenAIResponses:
		allowedLifecycles = []string{string(textExecutionLifecyclePollableResource), string(textExecutionLifecycleSynchronousCompletion)}
		parameters = ProviderCatalogProtocolParameters{
			ModelField: "model", TokenField: "max_output_tokens", MediaExecutionLifecycle: transport.Lifecycle,
			OutputFields: []string{"output[].content[].text", "output[].type", "output[].call_id", "output[].name", "output[].arguments"},
			FinishRules: ProviderCatalogFinishRules{
				Complete: []string{"completed"}, Continue: []string{"incomplete:max_output_tokens"},
			},
			ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
			ErrorRules:        []string{"cancelled", "failed", "refusal", "unknown_status"},
			UsageFields: ProviderCatalogUsageFields{
				Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens",
			},
		}
	case CatalogProtocolOpenAIChatCompletions:
		allowedLifecycles = []string{string(textExecutionLifecycleSynchronousCompletion)}
		if transport.ProtocolParameters.TokenField != string(chatCompletionTokenLimitMaxTokens) && transport.ProtocolParameters.TokenField != string(chatCompletionTokenLimitMaxCompletionTokens) {
			return providerCatalogAdapterContractError(field, transport.RequestProtocol)
		}
		parameters = ProviderCatalogProtocolParameters{
			ModelField: "model", TokenField: transport.ProtocolParameters.TokenField,
			MediaExecutionLifecycle: string(textExecutionLifecycleSynchronousCompletion),
			OutputFields:            []string{"choices[].message.content", "choices[].message.tool_calls"},
			FinishRules: ProviderCatalogFinishRules{
				Complete: []string{"stop", "tool_calls"}, Continue: []string{"length"},
			},
			ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
			ErrorRules:        []string{"content_filter", "unknown_finish_reason"},
			UsageFields: ProviderCatalogUsageFields{
				Input: "usage.prompt_tokens", Output: "usage.completion_tokens", Total: "usage.total_tokens",
			},
		}
	case CatalogProtocolAnthropicMessages:
		allowedLifecycles = []string{string(textExecutionLifecycleSynchronousCompletion)}
		expectedAuthentication = ProviderCatalogAuthentication{
			Kind: CatalogAuthenticationHeader, Field: transport.Authentication.Field, Header: "x-api-key",
		}
		expectedHeaders = []ProviderCatalogHeader{{Name: "anthropic-version", Value: "2023-06-01"}}
		parameters = ProviderCatalogProtocolParameters{
			ModelField: "model", TokenField: "max_tokens",
			MediaExecutionLifecycle: string(textExecutionLifecycleSynchronousCompletion),
			OutputFields:            []string{"content[].text"},
			FinishRules: ProviderCatalogFinishRules{
				Complete: []string{"end_turn", "stop_sequence"}, Continue: []string{"max_tokens"},
			},
			ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
			ErrorRules:        []string{"pause_turn", "refusal", "tool_use", "unknown_stop_reason"},
			UsageFields: ProviderCatalogUsageFields{
				Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "derived_input_plus_output",
			},
		}
	case CatalogProtocolGeminiInteractions:
		allowedLifecycles = []string{string(textExecutionLifecyclePollableResource), string(textExecutionLifecycleSynchronousCompletion)}
		expectedAuthentication = ProviderCatalogAuthentication{
			Kind: CatalogAuthenticationHeader, Field: transport.Authentication.Field, Header: "x-goog-api-key",
		}
		expectedHeaders = []ProviderCatalogHeader{{Name: "Api-Revision", Value: "2026-05-20"}}
		parameters = ProviderCatalogProtocolParameters{
			ModelField: "model", TokenField: "generation_config.max_output_tokens",
			MediaExecutionLifecycle: string(textExecutionLifecycleSynchronousCompletion),
			OutputFields:            []string{"outputs[].text"},
			FinishRules: ProviderCatalogFinishRules{
				Complete: []string{"completed"}, Continue: []string{"incomplete"},
			},
			ContinuationRules: []string{},
			ErrorRules:        []string{"blocked", "cancelled", "failed", "unknown_status"},
			UsageFields: ProviderCatalogUsageFields{
				Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens",
			},
		}
	case CatalogProtocolMultipartTranscription:
		allowedLifecycles = []string{string(textExecutionLifecycleSynchronousCompletion)}
		if transport.ProtocolParameters.ModelField != constants.EmptyString && transport.ProtocolParameters.ModelField != "model" {
			return providerCatalogAdapterContractError(field, transport.RequestProtocol)
		}
		parameters = ProviderCatalogProtocolParameters{
			ModelField: transport.ProtocolParameters.ModelField, TokenField: constants.EmptyString,
			OutputFields:      []string{"text"},
			FinishRules:       ProviderCatalogFinishRules{Complete: []string{"http_2xx"}, Continue: []string{}},
			ContinuationRules: []string{}, ErrorRules: []string{"malformed_response", "provider_error"},
			UsageFields: ProviderCatalogUsageFields{},
		}
	case CatalogProtocolXAIVideosGenerations:
		allowedLifecycles = []string{string(textExecutionLifecyclePollableResource)}
		parameters = ProviderCatalogProtocolParameters{
			ModelField: "model", TokenField: constants.EmptyString,
			OutputFields:      []string{"data[].url"},
			FinishRules:       ProviderCatalogFinishRules{Complete: []string{"completed"}, Continue: []string{"pending"}},
			ContinuationRules: []string{}, ErrorRules: []string{"failed", "unknown_status"},
			UsageFields: ProviderCatalogUsageFields{},
		}
	default:
		return providerCatalogAdapterContractError(field, transport.RequestProtocol)
	}

	if !slices.Contains(allowedLifecycles, transport.Lifecycle) ||
		transport.Authentication != expectedAuthentication ||
		!providerCatalogHeadersEqual(transport.Headers, expectedHeaders) ||
		!providerCatalogProtocolParametersEqual(transport.ProtocolParameters, parameters) {
		return providerCatalogAdapterContractError(field, transport.RequestProtocol)
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

func providerCatalogProtocolParametersEqual(actual ProviderCatalogProtocolParameters, expected ProviderCatalogProtocolParameters) bool {
	return actual.ModelField == expected.ModelField &&
		actual.TokenField == expected.TokenField &&
		actual.MediaExecutionLifecycle == expected.MediaExecutionLifecycle &&
		slices.Equal(actual.OutputFields, expected.OutputFields) &&
		slices.Equal(actual.FinishRules.Complete, expected.FinishRules.Complete) &&
		slices.Equal(actual.FinishRules.Continue, expected.FinishRules.Continue) &&
		slices.Equal(actual.ContinuationRules, expected.ContinuationRules) &&
		slices.Equal(actual.ErrorRules, expected.ErrorRules) &&
		actual.UsageFields == expected.UsageFields
}

func knownProviderCatalogProtocol(protocol string) bool {
	switch protocol {
	case CatalogProtocolOpenAIResponses,
		CatalogProtocolOpenAIChatCompletions,
		CatalogProtocolAnthropicMessages,
		CatalogProtocolGeminiInteractions,
		CatalogProtocolMultipartTranscription,
		CatalogProtocolXAIVideosGenerations:
		return true
	default:
		return false
	}
}

func compileProviderCatalogSchema(schema ProviderCatalogSchema, revision string) (ModelCatalog, error) {
	modelCatalog := ModelCatalog{
		Revision:   revision,
		Operations: append([]ModelOperationKind(nil), schema.Operations...),
		Publishers: append([]ModelPublisher(nil), schema.Publishers...),
		Families:   append([]ModelFamily(nil), schema.Families...),
		Models:     append([]ExactModel(nil), schema.Models...),
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
			offering := ProviderOffering{
				Provider:                provider.ID,
				Model:                   rawOffering.Model,
				ProviderModel:           rawOffering.UpstreamModel,
				Transport:               rawOffering.Transport,
				Operations:              append([]string(nil), rawOffering.Operations...),
				DefaultOperations:       append([]string(nil), rawOffering.DefaultOperations...),
				WireContract:            transport.RequestProtocol,
				ExecutionLifecycle:      transport.Lifecycle,
				MediaExecutionLifecycle: transport.ProtocolParameters.MediaExecutionLifecycle,
				RequestProfile:          rawOffering.RequestProfile,
				WebSearch:               rawOffering.WebSearch, CallerTools: rawOffering.CallerTools, Created: rawOffering.Created,
				OutputTokenLimit: rawOffering.OutputTokenLimit,
				ReasoningEffort:  rawOffering.ReasoningEffort,
				MediaInputs:      append([]string(nil), rawOffering.MediaInputs...),
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
	if _, validationError := validateModelCatalog(modelCatalog); validationError != nil {
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
