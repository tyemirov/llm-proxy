package llmproxyclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type capabilitiesDoer func(*http.Request) (*http.Response, error)

func (doer capabilitiesDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type failingCapabilitiesBody struct{}

func (failingCapabilitiesBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingCapabilitiesBody) Close() error             { return nil }

func TestPublicCapabilitiesResponseBoundaries(testingInstance *testing.T) {
	baseURL, parseError := url.Parse("https://proxy.test/v2")
	if parseError != nil {
		testingInstance.Fatal(parseError)
	}
	inlineRequestLimit := `{"id":"inline_request_bytes","media_type":"all","transport":"inline","status":"bounded","value":100,"unit":"bytes","scope":"request_encoded_bytes","source":"https://example.test/limits","last_verified":"2026-08-29"}`
	inlineRequestWrongScope := `{"id":"inline_request_bytes","media_type":"all","transport":"inline","status":"bounded","value":100,"unit":"bytes","scope":"request","source":"https://example.test/limits","last_verified":"2026-08-29"}`
	imageCountLimit := `{"id":"image_count","media_type":"image","transport":"any","status":"bounded","value":1,"unit":"files","scope":"request","source":"https://example.test/limits","last_verified":"2026-08-29"}`
	imageCountWrongScope := `{"id":"image_count","media_type":"image","transport":"any","status":"bounded","value":1,"unit":"files","scope":"attachment","source":"https://example.test/limits","last_verified":"2026-08-29"}`
	imageInlineLimit := `{"id":"image_inline_bytes","media_type":"image","transport":"inline","status":"unknown","value":null,"unit":"bytes","scope":"attachment","source":"https://example.test/limits","last_verified":"2026-08-29"}`
	imageFileLimit := `{"id":"image_file_bytes","media_type":"image","transport":"file","status":"bounded","value":100,"unit":"bytes","scope":"attachment","source":"https://example.test/limits","last_verified":"2026-08-29"}`
	audioCountLimit := `{"id":"audio_count","media_type":"audio","transport":"any","status":"unknown","value":null,"unit":"files","scope":"request","source":"https://example.test/limits","last_verified":"2026-08-29"}`
	audioFileLimit := `{"id":"audio_file_bytes","media_type":"audio","transport":"file","status":"bounded","value":100,"unit":"bytes","scope":"attachment","source":"https://example.test/limits","last_verified":"2026-08-29"}`
	inlineRoute := `"wire_contract":"openai_responses","execution_lifecycle":"pollable_resource"`
	fileRoute := `"wire_contract":"gemini_interactions","execution_lifecycle":"pollable_resource"`
	validOffering := `{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + inlineRoute + `,"media_limits":[` + inlineRequestLimit + `,` + imageCountLimit + `,` + imageInlineLimit + `]}`
	testCases := []struct {
		name       string
		response   *http.Response
		requestErr error
	}{
		{name: "transport", requestErr: errors.New("offline")},
		{name: "nil response"},
		{name: "nil body", response: &http.Response{StatusCode: http.StatusOK}},
		{name: "read", response: &http.Response{StatusCode: http.StatusOK, Body: failingCapabilitiesBody{}}},
		{name: "oversize", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", publicCapabilitiesMaximumBytes+1)))}},
		{name: "status", response: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"provider_error"}}`))}},
		{name: "invalid JSON", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{`))}},
		{name: "trailing JSON", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[]} {}`))}},
		{name: "missing offerings", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}},
		{name: "empty offerings", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[]}`))}},
		{name: "empty offering", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{}]}`))}},
		{name: "missing media limits", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["text"]}]}`))}},
		{name: "unsupported route", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["text"],"wire_contract":"future","execution_lifecycle":"pollable_resource","media_limits":[]}]} `))}},
		{name: "unsupported capability", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["future"],` + inlineRoute + `,"media_limits":[]}]}`))}},
		{name: "duplicate capability", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["text","text"],` + inlineRoute + `,"media_limits":[]}]}`))}},
		{name: "invalid media limit", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["text"],` + inlineRoute + `,"media_limits":[{}]}]}`))}},
		{name: "media limit without capability", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["text"],` + inlineRoute + `,"media_limits":[` + imageCountLimit + `]}]}`))}},
		{name: "audio limit without capability", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["text"],` + inlineRoute + `,"media_limits":[` + audioCountLimit + `]}]}`))}},
		{name: "media capability without limits", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + inlineRoute + `,"media_limits":[]}]}`))}},
		{name: "missing inline request limit", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + inlineRoute + `,"media_limits":[` + imageCountLimit + `,` + imageInlineLimit + `]}]}`))}},
		{name: "wrong inline request relationship", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + inlineRoute + `,"media_limits":[` + inlineRequestWrongScope + `,` + imageCountLimit + `,` + imageInlineLimit + `]}]}`))}},
		{name: "missing image count limit", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + inlineRoute + `,"media_limits":[` + inlineRequestLimit + `,` + imageInlineLimit + `]}]}`))}},
		{name: "wrong image count relationship", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + inlineRoute + `,"media_limits":[` + inlineRequestLimit + `,` + imageCountWrongScope + `,` + imageInlineLimit + `]}]}`))}},
		{name: "missing inline attachment limit", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + inlineRoute + `,"media_limits":[` + inlineRequestLimit + `,` + imageCountLimit + `]}]}`))}},
		{name: "file limit on inline route", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + inlineRoute + `,"media_limits":[` + inlineRequestLimit + `,` + imageCountLimit + `,` + imageFileLimit + `]}]}`))}},
		{name: "missing audio count limit", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["audio_input","text"],` + fileRoute + `,"media_limits":[` + inlineRequestLimit + `,` + audioFileLimit + `]}]}`))}},
		{name: "missing audio file limit", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["audio_input","text"],` + fileRoute + `,"media_limits":[` + inlineRequestLimit + `,` + audioCountLimit + `]}]}`))}},
		{name: "inline limit on file route", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + fileRoute + `,"media_limits":[` + inlineRequestLimit + `,` + imageCountLimit + `,` + imageInlineLimit + `]}]}`))}},
		{name: "duplicate media limit", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[{"identifier":"provider:model","provider":"provider","model":"model","capabilities":["image_input","text"],` + inlineRoute + `,"media_limits":[` + inlineRequestLimit + `,` + imageCountLimit + `,` + imageCountLimit + `,` + imageInlineLimit + `]}]}`))}},
		{name: "duplicate offering", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offerings":[` + validOffering + `,` + validOffering + `]}`))}},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subtest *testing.T) {
			client := Client{
				config: Config{baseURL: baseURL, secret: "secret"},
				httpClient: capabilitiesDoer(func(*http.Request) (*http.Response, error) {
					return testCase.response, testCase.requestErr
				}),
			}
			if _, capabilitiesError := client.GetPublicCapabilities(context.Background()); capabilitiesError == nil {
				subtest.Fatal("invalid response passed")
			}
		})
	}
}

