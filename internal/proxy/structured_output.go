package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const structuredOutputSchemaResource = "urn:llm-proxy:structured-output-schema"

var (
	errStructuredOutputInvalid     = errors.New("structured output is invalid")
	errStructuredOutputUnsupported = errors.New("structured output is unsupported for the selected route")
	marshalStructuredOutputSchema  = json.Marshal
	addStructuredOutputResource    = func(compiler *jsonschema.Compiler, resource string, document any) error {
		return compiler.AddResource(resource, document)
	}
)

type structuredOutputInput struct {
	Schema json.RawMessage `json:"schema"`
}

type structuredOutputSchema struct {
	canonical []byte
	document  any
	compiled  *jsonschema.Schema
}

type structuredOutputSchemaRules struct {
	provider             string
	keywords             map[string]struct{}
	formats              map[string]struct{}
	requireRootObject    bool
	requireClosedObjects bool
	requireAllProperties bool
	rejectRecursiveRoot  bool
	restrictArrayMinimum bool
	rejectComplexEnum    bool
	rejectReferenceAllOf bool
}

var (
	openAIStructuredOutputRules = structuredOutputSchemaRules{
		provider: "OpenAI", keywords: structuredOutputKeywords(
			"$defs", "$ref", "additionalProperties", "anyOf", "const", "definitions", "description", "enum",
			"exclusiveMaximum", "exclusiveMinimum", "format", "items", "maxItems", "maximum", "minItems",
			"minimum", "multipleOf", "pattern", "properties", "required", "title", "type",
		),
		formats:           structuredOutputKeywords("date-time", "time", "date", "duration", "email", "hostname", "ipv4", "ipv6", "uuid"),
		requireRootObject: true, requireClosedObjects: true, requireAllProperties: true,
	}
	geminiStructuredOutputRules = structuredOutputSchemaRules{
		provider: "Gemini", keywords: structuredOutputKeywords(
			"$defs", "$ref", "additionalProperties", "anyOf", "description", "enum", "format", "items", "maxItems",
			"maximum", "minItems", "minimum", "prefixItems", "properties", "required", "title", "type",
		),
		formats: structuredOutputKeywords("date-time", "date", "time"),
	}
	anthropicStructuredOutputRules = structuredOutputSchemaRules{
		provider: "Anthropic", keywords: structuredOutputKeywords(
			"$defs", "$ref", "additionalProperties", "allOf", "anyOf", "const", "default", "definitions",
			"description", "enum", "format", "items", "minItems", "pattern", "properties", "required", "title", "type",
		),
		formats:              structuredOutputKeywords("date-time", "time", "date", "duration", "email", "hostname", "uri", "ipv4", "ipv6", "uuid"),
		requireClosedObjects: true, rejectRecursiveRoot: true, restrictArrayMinimum: true,
		rejectComplexEnum: true, rejectReferenceAllOf: true,
	}
)

func structuredOutputKeywords(values ...string) map[string]struct{} {
	keywords := make(map[string]struct{}, len(values))
	for _, value := range values {
		keywords[value] = struct{}{}
	}
	return keywords
}

func newStructuredOutputSchema(rawInput json.RawMessage) (*structuredOutputSchema, error) {
	if len(rawInput) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(rawInput))
	decoder.DisallowUnknownFields()
	var input *structuredOutputInput
	if decodeError := decoder.Decode(&input); decodeError != nil || input == nil {
		return nil, fmt.Errorf("%w: structured_output must be one object", errStructuredOutputInvalid)
	}
	if decodeError := decoder.Decode(&struct{}{}); decodeError != io.EOF {
		return nil, fmt.Errorf("%w: structured_output must contain one JSON value", errStructuredOutputInvalid)
	}
	if len(input.Schema) == 0 || bytes.Equal(bytes.TrimSpace(input.Schema), []byte("null")) {
		return nil, fmt.Errorf("%w: structured_output.schema is required", errStructuredOutputInvalid)
	}
	var document map[string]any
	if decodeError := json.Unmarshal(input.Schema, &document); decodeError != nil || document == nil {
		return nil, fmt.Errorf("%w: schema must be one JSON object", errStructuredOutputInvalid)
	}
	canonical, marshalError := marshalStructuredOutputSchema(document)
	if marshalError != nil {
		return nil, fmt.Errorf("%w: encode schema: %v", errStructuredOutputInvalid, marshalError)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if resourceError := addStructuredOutputResource(compiler, structuredOutputSchemaResource, document); resourceError != nil {
		return nil, fmt.Errorf("%w: load schema: %v", errStructuredOutputInvalid, resourceError)
	}
	compiled, compileError := compiler.Compile(structuredOutputSchemaResource)
	if compileError != nil {
		return nil, fmt.Errorf("%w: compile schema: %v", errStructuredOutputInvalid, compileError)
	}
	return &structuredOutputSchema{canonical: canonical, document: document, compiled: compiled}, nil
}

