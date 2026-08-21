package proxy

import (
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProviderCatalogParserRejectsTrailingDocuments(t *testing.T) {
	document, marshalError := yaml.Marshal(internalCanonicalProviderCatalog().Schema())
	if marshalError != nil {
		t.Fatalf("marshal canonical provider catalog: %v", marshalError)
	}
	for _, testCase := range []struct {
		name     string
		trailing string
		expected string
	}{
		{name: "second document", trailing: "\n---\n{}\n", expected: "multiple YAML documents"},
		{name: "malformed second document", trailing: "\n---\n[\n", expected: "did not find expected node content"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, catalogError := ParseProviderCatalog(append(append([]byte(nil), document...), testCase.trailing...))
			assertInvalidProviderCatalogError(t, catalogError, testCase.expected)
		})
	}
}

func TestProviderCatalogSchemaRejectsEveryStructuralBoundary(t *testing.T) {
	testCases := []struct {
		name     string
		mutate   func(*ProviderCatalogSchema)
		expected string
	}{
		{name: "providers missing", mutate: func(schema *ProviderCatalogSchema) { schema.Providers = nil }, expected: "field=providers"},
		{name: "provider identifier", mutate: func(schema *ProviderCatalogSchema) { schema.Providers[0].ID = "OpenAI" }, expected: "reason=not_canonical"},
		{name: "provider identifier collides with prior alias", mutate: func(schema *ProviderCatalogSchema) {
			schema.Providers[0].Aliases = append(schema.Providers[0].Aliases, schema.Providers[1].ID)
		}, expected: "alias_collision=deepseek"},
		{name: "provider label", mutate: func(schema *ProviderCatalogSchema) { schema.Providers[0].Label = " OpenAI" }, expected: ".label"},
		{name: "provider alias", mutate: func(schema *ProviderCatalogSchema) { schema.Providers[0].Aliases = []string{"Future Alias"} }, expected: "reason=not_canonical"},
		{name: "provider fields", mutate: func(schema *ProviderCatalogSchema) { schema.Providers[0].Fields = nil }, expected: ".fields"},
		{name: "duplicate environment binding", mutate: func(schema *ProviderCatalogSchema) {
			schema.Providers[1].Fields[0].Environment = schema.Providers[0].Fields[0].Environment
		}, expected: "duplicate_binding="},
		{name: "provider transports", mutate: func(schema *ProviderCatalogSchema) { schema.Providers[0].Transports = nil }, expected: ".transports"},
		{name: "provider offerings", mutate: func(schema *ProviderCatalogSchema) { schema.Providers[0].Offerings = nil }, expected: ".offerings"},
		{name: "offering transport", mutate: func(schema *ProviderCatalogSchema) { schema.Providers[0].Offerings[0].Transport = "missing" }, expected: "reason=dangling_reference"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			schema := internalCanonicalProviderCatalog().Schema()
			testCase.mutate(&schema)
			_, catalogError := NewProviderCatalog(schema)
			assertInvalidProviderCatalogError(t, catalogError, testCase.expected)
		})
	}
}