func TestPublicCapabilitiesAcceptsRequiredMediaLimitRelationships(testingInstance *testing.T) {
	baseURL, parseError := url.Parse("https://proxy.test/v2")
	if parseError != nil {
		testingInstance.Fatal(parseError)
	}
	body := `{"offerings":[` +
		`{"identifier":"provider:text","provider":"provider","model":"text","capabilities":["text"],"wire_contract":"openai_responses","execution_lifecycle":"synchronous_completion","media_limits":[]},` +
		`{"identifier":"provider:media","provider":"provider","model":"media","capabilities":["audio_input","image_input","text"],"wire_contract":"gemini_interactions","execution_lifecycle":"pollable_resource","media_limits":[` +
		`{"id":"inline_request_bytes","media_type":"all","transport":"inline","status":"bounded","value":100,"unit":"bytes","scope":"request_encoded_bytes","source":"https://example.test/limits","last_verified":"2026-08-29"},` +
		`{"id":"image_count","media_type":"image","transport":"any","status":"bounded","value":1,"unit":"files","scope":"request","source":"https://example.test/limits","last_verified":"2026-08-29"},` +
		`{"id":"image_file_bytes","media_type":"image","transport":"file","status":"bounded","value":100,"unit":"bytes","scope":"attachment","source":"https://example.test/limits","last_verified":"2026-08-29"},` +
		`{"id":"audio_count","media_type":"audio","transport":"any","status":"unknown","value":null,"unit":"files","scope":"request","source":"https://example.test/limits","last_verified":"2026-08-29"},` +
		`{"id":"audio_file_bytes","media_type":"audio","transport":"file","status":"bounded","value":100,"unit":"bytes","scope":"attachment","source":"https://example.test/limits","last_verified":"2026-08-29"}` +
		`]}]}`
	client := Client{
		config: Config{baseURL: baseURL, secret: "secret"},
		httpClient: capabilitiesDoer(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
		}),
	}
	catalog, capabilitiesError := client.GetPublicCapabilities(context.Background())
	if capabilitiesError != nil || len(catalog.Offerings) != 2 {
		testingInstance.Fatalf("catalog=%+v error=%v", catalog, capabilitiesError)
	}
}

func TestPublicCapabilitiesEndpointPaths(testingInstance *testing.T) {
	testCases := map[string]string{
		"":           PublicCapabilitiesPath,
		"/":          PublicCapabilitiesPath,
		"/v2":        PublicCapabilitiesPath,
		"/prefix/v2": "/prefix" + PublicCapabilitiesPath,
		"/prefix":    "/prefix" + PublicCapabilitiesPath,
	}
	for input, expected := range testCases {
		if actual := publicCapabilitiesEndpointPath(input); actual != expected {
			testingInstance.Fatalf("input=%q actual=%q expected=%q", input, actual, expected)
		}
	}
}
