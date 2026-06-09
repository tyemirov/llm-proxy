package llmproxyclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

type clientDoer func(*http.Request) (*http.Response, error)

func (doer clientDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type errorReader struct{}

func (reader errorReader) Read(_ []byte) (int, error) {
	return 0, errors.New("read failed")
}

func (reader errorReader) Close() error {
	return nil
}

func testTextResponse(statusCode int, body string, headers http.Header) *http.Response {
	responseHeaders := http.Header{}
	for headerName, headerValues := range headers {
		for _, headerValue := range headerValues {
			responseHeaders.Add(headerName, headerValue)
		}
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     responseHeaders,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func resumableGatewayTimeout(responseID string) *http.Response {
	headers := http.Header{}
	headers.Set("X-LLM-Proxy-Resume-Provider", "openai")
	headers.Set("X-LLM-Proxy-Upstream-Response-ID", responseID)
	return testTextResponse(http.StatusGatewayTimeout, "still running", headers)
}

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

func TestClientPostResumesOpenAIBackgroundResponse(testingInstance *testing.T) {
	var capturedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		capturedPaths = append(capturedPaths, httpRequest.URL.String())
		switch len(capturedPaths) {
		case 1:
			if httpRequest.Method != http.MethodPost {
				testingInstance.Fatalf("method=%s want POST", httpRequest.Method)
			}
			responseWriter.Header().Set("X-LLM-Proxy-Resume-Provider", "openai")
			responseWriter.Header().Set("X-LLM-Proxy-Upstream-Response-ID", "resp_test")
			responseWriter.WriteHeader(http.StatusGatewayTimeout)
			_, _ = responseWriter.Write([]byte("still running"))
		case 2:
			if httpRequest.Method != http.MethodGet {
				testingInstance.Fatalf("method=%s want GET", httpRequest.Method)
			}
			_, _ = responseWriter.Write([]byte("resumed"))
		default:
			testingInstance.Fatalf("unexpected request count=%d", len(capturedPaths))
		}
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:  server.URL + "/review?provider=openai&keep=1",
		Secret:   "sekret",
		Provider: "openai",
		Timeout:  time.Second,
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client error: %v", clientError)
	}
	request, requestError := llmproxyclient.NewRequest(llmproxyclient.RequestInput{Prompt: "prompt", Model: "gpt-5-mini"})
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}

	responseText, postError := client.Post(context.Background(), request)

	if postError != nil {
		testingInstance.Fatalf("post error: %v", postError)
	}
	if responseText != "resumed" {
		testingInstance.Fatalf("response=%q want resumed", responseText)
	}
	if len(capturedPaths) != 2 {
		testingInstance.Fatalf("request count=%d want 2", len(capturedPaths))
	}
	resumeURL, parseError := url.Parse(capturedPaths[1])
	if parseError != nil {
		testingInstance.Fatalf("parse resume url: %v", parseError)
	}
	if resumeURL.Path != "/review/responses/resp_test" {
		testingInstance.Fatalf("resume path=%q", resumeURL.Path)
	}
	if resumeURL.Query().Get("key") != "sekret" || resumeURL.Query().Get("provider") != "openai" || resumeURL.Query().Get("format") != "text/plain" || resumeURL.Query().Get("keep") != "1" {
		testingInstance.Fatalf("resume query=%s", resumeURL.RawQuery)
	}
}

func TestClientPostMessagesResumesFromV2BasePath(testingInstance *testing.T) {
	var capturedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		capturedPaths = append(capturedPaths, httpRequest.URL.Path)
		switch len(capturedPaths) {
		case 1:
			responseWriter.Header().Set("X-LLM-Proxy-Resume-Provider", "openai")
			responseWriter.Header().Set("X-LLM-Proxy-Upstream-Response-ID", "resp_v2")
			responseWriter.WriteHeader(http.StatusGatewayTimeout)
			_, _ = responseWriter.Write([]byte("still running"))
		case 2:
			_, _ = responseWriter.Write([]byte("resumed messages"))
		default:
			testingInstance.Fatalf("unexpected request count=%d", len(capturedPaths))
		}
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: server.URL + "/v2",
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
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt"}},
		Model:    "gpt-5-mini",
	})
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}

	responseText, postError := client.PostMessages(context.Background(), request)

	if postError != nil {
		testingInstance.Fatalf("post error: %v", postError)
	}
	if responseText != "resumed messages" {
		testingInstance.Fatalf("response=%q", responseText)
	}
	if strings.Join(capturedPaths, ",") != "/v2,/responses/resp_v2" {
		testingInstance.Fatalf("paths=%v", capturedPaths)
	}
}

