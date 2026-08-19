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
	case textWireContractOpenAIResponses, textWireContractGeminiInteractions, textWireContractAnthropicMessages:
		return nil
	default:
		return errStructuredOutputUnsupported
	}
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
