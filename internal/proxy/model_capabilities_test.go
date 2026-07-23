package proxy_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

const (
	marshalPayloadErrorFormat        = "Failed to marshal payload: %v"
	temperatureFieldPresenceMismatch = "Mismatch in 'temperature' field presence. Got: %s, Want presence: %v"
	toolsFieldPresenceMismatch       = "Mismatch in 'tools' field presence. Got: %s, Want presence: %v"
	toolChoiceFieldPresenceMismatch  = "Mismatch in 'tool_choice' field presence. Got: %s, Want presence: %v"
	reasoningFieldPresenceMismatch   = "Mismatch in 'reasoning' field presence. Got: %s, Want presence: %v"
	reasoningFieldJSONFragment       = `"reasoning"`
	maxOutputTokensFieldJSONFragment = `"max_output_tokens"`
	modelFieldsMismatchFormat        = "model %s fields=%v want=%v"
	promptValue                      = "hello"
)

// TestResolveModelPayloadSchema verifies that payload schemas are returned for every request profile.
func TestResolveModelPayloadSchema(testFramework *testing.T) {
	testCases := []struct {
		requestProfile string
		expectFields   []string
	}{
		{"openai_responses_temperature", []string{"model", "input", "max_output_tokens", "background", "store", "temperature"}},
		{"openai_responses_temperature_tools", []string{"model", "input", "max_output_tokens", "background", "store", "temperature", "tools", "tool_choice"}},
		{"openai_responses_reasoning_tools", []string{"model", "input", "max_output_tokens", "background", "store", "tools", "tool_choice", "reasoning"}},
	}
	for _, testCase := range testCases {
		payloadSchema := proxy.ResolveModelPayloadSchema(testCase.requestProfile)
		if !equalSlices(payloadSchema.AllowedRequestFields, testCase.expectFields) {
			testFramework.Fatalf(modelFieldsMismatchFormat, testCase.requestProfile, payloadSchema.AllowedRequestFields, testCase.expectFields)
		}
	}
}

