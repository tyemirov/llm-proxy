package proxy_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

const geminiTranscriptionModel = "gemini-3.5-transcribe"

func TestGeminiTranscriptionPreservesEmptyDictationAtRestart(t *testing.T) {
	schema := testfixtures.ProviderCatalog(t).Schema()
	schema.Models = slices.DeleteFunc(schema.Models, func(model proxy.ProviderCatalogModel) bool { return model.ID == geminiTranscriptionModel })
	for index := range schema.Providers {
		if schema.Providers[index].ID == "gemini" {
			schema.Providers[index].Offerings = slices.DeleteFunc(schema.Providers[index].Offerings, func(offering proxy.ProviderCatalogOffering) bool { return offering.Model == geminiTranscriptionModel })
		}
	}
	previousCatalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	databasePath := t.TempDir() + "/tenant.db"
	previous := newManagementRouterWithDatabasePath(t, proxy.Configuration{ProviderCatalog: previousCatalog}, databasePath)
	cookie := managementSessionCookie(t, "gemini-transcription-restart")
	tenantID := managementDefaultTenantTestID(t, previous, cookie)
	response := putManagementProviderKey(t, previous, cookie, tenantID, "gemini", "test-gemini-key", "gemini-3.5-flash", "", context.Background())
	if response.Code != 200 {
		t.Fatalf("initial provider status=%d", response.Code)
	}
	before := requestProviderKeyVerificationProfile(t, previous, cookie, tenantID)
	if before.Tenant.Defaults.DictationProvider != "" {
		t.Fatal("expected empty initial dictation")
	}
	current := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	after := requestProviderKeyVerificationProfile(t, current, cookie, tenantID)
	if after.Tenant.Defaults != before.Tenant.Defaults {
		t.Fatalf("restart changed defaults: before=%+v after=%+v", before.Tenant.Defaults, after.Tenant.Defaults)
	}
}

