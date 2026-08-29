package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

const (
	testGeminiInteractionsPath     = "/interactions"
	testGeminiAPIRevisionHeader    = "Api-Revision"
	testGeminiAPIRevisionValue     = "2026-05-20"
	testGeminiInteractionUserStep  = "user_input"
	testGeminiInteractionModelStep = "model_output"
	testVisibilityRetryIntervalMS  = 1
)

type testGeminiInteractionUsage struct {
	Input  int
	Output int
	Total  int
}

type testGeminiInteractionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeGeminiInteractionRequest(testingInstance testing.TB, request *http.Request) map[string]any {
	testingInstance.Helper()
	bodyBytes, readError := io.ReadAll(request.Body)
	if readError != nil {
		testingInstance.Fatalf("read Gemini Interactions request: %v", readError)
	}
	var payload map[string]any
	if decodeError := json.Unmarshal(bodyBytes, &payload); decodeError != nil {
		testingInstance.Fatalf("decode Gemini Interactions request: %v", decodeError)
	}
	return payload
}

func assertGeminiInteractionHeaders(testingInstance testing.TB, request *http.Request, apiKey string) {
	testingInstance.Helper()
	if request.Header.Get("x-goog-api-key") != apiKey ||
		request.Header.Get(testGeminiAPIRevisionHeader) != testGeminiAPIRevisionValue {
		testingInstance.Fatalf("Gemini Interactions headers=%v", request.Header)
	}
}

func writeGeminiInteractionSnapshot(testingInstance testing.TB, responseWriter http.ResponseWriter, identifier string, status string, text string, usage *testGeminiInteractionUsage) {
	writeGeminiInteractionSnapshotWithErrors(testingInstance, responseWriter, identifier, status, text, usage, nil)
}

func writeGeminiInteractionSnapshotWithErrors(testingInstance testing.TB, responseWriter http.ResponseWriter, identifier string, status string, text string, usage *testGeminiInteractionUsage, interactionErrors []testGeminiInteractionError) {
	testingInstance.Helper()
	response := map[string]any{
		"id":     identifier,
		"status": status,
	}
	if len(interactionErrors) > 0 {
		response["errors"] = interactionErrors
	}
	if text != "" {
		response["steps"] = []map[string]any{{
			"type": testGeminiInteractionModelStep,
			"content": []map[string]any{{
				"type": "text",
				"text": text,
			}},
		}}
	}
	if usage != nil {
		response["usage"] = map[string]any{
			"total_input_tokens":  usage.Input,
			"total_output_tokens": usage.Output,
			"total_tokens":        usage.Total,
		}
	}
	responseWriter.Header().Set("Content-Type", "application/json")
	if encodeError := json.NewEncoder(responseWriter).Encode(response); encodeError != nil {
		testingInstance.Fatalf("encode Gemini Interactions response: %v", encodeError)
	}
}

func writeGeminiInteractionDeleted(testingInstance testing.TB, responseWriter http.ResponseWriter) {
	testingInstance.Helper()
	responseWriter.Header().Set("Content-Type", "application/json")
	if _, writeError := responseWriter.Write([]byte(`{}`)); writeError != nil {
		testingInstance.Fatalf("write Gemini Interactions deletion: %v", writeError)
	}
}

func geminiInteractionStepText(testingInstance testing.TB, rawStep any) string {
	testingInstance.Helper()
	step, stepOK := rawStep.(map[string]any)
	if !stepOK {
		testingInstance.Fatalf("Gemini Interactions step=%v", rawStep)
	}
	content, contentOK := step["content"].([]any)
	if !contentOK || len(content) == 0 {
		testingInstance.Fatalf("Gemini Interactions step content=%v", step["content"])
	}
	textContent, textOK := content[0].(map[string]any)
	if !textOK {
		testingInstance.Fatalf("Gemini Interactions text content=%v", content[0])
	}
	text, _ := textContent["text"].(string)
	return text
}
