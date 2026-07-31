package llmproxyclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestConfigMessagesPostURLShapesAuthenticatedV2JSONPostURL(testingInstance *testing.T) {
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:  "https://proxy.example/review?prompt=old&model=old&max_tokens=9&reasoning_effort=old&web_search=true&provider=gemini&keep=1",
		Secret:   "sekret",
		Provider: "deepseek",
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}

	messagesPostURL, messagesPostURLError := config.MessagesPostURL()
	if messagesPostURLError != nil {
		testingInstance.Fatalf("messages post URL error: %v", messagesPostURLError)
	}
	parsedURL, parseError := url.Parse(messagesPostURL)
	if parseError != nil {
		testingInstance.Fatalf("parse messages post url: %v", parseError)
	}
	if parsedURL.Path != "/review/v2" {
		testingInstance.Fatalf("messages path=%q", parsedURL.Path)
	}
	v2Config, v2ConfigError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: "https://proxy.example/v2?prompt=old",
		Secret:  "sekret",
	})
	if v2ConfigError != nil {
		testingInstance.Fatalf("v2 config error: %v", v2ConfigError)
	}
	existingMessagesPostURL, existingMessagesPostURLError := v2Config.MessagesPostURL()
	if existingMessagesPostURLError != nil {
		testingInstance.Fatalf("existing messages post URL error: %v", existingMessagesPostURLError)
	}
	parsedExistingMessagesURL, existingMessagesParseError := url.Parse(existingMessagesPostURL)
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
	for _, removedQueryKey := range []string{"prompt", "model", "max_tokens", "reasoning_effort", "web_search"} {
		if queryValues.Has(removedQueryKey) {
			testingInstance.Fatalf("query key %s should have been removed", removedQueryKey)
		}
	}
}

func TestClientPostMessagesSendsV2MessagesBody(testingInstance *testing.T) {
	firstOrder := messageOrder(1)
	secondOrder := messageOrder(2)
	contract := loadCanonicalOpenAPIContract(testingInstance)
	var capturedPath string
	var capturedTimeout string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		capturedPath = httpRequest.URL.Path
		capturedTimeout = httpRequest.Header.Get(llmproxycontract.HeaderRequestTimeoutSeconds)
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatalf("read body: %v", readError)
		}
		if decodeError := json.Unmarshal(bodyBytes, &capturedBody); decodeError != nil {
			testingInstance.Fatalf("decode body: %v", decodeError)
		}
		if validationError := contract.ValidateRequest("/v2", httpRequest.Method, httpRequest, bodyBytes); validationError != nil {
			testingInstance.Fatalf("Go package request violates OpenAPI: %v", validationError)
		}
		contractFields, fieldsError := contract.RequestPropertyNames("/v2", http.MethodPost)
		if fieldsError != nil {
			testingInstance.Fatalf("OpenAPI v2 fields: %v", fieldsError)
		}
		actualFields := make([]string, 0, len(capturedBody))
		for fieldName := range capturedBody {
			actualFields = append(actualFields, fieldName)
		}
		sort.Strings(actualFields)
		if !reflect.DeepEqual(actualFields, contractFields) {
			testingInstance.Fatalf("Go package v2 fields=%v OpenAPI fields=%v", actualFields, contractFields)
		}
		responseBody := []byte("ok")
		responseHeader := http.Header{}
		responseHeader.Set("Content-Type", "text/plain; charset=utf-8")
		responseHeader.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "12")
		if validationError := contract.ValidateResponse("/v2", http.MethodPost, http.StatusOK, responseHeader, responseBody); validationError != nil {
			testingInstance.Fatalf("Go package success fixture violates OpenAPI: %v", validationError)
		}
		for headerName, headerValues := range responseHeader {
			responseWriter.Header()[headerName] = headerValues
		}
		_, _ = responseWriter.Write(responseBody)
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: server.URL,
		Secret:  "sekret",
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client error: %v", clientError)
	}
	maxTokens := messageOrder(5)
	requestTimeoutSeconds := 12
	reasoningEffort := "high"
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages: []llmproxyclient.MessageInput{
			{Role: "assistant", Content: "Hi", Order: secondOrder},
			{Role: "user", Content: "Hello", Order: firstOrder},
		},
		Model:                 "deepseek-v4-flash",
		WebSearch:             true,
		MaxTokens:             maxTokens,
		ReasoningEffort:       &reasoningEffort,
		RequestTimeoutSeconds: &requestTimeoutSeconds,
	})
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}
	*maxTokens = 99
	reasoningEffort = "changed"
	requestTimeoutSeconds = 99

	responseText, postError := client.PostMessages(context.Background(), request)

	if postError != nil {
		testingInstance.Fatalf("post error: %v", postError)
	}
	if responseText != "ok" || capturedPath != "/v2" {
		testingInstance.Fatalf("response=%q path=%q", responseText, capturedPath)
	}
	if capturedTimeout != "12" {
		testingInstance.Fatalf("request timeout header=%q", capturedTimeout)
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
	if capturedBody["model"] != "deepseek-v4-flash" || capturedBody["web_search"] != true || capturedBody["max_tokens"] != float64(5) || capturedBody["reasoning_effort"] != "high" {
		testingInstance.Fatalf("body=%v", capturedBody)
	}
}

