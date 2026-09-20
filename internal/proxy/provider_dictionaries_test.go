package proxy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

const dictionaryInputJSON = `{"name":"Names","description":"Narration names","rules":[{"type":"alias","string_to_replace":"MPR","alias":"Marco Polo Research"},{"type":"phoneme","string_to_replace":"tomato","phoneme":"təˈmeɪtoʊ","alphabet":"ipa"}]}`
const dictionaryNativeJSON = `{"id":"native-dictionary","version_id":"native-version","name":"Names","description":"Narration names","created_by":"creator-account","creation_time_unix":1790000000,"version_rules_num":2,"permission_on_resource":"admin","upstream_private":"never public"}`

func TestProviderServicesDictionariesUseOneAccountWithoutModel(t *testing.T) {
	for _, provider := range []string{"elevenlabs", "dictionary-fixture"} {
		t.Run(provider, func(t *testing.T) {
			var posts atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("xi-api-key") != "dictionary-secret" {
					t.Error("lost shared provider credential")
					w.WriteHeader(401)
					return
				}
				if r.Method == http.MethodGet && r.URL.Path == "/v1/user/subscription" {
					_, _ = io.WriteString(w, elevenQuotaFixture)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v1/pronunciation-dictionaries/add-from-rules" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				posts.Add(1)
				var actual, expected map[string]any
				if err := json.NewDecoder(r.Body).Decode(&actual); err != nil {
					t.Error(err)
				}
				_ = json.Unmarshal([]byte(dictionaryInputJSON), &expected)
				a, _ := json.Marshal(actual)
				e, _ := json.Marshal(expected)
				if string(a) != string(e) || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("native dictionary request=%s", a)
				}
				_, _ = io.WriteString(w, dictionaryNativeJSON)
			}))
			defer upstream.Close()
			catalog := elevenResourceCatalog(t, provider, upstream.URL)
			router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), UpstreamCapacity: testfixtures.UpstreamCapacity(4, 100)})
			server := httptest.NewServer(router)
			defer server.Close()
			owner := managementSessionCookie(t, "dictionary-owner")
			tenantID := managementDefaultTenantTestID(t, router, owner)
			secret := generateManagementTenantSecret(t, router, owner, tenantID)
			connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Shared speech account", "provider": provider, "fields": map[string]string{"resource_token": "dictionary-secret"}}, http.StatusCreated)
			accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/"+provider, map[string]string{"connection_id": connection["id"].(string)}, http.StatusOK)
			config, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
			client, err := llmproxyclient.NewClient(config, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			input := llmproxyclient.MediaOperationInput{Capability: "audio.dictionary.create", Provider: provider, Input: json.RawMessage(dictionaryInputJSON), Controls: json.RawMessage(`{}`)}
			operation, err := client.CreateMediaOperation(ctx, "dictionary-creation", input)
			if err != nil {
				t.Fatalf("declared dictionary service must accept the request: %v", err)
			}
			complete, err := client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
			if err != nil || complete.State != "succeeded" || len(complete.Outputs) != 1 {
				t.Fatalf("dictionary result=%+v err=%v", complete, err)
			}
			asset, err := client.GetAsset(ctx, complete.Outputs[0].AssetID)
			if err != nil {
				t.Fatal(err)
			}
			data, err := client.DownloadAsset(ctx, asset)
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if len(result) != 9 || result["provider"] != provider || result["name"] != "Names" || result["version_rules_num"] != float64(2) || result["created_by"] != "creator-account" || result["creation_time_unix"] != float64(1790000000) || result["permission_on_resource"] != "admin" || result["description"] != "Narration names" {
				t.Fatalf("dictionary artifact=%s", data)
			}
			if !strings.HasPrefix(fmt.Sprint(result["dictionary_id"]), "dic_") || !strings.HasPrefix(fmt.Sprint(result["version_id"]), "div_") || strings.Contains(string(data), "native-") || strings.Contains(string(data), "upstream_private") {
				t.Fatalf("private native data exposed: %s", data)
			}
			repeated, err := client.CreateMediaOperation(ctx, "dictionary-creation", input)
			if err != nil || repeated.OperationID != operation.OperationID || posts.Load() != 1 {
				t.Fatalf("dictionary replay=%+v err=%v posts=%d", repeated, err, posts.Load())
			}
			capabilities, err := client.GetMediaCapabilities(ctx)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, service := range capabilities.Services {
				found = found || service.Provider == provider && service.Capability == input.Capability
			}
			if !found {
				t.Fatal("dictionary service absent from shared catalog discovery")
			}
		})
	}
}

