package llmproxyclient_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestClientPostAnalyzerSendsHashBoundContentAndReturnsRequestIdentity(t *testing.T) {
	contract := loadCanonicalOpenAPIContract(t)
	imageBytes := []byte("exact-image-bytes")
	audioBytes := []byte("exact-audio-bytes")
	imagePart, imageError := llmproxyclient.NewImageContent(llmproxyclient.ImageContentInput{
		MIMEType: "image/png",
		Data:     imageBytes,
		Detail:   llmproxyclient.ImageDetailHigh,
	})
	if imageError != nil {
		t.Fatalf("image content: %v", imageError)
	}
	audioPart, audioError := llmproxyclient.NewAudioContent(llmproxyclient.AudioContentInput{
		MIMEType: "audio/mp4",
		Data:     audioBytes,
	})
	if audioError != nil {
		t.Fatalf("audio content: %v", audioError)
	}
	systemPart, systemError := llmproxyclient.NewTextContent("Return only the required report.")
	if systemError != nil {
		t.Fatalf("system content: %v", systemError)
	}
	userPart, userError := llmproxyclient.NewTextContent("Inspect the supplied evidence.")
	if userError != nil {
		t.Fatalf("user content: %v", userError)
	}
	outputSchema, schemaError := llmproxyclient.NewStrictOutputSchema(llmproxyclient.StrictOutputSchemaInput{
		Name:        "semantic_qa_report",
		Description: "One bounded semantic QA decision.",
		Schema: json.RawMessage(`{
			"type":"object",
			"additionalProperties":false,
			"required":["outcome"],
			"properties":{"outcome":{"type":"string","enum":["pass","return"]}}
		}`),
	})
	if schemaError != nil {
		t.Fatalf("output schema: %v", schemaError)
	}
	reasoningEffort := "high"
	maxTokens := 1200
	requestTimeoutSeconds := 900
	request, requestError := llmproxyclient.NewAnalyzerRequest(llmproxyclient.AnalyzerRequestInput{
		Messages: []llmproxyclient.AnalyzerMessageInput{
			{Role: "system", Content: []llmproxyclient.ContentPart{systemPart}, Order: messageOrder(0)},
			{Role: "user", Content: []llmproxyclient.ContentPart{userPart, imagePart, audioPart}, Order: messageOrder(1)},
		},
		OutputSchema:          outputSchema,
		Model:                 "gpt-5.6",
		MaxTokens:             &maxTokens,
		ReasoningEffort:       &reasoningEffort,
		RequestTimeoutSeconds: &requestTimeoutSeconds,
	})
	if requestError != nil {
		t.Fatalf("analyzer request: %v", requestError)
	}

	var capturedBody map[string]any
	var capturedTimeout string
	var capturedAccept string
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		if httpRequest.URL.Path != "/v2/analyze" {
			t.Fatalf("path=%q", httpRequest.URL.Path)
		}
		capturedTimeout = httpRequest.Header.Get(llmproxycontract.HeaderRequestTimeoutSeconds)
		capturedAccept = httpRequest.Header.Get("Accept")
		if httpRequest.URL.Query().Has("format") {
			t.Fatalf("analyzer URL must not select text response format: %s", httpRequest.URL.RawQuery)
		}
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		if decodeError := json.Unmarshal(bodyBytes, &capturedBody); decodeError != nil {
			t.Fatalf("decode body: %v", decodeError)
		}
		if validationError := contract.ValidateRequest("/v2/analyze", httpRequest.Method, httpRequest, bodyBytes); validationError != nil {
			t.Fatalf("Go analyzer request violates OpenAPI: %v", validationError)
		}
		responseBody := []byte(`{"outcome":"pass"}`)
		responseHeader := http.Header{}
		responseHeader.Set("Content-Type", "application/json")
		responseHeader.Set(llmproxycontract.HeaderRequestID, "proxy-request-123")
		responseHeader.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "900")
		if validationError := contract.ValidateResponse("/v2/analyze", http.MethodPost, http.StatusOK, responseHeader, responseBody); validationError != nil {
			t.Fatalf("Go analyzer success fixture violates OpenAPI: %v", validationError)
		}
		for headerName, headerValues := range responseHeader {
			responseWriter.Header()[headerName] = headerValues
		}
		_, _ = responseWriter.Write(responseBody)
	}))
	t.Cleanup(server.Close)
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:  server.URL,
		Secret:   "sekret",
		Provider: "openai",
	})
	if configError != nil {
		t.Fatalf("config: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		t.Fatalf("client: %v", clientError)
	}

	response, postError := client.PostAnalyzer(context.Background(), request)
	if postError != nil {
		t.Fatalf("post analyzer: %v", postError)
	}
	if response.Text() != `{"outcome":"pass"}` || response.RequestID() != "proxy-request-123" {
		t.Fatalf("response text=%q request_id=%q", response.Text(), response.RequestID())
	}
	if capturedTimeout != "900" {
		t.Fatalf("timeout=%q", capturedTimeout)
	}
	if capturedAccept != "application/json" {
		t.Fatalf("accept=%q", capturedAccept)
	}
	if capturedBody["model"] != "gpt-5.6" ||
		capturedBody["max_tokens"] != float64(1200) ||
		capturedBody["reasoning_effort"] != "high" {
		t.Fatalf("request fields=%v", capturedBody)
	}
	messages, messagesOK := capturedBody["messages"].([]any)
	if !messagesOK || len(messages) != 2 {
		t.Fatalf("messages=%v", capturedBody["messages"])
	}
	userMessage := messages[1].(map[string]any)
	content := userMessage["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("content=%v", content)
	}
	assertBinaryContentPart(t, content[1], "image", "image/png", imageBytes)
	assertBinaryContentPart(t, content[2], "audio", "audio/mp4", audioBytes)
	imagePayload := content[1].(map[string]any)
	if imagePayload["detail"] != string(llmproxyclient.ImageDetailHigh) {
		t.Fatalf("image detail=%v", imagePayload["detail"])
	}
	outputSchemaPayload, outputSchemaOK := capturedBody["output_schema"].(map[string]any)
	if !outputSchemaOK ||
		outputSchemaPayload["name"] != "semantic_qa_report" ||
		outputSchemaPayload["description"] != "One bounded semantic QA decision." {
		t.Fatalf("output_schema=%v", capturedBody["output_schema"])
	}
}

