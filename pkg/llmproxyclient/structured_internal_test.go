package llmproxyclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

type structuredClientDoer func(*http.Request) (*http.Response, error)

func (doer structuredClientDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type failingStructuredBody struct{}

func (failingStructuredBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingStructuredBody) Close() error             { return nil }

func TestStructuredMessagesRequestAndReconciliation(testingInstance *testing.T) {
	schema := []byte(`{"type":"object","additionalProperties":false,"required":["decision"],"properties":{"decision":{"type":"string","enum":["pass","return"]}}}`)
	request, requestError := NewMessagesRequest(MessagesRequestInput{
		Messages:         []MessageInput{{Role: "user", Content: "review"}},
		Model:            "gpt-5.5",
		StructuredOutput: &StructuredOutputInput{JSONSchema: schema},
		IdempotencyKey:   "review:request-1",
	})
	if requestError != nil {
		testingInstance.Fatalf("new structured request: %v", requestError)
	}
	callCount := 0
	doer := structuredClientDoer(func(httpRequest *http.Request) (*http.Response, error) {
		callCount++
		if httpRequest.URL.Path != "/prefix/v2" || httpRequest.Header.Get(llmproxycontract.HeaderIdempotencyKey) != "review:request-1" {
			testingInstance.Fatalf("post path=%q headers=%v", httpRequest.URL.Path, httpRequest.Header)
		}
		body, readError := io.ReadAll(httpRequest.Body)
		if readError != nil || !bytes.Contains(body, []byte(`"structured_output":{"schema":{"additionalProperties":false`)) {
			testingInstance.Fatalf("structured body=%s read_error=%v", body, readError)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"decision":"pass"}`)), Header: http.Header{}}, nil
	})
	config, configError := NewConfig(ConfigInput{
		BaseURL: "https://proxy.test/prefix?provider=base-provider&model=stale-model&format=text/plain",
		Secret:  "secret",
	})
	if configError != nil {
		testingInstance.Fatal(configError)
	}
	client, clientError := NewClient(config, doer)
	if clientError != nil {
		testingInstance.Fatal(clientError)
	}
	output, postError := client.PostMessages(context.Background(), request)
	if postError != nil || output != `{"decision":"pass"}` || callCount != 1 {
		testingInstance.Fatalf("output=%q calls=%d error=%v", output, callCount, postError)
	}

	client.httpClient = structuredClientDoer(func(*http.Request) (*http.Response, error) {
		return structuredClientResponse(
			http.StatusAccepted,
			`{"state":"dispatched","proxy_request_id":"proxy-pending","started_at":"2026-08-18T00:00:00Z","updated_at":"2026-08-18T00:00:01Z","elapsed_seconds":1}`,
			"dispatched",
		), nil
	})
	pendingOutput, pendingPostError := client.PostMessages(context.Background(), request)
	var pendingError *StructuredRequestPendingError
	if pendingPostError == nil || pendingOutput != "" || pendingPostError.Error() != "llm_proxy_client_structured_request_pending: state=dispatched" ||
		!errors.Is(pendingPostError, ErrStructuredRequestPending) || !errors.As(pendingPostError, &pendingError) {
		testingInstance.Fatalf("pending output=%q error=%v", pendingOutput, pendingPostError)
	}
	pendingSnapshot := pendingError.Snapshot()
	if pendingSnapshot.State != "dispatched" || pendingSnapshot.ProxyRequestID != "proxy-pending" || pendingSnapshot.ElapsedSeconds != 1 {
		testingInstance.Fatalf("pending snapshot=%+v", pendingSnapshot)
	}
	client.httpClient = structuredClientDoer(func(*http.Request) (*http.Response, error) {
		return structuredClientResponse(http.StatusAccepted, `{}`, "dispatched"), nil
	})
	if _, malformedPendingError := client.PostMessages(context.Background(), request); !errors.Is(malformedPendingError, ErrClientHTTPFailure) {
		testingInstance.Fatalf("malformed pending error=%v", malformedPendingError)
	}

	statusDoer := structuredClientDoer(func(httpRequest *http.Request) (*http.Response, error) {
		if httpRequest.Method != http.MethodGet || httpRequest.URL.Path != "/prefix/v2/requests" ||
			httpRequest.URL.Query().Get(queryKey) != "secret" || httpRequest.URL.Query().Has(queryFormat) ||
			httpRequest.URL.Query().Has(queryProvider) || httpRequest.URL.Query().Has(queryModel) {
			testingInstance.Fatalf("reconciliation request=%s %s", httpRequest.Method, httpRequest.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader(`{"state":"dispatched","proxy_request_id":"proxy-1","started_at":"2026-08-18T00:00:00Z","updated_at":"2026-08-18T00:00:01Z","elapsed_seconds":1}`)),
			Header:     http.Header{},
		}, nil
	})
	client.httpClient = statusDoer
	status, statusError := client.GetStructuredRequest(context.Background(), "review:request-1")
	if statusError != nil || status.State != "dispatched" || status.ProxyRequestID != "proxy-1" || status.ElapsedSeconds != 1 {
		testingInstance.Fatalf("status=%+v error=%v", status, statusError)
	}
}

func TestStructuredMessagesRequestValidation(testingInstance *testing.T) {
	validSchema := []byte(`{"type":"object"}`)
	cases := []MessagesRequestInput{
		{Messages: []MessageInput{{Role: "user", Content: "review"}}, StructuredOutput: &StructuredOutputInput{}, IdempotencyKey: "key"},
		{Messages: []MessageInput{{Role: "user", Content: "review"}}, StructuredOutput: &StructuredOutputInput{JSONSchema: []byte(`{`)}, IdempotencyKey: "key"},
		{Messages: []MessageInput{{Role: "user", Content: "review"}}, StructuredOutput: &StructuredOutputInput{JSONSchema: []byte(`[]`)}, IdempotencyKey: "key"},
		{Messages: []MessageInput{{Role: "user", Content: "review"}}, StructuredOutput: &StructuredOutputInput{JSONSchema: []byte(`{} {}`)}, IdempotencyKey: "key"},
		{Messages: []MessageInput{{Role: "user", Content: "review"}}, StructuredOutput: &StructuredOutputInput{JSONSchema: []byte(`{"type":"missing"}`)}, IdempotencyKey: "key"},
		{Messages: []MessageInput{{Role: "user", Content: "review"}}, StructuredOutput: &StructuredOutputInput{JSONSchema: validSchema}},
		{Messages: []MessageInput{{Role: "user", Content: "review"}}, IdempotencyKey: "key"},
		{Messages: []MessageInput{{Role: "user", Content: "review"}}, StructuredOutput: &StructuredOutputInput{JSONSchema: validSchema}, IdempotencyKey: strings.Repeat("x", 129)},
		{Messages: []MessageInput{{Role: "user", Content: "review"}}, WebSearch: true, StructuredOutput: &StructuredOutputInput{JSONSchema: validSchema}, IdempotencyKey: "key"},
	}
	for index, input := range cases {
		if _, requestError := NewMessagesRequest(input); !errors.Is(requestError, ErrInvalidClientRequest) {
			testingInstance.Fatalf("case=%d error=%v", index, requestError)
		}
	}
	if !validClientIdempotencyKey("A:._-9") || validClientIdempotencyKey(" bad") {
		testingInstance.Fatal("idempotency key validation mismatch")
	}

	originalMarshal := marshalClientStructuredSchema
	originalAdd := addClientStructuredResource
	testingInstance.Cleanup(func() {
		marshalClientStructuredSchema = originalMarshal
		addClientStructuredResource = originalAdd
	})
	marshalClientStructuredSchema = func(any) ([]byte, error) { return nil, errors.New("marshal failed") }
	if _, requestError := newStructuredOutput(&StructuredOutputInput{JSONSchema: validSchema}); !errors.Is(requestError, ErrInvalidClientRequest) {
		testingInstance.Fatalf("marshal error=%v", requestError)
	}
	marshalClientStructuredSchema = originalMarshal
	addClientStructuredResource = func(*jsonschema.Compiler, string, any) error { return errors.New("resource failed") }
	if _, requestError := newStructuredOutput(&StructuredOutputInput{JSONSchema: validSchema}); !errors.Is(requestError, ErrInvalidClientRequest) {
		testingInstance.Fatalf("resource error=%v", requestError)
	}
}

func TestStructuredRequestReconciliationResponses(testingInstance *testing.T) {
	base, parseError := url.Parse("https://proxy.test/v2")
	if parseError != nil {
		testingInstance.Fatal(parseError)
	}
	client := Client{config: Config{baseURL: base, secret: "secret"}}
	if _, requestError := client.GetStructuredRequest(context.Background(), " bad"); !errors.Is(requestError, ErrInvalidClientRequest) {
		testingInstance.Fatalf("invalid key error=%v", requestError)
	}

	testCases := []struct {
		name        string
		response    *http.Response
		httpError   error
		wantState   string
		wantOutput  string
		wantRequest string
		wantCause   string
		wantCode    string
		wantFailure bool
	}{
		{name: "transport", httpError: errors.New("offline"), wantFailure: true},
		{name: "read", response: &http.Response{StatusCode: 200, Body: failingStructuredBody{}, Header: http.Header{}}, wantFailure: true},
		{name: "invalid success", response: structuredClientResponse(200, `not-json`, "succeeded"), wantFailure: true},
		{name: "success", response: structuredClientResponse(200, `{"ok":true}`, "succeeded"), wantState: "succeeded", wantOutput: `{"ok":true}`},
		{name: "bad pending JSON", response: structuredClientResponse(202, `{`, "dispatched"), wantFailure: true},
		{name: "trailing pending JSON", response: structuredClientResponse(202, `{"state":"dispatched"} {}`, "dispatched"), wantFailure: true},
		{name: "missing pending state", response: structuredClientResponse(202, `{}`, "dispatched"), wantFailure: true},
		{
			name: "provider failure",
			response: structuredClientResponse(
				502,
				`{"error":{"code":"structured_request_failed","state":"failed","cause":"provider_error","proxy_request_id":"proxy-2"}}`,
				"failed",
			),
			wantState: "failed", wantRequest: "proxy-2", wantCause: "provider_error",
			wantCode: llmproxycontract.ErrorCodeStructuredRequestFailed, wantFailure: true,
		},
		{
			name: "uncertain",
			response: structuredClientResponse(
				409,
				`{"error":{"code":"structured_request_outcome_unknown","state":"uncertain","cause":"structured_request_interrupted","proxy_request_id":"proxy-3"}}`,
				"uncertain",
			),
			wantState: "uncertain", wantRequest: "proxy-3", wantCause: "structured_request_interrupted",
			wantCode: llmproxycontract.ErrorCodeStructuredRequestOutcomeUnknown, wantFailure: true,
		},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subtest *testing.T) {
			client.httpClient = structuredClientDoer(func(*http.Request) (*http.Response, error) {
				return testCase.response, testCase.httpError
			})
			result, resultError := client.GetStructuredRequest(context.Background(), "key")
			if testCase.wantFailure != (resultError != nil) || result.State != testCase.wantState || result.Output != testCase.wantOutput || result.ProxyRequestID != testCase.wantRequest || result.FailureCode != testCase.wantCause {
				subtest.Fatalf("result=%+v error=%v", result, resultError)
			}
			if testCase.wantCode != "" {
				var httpFailure *HTTPFailure
				if !errors.As(resultError, &httpFailure) || httpFailure.ProxyErrorCode() != testCase.wantCode {
					subtest.Fatalf("http failure=%v want_code=%q", resultError, testCase.wantCode)
				}
			}
		})
	}
}

func structuredClientResponse(statusCode int, body string, state string) *http.Response {
	header := http.Header{}
	header.Set(llmproxycontract.HeaderStructuredRequestState, state)
	return &http.Response{StatusCode: statusCode, Body: io.NopCloser(strings.NewReader(body)), Header: header}
}

func TestStructuredRequestEndpointPath(testingInstance *testing.T) {
	testCases := map[string]string{
		"":                    "/v2/requests",
		"/":                   "/v2/requests",
		"/v2":                 "/v2/requests",
		"/prefix/v2":          "/prefix/v2/requests",
		"/prefix/v2/requests": "/prefix/v2/requests",
		"/prefix":             "/prefix/v2/requests",
	}
	for input, expected := range testCases {
		if actual := structuredRequestEndpointPath(input); actual != expected {
			testingInstance.Fatalf("input=%q actual=%q expected=%q", input, actual, expected)
		}
	}
}
