package proxy

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestStructuredOutputSchemaAndProviderMappings(testingInstance *testing.T) {
	schema, schemaError := newStructuredOutputSchema(json.RawMessage(`{"schema":{"type":"object","additionalProperties":false,"required":["decision"],"properties":{"decision":{"type":"string","enum":["pass","return"]}}}}`))
	if schemaError != nil {
		testingInstance.Fatalf("schema error: %v", schemaError)
	}
	if string(schema.canonical) != `{"additionalProperties":false,"properties":{"decision":{"enum":["pass","return"],"type":"string"}},"required":["decision"],"type":"object"}` {
		testingInstance.Fatalf("canonical schema=%s", schema.canonical)
	}
	if validationError := schema.validateResponse(`{"decision":"pass"}`); validationError != nil {
		testingInstance.Fatalf("valid response: %v", validationError)
	}
	for _, output := range []string{`not-json`, `{"decision":"pass"} {}`, `{"decision":"unknown"}`} {
		if validationError := schema.validateResponse(output); !errors.Is(validationError, ErrProviderAPI) {
			testingInstance.Fatalf("output=%q error=%v", output, validationError)
		}
	}
	if validationError := (*structuredOutputSchema)(nil).validateResponse("anything"); validationError != nil {
		testingInstance.Fatalf("nil schema error=%v", validationError)
	}

	openAIBytes, _ := json.Marshal(openAIStructuredTextFor(schema))
	if string(openAIBytes) != `{"format":{"type":"json_schema","name":"llm_proxy_structured_output","strict":true,"schema":{"additionalProperties":false,"properties":{"decision":{"enum":["pass","return"],"type":"string"}},"required":["decision"],"type":"object"}}}` {
		testingInstance.Fatalf("OpenAI mapping=%s", openAIBytes)
	}
	geminiBytes, _ := json.Marshal(geminiStructuredResponseFormats(schema))
	if string(geminiBytes) != `[{"type":"text","mime_type":"application/json","schema":{"additionalProperties":false,"properties":{"decision":{"enum":["pass","return"],"type":"string"}},"required":["decision"],"type":"object"}}]` {
		testingInstance.Fatalf("Gemini mapping=%s", geminiBytes)
	}
	anthropicBytes, _ := json.Marshal(anthropicStructuredOutputFor(schema))
	if string(anthropicBytes) != `{"format":{"type":"json_schema","schema":{"additionalProperties":false,"properties":{"decision":{"enum":["pass","return"],"type":"string"}},"required":["decision"],"type":"object"}}}` {
		testingInstance.Fatalf("Anthropic mapping=%s", anthropicBytes)
	}
	if openAIStructuredTextFor(nil) != nil || geminiStructuredResponseFormats(nil) != nil || anthropicStructuredOutputFor(nil) != nil {
		testingInstance.Fatal("nil schema must omit provider fields")
	}

	openAIPayload, _ := json.Marshal(buildRequestPayload("gpt-5.5", string(requestProfileOpenAIResponsesReasoningTools), []string{"review"}, false, nil, "high", true, true, schema, nil))
	if !jsonContainsPath(openAIPayload, "text", "format", "schema") {
		testingInstance.Fatalf("OpenAI payload lacks text.format.schema: %s", openAIPayload)
	}
	geminiExecution, geminiError := newGeminiInteractionExecution(
		textModelDefinition{
			providerIdentifier: newModelID("gemini-3-pro-preview"),
			executionLifecycle: textExecutionLifecyclePollableResource,
		},
		chatMessages{{role: chatRoleUser, content: "review"}}, nil, nil, "", schema,
	)
	geminiPayload := geminiExecution.request
	if geminiError != nil || len(geminiPayload.ResponseFormat) != 1 || geminiPayload.ResponseFormat[0].MIMEType != "application/json" {
		testingInstance.Fatalf("Gemini payload=%+v error=%v", geminiPayload, geminiError)
	}
	anthropicPayload := anthropicMessagesRequest{OutputConfig: anthropicStructuredOutputFor(schema)}
	encodedAnthropic, _ := json.Marshal(anthropicPayload)
	if !jsonContainsPath(encodedAnthropic, "output_config", "format", "schema") {
		testingInstance.Fatalf("Anthropic payload lacks output_config.format.schema: %s", encodedAnthropic)
	}
}

func jsonContainsPath(data []byte, path ...string) bool {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	current := value
	for _, segment := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current, ok = object[segment]
		if !ok {
			return false
		}
	}
	return current != nil
}