func TestAnalyzerConstructorsRejectInvalidInputsBeforeHTTP(t *testing.T) {
	if _, textError := llmproxyclient.NewTextContent(" \n\t "); !errors.Is(textError, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatalf("blank text error=%v", textError)
	}
	invalidImageCases := []llmproxyclient.ImageContentInput{
		{MIMEType: "image/png"},
		{MIMEType: "text/plain", Data: []byte("not an image")},
		{MIMEType: "image/png", Data: []byte("image"), Detail: "ultra"},
	}
	for _, input := range invalidImageCases {
		if _, imageError := llmproxyclient.NewImageContent(input); !errors.Is(imageError, llmproxyclient.ErrInvalidClientRequest) {
			t.Fatalf("image input=%+v error=%v", input, imageError)
		}
	}
	invalidAudioCases := []llmproxyclient.AudioContentInput{
		{MIMEType: "audio/mpeg"},
		{MIMEType: "audio/ogg", Data: []byte("audio")},
	}
	for _, input := range invalidAudioCases {
		if _, audioError := llmproxyclient.NewAudioContent(input); !errors.Is(audioError, llmproxyclient.ErrInvalidClientRequest) {
			t.Fatalf("audio input=%+v error=%v", input, audioError)
		}
	}
	invalidSchemaCases := []llmproxyclient.StrictOutputSchemaInput{
		{Name: "bad schema name", Schema: json.RawMessage(`{"type":"object"}`)},
		{Name: "report", Schema: json.RawMessage(`{`)},
		{Name: "report", Schema: json.RawMessage(`[]`)},
		{Name: "report"},
	}
	for _, input := range invalidSchemaCases {
		if _, schemaError := llmproxyclient.NewStrictOutputSchema(input); !errors.Is(schemaError, llmproxyclient.ErrInvalidClientRequest) {
			t.Fatalf("schema input=%+v error=%v", input, schemaError)
		}
	}

	validText, textError := llmproxyclient.NewTextContent("inspect")
	if textError != nil {
		t.Fatalf("valid text: %v", textError)
	}
	validImage, imageError := llmproxyclient.NewImageContent(llmproxyclient.ImageContentInput{
		MIMEType: "image/webp",
		Data:     []byte("image"),
		Detail:   llmproxyclient.ImageDetailAuto,
	})
	if imageError != nil {
		t.Fatalf("valid image: %v", imageError)
	}
	validSchema, schemaError := llmproxyclient.NewStrictOutputSchema(llmproxyclient.StrictOutputSchemaInput{
		Name:   "report",
		Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	})
	if schemaError != nil {
		t.Fatalf("valid schema: %v", schemaError)
	}
	blankReasoning := " \t "
	zero := 0
	negative := -1
	invalidRequestCases := []struct {
		name  string
		input llmproxyclient.AnalyzerRequestInput
	}{
		{name: "missing messages", input: llmproxyclient.AnalyzerRequestInput{OutputSchema: validSchema, Model: "gpt-5.6"}},
		{name: "missing schema", input: llmproxyclient.AnalyzerRequestInput{
			Messages: []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{validText}}},
			Model:    "gpt-5.6",
		}},
		{name: "missing model", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{validText}}},
			OutputSchema: validSchema,
		}},
		{name: "invalid max tokens", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{validText}}},
			OutputSchema: validSchema,
			Model:        "gpt-5.6",
			MaxTokens:    &zero,
		}},
		{name: "blank reasoning effort", input: llmproxyclient.AnalyzerRequestInput{
			Messages:        []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{validText}}},
			OutputSchema:    validSchema,
			Model:           "gpt-5.6",
			ReasoningEffort: &blankReasoning,
		}},
		{name: "invalid request timeout", input: llmproxyclient.AnalyzerRequestInput{
			Messages:              []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{validText}}},
			OutputSchema:          validSchema,
			Model:                 "gpt-5.6",
			RequestTimeoutSeconds: &zero,
		}},
		{name: "empty content", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "user"}},
			OutputSchema: validSchema,
			Model:        "gpt-5.6",
		}},
		{name: "nil content", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{nil}}},
			OutputSchema: validSchema,
			Model:        "gpt-5.6",
		}},
		{name: "unsupported role", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "assistant", Content: []llmproxyclient.ContentPart{validText}}},
			OutputSchema: validSchema,
			Model:        "gpt-5.6",
		}},
		{name: "system binary", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "system", Content: []llmproxyclient.ContentPart{validImage}}, {Role: "user", Content: []llmproxyclient.ContentPart{validText}}},
			OutputSchema: validSchema,
			Model:        "gpt-5.6",
		}},
		{name: "missing user", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "system", Content: []llmproxyclient.ContentPart{validText}}},
			OutputSchema: validSchema,
			Model:        "gpt-5.6",
		}},
		{name: "mixed order", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{validText}, Order: messageOrder(1)}, {Role: "user", Content: []llmproxyclient.ContentPart{validText}}},
			OutputSchema: validSchema,
			Model:        "gpt-5.6",
		}},
		{name: "negative order", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{validText}, Order: &negative}},
			OutputSchema: validSchema,
			Model:        "gpt-5.6",
		}},
		{name: "duplicate order", input: llmproxyclient.AnalyzerRequestInput{
			Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{validText}, Order: messageOrder(1)}, {Role: "user", Content: []llmproxyclient.ContentPart{validText}, Order: messageOrder(1)}},
			OutputSchema: validSchema,
			Model:        "gpt-5.6",
		}},
	}
	for _, testCase := range invalidRequestCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			if _, requestError := llmproxyclient.NewAnalyzerRequest(testCase.input); !errors.Is(requestError, llmproxyclient.ErrInvalidClientRequest) {
				subTest.Fatalf("request input=%+v error=%v", testCase.input, requestError)
			}
		})
	}
}

