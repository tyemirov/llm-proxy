package llmproxyclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestConfigPostURLShapesAuthenticatedJSONPostURL(testingInstance *testing.T) {
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:  "https://proxy.example/review?prompt=old&model=old&max_tokens=9&web_search=true&provider=gemini&keep=1",
		Secret:   "sekret",
		Provider: "deepseek",
		Timeout:  time.Second,
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}

	parsedURL, parseError := url.Parse(config.PostURL())
	if parseError != nil {
		testingInstance.Fatalf("parse post url: %v", parseError)
	}
	if parsedURL.Path != "/review" {
		testingInstance.Fatalf("path=%q", parsedURL.Path)
	}
	parsedMessagesURL, messagesParseError := url.Parse(config.MessagesPostURL())
	if messagesParseError != nil {
		testingInstance.Fatalf("parse messages post url: %v", messagesParseError)
	}
	if parsedMessagesURL.Path != "/review/v2" {
		testingInstance.Fatalf("messages path=%q", parsedMessagesURL.Path)
	}
	v2Config, v2ConfigError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: "https://proxy.example/v2?prompt=old",
		Secret:  "sekret",
		Timeout: time.Second,
	})
	if v2ConfigError != nil {
		testingInstance.Fatalf("v2 config error: %v", v2ConfigError)
	}
	parsedExistingMessagesURL, existingMessagesParseError := url.Parse(v2Config.MessagesPostURL())
	if existingMessagesParseError != nil {
		testingInstance.Fatalf("parse existing messages post url: %v", existingMessagesParseError)
	}
	if parsedExistingMessagesURL.Path != "/v2" {
		testingInstance.Fatalf("existing messages path=%q", parsedExistingMessagesURL.Path)
	}
	queryValues := parsedURL.Query()
	if queryValues.Get("key") != "sekret" {
		testingInstance.Fatalf("key=%q", queryValues.Get("key"))
	}
	if queryValues.Get("format") != "text/plain" {
		testingInstance.Fatalf("format=%q", queryValues.Get("format"))
	}
	if queryValues.Get("provider") != "deepseek" {
		testingInstance.Fatalf("provider=%q", queryValues.Get("provider"))
	}
	if queryValues.Get("keep") != "1" {
		testingInstance.Fatalf("keep=%q", queryValues.Get("keep"))
	}
	for _, removedQueryKey := range []string{"prompt", "model", "max_tokens", "web_search"} {
		if queryValues.Has(removedQueryKey) {
			testingInstance.Fatalf("query key %s should have been removed", removedQueryKey)
		}
	}
}

func TestClientPostMessagesSendsV2MessagesBody(testingInstance *testing.T) {
	firstOrder := messageOrder(1)
	secondOrder := messageOrder(2)
	var capturedPath string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		capturedPath = httpRequest.URL.Path
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatalf("read body: %v", readError)
		}
		if decodeError := json.Unmarshal(bodyBytes, &capturedBody); decodeError != nil {
			testingInstance.Fatalf("decode body: %v", decodeError)
		}
		_, _ = responseWriter.Write([]byte("ok"))
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: server.URL,
		Secret:  "sekret",
		Timeout: time.Second,
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client error: %v", clientError)
	}
	maxTokens := messageOrder(5)
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages: []llmproxyclient.MessageInput{
			{Role: "assistant", Content: "Hi", Order: secondOrder},
			{Role: "user", Content: "Hello", Order: firstOrder},
		},
		Model:     "deepseek-v4-flash",
		WebSearch: true,
		MaxTokens: maxTokens,
	})
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}

	responseText, postError := client.PostMessages(context.Background(), request)

	if postError != nil {
		testingInstance.Fatalf("post error: %v", postError)
	}
	if responseText != "ok" || capturedPath != "/v2" {
		testingInstance.Fatalf("response=%q path=%q", responseText, capturedPath)
	}
	if capturedBody["prompt"] != nil || capturedBody["system_prompt"] != nil {
		testingInstance.Fatalf("legacy fields must be omitted for v2 messages body: %v", capturedBody)
	}
	rawMessages, ok := capturedBody["messages"].([]any)
	if !ok || len(rawMessages) != 2 {
		testingInstance.Fatalf("messages=%v", capturedBody["messages"])
	}
	firstMessage, ok := rawMessages[0].(map[string]any)
	if !ok || firstMessage["role"] != "user" || firstMessage["content"] != "Hello" || firstMessage["order"] != float64(1) {
		testingInstance.Fatalf("firstMessage=%v", rawMessages[0])
	}
	if capturedBody["model"] != "deepseek-v4-flash" || capturedBody["web_search"] != true || capturedBody["max_tokens"] != float64(5) {
		testingInstance.Fatalf("body=%v", capturedBody)
	}
}

