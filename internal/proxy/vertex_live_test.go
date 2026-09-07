package proxy_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

// TestGeminiCurrentModelsVertexLive qualifies the public proxy with a real runtime identity.
// The normal suite skips paid requests unless the operator selects this lane.
func TestGeminiCurrentModelsVertexLive(t *testing.T) {
	if os.Getenv("LLM_PROXY_LIVE_VERTEX") != "true" {
		t.Skip("explicit Vertex acceptance required")
	}
	credentialFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	audioFile := os.Getenv("LLM_PROXY_LIVE_VERTEX_AUDIO")
	if credentialFile == "" || project == "" || audioFile == "" {
		t.Fatal("GOOGLE_APPLICATION_CREDENTIALS, GOOGLE_CLOUD_PROJECT, and LLM_PROXY_LIVE_VERTEX_AUDIO are required")
	}
	audio, err := os.ReadFile(audioFile)
	if err != nil {
		t.Fatal("read Vertex acceptance audio failed")
	}
	schema := testfixtures.ProviderCatalog(t).Schema()
	models := []string{"gemini-3.5-flash", "gemini-3.1-pro-preview", "gemini-3.8-flash", "gemini-3.5-flash-lite"}
	for i := range schema.Providers {
		if schema.Providers[i].ID == "vertex" {
			schema.Providers[i].Enabled = proxy.ModelEnabled
			for j := range schema.Providers[i].Offerings {
				if schema.Providers[i].Offerings[j].Model == "gemini-3.5-transcribe-preview" {
					schema.Providers[i].Offerings[j].DefaultOperations = []string{proxy.ModelOperationDictation}
				}
			}
		}
	}
	for i := range schema.Models {
		for _, model := range models {
			if schema.Models[i].ID == model || schema.Models[i].ID == "gemini-3.5-transcribe-preview" {
				schema.Models[i].Enabled = proxy.ModelEnabled
			}
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.ProviderKeys["vertex"] = "vertex-acceptance"
	router, err := proxy.BuildRouterWithManagedTenantForTest(t, proxy.Configuration{
		ProviderCatalog: catalog, AssetStorePath: t.TempDir(), RequestTimeoutSeconds: 45,
		GoogleCredentialProfiles: []proxy.GoogleCredentialProfile{{ID: "vertex-acceptance", TenantID: "test", Project: project, Location: "global", CredentialsFile: credentialFile, CredentialType: "service_account"}},
	}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal("build Vertex acceptance proxy failed")
	}
	server := httptest.NewServer(router)
	defer server.Close()
	client := &http.Client{Timeout: 50 * time.Second}
	picture := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			picture.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, picture); err != nil {
		t.Fatal(err)
	}
	for _, model := range models {
		cases := []string{"default", "low", "medium", "high", "schema", "image", "audio"}
		if model == "gemini-3.5-flash" {
			cases = []string{"default", "schema", "image", "audio"}
		}
		if model == "gemini-3.5-flash-lite" {
			cases = append(cases, "minimal")
		}
		for _, scenario := range cases {
			t.Run(model+"/"+scenario, func(t *testing.T) {
				message := map[string]any{"role": "user", "content": "Return only the word OK."}
				payload := map[string]any{"model": model, "max_tokens": 4096, "messages": []any{message}}
				expected := "ok"
				switch scenario {
				case "low", "medium", "high", "minimal":
					payload["reasoning_effort"] = scenario
				case "schema":
					payload["structured_output"] = map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string", "enum": []string{"OK"}}}, "required": []string{"answer"}, "additionalProperties": false}}
				case "image":
					message["content"] = "Name the dominant color in this image. Return only the color."
					message["attachments"] = []any{map[string]string{"type": "image", "mime_type": "image/png", "data": base64.StdEncoding.EncodeToString(imageBytes.Bytes())}}
					expected = "red"
				case "audio":
					message["content"] = "Transcribe this audio. Return only the transcript."
					message["attachments"] = []any{map[string]string{"type": "audio", "mime_type": "audio/wav", "data": base64.StdEncoding.EncodeToString(audio)}}
					expected = "quick brown fox"
				}
				if scenario == "image" || scenario == "audio" {
					payload["structured_output"] = map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]string{"type": "string"}}, "required": []string{"answer"}, "additionalProperties": false}}
				}
				body, _ := json.Marshal(payload)
				request, err := http.NewRequest(http.MethodPost, server.URL+"/v2?key="+TestSecret+"&provider=vertex", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if scenario != "schema" && scenario != "image" && scenario != "audio" {
					query := url.Values{"key": {TestSecret}, "provider": {"vertex"}, "model": {model}, "prompt": {"Return only the word OK."}, "max_tokens": {"4096"}}
					if scenario != "default" {
						query.Set("reasoning_effort", scenario)
					}
					request, err = http.NewRequest(http.MethodGet, server.URL+"/?"+query.Encode(), nil)
					if err != nil {
						t.Fatal(err)
					}
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", "vertex-live-"+model+"-"+scenario)
				started := time.Now()
				response, err := client.Do(request)
				if err != nil {
					t.Fatal("Vertex public request failed")
				}
				result, readErr := io.ReadAll(response.Body)
				response.Body.Close()
				accepted := readErr == nil && response.StatusCode == http.StatusOK && strings.Contains(strings.ToLower(string(result)), expected)
				if scenario == "schema" {
					var answer struct{ Answer string }
					accepted = accepted && json.Unmarshal(result, &answer) == nil && answer.Answer == "OK"
				}
				evidence, _ := json.Marshal(map[string]any{"model": model, "case": scenario, "http_status": response.StatusCode, "accepted": accepted, "total_tokens": response.Header.Get("X-LLM-Proxy-Total-Tokens"), "elapsed_ms": time.Since(started).Milliseconds()})
				t.Logf("VERTEX_EVIDENCE %s", evidence)
				if !accepted {
					t.Errorf("public response=%s", result)
				}
			})
		}
	}
	for _, path := range []string{"/dictate?key=" + TestSecret + "&provider=vertex&model=gemini-3.5-transcribe-preview", "/v1/audio/transcriptions"} {
		t.Run(path[:strings.Index(path+"?", "?")], func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			field := "audio"
			if strings.HasPrefix(path, "/v1/") {
				field = "file"
				writer.WriteField("model", "vertex/gemini-3.5-transcribe-preview")
			}
			part, err := writer.CreateFormFile(field, "speech.wav")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = part.Write(audio); err != nil {
				t.Fatal(err)
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			request, err := http.NewRequest(http.MethodPost, server.URL+path, &body)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", writer.FormDataContentType())
			request.Header.Set("Authorization", "Bearer "+TestSecret)
			started := time.Now()
			response, err := client.Do(request)
			if err != nil {
				t.Fatal("Vertex dictation request failed")
			}
			result, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			accepted := response.StatusCode == 200 && readErr == nil && strings.Contains(strings.ToLower(string(result)), "quick brown fox")
			evidence, _ := json.Marshal(map[string]any{"model": "gemini-3.5-transcribe-preview", "case": strings.Split(path, "?")[0], "http_status": response.StatusCode, "accepted": accepted, "elapsed_ms": time.Since(started).Milliseconds()})
			t.Logf("VERTEX_EVIDENCE %s", evidence)
			if !accepted {
				t.Errorf("public dictation response=%s", result)
			}
		})
	}

}