func TestProviderCatalogFieldValidationRejectsEveryInvalidShape(t *testing.T) {
	fieldCases := []struct {
		name     string
		fields   func() []ProviderCatalogField
		expected string
	}{
		{name: "missing", fields: func() []ProviderCatalogField { return nil }, expected: "field=fields"},
		{name: "identifier", fields: func() []ProviderCatalogField {
			credential := internalValidCredentialField()
			credential.ID = "API Key"
			return []ProviderCatalogField{credential}
		}, expected: "reason=not_canonical"},
		{name: "duplicate", fields: func() []ProviderCatalogField {
			credential := internalValidCredentialField()
			return []ProviderCatalogField{credential, credential}
		}, expected: "duplicate_identifier=api_key"},
		{name: "label", fields: func() []ProviderCatalogField {
			credential := internalValidCredentialField()
			credential.Label = " "
			return []ProviderCatalogField{credential}
		}, expected: "field=fields[0]"},
		{name: "default missing", fields: func() []ProviderCatalogField {
			credential := internalValidCredentialField()
			credential.Default = nil
			return []ProviderCatalogField{credential}
		}, expected: "field=fields[0]"},
		{name: "credential shape", fields: func() []ProviderCatalogField {
			credential := internalValidCredentialField()
			credential.Secret = false
			return []ProviderCatalogField{credential}
		}, expected: "invalid_credential_field"},
		{name: "setting shape", fields: func() []ProviderCatalogField {
			setting := internalValidSettingField()
			setting.Type = CatalogProviderFieldTypeOpaque
			return []ProviderCatalogField{internalValidCredentialField(), setting}
		}, expected: "invalid_setting_field"},
		{name: "setting negative minimum length", fields: func() []ProviderCatalogField {
			setting := internalValidSettingField()
			setting.Validation.MinimumLength = -1
			return []ProviderCatalogField{internalValidCredentialField(), setting}
		}, expected: "invalid_setting_field"},
		{name: "kind", fields: func() []ProviderCatalogField {
			credential := internalValidCredentialField()
			credential.Kind = "future"
			return []ProviderCatalogField{credential}
		}, expected: "kind=future"},
		{name: "environment", fields: func() []ProviderCatalogField {
			credential := internalValidCredentialField()
			credential.Environment = "invalid-name"
			return []ProviderCatalogField{credential}
		}, expected: ".environment"},
		{name: "pattern", fields: func() []ProviderCatalogField {
			setting := internalValidSettingField()
			setting.Validation.Pattern = "["
			return []ProviderCatalogField{internalValidCredentialField(), setting}
		}, expected: ".validation.pattern"},
		{name: "default value", fields: func() []ProviderCatalogField {
			setting := internalValidSettingField()
			invalidDefault := "ftp://provider.example"
			setting.Default = &invalidDefault
			return []ProviderCatalogField{internalValidCredentialField(), setting}
		}, expected: ".default"},
		{name: "credential missing", fields: func() []ProviderCatalogField {
			return []ProviderCatalogField{internalValidSettingField()}
		}, expected: "credential_missing"},
	}
	for _, testCase := range fieldCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, fieldError := validateProviderCatalogFields(testCase.fields(), "fields")
			assertInvalidProviderCatalogError(t, fieldError, testCase.expected)
		})
	}

	patternField := internalValidCredentialField()
	patternField.Validation.Pattern = `^token-[0-9]+$`
	for _, testCase := range []struct {
		name       string
		definition ProviderCatalogField
		value      string
		expected   string
	}{
		{name: "whitespace", definition: internalValidCredentialField(), value: " token", expected: "provider_field_invalid"},
		{name: "pattern mismatch", definition: patternField, value: "token-invalid", expected: "provider_field_invalid"},
		{name: "URL structure", definition: internalValidSettingField(), value: "https://user@provider.example", expected: "provider_field_invalid"},
		{name: "URL scheme", definition: internalValidSettingField(), value: "ftp://provider.example", expected: "provider_field_invalid"},
	} {
		t.Run("value "+testCase.name, func(t *testing.T) {
			_, valueError := validatedProviderFieldValue(testCase.definition, testCase.value)
			if valueError == nil || !strings.Contains(valueError.Error(), testCase.expected) {
				t.Fatalf("value error=%v want %q", valueError, testCase.expected)
			}
		})
	}
	if value, valueError := validatedProviderFieldValue(internalValidCredentialField(), "token"); valueError != nil || value != "token" {
		t.Fatalf("valid credential value=%q error=%v", value, valueError)
	}
	if value, valueError := validatedProviderFieldValue(internalValidSettingField(), "https://provider.example"); valueError != nil || value != "https://provider.example" {
		t.Fatalf("valid setting value=%q error=%v", value, valueError)
	}
}