func loadCanonicalOpenAPIContract(testingInstance *testing.T) *openapitest.Contract {
	testingInstance.Helper()
	contract, loadError := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if loadError != nil {
		testingInstance.Fatalf("load canonical OpenAPI contract: %v", loadError)
	}
	return contract
}

func TestClientOmitsModelWhenRequestUsesProviderDefault(testingInstance *testing.T) {
	var capturedPath string
	var capturedTimeout string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatalf("read body: %v", readError)
		}
		if decodeError := json.Unmarshal(bodyBytes, &capturedBody); decodeError != nil {
			testingInstance.Fatalf("decode body: %v", decodeError)
		}
		capturedPath = httpRequest.URL.RequestURI()
		capturedTimeout = httpRequest.Header.Get(llmproxycontract.HeaderRequestTimeoutSeconds)
		_, _ = responseWriter.Write([]byte("ok"))
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: server.URL + "/review?provider=gemini&model=stale&keep=1",
		Secret:  "sekret",
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client error: %v", clientError)
	}
	messagesRequest, messagesRequestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Use provider default"}},
	})
	if messagesRequestError != nil {
		testingInstance.Fatalf("messages request error: %v", messagesRequestError)
	}

	if _, postError := client.PostMessages(context.Background(), messagesRequest); postError != nil {
		testingInstance.Fatalf("post messages error: %v", postError)
	}

	parsedURL, parseError := url.Parse(capturedPath)
	if parseError != nil {
		testingInstance.Fatalf("parse request URL: %v", parseError)
	}
	queryValues := parsedURL.Query()
	if parsedURL.Path != "/review/v2" || queryValues.Get("provider") != "gemini" {
		testingInstance.Fatalf("path=%s provider=%q", capturedPath, queryValues.Get("provider"))
	}
	if queryValues.Has("model") {
		testingInstance.Fatalf("path=%s must not include model query", capturedPath)
	}
	if _, hasModel := capturedBody["model"]; hasModel {
		testingInstance.Fatalf("body must omit model when using provider default: %v", capturedBody)
	}
	if capturedTimeout != "" {
		testingInstance.Fatalf("request timeout header should be omitted: %q", capturedTimeout)
	}
}

