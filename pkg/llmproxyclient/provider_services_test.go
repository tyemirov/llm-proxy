package llmproxyclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderServicesClientExactShapes(t *testing.T) {
	operation := strings.Replace(validMediaOperationJSON("succeeded"), `"model":"grok-imagine-video-1.5",`, "", 1)
	// The shared fixture is a video operation; this fixture declares alignment.
	var document map[string]any
	if err := json.Unmarshal([]byte(operation), &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "model")
	document["capability"] = "audio.align"
	body, _ := json.Marshal(document)
	service := `{"capability":"audio.align","provider":"elevenlabs","controls":[],"limits":[]}`
	for _, scenario := range []struct {
		name, services string
		valid          bool
	}{
		{"available", "[" + service + "]", true}, {"empty", "[]", true}, {"null", "null", false}, {"duplicate", "[" + service + "," + service + "]", false}, {"missing provider", `[{"capability":"audio.align","controls":[],"limits":[]}]`, false}, {"unknown", `[{"capability":"future","provider":"elevenlabs","controls":[],"limits":[]}]`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/model/v1/capabilities" {
					_, _ = io.WriteString(w, `{"catalog_revision":"current","routes":[],"resources":[],"services":`+scenario.services+`}`)
					return
				}
				if r.Method == http.MethodPost {
					data, _ := io.ReadAll(r.Body)
					if strings.Contains(string(data), `"model"`) {
						t.Error("service request emitted a model")
					}
					w.WriteHeader(http.StatusAccepted)
				}
				_, _ = w.Write(body)
			}))
			defer server.Close()
			config, err := NewConfig(ConfigInput{BaseURL: server.URL, Secret: "tenant"})
			if err != nil {
				t.Fatal(err)
			}
			client, err := NewClient(config, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GetMediaCapabilities(context.Background())
			if (err == nil) != scenario.valid {
				t.Fatalf("discovery error=%v", err)
			}
			result, err := client.CreateMediaOperation(context.Background(), "alignment", MediaOperationInput{Capability: "audio.align", Provider: "elevenlabs", Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`)})
			if err != nil || result.Model != "" {
				t.Fatalf("service=%+v err=%v", result, err)
			}
		})
	}
}

func TestProviderServicesClientRejectsPresentInvalidModel(t *testing.T) {
	for _, model := range []string{`null`, `""`, `" "`, `5`} {
		body := strings.Replace(validMediaOperationJSON("succeeded"), `"model":"grok-imagine-video-1.5"`, `"model":`+model, 1)
		var record map[string]json.RawMessage
		_ = json.Unmarshal([]byte(body), &record)
		record["model"] = json.RawMessage(model)
		record["capability"] = json.RawMessage(`"audio.align"`)
		data, _ := json.Marshal(record)
		client := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
			return mediaOperationTestResponse(http.StatusOK, string(data)), nil
		}))
		if _, err := client.GetMediaOperation(context.Background(), "mop_0123456789abcdef0123456789abcdef"); err == nil {
			t.Fatalf("invalid model %s accepted", model)
		}
	}
}

func TestProviderServicesClientRejectsMalformedHTTPResponses(t *testing.T) {
	for _, body := range []string{`{`, `{"operation_id":[]}`, `{"unexpected":"private response"}`, validMediaOperationJSON("succeeded") + ` {}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
		configuration, err := NewConfig(ConfigInput{BaseURL: server.URL, Secret: "tenant"})
		if err != nil {
			t.Fatal(err)
		}
		client, err := NewClient(configuration, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.GetMediaOperation(context.Background(), "mop_0123456789abcdef0123456789abcdef")
		server.Close()
		if err == nil {
			t.Fatalf("malformed service response accepted: %s", body)
		}
	}
}
