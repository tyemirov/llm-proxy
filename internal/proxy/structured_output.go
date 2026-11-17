package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const structuredOutputSchemaResource = "urn:llm-proxy:structured-output-schema"
const structuredOutputSchemaName = "llm_proxy_structured_output"

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

// StructuredOutputPolicy declares the schema subset enforced by an offering.
// Zero numeric limits mean unbounded; all categorical fields are explicit.
type StructuredOutputPolicy struct {
	Keywords           []string               `yaml:"keywords" mapstructure:"keywords"`
	Formats            []string               `yaml:"formats" mapstructure:"formats"`
	Root               string                 `yaml:"root" mapstructure:"root"`
	Objects            string                 `yaml:"objects" mapstructure:"objects"`
	RequiredProperties string                 `yaml:"required_properties" mapstructure:"required_properties"`
	References         string                 `yaml:"references" mapstructure:"references"`
	EnumValues         string                 `yaml:"enum_values" mapstructure:"enum_values"`
	ConstValues        string                 `yaml:"const_values" mapstructure:"const_values"`
	ArrayItems         string                 `yaml:"array_items" mapstructure:"array_items"`
	ArrayMinimum       string                 `yaml:"array_minimum" mapstructure:"array_minimum"`
	ReferenceAllOf     string                 `yaml:"reference_all_of" mapstructure:"reference_all_of"`
	TextOnly           bool                   `yaml:"text_only" mapstructure:"text_only"`
	Limits             StructuredOutputLimits `yaml:"limits" mapstructure:"limits"`
}