func TestClientReloadsAtomicallyReplacedModelProfile(testingInstance *testing.T) {
	profilePath := filepath.Join(testingInstance.TempDir(), "current-model.json")
	replaceModelProfile(testingInstance, profilePath, `{"provider":"gemini","model":"gemini-2.5-flash"}`)

	type capturedModelProfileRequest struct {
		provider string
		model    string
	}
	capturedRequests := []capturedModelProfileRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatalf("read body: %v", readError)
		}
		body := map[string]any{}
		if decodeError := json.Unmarshal(bodyBytes, &body); decodeError != nil {
			testingInstance.Fatalf("decode body: %v", decodeError)
		}
		model, modelExists := body["model"].(string)
		if !modelExists {
			testingInstance.Fatalf("profile request omitted model: %v", body)
		}
		capturedRequests = append(capturedRequests, capturedModelProfileRequest{
			provider: httpRequest.URL.Query().Get("provider"),
			model:    model,
		})
		_, _ = responseWriter.Write([]byte("ok"))
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:            server.URL,
		Secret:             "sekret",
		ModelProfilePath:   profilePath,
		ModelProfileReader: os.ReadFile,
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client error: %v", clientError)
	}
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Select my model"}},
	})
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}

	if _, postError := client.PostMessages(context.Background(), request); postError != nil {
		testingInstance.Fatalf("post profile A: %v", postError)
	}
	replaceModelProfile(testingInstance, profilePath, `{"provider":"openai","model":"gpt-5-mini"}`)
	if _, postError := client.PostMessages(context.Background(), request); postError != nil {
		testingInstance.Fatalf("post profile B: %v", postError)
	}

	wantRequests := []capturedModelProfileRequest{
		{provider: "gemini", model: "gemini-2.5-flash"},
		{provider: "openai", model: "gpt-5-mini"},
	}
	if len(capturedRequests) != len(wantRequests) {
		testingInstance.Fatalf("requests=%v", capturedRequests)
	}
	for requestIndex, wantRequest := range wantRequests {
		if capturedRequests[requestIndex] != wantRequest {
			testingInstance.Fatalf("request[%d]=%+v want=%+v", requestIndex, capturedRequests[requestIndex], wantRequest)
		}
	}
}

func TestConfigMessagesPostURLReloadsModelProfile(testingInstance *testing.T) {
	profilePath := filepath.Join(testingInstance.TempDir(), "current-model.json")
	replaceModelProfile(testingInstance, profilePath, `{"provider":"gemini","model":"gemini-2.5-flash"}`)
	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:            "https://proxy.example/review",
		Secret:             "sekret",
		ModelProfilePath:   profilePath,
		ModelProfileReader: os.ReadFile,
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}

	firstURL, firstURLError := config.MessagesPostURL()
	if firstURLError != nil {
		testingInstance.Fatalf("first messages post URL error: %v", firstURLError)
	}
	firstParsedURL, firstParseError := url.Parse(firstURL)
	if firstParseError != nil {
		testingInstance.Fatalf("parse first URL: %v", firstParseError)
	}
	if firstParsedURL.Query().Get("provider") != "gemini" {
		testingInstance.Fatalf("first provider=%q", firstParsedURL.Query().Get("provider"))
	}

	replaceModelProfile(testingInstance, profilePath, `{"provider":"openai","model":"gpt-5-mini"}`)
	secondURL, secondURLError := config.MessagesPostURL()
	if secondURLError != nil {
		testingInstance.Fatalf("second messages post URL error: %v", secondURLError)
	}
	secondParsedURL, secondParseError := url.Parse(secondURL)
	if secondParseError != nil {
		testingInstance.Fatalf("parse second URL: %v", secondParseError)
	}
	if secondParsedURL.Query().Get("provider") != "openai" {
		testingInstance.Fatalf("second provider=%q", secondParsedURL.Query().Get("provider"))
	}

	if removeError := os.Remove(profilePath); removeError != nil {
		testingInstance.Fatalf("remove profile: %v", removeError)
	}
	if _, postURLError := config.MessagesPostURL(); !errors.Is(postURLError, llmproxyclient.ErrInvalidModelProfile) {
		testingInstance.Fatalf("missing-profile URL error=%v", postURLError)
	}
}