func TestProviderCatalogTransportValidationRejectsEveryInvalidShape(t *testing.T) {
	testCases := []struct {
		name     string
		mutate   func(*[]ProviderCatalogTransport, map[string]ProviderCatalogField)
		expected string
	}{
		{name: "missing", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) { *transports = nil }, expected: "field=transports"},
		{name: "identifier", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].ID = "Text Route"
		}, expected: "reason=not_canonical"},
		{name: "duplicate", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			*transports = append(*transports, (*transports)[0])
		}, expected: "duplicate_identifier="},
		{name: "endpoint", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].Endpoint.Method = "GET"
		}, expected: ".endpoint"},
		{name: "authentication field", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].Authentication.Field = "missing"
		}, expected: "reason=dangling_reference"},
		{name: "authentication field optional", mutate: func(_ *[]ProviderCatalogTransport, fields map[string]ProviderCatalogField) {
			credential := fields[CatalogCredentialAPIKey]
			credential.Required = false
			fields[CatalogCredentialAPIKey] = credential
		}, expected: "reason=dangling_reference"},
		{name: "authentication", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].Authentication.Header = "X-Key"
		}, expected: ".authentication"},
		{name: "headers", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].Headers = []ProviderCatalogHeader{{Name: "", Value: "value"}}
		}, expected: ".headers"},
		{name: "protocol", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].RequestProtocol = "future"
		}, expected: "unsupported_protocol"},
		{name: "protocol mismatch", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].ResponseProtocol = CatalogProtocolAnthropicMessages
		}, expected: "protocol_mismatch"},
		{name: "lifecycle", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].Lifecycle = "future"
		}, expected: ".lifecycle"},
		{name: "parameters", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].ProtocolParameters.OutputFields = nil
		}, expected: ".protocol_parameters"},
		{name: "adapter", mutate: func(transports *[]ProviderCatalogTransport, _ map[string]ProviderCatalogField) {
			(*transports)[0].ProtocolParameters.ModelField = "future"
		}, expected: "adapter_contract_mismatch"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			provider := internalCanonicalProviderCatalog().Schema().Providers[0]
			fields := map[string]ProviderCatalogField{}
			for _, field := range provider.Fields {
				fields[field.ID] = field
			}
			testCase.mutate(&provider.Transports, fields)
			_, transportError := validateProviderCatalogTransports(provider.Transports, fields, "transports")
			assertInvalidProviderCatalogError(t, transportError, testCase.expected)
		})
	}
}

