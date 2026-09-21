package llmproxyclient_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestProviderResourcesClientDiscovery(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		valid      bool
	}{
		{"empty", `{"catalog_revision":"current","routes":[],"services":[],"resources":[]}`, true},
		{"voice", `{"catalog_revision":"current","routes":[],"services":[],"resources":[{"provider":"speech","kind":"voices"}]}`, true},
		{"missing", `{"catalog_revision":"current","routes":[],"services":[]}`, false},
		{"null", `{"catalog_revision":"current","routes":[],"services":[],"resources":null}`, false},
		{"unknown", `{"catalog_revision":"current","routes":[],"services":[],"resources":[{"provider":"speech","kind":"unknown"}]}`, false},
		{"no provider", `{"catalog_revision":"current","routes":[],"services":[],"resources":[{"kind":"voices"}]}`, false},
		{"duplicate", `{"catalog_revision":"current","routes":[],"services":[],"resources":[{"provider":"speech","kind":"voices"},{"provider":"speech","kind":"voices"}]}`, false},
		{"private field", `{"catalog_revision":"current","routes":[],"services":[],"resources":[{"provider":"speech","kind":"voices","transport":"private"}]}`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet || request.URL.Path != "/model/v1/capabilities" || request.Header.Get("Authorization") != "Bearer tenant-secret" {
					t.Error("invalid discovery request")
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(scenario.body))
			}))
			defer server.Close()
			config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "tenant-secret"})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(config, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GetMediaCapabilities(t.Context())
			if (err == nil) != scenario.valid {
				t.Fatalf("valid=%t error=%v", scenario.valid, err)
			}
		})
	}
}