func TestClientRejectsInvalidOrCompetingModelProfilesBeforeHTTP(testingInstance *testing.T) {
	profilePath := filepath.Join(testingInstance.TempDir(), "current-model.json")
	replaceModelProfile(testingInstance, profilePath, `{"provider":"gemini","model":"gemini-2.5-flash"}`)
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		requestCount++
		_, _ = responseWriter.Write([]byte("ok"))
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:            server.URL,
		Secret:             "sekret",
		ModelProfilePath:   profilePath,
		ModelProfileReader: os.ReadFile,
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client error: %v", clientError)
	}
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Keep profile current"}},
	})
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}
	if _, postError := client.PostMessages(context.Background(), request); postError != nil {
		testingInstance.Fatalf("post valid profile: %v", postError)
	}
	if requestCount != 1 {
		testingInstance.Fatalf("request count after valid profile=%d", requestCount)
	}

	invalidProfileCases := []struct {
		name         string
		prepare      func(*testing.T, string)
		errorMessage string
	}{
		{
			name: "empty document",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, "")
			},
			errorMessage: "decode model_profile",
		},
		{
			name: "invalid UTF-8 document",
			prepare: func(subTest *testing.T, path string) {
				invalidProfile := []byte(`{"provider":"gemini","model":"gemini-2.5-flash"}`)
				invalidProfile[len(invalidProfile)-3] = 0xff
				replaceModelProfileBytes(subTest, path, invalidProfile)
			},
			errorMessage: "valid UTF-8",
		},
		{
			name: "array document",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, "[]")
			},
			errorMessage: "document must be an object",
		},
		{
			name: "malformed",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, `{"provider":"gemini"`)
			},
			errorMessage: "decode model_profile",
		},
		{
			name: "malformed second field",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, `{"provider":"gemini",`)
			},
			errorMessage: "decode model_profile",
		},
		{
			name: "non-string value",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, `{"provider":7,"model":"gemini-2.5-flash"}`)
			},
			errorMessage: "decode model_profile",
		},
		{
			name: "incomplete",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, `{"provider":"gemini"}`)
			},
			errorMessage: "missing model",
		},
		{
			name: "missing provider",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, `{"model":"gemini-2.5-flash"}`)
			},
			errorMessage: "missing provider",
		},
		{
			name: "unexpected field",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, `{"provider":"gemini","model":"gemini-2.5-flash","secret":"forbidden"}`)
			},
			errorMessage: "unsupported field",
		},
		{
			name: "duplicate field",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, `{"provider":"gemini","provider":"openai","model":"gpt-5-mini"}`)
			},
			errorMessage: "duplicate field",
		},
		{
			name: "trailing malformed JSON",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, `{"provider":"gemini","model":"gemini-2.5-flash"} trailing`)
			},
			errorMessage: "decode model_profile",
		},
		{
			name: "trailing JSON document",
			prepare: func(subTest *testing.T, path string) {
				replaceModelProfile(subTest, path, `{"provider":"gemini","model":"gemini-2.5-flash"}{}`)
			},
			errorMessage: "one JSON value",
		},
		{
			name: "unreadable",
			prepare: func(subTest *testing.T, path string) {
				if removeError := os.Remove(path); removeError != nil {
					subTest.Fatalf("remove profile: %v", removeError)
				}
			},
			errorMessage: "read model_profile",
		},
	}
	for _, invalidProfileCase := range invalidProfileCases {
		testingInstance.Run(invalidProfileCase.name, func(subTest *testing.T) {
			invalidProfileCase.prepare(subTest, profilePath)
			_, postError := client.PostMessages(context.Background(), request)
			if !errors.Is(postError, llmproxyclient.ErrInvalidModelProfile) {
				subTest.Fatalf("post error=%v", postError)
			}
			if !strings.Contains(postError.Error(), invalidProfileCase.errorMessage) {
				subTest.Fatalf("post error=%v missing %q", postError, invalidProfileCase.errorMessage)
			}
			if requestCount != 1 {
				subTest.Fatalf("invalid profile reused a prior model; request count=%d", requestCount)
			}
			replaceModelProfile(subTest, profilePath, `{"provider":"gemini","model":"gemini-2.5-flash"}`)
		})
	}

	pinnedRequest, pinnedRequestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Do not compete"}},
		Model:    "gpt-5-mini",
	})
	if pinnedRequestError != nil {
		testingInstance.Fatalf("pinned request error: %v", pinnedRequestError)
	}
	if _, postError := client.PostMessages(context.Background(), pinnedRequest); !errors.Is(postError, llmproxyclient.ErrInvalidModelProfile) {
		testingInstance.Fatalf("pinned profile post error=%v", postError)
	}
	if requestCount != 1 {
		testingInstance.Fatalf("competing request model reached HTTP; request count=%d", requestCount)
	}
}