func TestClientPostSendsMessagesBody(testingInstance *testing.T) {
	firstOrder := messageOrder(1)
	secondOrder := messageOrder(2)
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatalf("read body: %v", readError)
		}
		if decodeError := json.Unmarshal(bodyBytes, &capturedBody); decodeError != nil {
			testingInstance.Fatalf("decode body: %v", decodeError)
		}
		_, _ = responseWriter.Write([]byte("ok"))
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: server.URL,
		Secret:  "sekret",
		Timeout: time.Second,
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client error: %v", clientError)
	}
	request, requestError := llmproxyclient.NewRequest(llmproxyclient.RequestInput{
		Messages: []llmproxyclient.MessageInput{
			{Role: "assistant", Content: "Hi", Order: secondOrder},
			{Role: "user", Content: "Hello", Order: firstOrder},
		},
		Model:        "deepseek-v4-flash",
		SystemPrompt: "outer system",
		WebSearch:    true,
	})
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}

	responseText, postError := client.Post(context.Background(), request)

	if postError != nil {
		testingInstance.Fatalf("post error: %v", postError)
	}
	if responseText != "ok" {
		testingInstance.Fatalf("response=%q", responseText)
	}
	if capturedBody["prompt"] != nil {
		testingInstance.Fatalf("prompt must be omitted for messages body: %v", capturedBody)
	}
	if capturedBody["model"] != "deepseek-v4-flash" || capturedBody["system_prompt"] != "outer system" || capturedBody["web_search"] != true {
		testingInstance.Fatalf("body=%v", capturedBody)
	}
	rawMessages, ok := capturedBody["messages"].([]any)
	if !ok || len(rawMessages) != 2 {
		testingInstance.Fatalf("messages=%v", capturedBody["messages"])
	}
	firstMessage, ok := rawMessages[0].(map[string]any)
	if !ok || firstMessage["role"] != "user" || firstMessage["content"] != "Hello" || firstMessage["order"] != float64(1) {
		testingInstance.Fatalf("firstMessage=%v", rawMessages[0])
	}
}

func TestMessagesRequestRejectsInvalidInputs(testingInstance *testing.T) {
	testCases := []struct {
		name        string
		input       llmproxyclient.MessagesRequestInput
		errorString string
	}{
		{
			name:        "missing messages",
			input:       llmproxyclient.MessagesRequestInput{},
			errorString: "missing messages",
		},
		{
			name:        "invalid max tokens",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt"}}, MaxTokens: messageOrder(0)},
			errorString: "max_tokens must be positive",
		},
		{
			name:        "unsupported role",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "tool", Content: "tool result"}}},
			errorString: "role unsupported",
		},
	}

	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			_, requestError := llmproxyclient.NewMessagesRequest(testCase.input)
			if requestError == nil || !strings.Contains(requestError.Error(), testCase.errorString) {
				subTest.Fatalf("error=%v want contains %q", requestError, testCase.errorString)
			}
		})
	}
}

func TestRequestRejectsInvalidMessageInputs(testingInstance *testing.T) {
	testCases := []struct {
		name        string
		input       llmproxyclient.RequestInput
		errorString string
	}{
		{
			name: "conflicting prompt and messages",
			input: llmproxyclient.RequestInput{
				Prompt:   "prompt",
				Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "message"}},
			},
			errorString: "choose prompt or messages",
		},
		{
			name:        "unsupported role",
			input:       llmproxyclient.RequestInput{Messages: []llmproxyclient.MessageInput{{Role: "tool", Content: "tool result"}}},
			errorString: "role unsupported",
		},
		{
			name:        "empty content",
			input:       llmproxyclient.RequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user"}}},
			errorString: "content is empty",
		},
		{
			name:        "missing user message",
			input:       llmproxyclient.RequestInput{Messages: []llmproxyclient.MessageInput{{Role: "assistant", Content: "prefill"}}},
			errorString: "messages must include a user message",
		},
		{
			name:        "system prompt conflicts with system message",
			input:       llmproxyclient.RequestInput{SystemPrompt: "outer", Messages: []llmproxyclient.MessageInput{{Role: "system", Content: "inner"}, {Role: "user", Content: "prompt"}}},
			errorString: "system_prompt conflicts",
		},
		{
			name:        "mixed order fields",
			input:       llmproxyclient.RequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt", Order: messageOrder(1)}, {Role: "assistant", Content: "answer"}}},
			errorString: "order missing",
		},
		{
			name:        "duplicate order",
			input:       llmproxyclient.RequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt", Order: messageOrder(1)}, {Role: "assistant", Content: "answer", Order: messageOrder(1)}}},
			errorString: "duplicate messages order",
		},
		{
			name:        "negative order",
			input:       llmproxyclient.RequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt", Order: messageOrder(-1)}}},
			errorString: "order is negative",
		},
	}

	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			_, requestError := llmproxyclient.NewRequest(testCase.input)
			if requestError == nil || !strings.Contains(requestError.Error(), testCase.errorString) {
				subTest.Fatalf("error=%v want contains %q", requestError, testCase.errorString)
			}
		})
	}
}

func messageOrder(value int) *int {
	return &value
}