func dictionaryOperationInput() llmproxyclient.MediaOperationInput {
	return llmproxyclient.MediaOperationInput{Capability: "audio.dictionary.create", Provider: "elevenlabs", Input: json.RawMessage(dictionaryInputJSON), Controls: json.RawMessage(`{}`)}
}

func TestProviderServicesDictionariesRejectInvalidInputBeforeSubmission(t *testing.T) {
	var posts atomic.Int32
	client, _, _ := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		_, _ = io.WriteString(w, dictionaryNativeJSON)
	})
	for index, body := range []string{
		`{}`, `{"name":" ","rules":[]}`, `{"name":"Names","rules":null}`, `{"name":"Names","rules":[]}`, `{"name":"Names","rules":[{}]}`,
		`{"name":"Names","rules":[{"type":"alias","string_to_replace":"x"}]}`,
		`{"name":"Names","rules":[{"type":"alias","string_to_replace":"x","alias":"y","phoneme":""}]}`,
		`{"name":"Names","rules":[{"type":"phoneme","string_to_replace":"x","phoneme":"y"}]}`,
		`{"name":"Names","rules":[{"type":"phoneme","string_to_replace":"x","phoneme":"y","alphabet":"ipa","alias":""}]}`,
		`{"name":"Names","rules":[{"type":"unknown","string_to_replace":"x"}]}`,
		`{"name":"Names","rules":[{"type":"alias","string_to_replace":" ","alias":"y"}]}`,
		strings.Replace(dictionaryInputJSON, `"name":"Names"`, `"name":"Names","api_key":"forbidden"`, 1),
	} {
		input := dictionaryOperationInput()
		input.Input = json.RawMessage(body)
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-dictionary-%d", index), input); err == nil {
			t.Fatalf("invalid dictionary accepted: %s", body)
		}
	}
	input := dictionaryOperationInput()
	input.Controls = json.RawMessage(`{"unknown":true}`)
	if _, err := client.CreateMediaOperation(t.Context(), "invalid-dictionary-controls", input); err == nil {
		t.Fatal("invalid controls accepted")
	}
	if posts.Load() != 0 {
		t.Fatalf("invalid requests dispatched %d times", posts.Load())
	}
}