func TestProviderCatalogEndpointAndProtocolEdges(t *testing.T) {
	fields := map[string]ProviderCatalogField{
		"api_key":  internalValidCredentialField(),
		"base_url": internalValidSettingField(),
	}
	validEndpoint := ProviderCatalogEndpoint{Method: CatalogEndpointMethodPost, DefaultBaseURL: "https://provider.example", Path: "/responses"}
	endpointCases := []struct {
		name     string
		endpoint ProviderCatalogEndpoint
		expected string
	}{
		{name: "method", endpoint: ProviderCatalogEndpoint{Method: "GET", DefaultBaseURL: "https://provider.example", Path: "/responses"}, expected: "field=endpoint"},
		{name: "source count", endpoint: ProviderCatalogEndpoint{Method: CatalogEndpointMethodPost, Path: "/responses"}, expected: "endpoint_source_count"},
		{name: "setting field", endpoint: ProviderCatalogEndpoint{Method: CatalogEndpointMethodPost, SettingField: "missing", Path: "/responses"}, expected: "dangling_reference"},
		{name: "URL parse", endpoint: ProviderCatalogEndpoint{Method: CatalogEndpointMethodPost, DefaultBaseURL: "%", Path: "/responses"}, expected: ".default_base_url"},
		{name: "URL security", endpoint: ProviderCatalogEndpoint{Method: CatalogEndpointMethodPost, DefaultBaseURL: "http://192.0.2.1", Path: "/responses"}, expected: ".default_base_url"},
	}
	for _, testCase := range endpointCases {
		t.Run("endpoint "+testCase.name, func(t *testing.T) {
			assertInvalidProviderCatalogError(t, validateProviderCatalogEndpoint(testCase.endpoint, fields, "endpoint"), testCase.expected)
		})
	}
	for _, endpoint := range []ProviderCatalogEndpoint{
		validEndpoint,
		{Method: CatalogEndpointMethodPost, SettingField: "base_url", Path: "/responses"},
		{Method: CatalogEndpointMethodPost, DefaultBaseURL: "http://localhost:8080", Path: "/responses"},
		{Method: CatalogEndpointMethodPost, DefaultBaseURL: "http://127.0.0.1:8080", Path: "/responses"},
	} {
		if endpointError := validateProviderCatalogEndpoint(endpoint, fields, "endpoint"); endpointError != nil {
			t.Fatalf("valid endpoint=%+v error=%v", endpoint, endpointError)
		}
	}

	authenticationCases := []ProviderCatalogAuthentication{
		{Kind: CatalogAuthenticationBearer, Header: "X-Key", Prefix: "Bearer "},
		{Kind: CatalogAuthenticationHeader, Header: " ", Prefix: ""},
		{Kind: "future", Header: "Authorization"},
	}
	for authenticationIndex, authentication := range authenticationCases {
		if authenticationError := validateProviderCatalogAuthentication(authentication, "authentication"); authenticationError == nil {
			t.Fatalf("authentication case %d accepted %+v", authenticationIndex, authentication)
		}
	}
	if authenticationError := validateProviderCatalogAuthentication(ProviderCatalogAuthentication{Kind: CatalogAuthenticationHeader, Header: "x-api-key"}, "authentication"); authenticationError != nil {
		t.Fatalf("valid header authentication: %v", authenticationError)
	}

	if headerError := validateProviderCatalogHeaders([]ProviderCatalogHeader{{Name: " ", Value: "value"}}, "headers"); headerError == nil {
		t.Fatal("blank catalog header was accepted")
	}
	if headerError := validateProviderCatalogHeaders([]ProviderCatalogHeader{{Name: "X-Test", Value: "one"}, {Name: "x-test", Value: "two"}}, "headers"); headerError == nil {
		t.Fatal("duplicate catalog header was accepted")
	}

	parameters := internalCanonicalProviderCatalog().Schema().Providers[0].Transports[0].ProtocolParameters
	parameters.ModelField = " model"
	if parameterError := validateProviderCatalogProtocolParameters(parameters, "parameters"); parameterError == nil {
		t.Fatal("noncanonical protocol model field was accepted")
	}
	parameters = internalCanonicalProviderCatalog().Schema().Providers[0].Transports[0].ProtocolParameters
	parameters.OutputFields[0] = " "
	if parameterError := validateProviderCatalogProtocolParameters(parameters, "parameters"); parameterError == nil {
		t.Fatal("blank protocol value was accepted")
	}
	parameters = internalCanonicalProviderCatalog().Schema().Providers[0].Transports[0].ProtocolParameters
	parameters.OutputFields = append(parameters.OutputFields, parameters.OutputFields[0])
	if parameterError := validateProviderCatalogProtocolParameters(parameters, "parameters"); parameterError == nil {
		t.Fatal("duplicate protocol value was accepted")
	}

	chatTransport := internalProtocolTransport(t, CatalogProtocolOpenAIChatCompletions)
	chatTransport.ProtocolParameters.TokenField = "future"
	assertInvalidProviderCatalogError(t, validateProviderCatalogAdapterContract(chatTransport, "transport"), "adapter_contract_mismatch")
	transcriptionTransport := internalProtocolTransport(t, CatalogProtocolMultipartTranscription)
	transcriptionTransport.ProtocolParameters.ModelField = "future"
	assertInvalidProviderCatalogError(t, validateProviderCatalogAdapterContract(transcriptionTransport, "transport"), "adapter_contract_mismatch")
	unknownTransport := chatTransport
	unknownTransport.RequestProtocol = "future"
	assertInvalidProviderCatalogError(t, validateProviderCatalogAdapterContract(unknownTransport, "transport"), "adapter_contract_mismatch")
}