func TestClientPostAnalyzerRejectsHTTPFailureAndModelProfile(t *testing.T) {
	request := validAnalyzerClientRequest(t)
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		calls++
		if httpRequest.URL.Path != "/v2/analyze" {
			t.Fatalf("path=%q", httpRequest.URL.Path)
		}
		responseWriter.WriteHeader(http.StatusBadGateway)
		_, _ = responseWriter.Write([]byte(`{"error":"upstream"}`))
	}))
	t.Cleanup(server.Close)

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "sekret"})
	if configError != nil {
		t.Fatalf("config: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		t.Fatalf("client: %v", clientError)
	}
	if _, postError := client.PostAnalyzer(context.Background(), request); !errors.Is(postError, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatalf("HTTP failure error=%v", postError)
	}

	profileConfig, profileConfigError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:          server.URL,
		Secret:           "sekret",
		ModelProfilePath: "/profiles/current.json",
		ModelProfileReader: func(string) ([]byte, error) {
			return []byte(`{"provider":"openai","model":"gpt-5.6"}`), nil
		},
	})
	if profileConfigError != nil {
		t.Fatalf("profile config: %v", profileConfigError)
	}
	profileClient, profileClientError := llmproxyclient.NewClient(profileConfig, server.Client())
	if profileClientError != nil {
		t.Fatalf("profile client: %v", profileClientError)
	}
	if _, postError := profileClient.PostAnalyzer(context.Background(), request); !errors.Is(postError, llmproxyclient.ErrInvalidModelProfile) {
		t.Fatalf("model profile error=%v", postError)
	}
	if calls != 1 {
		t.Fatalf("HTTP calls=%d want=1", calls)
	}
}