func validateStructuredOutputRoute(model textModelDefinition, schema *structuredOutputSchema) error {
	if schema == nil {
		return nil
	}
	switch model.wireContract {
	case textWireContractOpenAIResponses:
		return validateStructuredOutputSchemaSubset(schema.document.(map[string]any), openAIStructuredOutputRules, true)
	case textWireContractGeminiInteractions:
		return validateStructuredOutputSchemaSubset(schema.document.(map[string]any), geminiStructuredOutputRules, true)
	case textWireContractAnthropicMessages:
		return validateStructuredOutputSchemaSubset(schema.document.(map[string]any), anthropicStructuredOutputRules, true)
	default:
		return errStructuredOutputUnsupported
	}
}

func validateStructuredOutputSchemaSubset(document map[string]any, rules structuredOutputSchemaRules, root bool) error {
	if root && rules.requireRootObject {
		if document["type"] != "object" || document["anyOf"] != nil {
			return structuredOutputSchemaSubsetError(rules, "root must be one object")
		}
	}
	for keyword := range document {
		if _, supported := rules.keywords[keyword]; !supported {
			return structuredOutputSchemaSubsetError(rules, "contains an unsupported keyword")
		}
	}
	if objectProperties, objectSchema := document["properties"].(map[string]any); objectSchema || structuredOutputTypeIncludes(document["type"], "object") {
		if rules.requireClosedObjects && document["additionalProperties"] != false {
			return structuredOutputSchemaSubsetError(rules, "objects must set additionalProperties to false")
		}
		if rules.requireAllProperties && !structuredOutputAllPropertiesRequired(objectProperties, document["required"]) {
			return structuredOutputSchemaSubsetError(rules, "all object properties must be required")
		}
	}
	if format, present := document["format"].(string); present {
		if _, supported := rules.formats[format]; !supported {
			return structuredOutputSchemaSubsetError(rules, "contains an unsupported string format")
		}
	}
	if reference, present := document["$ref"].(string); present && (!strings.HasPrefix(reference, "#") || rules.rejectRecursiveRoot && reference == "#") {
		return structuredOutputSchemaSubsetError(rules, "contains an unsupported reference")
	}
	if minimum, present := document["minItems"].(float64); present && rules.restrictArrayMinimum && minimum != 0 && minimum != 1 {
		return structuredOutputSchemaSubsetError(rules, "contains an unsupported array minimum")
	}
	if enumValues, present := document["enum"].([]any); present && rules.rejectComplexEnum && !structuredOutputPrimitiveEnum(enumValues) {
		return structuredOutputSchemaSubsetError(rules, "contains a complex enum value")
	}
	if rules.rejectReferenceAllOf && structuredOutputAllOfContainsReference(document["allOf"]) {
		return structuredOutputSchemaSubsetError(rules, "contains allOf with a reference")
	}
	children, childError := structuredOutputChildSchemas(document)
	if childError != nil {
		return structuredOutputSchemaSubsetError(rules, "contains a boolean schema")
	}
	for _, child := range children {
		if validationError := validateStructuredOutputSchemaSubset(child, rules, false); validationError != nil {
			return validationError
		}
	}
	return nil
}

func structuredOutputTypeIncludes(rawType any, expected string) bool {
	if rawType == expected {
		return true
	}
	types, multiple := rawType.([]any)
	if !multiple {
		return false
	}
	for _, schemaType := range types {
		if schemaType == expected {
			return true
		}
	}
	return false
}

func structuredOutputAllPropertiesRequired(properties map[string]any, rawRequired any) bool {
	if len(properties) == 0 {
		return true
	}
	required, present := rawRequired.([]any)
	if !present || len(required) != len(properties) {
		return false
	}
	for _, property := range required {
		if _, found := properties[property.(string)]; !found {
			return false
		}
	}
	return true
}

