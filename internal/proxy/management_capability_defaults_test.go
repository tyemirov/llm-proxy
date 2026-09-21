package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestManagementCapabilityDefaultsRejectMediaOnlyDictation(t *testing.T) {
	fixture, listener := newDictatorAcceptanceUpstream(t, proxy.ProviderNameDictator)
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, "file:dictation-capability-"+t.Name()+"?mode=memory&cache=shared")
	owner := managementSessionCookie(t, "dictation-capability-owner")
	tenantID := managementDefaultTenantTestID(t, router, owner)
	connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{
		"name": "Speech", "provider": proxy.ProviderNameDictator,
		"fields": map[string]string{"grpc_address": listener.Addr().String(), "grpc_auth_token": fixture.token, "grpc_tls": "false"},
	}, http.StatusCreated)
	tenantPath := "/tenants/" + tenantID
	accountConnectionExchange(t, router, owner, http.MethodPut, tenantPath+"/connections/dictator", map[string]any{"connection_id": connection["id"]}, http.StatusOK)
	accountConnectionExchange(t, router, owner, http.MethodPut, tenantPath+"/defaults", map[string]string{
		"transcription_provider": proxy.ProviderNameDictator, "transcription_model": proxy.ModelNameDictatorWhisperBase, "reasoning_effort": "",
	}, http.StatusOK)
	secret := generateManagementTenantSecret(t, router, owner, tenantID)
	server := httptest.NewServer(router)
	defer server.Close()
	for _, scenario := range []struct{ name, path, field, model, message string }{
		{"default", "/dictate", "audio", "", "unsupported provider endpoint"},
		{"explicit", "/dictate?provider=dictator&model=whisper-base", "audio", "", "unsupported provider endpoint"},
		{"client protocol", "/v1/audio/transcriptions", "file", "dictator/whisper-base", "Invalid or unavailable transcription route."},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			if scenario.model != "" {
				if err := form.WriteField("model", scenario.model); err != nil {
					t.Fatal(err)
				}
			}
			file, err := form.CreateFormFile(scenario.field, "audio.wav")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(file, "fixture audio"); err != nil {
				t.Fatal(err)
			}
			if err := form.Close(); err != nil {
				t.Fatal(err)
			}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+scenario.path, &body)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+secret)
			if scenario.field == "audio" {
				query := request.URL.Query()
				query.Set("key", secret)
				request.URL.RawQuery = query.Encode()
			}
			request.Header.Set("Content-Type", form.FormDataContentType())
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			payload, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(payload), scenario.message) {
				t.Fatalf("dictation route: status=%d body=%s", response.StatusCode, payload)
			}
		})
	}
	if fixture.submissions.Load() != 0 {
		t.Fatalf("unsupported dictation sent %d gRPC jobs", fixture.submissions.Load())
	}
}

