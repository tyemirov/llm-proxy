package llmproxyclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestClientReadsPublicCapabilitiesWithoutAuthentication(testingInstance *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/prefix/api/public/capabilities" {
			testingInstance.Fatalf("request=%s %s", request.Method, request.URL.String())
		}
		if request.URL.RawQuery != "" || request.Header.Get("Accept") != "application/json" {
			testingInstance.Fatalf("query=%q headers=%v", request.URL.RawQuery, request.Header)
		}
		_ = json.NewEncoder(responseWriter).Encode(map[string]any{
			"revision": "current",
			"offerings": []map[string]any{{
				"identifier": "openai:gpt-5.5", "provider": "openai", "model": "gpt-5.5",
				"capabilities": []string{"image_input", "text"}, "wire_contract": "openai_responses",
				"execution_lifecycle":       "pollable_resource",
				"media_execution_lifecycle": "pollable_resource",
				"media_limits": []map[string]any{
					{
						"id": "inline_request_bytes", "media_type": "all", "transport": "inline",
						"status": "bounded", "value": 100, "unit": "bytes", "scope": "request_encoded_bytes",
						"source": "https://example.test/limits", "last_verified": "2026-08-29",
					},
					{
						"id": "image_count", "media_type": "image", "transport": "any",
						"status": "bounded", "value": 4, "unit": "files", "scope": "request",
						"source": "https://example.test/limits", "last_verified": "2026-08-29",
					},
					{
						"id": "image_inline_bytes", "media_type": "image", "transport": "inline",
						"status": "unknown", "value": nil, "unit": "bytes", "scope": "attachment_encoded_bytes",
						"source": "https://example.test/limits", "last_verified": "2026-08-29",
					},
				},
			}},
		})
	}))
	defer server.Close()

	config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: server.URL + "/prefix/v2?key=must-not-leak&provider=stale#fragment",
		Secret:  "secret",
	})
	if configError != nil {
		testingInstance.Fatal(configError)
	}
	client, clientError := llmproxyclient.NewClient(config, server.Client())
	if clientError != nil {
		testingInstance.Fatal(clientError)
	}
	catalog, capabilitiesError := client.GetPublicCapabilities(context.Background())
	if capabilitiesError != nil {
		testingInstance.Fatal(capabilitiesError)
	}
	if len(catalog.Offerings) != 1 || catalog.Offerings[0].Identifier != "openai:gpt-5.5" ||
		catalog.Offerings[0].WireContract != "openai_responses" ||
		catalog.Offerings[0].MediaExecutionLifecycle != "pollable_resource" ||
		len(catalog.Offerings[0].MediaLimits) != 3 || *catalog.Offerings[0].MediaLimits[1].Value != 4 {
		testingInstance.Fatalf("catalog=%+v", catalog)
	}
}

func TestClientReadsPublicStructuredOutputCapability(testingInstance *testing.T) {
	for _, capability := range []string{"structured_output", "future_unsupported_capability"} {
		testingInstance.Run(capability, func(testingInstance *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet || request.URL.Path != llmproxyclient.PublicCapabilitiesPath || request.URL.RawQuery != "" || request.Header.Get("Authorization") != "" {
					testingInstance.Errorf("unexpected public discovery request %s %s", request.Method, request.URL.Path)
				}
				_ = json.NewEncoder(responseWriter).Encode(map[string]any{"offerings": []map[string]any{{
					"identifier": "dashscope:qwen3.8-flash", "provider": "dashscope", "model": "qwen3.8-flash",
					"capabilities": []string{capability, "text"}, "wire_contract": "dashscope_responses",
					"execution_lifecycle": "synchronous_completion", "media_limits": []any{},
				}}})
			}))
			defer server.Close()
			configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "discovery-test-secret"})
			if err != nil {
				testingInstance.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(configuration, server.Client())
			if err != nil {
				testingInstance.Fatal(err)
			}
			catalog, err := client.GetPublicCapabilities(context.Background())
			if capability == "future_unsupported_capability" {
				if err == nil {
					testingInstance.Fatal("unknown capability must remain rejected")
				}
				return
			}
			if err != nil {
				testingInstance.Fatal(err)
			}
			if len(catalog.Offerings) != 1 || catalog.Offerings[0].Identifier != "dashscope:qwen3.8-flash" || !slices.Contains(catalog.Offerings[0].Capabilities, capability) {
				testingInstance.Fatalf("structured capability was not preserved: %+v", catalog)
			}
		})
	}
}