func TestProviderServicesDictionariesNativeFailuresNeverResubmit(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
		state  string
	}{
		{"rejected", 401, `{"detail":"native secret"}`, "failed"},
		{"timeout", 408, `{}`, "uncertain"},
		{"server", 503, `{}`, "uncertain"},
		{"malformed", 200, `{`, "uncertain"},
		{"missing ids", 200, `{}`, "uncertain"},
		{"negative time", 200, strings.Replace(dictionaryNativeJSON, `1790000000`, `-1`, 1), "uncertain"},
		{"negative count", 200, strings.Replace(dictionaryNativeJSON, `"version_rules_num":2`, `"version_rules_num":-1`, 1), "uncertain"},
		{"missing time", 200, strings.Replace(dictionaryNativeJSON, `"creation_time_unix":1790000000,`, ``, 1), "uncertain"},
		{"missing count", 200, strings.Replace(dictionaryNativeJSON, `"version_rules_num":2,`, ``, 1), "uncertain"},
		{"oversize", 200, strings.Repeat("x", (1<<20)+1), "uncertain"},
		{"truncated", -1, `{`, "uncertain"},
		{"disconnected", 0, ``, "uncertain"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var posts atomic.Int32
			client, database, restart := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				if scenario.status == 0 {
					connection, _, _ := w.(http.Hijacker).Hijack()
					_ = connection.Close()
					return
				}
				if scenario.status == -1 {
					w.Header().Set("Content-Length", "100")
					_, _ = io.WriteString(w, scenario.body)
					return
				}
				w.WriteHeader(scenario.status)
				_, _ = io.WriteString(w, scenario.body)
			})
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			accepted, err := client.CreateMediaOperation(ctx, "dictionary-failure", dictionaryOperationInput())
			if err != nil {
				t.Fatal(err)
			}
			completed, err := client.WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
			if err != nil || completed.State != scenario.state || len(completed.Outputs) != 0 {
				t.Fatalf("result=%+v err=%v", completed, err)
			}
			if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Updates(map[string]any{"public_state": "running", "provider_execution_state": "dispatched", "terminal_at": nil}).Error; err != nil {
				t.Fatal(err)
			}
			recovered, err := restart().WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
			if err != nil || recovered.State != "uncertain" || posts.Load() != 1 {
				t.Fatalf("recovery=%+v err=%v posts=%d", recovered, err, posts.Load())
			}
		})
	}
}

func TestProviderServicesDictionariesRetainIdentityAfterRecoveryAndOperationExpiry(t *testing.T) {
	var posts atomic.Int32
	client, database, restart := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		_, _ = io.WriteString(w, dictionaryNativeJSON)
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	input := dictionaryOperationInput()
	accepted, err := client.CreateMediaOperation(ctx, "retained-dictionary", input)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := client.WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
	if err != nil || completed.State != "succeeded" {
		t.Fatalf("created=%+v err=%v", completed, err)
	}
	var saved struct{ DictionaryID, VersionID, NativeDictionaryID, NativeVersionID, CredentialReference, TenantID, Provider string }
	if err := database.Table("media_dictionary_records").First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.NativeDictionaryID != "native-dictionary" || saved.NativeVersionID != "native-version" || saved.CredentialReference == "" || saved.TenantID == "" || saved.Provider != "elevenlabs" {
		t.Fatalf("dictionary authority was not retained")
	}
	if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Updates(map[string]any{"public_state": "running", "provider_execution_state": "dispatched", "terminal_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	// Retain the native observation but remove the publication transaction to
	// model a worker lost after acceptance and before terminal persistence.
	if err := database.Exec("DELETE FROM media_operation_asset_reference_records WHERE operation_id = ?", accepted.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("DELETE FROM media_dictionary_records WHERE dictionary_id = ?", saved.DictionaryID).Error; err != nil {
		t.Fatal(err)
	}
	recoveredClient := restart()
	recovered, err := recoveredClient.WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
	if err != nil || recovered.State != "succeeded" || posts.Load() != 1 {
		t.Fatalf("recovered=%+v err=%v posts=%d", recovered, err, posts.Load())
	}
	var count int64
	if err := database.Table("media_dictionary_records").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("dictionary count=%d err=%v", count, err)
	}
	if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Update("terminal_at", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)).Error; err != nil {
		t.Fatal(err)
	}
	expiredClient := restart()
	if _, err := expiredClient.GetMediaOperation(ctx, accepted.OperationID); err == nil {
		t.Fatal("operation did not expire")
	}
	if err := database.Table("media_dictionary_records").Where("dictionary_id = ? AND version_id = ?", saved.DictionaryID, saved.VersionID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("dictionary expired with operation: count=%d err=%v", count, err)
	}
	if _, err := expiredClient.CreateMediaOperation(ctx, "retained-dictionary", input); err == nil || posts.Load() != 1 {
		t.Fatal("expired dictionary intent submitted again")
	}
}

func TestProviderServicesDictionariesPreserveUncertainStorageFailures(t *testing.T) {
	for _, fault := range []string{"handle", "dictionary"} {
		t.Run(fault, func(t *testing.T) {
			var posts atomic.Int32
			client, database, _ := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				_, _ = io.WriteString(w, dictionaryNativeJSON)
			})
			statement := `CREATE TRIGGER reject_dictionary_record BEFORE INSERT ON media_dictionary_records BEGIN SELECT RAISE(FAIL, 'injected dictionary persistence failure'); END`
			if fault == "handle" {
				statement = `CREATE TRIGGER reject_dictionary_handle BEFORE UPDATE OF provider_handle ON media_operation_records WHEN OLD.provider_handle = '' AND NEW.provider_handle != '' BEGIN SELECT RAISE(FAIL, 'injected handle persistence failure'); END`
			}
			if err := database.Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			operation, err := client.CreateMediaOperation(ctx, "dictionary-storage", dictionaryOperationInput())
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
			if err != nil || result.State != "uncertain" || len(result.Outputs) != 0 || posts.Load() != 1 {
				t.Fatalf("storage failure=%+v err=%v posts=%d", result, err, posts.Load())
			}
		})
	}
}