// StructuredOutputLimits bounds schema complexity before provider dispatch.
type StructuredOutputLimits struct {
	NestingDepth            int `yaml:"nesting_depth" mapstructure:"nesting_depth"`
	ObjectProperties        int `yaml:"object_properties" mapstructure:"object_properties"`
	EnumValues              int `yaml:"enum_values" mapstructure:"enum_values"`
	CountedSchemaCharacters int `yaml:"counted_schema_characters" mapstructure:"counted_schema_characters"`
	LargeEnumThreshold      int `yaml:"large_enum_threshold" mapstructure:"large_enum_threshold"`
	LargeEnumCharacters     int `yaml:"large_enum_characters" mapstructure:"large_enum_characters"`
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

func (policy *StructuredOutputPolicy) rules() structuredOutputSchemaRules {
	return structuredOutputSchemaRules{provider: "selected offering", keywords: structuredOutputKeywords(policy.Keywords...), formats: structuredOutputKeywords(policy.Formats...), requireRootObject: policy.Root == "object" || policy.Root == "object_with_properties", requireClosedObjects: policy.Objects == "closed", requireAllProperties: policy.RequiredProperties == "all", rejectRecursiveRoot: policy.References == "local_nonroot", restrictArrayMinimum: policy.ArrayMinimum == "zero_or_one", rejectComplexEnum: policy.EnumValues == "primitive", rejectReferenceAllOf: policy.ReferenceAllOf == "reject"}
}

func validateStructuredOutputPolicy(policy *StructuredOutputPolicy) error {
	if policy == nil {
		return nil
	}
	choices := []struct {
		value   string
		allowed []string
	}{
		{policy.Root, []string{"any", "object", "object_with_properties"}}, {policy.Objects, []string{"any", "closed"}},
		{policy.RequiredProperties, []string{"explicit", "all"}}, {policy.References, []string{"local", "local_nonroot", "local_nonrecursive"}},
		{policy.ConstValues, []string{"any", "primitive"}}, {policy.ArrayItems, []string{"optional", "required"}}, {policy.EnumValues, []string{"any", "primitive"}}, {policy.ArrayMinimum, []string{"any", "zero_or_one"}}, {policy.ReferenceAllOf, []string{"allow", "reject"}},
	}
	for _, choice := range choices {
		found := false
		for _, value := range choice.allowed {
			if choice.value == value {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: structured_output policy value=%q", ErrInvalidModelCatalog, choice.value)
		}
	}
	supported := structuredOutputKeywords("$defs", "$ref", "$schema", "additionalProperties", "allOf", "anyOf", "const", "default", "definitions", "description", "enum", "exclusiveMaximum", "exclusiveMinimum", "format", "items", "maxItems", "maximum", "minItems", "minimum", "multipleOf", "pattern", "prefixItems", "properties", "required", "title", "type")
	if len(policy.Keywords) == 0 {
		return fmt.Errorf("%w: structured_output keywords required", ErrInvalidModelCatalog)
	}
	seen := map[string]bool{}
	for _, keyword := range policy.Keywords {
		if _, ok := supported[keyword]; !ok || seen[keyword] {
			return fmt.Errorf("%w: structured_output keyword=%q", ErrInvalidModelCatalog, keyword)
		}
		seen[keyword] = true
	}
	for _, limit := range []int{policy.Limits.NestingDepth, policy.Limits.ObjectProperties, policy.Limits.EnumValues, policy.Limits.CountedSchemaCharacters, policy.Limits.LargeEnumThreshold, policy.Limits.LargeEnumCharacters} {
		if limit < 0 {
			return fmt.Errorf("%w: structured_output limits cannot be negative", ErrInvalidModelCatalog)
		}
	}
	return nil
}

func cloneStructuredOutputPolicy(policy *StructuredOutputPolicy) *StructuredOutputPolicy {
	if policy == nil {
		return nil
	}
	cloned := *policy
	cloned.Keywords = append([]string(nil), policy.Keywords...)
	cloned.Formats = append([]string(nil), policy.Formats...)
	return &cloned
}

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
	if model.structuredOutput == nil {
		return errStructuredOutputUnsupported
	}
	document := schema.document.(map[string]any)
	if err := validateStructuredOutputSchemaSubset(document, model.structuredOutput.rules(), true); err != nil {
		return err
	}
	return validateStructuredOutputComplexity(document, model.structuredOutput)
}

func validateStructuredOutputMessages(model textModelDefinition, schema *structuredOutputSchema, messages chatMessages) error {
	if schema != nil && model.structuredOutput != nil && model.structuredOutput.TextOnly && messages.mediaCount() != 0 {
		return fmt.Errorf("%w: selected offering structured output requires text-only messages", errStructuredOutputUnsupported)
	}
	return nil
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
		return newStructuredOutputResponseFailure(structuredOutputNotJSON, nil)
	}
	if decodeError := decoder.Decode(&struct{}{}); decodeError != io.EOF {
		return newStructuredOutputResponseFailure(structuredOutputMultipleJSON, nil)
	}
	if validationError := schema.compiled.Validate(value); validationError != nil {
		return newStructuredOutputResponseFailure(structuredOutputSchemaViolation, validationError)
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
		Type: "json_schema", Name: structuredOutputSchemaName, Strict: true, Schema: schema.document,
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

type anthropicOutputConfig struct {
	Format *anthropicStructuredOutputFormat `json:"format,omitempty"`
	Effort string                           `json:"effort,omitempty"`
}

type anthropicStructuredOutputFormat struct {
	Type   string `json:"type"`
	Schema any    `json:"schema"`
}

func anthropicOutputConfigFor(schema *structuredOutputSchema, reasoningEffort string) *anthropicOutputConfig {
	if schema == nil && reasoningEffort == "" {
		return nil
	}
	config := &anthropicOutputConfig{Effort: reasoningEffort}
	if schema != nil {
		config.Format = &anthropicStructuredOutputFormat{Type: "json_schema", Schema: schema.document}
	}
	return config
}

func structuredOutputCodecSupported(codec string) bool {
	switch textWireContract(codec) {
	case textWireContractOpenAIResponses, textWireContractXAIResponses, textWireContractDashScopeResponses, textWireContractOpenAIChatCompletions, textWireContractGeminiInteractions, textWireContractVertexGenerateContent, textWireContractAnthropicMessages:
		return true
	default:
		return false
	}
}

func validateStructuredOutputComplexity(document map[string]any, policy *StructuredOutputPolicy) error {
	rules := policy.rules()
	fail := func(reason string) error { return structuredOutputSchemaSubsetError(rules, reason) }
	if policy.Root == "object_with_properties" {
		if _, ok := document["properties"].(map[string]any); !ok {
			return fail("root must define properties")
		}
	}
	properties, enums, characters := 0, 0, 0
	var count func(map[string]any) error
	count = func(node map[string]any) error {
		for _, key := range []string{"properties", "$defs", "definitions"} {
			if entries, ok := node[key].(map[string]any); ok {
				for name := range entries {
					characters += utf8.RuneCountInString(name)
				}
				if key == "properties" {
					properties += len(entries)
				}
			}
		}
		if values, ok := node["enum"].([]any); ok {
			enums += len(values)
			enumChars := 0
			for _, value := range values {
				if str, ok := value.(string); ok {
					enumChars += utf8.RuneCountInString(str)
				}
			}
			characters += enumChars
			if policy.Limits.LargeEnumThreshold > 0 && len(values) > policy.Limits.LargeEnumThreshold && policy.Limits.LargeEnumCharacters > 0 && enumChars > policy.Limits.LargeEnumCharacters {
				return fail("exceeds large enum character limit")
			}
		}
		if value, present := node["const"]; present {
			if policy.ConstValues == "primitive" && !structuredOutputPrimitiveEnum([]any{value}) {
				return fail("contains a complex constant")
			}
			if str, ok := value.(string); ok {
				characters += utf8.RuneCountInString(str)
			}
		}
		if policy.ArrayItems == "required" && structuredOutputTypeIncludes(node["type"], "array") {
			if _, ok := node["items"].(map[string]any); !ok {
				return fail("arrays must define an item schema")
			}
		}
		for _, child := range structuredOutputLocatedChildren(node, "#") {
			if err := count(child.schema); err != nil {
				return err
			}
		}
		return nil
	}
	if err := count(document); err != nil {
		return err
	}
	for _, bound := range []struct {
		value, limit int
		label        string
	}{{properties, policy.Limits.ObjectProperties, "object property"}, {enums, policy.Limits.EnumValues, "enum value"}, {characters, policy.Limits.CountedSchemaCharacters, "counted schema character"}} {
		if bound.limit > 0 && bound.value > bound.limit {
			return fail("exceeds " + bound.label + " limit")
		}
	}
	if policy.References != "local_nonrecursive" && policy.Limits.NestingDepth == 0 {
		return nil
	}
	depth, err := structuredOutputGraphDepth(document)
	if err != nil {
		return fail(err.Error())
	}
	if policy.Limits.NestingDepth > 0 && depth > policy.Limits.NestingDepth {
		return fail("exceeds nesting depth limit")
	}
	return nil
}

// structuredOutputGraphDepth validates each schema node once. Cached depths are
// relative to the node, so reuse from deeper parents still enforces the limit.
func structuredOutputGraphDepth(document map[string]any) (int, error) {
	completed := map[string]int{}
	active := map[string]bool{}
	var visit func(string, map[string]any) (int, error)
	visit = func(location string, node map[string]any) (int, error) {
		if active[location] {
			return 0, errors.New("contains a recursive reference")
		}
		if depth, found := completed[location]; found {
			return depth, nil
		}
		active[location] = true
		depth := 1
		if reference, present := node["$ref"].(string); present {
			target, err := structuredOutputReference(document, reference)
			if err != nil {
				return 0, errors.New("contains an unsupported reference")
			}
			referenceDepth, err := visit(reference, target)
			if err != nil {
				return 0, err
			}
			depth = max(depth, referenceDepth)
		}
		for _, child := range structuredOutputLocatedChildren(node, location) {
			childDepth, err := visit(child.location, child.schema)
			if err != nil {
				return 0, err
			}
			depth = max(depth, childDepth+1)
		}
		delete(active, location)
		completed[location] = depth
		return depth, nil
	}
	return visit("#", document)
}

type structuredOutputLocatedSchema struct {
	location string
	schema   map[string]any
}

func structuredOutputLocatedChildren(document map[string]any, location string) []structuredOutputLocatedSchema {
	children := []structuredOutputLocatedSchema{}
	for _, keyword := range []string{"properties", "$defs", "definitions"} {
		if schemas, present := document[keyword].(map[string]any); present {
			for name, raw := range schemas {
				escaped := strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
				children = append(children, structuredOutputLocatedSchema{location: location + "/" + keyword + "/" + escaped, schema: raw.(map[string]any)})
			}
		}
	}
	for _, keyword := range []string{"items", "additionalProperties"} {
		if schema, present := document[keyword].(map[string]any); present {
			children = append(children, structuredOutputLocatedSchema{location: location + "/" + keyword, schema: schema})
		}
	}
	for _, keyword := range []string{"anyOf", "allOf", "prefixItems"} {
		if schemas, present := document[keyword].([]any); present {
			for index, raw := range schemas {
				children = append(children, structuredOutputLocatedSchema{location: location + "/" + keyword + "/" + strconv.Itoa(index), schema: raw.(map[string]any)})
			}
		}
	}
	return children
}

func structuredOutputReference(root map[string]any, reference string) (map[string]any, error) {
	if reference == "#" {
		return root, nil
	}
	if !strings.HasPrefix(reference, "#/") {
		return nil, errStructuredOutputUnsupported
	}
	var value any = root
	for _, part := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, errStructuredOutputUnsupported
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		value = object[part]
	}
	target, ok := value.(map[string]any)
	if !ok {
		return nil, errStructuredOutputUnsupported
	}
	return target, nil
}