func TestConfigRejectsModelProfileSourceConflicts(testingInstance *testing.T) {
	profileReader := llmproxyclient.ModelProfileReader(os.ReadFile)
	testCases := []struct {
		name        string
		input       llmproxyclient.ConfigInput
		errorString string
	}{
		{
			name: "reader without path",
			input: llmproxyclient.ConfigInput{
				BaseURL:            "https://proxy.example",
				Secret:             "sekret",
				ModelProfileReader: profileReader,
			},
			errorString: "model_profile_reader requires model_profile_path",
		},
		{
			name: "missing reader",
			input: llmproxyclient.ConfigInput{
				BaseURL:          "https://proxy.example",
				Secret:           "sekret",
				ModelProfilePath: "/profiles/user.json",
			},
			errorString: "model_profile_path requires model_profile_reader",
		},
		{
			name: "configured provider",
			input: llmproxyclient.ConfigInput{
				BaseURL:            "https://proxy.example",
				Secret:             "sekret",
				Provider:           "gemini",
				ModelProfilePath:   "/profiles/user.json",
				ModelProfileReader: profileReader,
			},
			errorString: "conflicts with provider",
		},
		{
			name: "base URL provider",
			input: llmproxyclient.ConfigInput{
				BaseURL:            "https://proxy.example?provider=gemini",
				Secret:             "sekret",
				ModelProfilePath:   "/profiles/user.json",
				ModelProfileReader: profileReader,
			},
			errorString: "base_url provider query",
		},
		{
			name: "base URL model",
			input: llmproxyclient.ConfigInput{
				BaseURL:            "https://proxy.example?model=gpt-5-mini",
				Secret:             "sekret",
				ModelProfilePath:   "/profiles/user.json",
				ModelProfileReader: profileReader,
			},
			errorString: "base_url model query",
		},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			_, configError := llmproxyclient.NewConfig(testCase.input)
			if !errors.Is(configError, llmproxyclient.ErrInvalidClientConfig) {
				subTest.Fatalf("config error=%v", configError)
			}
			if !strings.Contains(configError.Error(), testCase.errorString) {
				subTest.Fatalf("config error=%v missing %q", configError, testCase.errorString)
			}
		})
	}
}