func structuredOutputPrimitiveEnum(values []any) bool {
	for _, value := range values {
		switch value.(type) {
		case nil, bool, float64, string:
		default:
			return false
		}
	}
	return true
}

func structuredOutputAllOfContainsReference(rawAllOf any) bool {
	allOf, present := rawAllOf.([]any)
	if !present {
		return false
	}
	for _, rawSchema := range allOf {
		if schema, object := rawSchema.(map[string]any); object && schema["$ref"] != nil {
			return true
		}
	}
	return false
}

func structuredOutputChildSchemas(document map[string]any) ([]map[string]any, error) {
	children := make([]map[string]any, 0)
	for _, objectKeyword := range []string{"properties", "$defs", "definitions"} {
		if schemas, present := document[objectKeyword].(map[string]any); present {
			for _, rawSchema := range schemas {
				schema, object := rawSchema.(map[string]any)
				if !object {
					return nil, errStructuredOutputUnsupported
				}
				children = append(children, schema)
			}
		}
	}
	if rawItems, present := document["items"]; present {
		items, object := rawItems.(map[string]any)
		if !object {
			return nil, errStructuredOutputUnsupported
		}
		children = append(children, items)
	}
	if rawAdditional, present := document["additionalProperties"]; present {
		if schema, object := rawAdditional.(map[string]any); object {
			children = append(children, schema)
		} else if _, boolean := rawAdditional.(bool); !boolean {
			return nil, errStructuredOutputUnsupported
		}
	}
	for _, arrayKeyword := range []string{"anyOf", "allOf", "prefixItems"} {
		if schemas, present := document[arrayKeyword].([]any); present {
			for _, rawSchema := range schemas {
				schema, object := rawSchema.(map[string]any)
				if !object {
					return nil, errStructuredOutputUnsupported
				}
				children = append(children, schema)
			}
		}
	}
	return children, nil
}

func structuredOutputSchemaSubsetError(rules structuredOutputSchemaRules, reason string) error {
	return fmt.Errorf("%w: %s schema %s", errStructuredOutputUnsupported, rules.provider, reason)
}

func (schema *structuredOutputSchema) validateResponse(rawOutput string) error {
	if schema == nil {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(rawOutput))
	decoder.UseNumber()
	var value any
	if decodeError := decoder.Decode(&value); decodeError != nil {
		return fmt.Errorf("%w: provider output is not JSON", ErrProviderAPI)
	}
	if decodeError := decoder.Decode(&struct{}{}); decodeError != io.EOF {
		return fmt.Errorf("%w: provider output contains multiple JSON values", ErrProviderAPI)
	}
	if validationError := schema.compiled.Validate(value); validationError != nil {
		return fmt.Errorf("%w: provider output violates structured schema", ErrProviderAPI)
	}
	return nil
}

type openAIStructuredText struct {
	Format openAIStructuredTextFormat `json:"format"`
}

type openAIStructuredTextFormat struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Strict bool   `json:"strict"`
	Schema any    `json:"schema"`
}

func openAIStructuredTextFor(schema *structuredOutputSchema) *openAIStructuredText {
	if schema == nil {
		return nil
	}
	return &openAIStructuredText{Format: openAIStructuredTextFormat{
		Type: "json_schema", Name: "llm_proxy_structured_output", Strict: true, Schema: schema.document,
	}}
}

type geminiStructuredResponseFormat struct {
	Type     string `json:"type"`
	MIMEType string `json:"mime_type"`
	Schema   any    `json:"schema"`
}

func geminiStructuredResponseFormats(schema *structuredOutputSchema) []geminiStructuredResponseFormat {
	if schema == nil {
		return nil
	}
	return []geminiStructuredResponseFormat{{Type: "text", MIMEType: "application/json", Schema: schema.document}}
}

type anthropicStructuredOutputConfig struct {
	Format anthropicStructuredOutputFormat `json:"format"`
}

type anthropicStructuredOutputFormat struct {
	Type   string `json:"type"`
	Schema any    `json:"schema"`
}

func anthropicStructuredOutputFor(schema *structuredOutputSchema) *anthropicStructuredOutputConfig {
	if schema == nil {
		return nil
	}
	return &anthropicStructuredOutputConfig{Format: anthropicStructuredOutputFormat{
		Type: "json_schema", Schema: schema.document,
	}}
}