func TestClientPostAnalyzerKeepsExistingAnalyzerEndpoint(t *testing.T) {
	request := validAnalyzerClientRequest(t)
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		if httpRequest.URL.Path != "/v2/analyze" {
			t.Fatalf("path=%q", httpRequest.URL.Path)
		}
		responseWriter.Header().Set(llmproxycontract.HeaderRequestID, "existing-endpoint")
		_, _ = responseWriter.Write([]byte(`{"outcome":"pass"}`))
	}))
	t.Cleanup(server.Close)
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: server.URL + "/v2/analyze/",
		Secret:  "sekret",
	})
	if configError != nil {
		t.Fatalf("config: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		t.Fatalf("client: %v", clientError)
	}
	if _, postError := client.PostAnalyzer(context.Background(), request); postError != nil {
		t.Fatalf("post analyzer: %v", postError)
	}
}

func TestClientPostAnalyzerRejectsSuccessfulResponseWithoutRequestIdentity(t *testing.T) {
	textPart, textError := llmproxyclient.NewTextContent("inspect")
	if textError != nil {
		t.Fatalf("text: %v", textError)
	}
	outputSchema, schemaError := llmproxyclient.NewStrictOutputSchema(llmproxyclient.StrictOutputSchemaInput{
		Name:   "report",
		Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	})
	if schemaError != nil {
		t.Fatalf("schema: %v", schemaError)
	}
	request, requestError := llmproxyclient.NewAnalyzerRequest(llmproxyclient.AnalyzerRequestInput{
		Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{textPart}}},
		OutputSchema: outputSchema,
		Model:        "gpt-5.6",
	})
	if requestError != nil {
		t.Fatalf("request: %v", requestError)
	}
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		_, _ = responseWriter.Write([]byte(`{"outcome":"pass"}`))
	}))
	t.Cleanup(server.Close)
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "sekret"})
	if configError != nil {
		t.Fatalf("config: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		t.Fatalf("client: %v", clientError)
	}

	if _, postError := client.PostAnalyzer(context.Background(), request); !errors.Is(postError, llmproxyclient.ErrClientHTTPFailure) ||
		!strings.Contains(postError.Error(), "missing request id") {
		t.Fatalf("post error=%v", postError)
	}
}

func validAnalyzerClientRequest(t *testing.T) llmproxyclient.AnalyzerRequest {
	t.Helper()
	textPart, textError := llmproxyclient.NewTextContent("inspect")
	if textError != nil {
		t.Fatalf("text: %v", textError)
	}
	outputSchema, schemaError := llmproxyclient.NewStrictOutputSchema(llmproxyclient.StrictOutputSchemaInput{
		Name:   "report",
		Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	})
	if schemaError != nil {
		t.Fatalf("schema: %v", schemaError)
	}
	request, requestError := llmproxyclient.NewAnalyzerRequest(llmproxyclient.AnalyzerRequestInput{
		Messages:     []llmproxyclient.AnalyzerMessageInput{{Role: "user", Content: []llmproxyclient.ContentPart{textPart}}},
		OutputSchema: outputSchema,
		Model:        "gpt-5.6",
	})
	if requestError != nil {
		t.Fatalf("request: %v", requestError)
	}
	return request
}

func assertBinaryContentPart(t *testing.T, rawPart any, partType string, mimeType string, expectedBytes []byte) {
	t.Helper()
	part, partOK := rawPart.(map[string]any)
	if !partOK {
		t.Fatalf("part=%v", rawPart)
	}
	expectedDigest := sha256.Sum256(expectedBytes)
	if part["type"] != partType ||
		part["mime_type"] != mimeType ||
		part["data"] != encodeBase64(expectedBytes) ||
		part["sha256"] != hex.EncodeToString(expectedDigest[:]) {
		t.Fatalf("part=%v", part)
	}
}

func encodeBase64(value []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	if len(value) == 0 {
		return ""
	}
	var builder strings.Builder
	for byteIndex := 0; byteIndex < len(value); byteIndex += 3 {
		remaining := len(value) - byteIndex
		first := value[byteIndex]
		var second byte
		var third byte
		if remaining > 1 {
			second = value[byteIndex+1]
		}
		if remaining > 2 {
			third = value[byteIndex+2]
		}
		builder.WriteByte(alphabet[first>>2])
		builder.WriteByte(alphabet[((first&0x03)<<4)|(second>>4)])
		if remaining > 1 {
			builder.WriteByte(alphabet[((second&0x0f)<<2)|(third>>6)])
		} else {
			builder.WriteByte('=')
		}
		if remaining > 2 {
			builder.WriteByte(alphabet[third&0x3f])
		} else {
			builder.WriteByte('=')
		}
	}
	return builder.String()
}