func TestGeminiTranscriptionHTTP(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var payload struct {
			Model string `json:"model"`
			Input []struct {
				Type, Data string
				MIMEType   string `json:"mime_type"`
			} `json:"input"`
			Background       bool `json:"background"`
			Store            bool `json:"store"`
			GenerationConfig any  `json:"generation_config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.URL.Path != "/interactions" || r.Header.Get("x-goog-api-key") == "" || r.Header.Get("Api-Revision") != "2026-05-20" || payload.Model != geminiTranscriptionModel || payload.Background || payload.Store || payload.GenerationConfig != nil || len(payload.Input) != 1 {
			t.Errorf("unexpected transcription request: method=%s path=%s payload=%+v", r.Method, r.URL.Path, payload)
		} else {
			audio, err := base64.StdEncoding.DecodeString(payload.Input[0].Data)
			if err != nil || string(audio) != testAudioPayload || payload.Input[0].Type != "audio" || payload.Input[0].MIMEType != "audio/wav" {
				t.Errorf("unexpected audio input")
			}
		}
		writeGeminiInteractionSnapshot(t, w, "private-transcription-id", "completed", "A clear transcript.", nil)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"gemini": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, path := range []string{"/dictate?key=" + TestSecret + "&provider=gemini&model=" + geminiTranscriptionModel, "/dictate?key=" + TestSecret + "&provider=gemini", "/v1/audio/transcriptions"} {
		status, body := requestGeminiTranscription(t, server.URL, path, "speech.wav", []byte(testAudioPayload))
		if status != http.StatusOK || decodeTextResponse(t, body) != "A clear transcript." || strings.Contains(string(body), "private-transcription-id") {
			t.Fatalf("path=%s status=%d body=%s", path, status, body)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
	status, body := requestGeminiTranscription(t, server.URL, "/dictate?key="+TestSecret+"&provider=gemini", "speech.wav", nil)
	if status != 400 || calls.Load() != 3 || !bytes.Contains(body, []byte("invalid_audio_input")) {
		t.Fatalf("empty audio status=%d calls=%d body=%s", status, calls.Load(), body)
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+TestSecret)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	models, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 || !bytes.Contains(models, []byte(`"id":"gemini/`+geminiTranscriptionModel+`"`)) {
		t.Fatalf("model discovery status=%d", response.StatusCode)
	}
}

func TestGeminiTranscriptionManagement(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGeminiInteractionSnapshot(t, w, "private-managed-id", "completed", "Managed transcript.", nil)
	}))
	defer upstream.Close()
	router := newManagementRouter(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"gemini": upstream.URL}, nil)})
	cookie := managementSessionCookie(t, "gemini-transcription")
	tenantID := managementDefaultTenantTestID(t, router, cookie)
	tenantPath := managementDefaultTenantTestPath(t, router, cookie, "")
	connection := putManagementProviderKey(t, router, cookie, tenantID, "gemini", "test-gemini-transcription-key", "gemini-3.5-flash", geminiTranscriptionModel, context.Background())
	if connection.Code != 200 {
		t.Fatalf("connection: %d %s", connection.Code, connection.Body)
	}
	before := requestProviderKeyVerificationProfile(t, router, cookie, tenantID)
	defaults := managementDefaultsRequestBody(t, before.Tenant.Defaults.Provider, before.Tenant.Defaults.Model, "gemini", geminiTranscriptionModel, "")
	request := authenticatedJSONRequest(http.MethodPut, tenantPath+"/defaults", defaults, cookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("defaults: %d %s", response.Code, response.Body)
	}
	profile := requestProviderKeyVerificationProfile(t, router, cookie, tenantID)
	if profile.Tenant.Defaults.Provider != before.Tenant.Defaults.Provider || profile.Tenant.Defaults.Model != before.Tenant.Defaults.Model || profile.Tenant.Defaults.DictationProvider != "gemini" || profile.Tenant.Defaults.DictationModel != geminiTranscriptionModel {
		t.Fatalf("saved defaults=%+v", profile.Tenant.Defaults)
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, cookie)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, secretRequest)
	var secret struct {
		Secret string `json:"secret"`
	}
	if response.Code != 200 {
		t.Fatalf("secret status=%d", response.Code)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &secret); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	status, body := requestGeminiTranscription(t, server.URL, "/dictate?key="+secret.Secret, "speech.wav", []byte(testAudioPayload))
	if status != 200 || decodeTextResponse(t, body) != "Managed transcript." {
		t.Fatalf("managed dictation status=%d body=%s", status, body)
	}
	usage := waitForManagementValue(t, func() managementTenantUsageTestResponse {
		return requestManagementTenantUsage(t, router, cookie, tenantID)
	}, func(value managementTenantUsageTestResponse) bool { return value.Totals.Requests == 1 })
	if usage.Totals.Requests != 1 {
		t.Fatalf("usage requests=%d", usage.Totals.Requests)
	}
	status, _ = requestGeminiTranscription(t, server.URL, "/dictate?key="+secret.Secret, "speech.txt", []byte(testAudioPayload))
	if status != 400 {
		t.Fatalf("unsupported audio status=%d", status)
	}
	rejectedUsage := waitForManagementValue(t, func() managementUsageTestResponse { return requestManagementUsage(t, router, cookie, "30d") }, func(value managementUsageTestResponse) bool {
		return value.Totals.Requests == 2 || value.RejectedRequests == 1
	})
	if rejectedUsage.Totals.Requests != 1 || rejectedUsage.RejectedRequests != 1 {
		t.Fatalf("unsupported audio accounting: accepted=%d rejected=%d", rejectedUsage.Totals.Requests, rejectedUsage.RejectedRequests)
	}
	rejections := requestManagementUsageRejections(t, router, cookie, tenantPath, "30d", 10, "")
	if len(rejections.Rejections) != 1 || rejections.Rejections[0].OutcomeCode != "invalid_request" {
		t.Fatalf("audio rejections=%+v", rejections)
	}
}

func TestGeminiTranscriptionFailures(t *testing.T) {
	for _, endpoint := range []string{"/dictate?key=" + TestSecret + "&provider=gemini", "/v1/audio/transcriptions"} {
		for _, scenario := range []struct {
			name, filename, response    string
			upstreamStatus, want, calls int
		}{
			{"unsupported format", "speech.txt", "", 200, 400, 0},
			{"empty result", "speech.wav", `{"status":"completed","steps":[]}`, 200, 502, 1},
			{"incomplete result", "speech.wav", `{"status":"incomplete","steps":[{"type":"model_output","content":[{"type":"text","text":"private partial transcript"}]}]}`, 200, 502, 1},
			{"failed result", "speech.wav", `{"status":"failed","errors":[]}`, 200, 502, 1},
			{"malformed result", "speech.wav", `{"private`, 200, 502, 1},
			{"pending result", "speech.wav", `{"status":"in_progress","id":"private-id"}`, 200, 502, 1},
			{"media rejection", "speech.wav", `private provider body`, 413, 413, 1},
			{"rate limit", "speech.wav", `private provider body`, 429, 429, 1},
		} {
			t.Run(endpoint+"/"+scenario.name, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.WriteHeader(scenario.upstreamStatus)
					_, _ = io.WriteString(w, scenario.response)
				}))
				defer upstream.Close()
				router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"gemini": upstream.URL}, nil)}, zap.NewNop().Sugar())
				if err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(router)
				defer server.Close()
				status, body := requestGeminiTranscription(t, server.URL, endpoint, scenario.filename, []byte(testAudioPayload))
				if status != scenario.want || int(calls.Load()) != scenario.calls || strings.Contains(string(body), "private") {
					t.Fatalf("status=%d calls=%d body=%s", status, calls.Load(), body)
				}
			})
		}
	}
}

