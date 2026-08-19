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

	openAIPayload, _ := json.Marshal(buildRequestPayload("gpt-5.5", string(requestProfileOpenAIResponsesReasoningTools), []string{"review"}, false, nil, "high", true, true, schema))
	if !jsonContainsPath(openAIPayload, "text", "format", "schema") {
		testingInstance.Fatalf("OpenAI payload lacks text.format.schema: %s", openAIPayload)
	}
	geminiPayload, geminiError := newGeminiInteractionRequest(
		textModelDefinition{providerIdentifier: newModelID("gemini-3-pro-preview")},
		chatMessages{{role: chatRoleUser, content: "review"}}, nil, nil, false, schema,
	)
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
	for _, wireContract := range []textWireContract{textWireContractOpenAIResponses, textWireContractGeminiInteractions, textWireContractAnthropicMessages} {
		if routeError := validateStructuredOutputRoute(textModelDefinition{wireContract: wireContract}, &structuredOutputSchema{}); routeError != nil {
			testingInstance.Fatalf("wire=%s error=%v", wireContract, routeError)
		}
	}
	if routeError := validateStructuredOutputRoute(textModelDefinition{wireContract: textWireContractOpenAIChatCompletions}, &structuredOutputSchema{}); !errors.Is(routeError, errStructuredOutputUnsupported) {
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