func TestClientPostRepeatsResumeUntilStoredResponseCompletes(testingInstance *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		requestCount++
		if requestCount <= 2 {
			responseWriter.Header().Set("X-LLM-Proxy-Resume-Provider", "openai")
			responseWriter.Header().Set("X-LLM-Proxy-Upstream-Response-ID", fmt.Sprintf("resp_%d", requestCount))
			responseWriter.WriteHeader(http.StatusGatewayTimeout)
			_, _ = responseWriter.Write([]byte("still running"))
			return
		}
		_, _ = responseWriter.Write([]byte("complete"))
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
	request, requestError := llmproxyclient.NewRequest(llmproxyclient.RequestInput{Prompt: "prompt"})
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}

	responseText, postError := client.Post(context.Background(), request)

	if postError != nil {
		testingInstance.Fatalf("post error: %v", postError)
	}
	if responseText != "complete" || requestCount != 3 {
		testingInstance.Fatalf("response=%q request_count=%d", responseText, requestCount)
	}
}

func TestClientPostResumeFailuresRemainHTTPFailures(testingInstance *testing.T) {
	testCases := []struct {
		name             string
		doer             clientDoer
		expectedFragment string
	}{
		{
			name: "resume request transport error",
			doer: func() clientDoer {
				requestCount := 0
				return func(_ *http.Request) (*http.Response, error) {
					requestCount++
					if requestCount == 1 {
						return resumableGatewayTimeout("resp_test"), nil
					}
					return nil, errors.New("network unavailable")
				}
			}(),
			expectedFragment: "resume request: network unavailable",
		},
		{
			name: "resume response read error",
			doer: func() clientDoer {
				requestCount := 0
				return func(_ *http.Request) (*http.Response, error) {
					requestCount++
					if requestCount == 1 {
						return resumableGatewayTimeout("resp_test"), nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{},
						Body:       errorReader{},
					}, nil
				}
			}(),
			expectedFragment: "read response body: read failed",
		},
		{
			name: "resume response lacks next token",
			doer: func() clientDoer {
				requestCount := 0
				return func(_ *http.Request) (*http.Response, error) {
					requestCount++
					if requestCount == 1 {
						return resumableGatewayTimeout("resp_test"), nil
					}
					return testTextResponse(http.StatusGatewayTimeout, "missing token", http.Header{}), nil
				}
			}(),
			expectedFragment: `status=504 body="missing token"`,
		},
		{
			name: "resume attempts exhausted",
			doer: func(_ *http.Request) (*http.Response, error) {
				return resumableGatewayTimeout("resp_test"), nil
			},
			expectedFragment: "OpenAI background response did not complete after resume attempts",
		},
	}

	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
				BaseURL: "https://proxy.example",
				Secret:  "sekret",
				Timeout: time.Second,
			})
			if configError != nil {
				subTest.Fatalf("config error: %v", configError)
			}
			client, clientError := llmproxyclient.NewClient(config, testCase.doer)
			if clientError != nil {
				subTest.Fatalf("client error: %v", clientError)
			}
			request, requestError := llmproxyclient.NewRequest(llmproxyclient.RequestInput{Prompt: "prompt"})
			if requestError != nil {
				subTest.Fatalf("request error: %v", requestError)
			}

			_, postError := client.Post(context.Background(), request)

			if postError == nil || !strings.Contains(postError.Error(), testCase.expectedFragment) {
				subTest.Fatalf("error=%v want contains %q", postError, testCase.expectedFragment)
			}
		})
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