func TestGeminiTranscriptionFilesCleanup(t *testing.T) {
	audio := bytes.Repeat([]byte("a"), 15_000_000)
	digest := sha256.Sum256(audio)
	for _, scenario := range []string{"complete", "provider failure", "cleanup failure", "processing failure", "timeout"} {
		t.Run(scenario, func(t *testing.T) {
			var deletes, interactions atomic.Int32
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-goog-api-key") == "" {
					t.Error("missing credential")
				}
				switch r.URL.Path {
				case "/upload/v1beta/files":
					if r.Header.Get("X-Goog-Upload-Header-Content-Length") != strconv.Itoa(len(audio)) || r.Header.Get("X-Goog-Upload-Header-Content-Type") != "audio/wav" {
						t.Error("invalid upload metadata")
					}
					w.Header().Set("X-Goog-Upload-URL", upstream.URL+"/upload-session")
				case "/upload-session":
					data, err := io.ReadAll(r.Body)
					if err != nil || !bytes.Equal(data, audio) {
						t.Error("upload bytes differ")
					}
					state := "ACTIVE"
					if scenario == "processing failure" {
						state = "FAILED"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"file": map[string]any{"name": "files/private-audio", "uri": upstream.URL + "/files/private-audio", "mimeType": "audio/wav", "sizeBytes": strconv.Itoa(len(audio)), "sha256Hash": base64.StdEncoding.EncodeToString(digest[:]), "state": state}})
				case "/interactions":
					interactions.Add(1)
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					input := body["input"].([]any)[0].(map[string]any)
					if input["uri"] != upstream.URL+"/files/private-audio" || input["data"] != nil || body["background"] != false || body["store"] != false {
						t.Error("invalid file transcription payload")
					}
					if scenario == "timeout" {
						<-r.Context().Done()
						return
					}
					if scenario == "provider failure" {
						w.WriteHeader(400)
						return
					}
					writeGeminiInteractionSnapshot(t, w, "private-id", "completed", "A clear transcript.", nil)
				case "/files/private-audio":
					if r.Method != "DELETE" {
						t.Errorf("method=%s", r.Method)
					}
					deletes.Add(1)
					if scenario == "cleanup failure" {
						w.WriteHeader(400)
					}
				default:
					t.Errorf("unexpected path=%s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			timeout := 30
			if scenario == "timeout" {
				timeout = 1
			}
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{RequestTimeoutSeconds: timeout, MaxInputAudioBytes: 20_000_000, Endpoints: providerEndpointOverrides(map[string]string{"gemini": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			status, body := requestGeminiTranscription(t, server.URL, "/dictate?key="+TestSecret+"&provider=gemini", "speech.wav", audio)
			want := 502
			if scenario == "complete" {
				want = 200
			}
			if scenario == "timeout" {
				want = 504
			}
			if status != want || deletes.Load() != 1 || strings.Contains(string(body), "private") || (want != 200 && strings.Contains(string(body), "A clear transcript")) {
				t.Fatalf("status=%d deletes=%d body=%s", status, deletes.Load(), body)
			}
			if scenario == "processing failure" && interactions.Load() != 0 {
				t.Fatal("failed upload was used")
			}
		})
	}
}

func requestGeminiTranscription(t *testing.T, baseURL, path, filename string, audio []byte) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	field := "audio"
	if path == "/v1/audio/transcriptions" {
		field = "file"
		if err := form.WriteField("model", "gemini/"+geminiTranscriptionModel); err != nil {
			t.Fatal(err)
		}
	}
	part, err := form.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(audio); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+path, &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+TestSecret)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}