func TestProviderCatalogConnectionValueBoundaries(t *testing.T) {
	schema := internalCanonicalProviderCatalog().Schema()
	schema.Providers[0].Fields[0].Environment = ""
	catalog, catalogError := NewProviderCatalog(schema)
	if catalogError != nil {
		t.Fatalf("compile provider catalog without optional environment binding: %v", catalogError)
	}
	bindings, bindingError := catalog.ResolveEnvironmentBindings(map[string]string{"OPENAI_API_KEY": "unused"})
	if bindingError != nil || bindings[ProviderNameOpenAI] != nil {
		t.Fatalf("environment bindings=%v error=%v", bindings, bindingError)
	}

	canonicalCatalog := internalCanonicalProviderCatalog()
	if _, bindingError := canonicalCatalog.ResolveEnvironmentBindings(map[string]string{"OPENAI_API_KEY": " invalid"}); bindingError == nil {
		t.Fatal("invalid catalog environment value was accepted")
	}
	for _, testCase := range []struct {
		name   string
		values map[string]map[string]string
	}{
		{name: "provider", values: map[string]map[string]string{"future": {"api_key": "value"}}},
		{name: "field", values: map[string]map[string]string{ProviderNameOpenAI: {"future": "value"}}},
		{name: "value", values: map[string]map[string]string{ProviderNameOpenAI: {"api_key": " invalid"}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, valuesError := canonicalCatalog.validatedConnectionValues(testCase.values); valuesError == nil {
				t.Fatalf("invalid connection values were accepted: %v", testCase.values)
			}
			if _, configurationError := NewConfiguration(Configuration{ProviderCatalog: canonicalCatalog, ProviderConnectionValues: testCase.values}); configurationError == nil {
				t.Fatalf("invalid runtime connection values were accepted: %v", testCase.values)
			}
		})
	}
}

func internalValidCredentialField() ProviderCatalogField {
	empty := ""
	return ProviderCatalogField{
		ID: "api_key", Label: "API key", Kind: CatalogProviderFieldKindCredential,
		Type: CatalogProviderFieldTypeOpaque, Required: true, Default: &empty, Secret: true,
		Validation: ProviderCatalogFieldValidation{MinimumLength: 1}, Environment: "TEST_API_KEY",
	}
}

func internalValidSettingField() ProviderCatalogField {
	empty := ""
	return ProviderCatalogField{
		ID: "base_url", Label: "Base URL", Kind: CatalogProviderFieldKindSetting,
		Type: CatalogProviderFieldTypeURL, Required: true, Default: &empty,
		Validation: ProviderCatalogFieldValidation{AllowedSchemes: []string{"https"}}, Environment: "TEST_BASE_URL",
	}
}

func internalProtocolTransport(t *testing.T, protocol string) ProviderCatalogTransport {
	t.Helper()
	for _, provider := range internalCanonicalProviderCatalog().Schema().Providers {
		for _, transport := range provider.Transports {
			if transport.RequestProtocol == protocol {
				return transport
			}
		}
	}
	t.Fatalf("canonical catalog does not contain protocol %s", protocol)
	return ProviderCatalogTransport{}
}

func assertInvalidProviderCatalogError(t *testing.T, catalogError error, expected string) {
	t.Helper()
	if catalogError == nil || !errors.Is(catalogError, ErrInvalidModelCatalog) || !strings.Contains(catalogError.Error(), expected) {
		t.Fatalf("catalog error=%v want invalid catalog containing %q", catalogError, expected)
	}
}