func TestProviderServicesDictionariesRejectChangedBindingWithoutSubmission(t *testing.T) {
	for _, state := range []string{"queued", "running"} {
		t.Run(state, func(t *testing.T) {
			var posts atomic.Int32
			client, database, restart := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				_, _ = io.WriteString(w, dictionaryNativeJSON)
			})
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			operation, err := client.CreateMediaOperation(ctx, "dictionary-binding", dictionaryOperationInput())
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
			if err != nil || result.State != "succeeded" {
				t.Fatalf("initial=%+v err=%v", result, err)
			}
			providerState := "dispatched"
			expected := "uncertain"
			if state == "queued" {
				providerState = "not_dispatched"
				expected = "failed"
			}
			if err := database.Table("media_operation_records").Where("operation_id = ?", operation.OperationID).Updates(map[string]any{"execution_binding": "obsolete", "public_state": state, "provider_execution_state": providerState, "terminal_at": nil}).Error; err != nil {
				t.Fatal(err)
			}
			result, err = restart().WaitMediaOperation(ctx, operation.OperationID, time.Millisecond)
			if err != nil || result.State != expected || posts.Load() != 1 {
				t.Fatalf("binding=%+v err=%v posts=%d", result, err, posts.Load())
			}
		})
	}
}

func TestProviderServicesDictionariesCancellationKeepsNativeCreationSingle(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var posts atomic.Int32
	client, _, _ := providerServicesFixture(t, func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		close(entered)
		<-release
		_, _ = io.WriteString(w, dictionaryNativeJSON)
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, err := client.CreateMediaOperation(ctx, "dictionary-active", dictionaryOperationInput())
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	t.Cleanup(func() {
		close(release)
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result, err := client.WaitMediaOperation(cleanup, first.OperationID, time.Millisecond)
		if err != nil || result.State != "succeeded" {
			t.Errorf("released dictionary=%+v err=%v", result, err)
		}
	})
	result, err := client.CancelMediaOperation(ctx, first.OperationID)
	if err != nil || result.CancellationState != "unsupported" {
		t.Fatalf("running cancellation=%+v err=%v", result, err)
	}
	second, err := client.CreateMediaOperation(ctx, "dictionary-queued", dictionaryOperationInput())
	if err != nil {
		t.Fatal(err)
	}
	result, err = client.CancelMediaOperation(ctx, second.OperationID)
	if err != nil || result.State != "cancelled" || posts.Load() != 1 {
		t.Fatalf("queued cancellation=%+v err=%v posts=%d", result, err, posts.Load())
	}
}