// TestBuildRequestPayload verifies the correct payload structure is built for each request profile.
func TestBuildRequestPayload(testFramework *testing.T) {
	testCases := []struct {
		name              string
		modelIdentifier   string
		requestProfile    string
		webSearchEnabled  bool
		reasoningEffort   string
		expectTemperature bool
		expectTools       bool
		expectReasoning   bool
	}{
		{
			name:              "GPT-5 with web search",
			modelIdentifier:   proxy.ModelNameGPT5,
			requestProfile:    "openai_responses_reasoning_tools",
			webSearchEnabled:  true,
			reasoningEffort:   "high",
			expectTemperature: false,
			expectTools:       true,
			expectReasoning:   true,
		},
		{
			name:              "GPT-5 retains saved reasoning effort without web search",
			modelIdentifier:   proxy.ModelNameGPT5,
			requestProfile:    "openai_responses_reasoning_tools",
			webSearchEnabled:  false,
			reasoningEffort:   "high",
			expectTemperature: false,
			expectTools:       false,
			expectReasoning:   true,
		},
		{
			name:              "GPT-5.5 with web search",
			modelIdentifier:   proxy.ModelNameGPT55,
			requestProfile:    "openai_responses_reasoning_tools",
			webSearchEnabled:  true,
			reasoningEffort:   "high",
			expectTemperature: false,
			expectTools:       true,
			expectReasoning:   true,
		},
		{
			name:              "GPT-5.5 pro with web search",
			modelIdentifier:   proxy.ModelNameGPT55Pro,
			requestProfile:    "openai_responses_reasoning_tools",
			webSearchEnabled:  true,
			reasoningEffort:   "high",
			expectTemperature: false,
			expectTools:       true,
			expectReasoning:   true,
		},
		{
			name:              "GPT-4o with web search",
			modelIdentifier:   proxy.ModelNameGPT4o,
			requestProfile:    "openai_responses_temperature_tools",
			webSearchEnabled:  true,
			expectTemperature: true,
			expectTools:       true,
			expectReasoning:   false,
		},
		{
			name:              "GPT-4o-mini (no tools)",
			modelIdentifier:   proxy.ModelNameGPT4oMini,
			requestProfile:    "openai_responses_temperature",
			webSearchEnabled:  true,
			expectTemperature: true,
			expectTools:       false,
			expectReasoning:   false,
		},
		{
			name:              "GPT-5-mini with saved reasoning effort",
			modelIdentifier:   proxy.ModelNameGPT5Mini,
			requestProfile:    "openai_responses_reasoning_tools",
			webSearchEnabled:  false,
			reasoningEffort:   "high",
			expectTemperature: false,
			expectTools:       false,
			expectReasoning:   true,
		},
		{
			name:              "unknown profile without web search",
			modelIdentifier:   "future-model",
			requestProfile:    "future_profile",
			webSearchEnabled:  false,
			expectTemperature: false,
			expectTools:       false,
			expectReasoning:   false,
		},
		{
			name:              "unknown profile with web search",
			modelIdentifier:   "future-model",
			requestProfile:    "future_profile",
			webSearchEnabled:  true,
			expectTemperature: false,
			expectTools:       false,
			expectReasoning:   false,
		},
	}

	for _, testCase := range testCases {
		testFramework.Run(testCase.name, func(subTestFramework *testing.T) {
			payload := proxy.BuildRequestPayload(testCase.modelIdentifier, testCase.requestProfile, promptValue, testCase.webSearchEnabled, nil, testCase.reasoningEffort)
			payloadBytes, marshalError := json.Marshal(payload)
			if marshalError != nil {
				subTestFramework.Fatalf(marshalPayloadErrorFormat, marshalError)
			}
			payloadJSON := string(payloadBytes)

			if testCase.expectTemperature != strings.Contains(payloadJSON, `"temperature"`) {
				subTestFramework.Errorf(temperatureFieldPresenceMismatch, payloadJSON, testCase.expectTemperature)
			}
			if testCase.expectTools != strings.Contains(payloadJSON, `"tools"`) {
				subTestFramework.Errorf(toolsFieldPresenceMismatch, payloadJSON, testCase.expectTools)
			}
			if testCase.expectTools != strings.Contains(payloadJSON, `"tool_choice"`) {
				subTestFramework.Errorf(toolChoiceFieldPresenceMismatch, payloadJSON, testCase.expectTools)
			}
			reasoningFieldPresent := strings.Contains(payloadJSON, reasoningFieldJSONFragment)
			if reasoningFieldPresent != testCase.expectReasoning {
				subTestFramework.Errorf(reasoningFieldPresenceMismatch, payloadJSON, testCase.expectReasoning)
			}
			if testCase.expectReasoning && !strings.Contains(payloadJSON, `"effort":"`+testCase.reasoningEffort+`"`) {
				subTestFramework.Errorf("reasoning effort=%q missing from payload: %s", testCase.reasoningEffort, payloadJSON)
			}
			if strings.Contains(payloadJSON, maxOutputTokensFieldJSONFragment) {
				subTestFramework.Errorf("max_output_tokens must be omitted without request max_tokens: %s", payloadJSON)
			}
			if !strings.Contains(payloadJSON, `"background":true`) {
				subTestFramework.Errorf("background must be enabled for OpenAI Responses polling: %s", payloadJSON)
			}
			if !strings.Contains(payloadJSON, `"store":true`) {
				subTestFramework.Errorf("store must be enabled for OpenAI Responses polling: %s", payloadJSON)
			}

			maxTokens := 555
			cappedPayload := proxy.BuildRequestPayload(testCase.modelIdentifier, testCase.requestProfile, promptValue, testCase.webSearchEnabled, &maxTokens, testCase.reasoningEffort)
			cappedPayloadBytes, cappedMarshalError := json.Marshal(cappedPayload)
			if cappedMarshalError != nil {
				subTestFramework.Fatalf(marshalPayloadErrorFormat, cappedMarshalError)
			}
			if !strings.Contains(string(cappedPayloadBytes), `"max_output_tokens":555`) {
				subTestFramework.Errorf("max_output_tokens missing with request max_tokens: %s", string(cappedPayloadBytes))
			}
		})
	}
}

func TestResolveModelPayloadSchemaReturnsEmptyForUnknownModel(testFramework *testing.T) {
	payloadSchema := proxy.ResolveModelPayloadSchema(" future-model ")
	if len(payloadSchema.AllowedRequestFields) != 0 {
		testFramework.Fatalf("fields=%v want empty", payloadSchema.AllowedRequestFields)
	}
}

// equalSlices reports whether both string slices contain the same elements in
// the same order.
func equalSlices(first []string, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}