func TestClientSendsUnknownModelProfilePairToProxy(testingInstance *testing.T) {
	contract := loadCanonicalOpenAPIContract(testingInstance)
	profilePath := filepath.Join(testingInstance.TempDir(), "current-model.json")
	replaceModelProfile(testingInstance, profilePath, `{"provider":"unknown","model":"unknown-model"}`)
	var capturedProvider string
	var capturedModel string
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		capturedProvider = httpRequest.URL.Query().Get("provider")
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatalf("read body: %v", readError)
		}
		body := map[string]any{}
		if decodeError := json.Unmarshal(bodyBytes, &body); decodeError != nil {
			testingInstance.Fatalf("decode body: %v", decodeError)
		}
		if validationError := contract.ValidateRequest("/v2", httpRequest.Method, httpRequest, bodyBytes); validationError != nil {
			testingInstance.Fatalf("Go package error request violates OpenAPI: %v", validationError)
		}
		capturedModel, _ = body["model"].(string)
		responseBody := []byte("unknown provider/model pair")
		responseHeader := http.Header{}
		responseHeader.Set("Content-Type", "text/plain; charset=utf-8")
		responseHeader.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "360")
		if validationError := contract.ValidateResponse("/v2", http.MethodPost, http.StatusBadRequest, responseHeader, responseBody); validationError != nil {
			testingInstance.Fatalf("Go package error fixture violates OpenAPI: %v", validationError)
		}
		for headerName, headerValues := range responseHeader {
			responseWriter.Header()[headerName] = headerValues
		}
		responseWriter.WriteHeader(http.StatusBadRequest)
		_, _ = responseWriter.Write(responseBody)
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:            server.URL,
		Secret:             "sekret",
		ModelProfilePath:   profilePath,
		ModelProfileReader: os.ReadFile,
	})
	if configError != nil {
		testingInstance.Fatalf("config error: %v", configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatalf("client error: %v", clientError)
	}
	request, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Route this pair"}},
	})
	if requestError != nil {
		testingInstance.Fatalf("request error: %v", requestError)
	}

	if _, postError := client.PostMessages(context.Background(), request); !errors.Is(postError, llmproxyclient.ErrClientHTTPFailure) {
		testingInstance.Fatalf("post error=%v", postError)
	}
	if capturedProvider != "unknown" || capturedModel != "unknown-model" {
		testingInstance.Fatalf("proxy received provider=%q model=%q", capturedProvider, capturedModel)
	}
}

func TestMessagesRequestRejectsInvalidInputs(testingInstance *testing.T) {
	emptyReasoningEffort := ""
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
			name:        "blank reasoning effort",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt"}}, ReasoningEffort: &emptyReasoningEffort},
			errorString: "reasoning_effort must be nonblank",
		},
		{
			name:        "invalid request timeout",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt"}}, RequestTimeoutSeconds: messageOrder(0)},
			errorString: "request_timeout_seconds must be positive",
		},
		{
			name:        "unsupported role",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "tool", Content: "tool result"}}},
			errorString: "role unsupported",
		},
		{
			name:        "empty content",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user"}}},
			errorString: "content is empty",
		},
		{
			name:        "missing user message",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "assistant", Content: "prefill"}}},
			errorString: "messages must include a user message",
		},
		{
			name:        "mixed order fields",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt", Order: messageOrder(1)}, {Role: "assistant", Content: "answer"}}},
			errorString: "order missing",
		},
		{
			name:        "duplicate order",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt", Order: messageOrder(1)}, {Role: "assistant", Content: "answer", Order: messageOrder(1)}}},
			errorString: "duplicate messages order",
		},
		{
			name:        "negative order",
			input:       llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "prompt", Order: messageOrder(-1)}}},
			errorString: "order is negative",
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

func replaceModelProfile(testingInstance *testing.T, profilePath string, profileDocument string) {
	testingInstance.Helper()
	replaceModelProfileBytes(testingInstance, profilePath, []byte(profileDocument))
}

func replaceModelProfileBytes(testingInstance *testing.T, profilePath string, profileBytes []byte) {
	testingInstance.Helper()
	replacementPath := filepath.Join(filepath.Dir(profilePath), "next-model.json")
	if writeError := os.WriteFile(replacementPath, profileBytes, 0600); writeError != nil {
		testingInstance.Fatalf("write replacement model profile: %v", writeError)
	}
	if renameError := os.Rename(replacementPath, profilePath); renameError != nil {
		testingInstance.Fatalf("replace model profile: %v", renameError)
	}
}

func messageOrder(value int) *int {
	return &value
}