func TestManagementCapabilityDefaultsSurviveRestartAndConnectionRemoval(t *testing.T) {
	for _, removedProvider := range []string{proxy.ProviderNameOpenAI, proxy.ProviderNameDictator} {
		t.Run(removedProvider, func(t *testing.T) {
			fixture, listener := newDictatorAcceptanceUpstream(t, proxy.ProviderNameDictator)
			databasePath := filepath.Join(t.TempDir(), "management.sqlite")
			router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			owner := managementSessionCookie(t, "capability-default-owner")
			tenantID := managementDefaultTenantTestID(t, router, owner)
			tenantPath := "/api/management/tenants/" + tenantID
			keyResponse := putManagementProviderKey(t, router, owner, tenantID, proxy.ProviderNameOpenAI, testManagementOpenAIKey, proxy.ModelNameGPT41, "", context.Background())
			if keyResponse.Code != http.StatusOK {
				t.Fatalf("save text connection: status=%d body=%s", keyResponse.Code, keyResponse.Body.String())
			}
			server := httptest.NewServer(router)
			defer server.Close()
			exchange := func(method, path string, body any, idempotencyKey string, expectedStatus int) map[string]any {
				t.Helper()
				payload, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				request := authenticatedJSONRequest(method, server.URL+path, string(payload), owner)
				request.RequestURI = ""
				if idempotencyKey != "" {
					request.Header.Set("Idempotency-Key", idempotencyKey)
				}
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				responseBytes, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != expectedStatus {
					t.Fatalf("%s %s: status=%d want=%d response=%s", method, path, response.StatusCode, expectedStatus, responseBytes)
				}
				var value map[string]any
				if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated {
					if err := json.Unmarshal(responseBytes, &value); err != nil {
						t.Fatalf("decode %s %s status=%d: %v", method, path, response.StatusCode, err)
					}
				}
				return value
			}
			connection := exchange(http.MethodPost, "/api/management/connections", map[string]any{
				"name": "Speech", "provider": proxy.ProviderNameDictator,
				"fields": map[string]string{"grpc_address": listener.Addr().String(), "grpc_auth_token": fixture.token, "grpc_tls": "false"},
			}, "capability-default-connection", http.StatusCreated)
			exchange(http.MethodPut, tenantPath+"/connections/dictator", map[string]any{"connection_id": connection["id"]}, "", http.StatusOK)
			exchange(http.MethodPut, tenantPath+"/defaults", map[string]string{"speech_provider": "dictator", "speech_model": "whisper-base", "reasoning_effort": ""}, "", http.StatusBadRequest)
			defaults := map[string]string{
				"provider": proxy.ProviderNameOpenAI, "model": proxy.ModelNameGPT41,
				"transcription_provider": proxy.ProviderNameDictator, "transcription_model": "whisper-base",
				"speech_provider": proxy.ProviderNameDictator, "speech_model": "silero-ru",
				"system_prompt": "Keep the project prompt.", "reasoning_effort": "",
			}
			exchange(http.MethodPut, tenantPath+"/defaults", defaults, "", http.StatusOK)
			assertDefaults := func() {
				t.Helper()
				profile := exchange(http.MethodGet, tenantPath, nil, "", http.StatusOK)
				for _, item := range profile["providers"].([]any) {
					provider := item.(map[string]any)
					if provider["id"] == proxy.ProviderNameDictator {
						models, err := json.Marshal(provider["speech_models"])
						if err != nil || string(models) != `["qwen3-tts","silero-ru"]` {
							t.Fatalf("speech default model list=%s error=%v", models, err)
						}
					}
				}
				actual := profile["tenant"].(map[string]any)["defaults"].(map[string]any)
				for field, expected := range defaults {
					if actual[field] != expected {
						t.Fatalf("default %s=%v want=%s", field, actual[field], expected)
					}
				}
				for _, obsolete := range []string{"dictation_provider", "dictation_model"} {
					if _, exists := actual[obsolete]; exists {
						t.Fatalf("obsolete default field %s", obsolete)
					}
				}
			}
			assertDefaults()
			defaults["model"] = proxy.ModelNameGPT4oMini
			exchange(http.MethodPut, tenantPath+"/defaults", defaults, "", http.StatusOK)
			assertDefaults()
			server.Close()
			router = newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			server = httptest.NewServer(router)
			defer server.Close()
			assertDefaults()
			exchange(http.MethodPut, tenantPath+"/defaults", map[string]string{"dictation_provider": "openai"}, "", http.StatusBadRequest)
			exchange(http.MethodDelete, tenantPath+"/connections/"+removedProvider, nil, "", http.StatusConflict)
			exchange(http.MethodDelete, tenantPath+"/connections/"+removedProvider+"?clear_defaults=true", nil, "", http.StatusNoContent)
			if removedProvider == proxy.ProviderNameOpenAI {
				defaults["provider"], defaults["model"] = "", ""
			} else {
				defaults["transcription_provider"], defaults["transcription_model"] = "", ""
				defaults["speech_provider"], defaults["speech_model"] = "", ""
			}
			assertDefaults()
		})
	}
}

func TestManagementCapabilityDefaultsRejectObsoleteDatabaseColumns(t *testing.T) {
	for _, change := range []struct {
		name string
		sql  string
	}{
		{"obsolete defaults", "ALTER TABLE managed_tenant_records ADD COLUMN default_dictation_provider TEXT"},
		{"missing transcription", "ALTER TABLE managed_tenant_records DROP COLUMN default_transcription_provider"},
		{"missing speech", "ALTER TABLE managed_tenant_records DROP COLUMN default_speech_model"},
	} {
		t.Run(change.name, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "management.sqlite")
			newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			database, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := database.Exec(change.sql).Error; err != nil {
				t.Fatal(err)
			}
			configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath)
			if _, err := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar()); err == nil || !strings.Contains(err.Error(), "default") {
				t.Fatalf("startup must reject obsolete or incomplete defaults: %v", err)
			}
		})
	}
}

func TestManagementCapabilityDefaultsRejectInvalidSpeechPairs(t *testing.T) {
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, filepath.Join(t.TempDir(), "management.sqlite"))
	owner := managementSessionCookie(t, "invalid-speech-default-owner")
	tenantID := managementDefaultTenantTestID(t, router, owner)
	for _, pair := range []struct{ provider, model string }{
		{"dictator", ""}, {"", "silero-ru"}, {"unknown-provider", "silero-ru"},
		{"openai", proxy.ModelNameGPT41}, {"dictator", "unknown-model"},
		{"dictator", "whisper-base"},
	} {
		t.Run(pair.provider+"/"+pair.model, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{"speech_provider": pair.provider, "speech_model": pair.model, "reasoning_effort": ""})
			if err != nil {
				t.Fatal(err)
			}
			request := authenticatedJSONRequest(http.MethodPut, "/api/management/tenants/"+tenantID+"/defaults", string(payload), owner)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "managed_routing_defaults_invalid") {
				t.Fatalf("invalid speech pair: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestManagementStartupRejectsInvalidMediaDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "management.sqlite")
	newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	database, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"DROP TABLE media_operation_records", "CREATE VIEW media_operation_records AS SELECT 1 AS invalid_schema"} {
		if err := database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath)
	if _, err := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar()); err == nil || !strings.Contains(err.Error(), "media_operation_store") {
		t.Fatalf("invalid media database startup error=%v", err)
	}
}