func TestStructuredOutputValidationFailures(testingInstance *testing.T) {
	invalidInputs := []json.RawMessage{
		json.RawMessage(`null`),
		json.RawMessage(`{`),
		json.RawMessage(`{} {}`),
		json.RawMessage(`{}`),
		json.RawMessage(`{"schema":null}`),
		json.RawMessage(`{"schema":[]}`),
		json.RawMessage(`{"schema":{} {}}`),
		json.RawMessage(`{"schema":{"type":"missing"}}`),
		json.RawMessage(`{"schema":{},"extra":true}`),
	}
	for index, input := range invalidInputs {
		if _, schemaError := newStructuredOutputSchema(input); !errors.Is(schemaError, errStructuredOutputInvalid) {
			testingInstance.Fatalf("case=%d error=%v", index, schemaError)
		}
	}
	if schema, schemaError := newStructuredOutputSchema(nil); schemaError != nil || schema != nil {
		testingInstance.Fatalf("absent schema=%v error=%v", schema, schemaError)
	}
	validRouteSchema, validRouteSchemaError := newStructuredOutputSchema(json.RawMessage(`{"schema":{"type":"object","additionalProperties":false,"required":["value"],"properties":{"value":{"type":"string"}}}}`))
	if validRouteSchemaError != nil {
		testingInstance.Fatal(validRouteSchemaError)
	}
	for _, wireContract := range []textWireContract{textWireContractOpenAIResponses, textWireContractGeminiInteractions, textWireContractAnthropicMessages} {
		if routeError := validateStructuredOutputRoute(textModelDefinition{wireContract: wireContract}, validRouteSchema); routeError != nil {
			testingInstance.Fatalf("wire=%s error=%v", wireContract, routeError)
		}
	}
	if routeError := validateStructuredOutputRoute(textModelDefinition{wireContract: textWireContractOpenAIChatCompletions}, validRouteSchema); !errors.Is(routeError, errStructuredOutputUnsupported) {
		testingInstance.Fatalf("unsupported route error=%v", routeError)
	}
	if routeError := validateStructuredOutputRoute(textModelDefinition{}, nil); routeError != nil {
		testingInstance.Fatalf("absent route schema error=%v", routeError)
	}

	originalMarshal := marshalStructuredOutputSchema
	originalAdd := addStructuredOutputResource
	testingInstance.Cleanup(func() {
		marshalStructuredOutputSchema = originalMarshal
		addStructuredOutputResource = originalAdd
	})
	marshalStructuredOutputSchema = func(any) ([]byte, error) { return nil, errors.New("marshal failed") }
	if _, schemaError := newStructuredOutputSchema(json.RawMessage(`{"schema":{}}`)); !errors.Is(schemaError, errStructuredOutputInvalid) {
		testingInstance.Fatalf("marshal error=%v", schemaError)
	}
	marshalStructuredOutputSchema = originalMarshal
	addStructuredOutputResource = func(*jsonschema.Compiler, string, any) error { return errors.New("resource failed") }
	if _, schemaError := newStructuredOutputSchema(json.RawMessage(`{"schema":{}}`)); !errors.Is(schemaError, errStructuredOutputInvalid) {
		testingInstance.Fatalf("resource error=%v", schemaError)
	}
}

func TestStructuredOutputProviderSchemaAdmission(testingInstance *testing.T) {
	validCases := []struct {
		name         string
		wireContract textWireContract
		schema       string
	}{
		{
			name: "OpenAI nested subset", wireContract: textWireContractOpenAIResponses,
			schema: `{"type":"object","additionalProperties":false,"required":["choice","items"],"properties":{"choice":{"anyOf":[{"type":"string"},{"type":"null"}]},"items":{"type":"array","minItems":1,"maxItems":2,"items":{"type":"object","additionalProperties":false,"required":["score"],"properties":{"score":{"type":"number","minimum":0,"maximum":1}}}}},"$defs":{"label":{"type":"string","pattern":"^[a-z]+$"}}}`,
		},
		{
			name: "Gemini documented subset", wireContract: textWireContractGeminiInteractions,
			schema: `{"type":"object","properties":{"when":{"type":"string","format":"date-time"},"scores":{"type":"array","prefixItems":[{"type":"number","minimum":0}],"items":{"type":"number","maximum":1},"minItems":1,"maxItems":2}},"additionalProperties":{"type":"string"}}`,
		},
		{
			name: "Anthropic documented subset", wireContract: textWireContractAnthropicMessages,
			schema: `{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string","format":"uri","default":"https://example.com"},"values":{"type":"array","minItems":1,"items":{"allOf":[{"type":"string","pattern":"^[a-z]+$"}]}}},"required":["label"],"definitions":{"fallback":{"type":"string","enum":["a",1,true,null]}}}`,
		},
	}
	for _, testCase := range validCases {
		testingInstance.Run(testCase.name, func(subtest *testing.T) {
			schema := mustStructuredOutputSchema(subtest, testCase.schema)
			if routeError := validateStructuredOutputRoute(textModelDefinition{wireContract: testCase.wireContract}, schema); routeError != nil {
				subtest.Fatalf("route error=%v", routeError)
			}
		})
	}

	invalidCases := []struct {
		name         string
		wireContract textWireContract
		schema       string
	}{
		{name: "OpenAI root array", wireContract: textWireContractOpenAIResponses, schema: `{"type":"array","items":{"type":"string"}}`},
		{name: "OpenAI root anyOf", wireContract: textWireContractOpenAIResponses, schema: `{"type":"object","additionalProperties":false,"anyOf":[{"type":"object","additionalProperties":false}]}`},
		{name: "OpenAI open object", wireContract: textWireContractOpenAIResponses, schema: `{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`},
		{name: "OpenAI optional property", wireContract: textWireContractOpenAIResponses, schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string"}}}`},
		{name: "OpenAI wrong required property", wireContract: textWireContractOpenAIResponses, schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string"}},"required":["other"]}`},
		{name: "OpenAI unsupported composition", wireContract: textWireContractOpenAIResponses, schema: `{"type":"object","additionalProperties":false,"allOf":[{"type":"object","additionalProperties":false}]}`},
		{name: "OpenAI unsupported format", wireContract: textWireContractOpenAIResponses, schema: `{"type":"object","additionalProperties":false,"required":["value"],"properties":{"value":{"type":"string","format":"uri"}}}`},
		{name: "OpenAI boolean property schema", wireContract: textWireContractOpenAIResponses, schema: `{"type":"object","additionalProperties":false,"required":["value"],"properties":{"value":true}}`},
		{name: "Gemini unsupported keyword", wireContract: textWireContractGeminiInteractions, schema: `{"type":"string","minLength":1}`},
		{name: "Gemini unsupported format", wireContract: textWireContractGeminiInteractions, schema: `{"type":"string","format":"email"}`},
		{name: "Gemini boolean item schema", wireContract: textWireContractGeminiInteractions, schema: `{"type":"array","items":true}`},
		{name: "Anthropic open object", wireContract: textWireContractAnthropicMessages, schema: `{"type":"object","properties":{"value":{"type":"string"}}}`},
		{name: "Anthropic numeric constraint", wireContract: textWireContractAnthropicMessages, schema: `{"type":"number","minimum":1}`},
		{name: "Anthropic recursive root", wireContract: textWireContractAnthropicMessages, schema: `{"$ref":"#"}`},
		{name: "Anthropic array minimum", wireContract: textWireContractAnthropicMessages, schema: `{"type":"array","minItems":2,"items":{"type":"string"}}`},
		{name: "Anthropic complex enum", wireContract: textWireContractAnthropicMessages, schema: `{"enum":[{"value":1}]}`},
		{name: "Anthropic reference allOf", wireContract: textWireContractAnthropicMessages, schema: `{"allOf":[{"$ref":"#/$defs/value"}],"$defs":{"value":{"type":"string"}}}`},
		{name: "Anthropic unsupported format", wireContract: textWireContractAnthropicMessages, schema: `{"type":"string","format":"json-pointer"}`},
		{name: "Anthropic boolean definition", wireContract: textWireContractAnthropicMessages, schema: `{"$defs":{"value":false},"type":"string"}`},
	}
	for _, testCase := range invalidCases {
		testingInstance.Run(testCase.name, func(subtest *testing.T) {
			schema := mustStructuredOutputSchema(subtest, testCase.schema)
			if routeError := validateStructuredOutputRoute(textModelDefinition{wireContract: testCase.wireContract}, schema); !errors.Is(routeError, errStructuredOutputUnsupported) {
				subtest.Fatalf("route error=%v", routeError)
			}
		})
	}
	externalReferenceSchema := &structuredOutputSchema{document: map[string]any{"$ref": "https://example.com/schema"}}
	if routeError := validateStructuredOutputRoute(textModelDefinition{wireContract: textWireContractAnthropicMessages}, externalReferenceSchema); !errors.Is(routeError, errStructuredOutputUnsupported) {
		testingInstance.Fatalf("external reference route error=%v", routeError)
	}
	if !structuredOutputTypeIncludes("object", "object") ||
		!structuredOutputTypeIncludes([]any{"string", "object"}, "object") ||
		structuredOutputTypeIncludes([]any{"string"}, "object") {
		testingInstance.Fatal("schema type membership is invalid")
	}
	if !structuredOutputAllPropertiesRequired(nil, nil) {
		testingInstance.Fatal("an empty object has no missing properties")
	}
	if _, childError := structuredOutputChildSchemas(map[string]any{"additionalProperties": "invalid"}); !errors.Is(childError, errStructuredOutputUnsupported) {
		testingInstance.Fatalf("additional property schema error=%v", childError)
	}
	if _, childError := structuredOutputChildSchemas(map[string]any{"anyOf": []any{true}}); !errors.Is(childError, errStructuredOutputUnsupported) {
		testingInstance.Fatalf("composition child schema error=%v", childError)
	}
}

func mustStructuredOutputSchema(testingInstance *testing.T, rawSchema string) *structuredOutputSchema {
	testingInstance.Helper()
	schema, schemaError := newStructuredOutputSchema(json.RawMessage(`{"schema":` + rawSchema + `}`))
	if schemaError != nil {
		testingInstance.Fatalf("schema error=%v", schemaError)
	}
	return schema
}
