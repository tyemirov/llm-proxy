package proxy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

var qualifiedGeminiModels = []string{"gemini-3.6-flash", "gemini-3.7-flash"}

func TestGeminiQualifiedModelsHTTP(t *testing.T) {
	testGeminiModelsHTTP(t, testfixtures.ProviderCatalog(t), qualifiedGeminiModels)
}

func TestGeminiQualifiedModelsMediaAndStructuredOutput(t *testing.T) {
	testGeminiModelsMediaAndStructuredOutput(t, testfixtures.ProviderCatalog(t), qualifiedGeminiModels)
}

func TestGeminiQualifiedModelsIncomplete(t *testing.T) {
	for _, model := range qualifiedGeminiModels {
		t.Run(model, func(t *testing.T) {
			var methods []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.Method == http.MethodDelete {
					writeGeminiInteractionDeleted(t, w)
					return
				}
				writeGeminiInteractionSnapshot(t, w, "qualified-incomplete", "incomplete", "partial answer", nil)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"gemini": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", "/?key="+TestSecret+"&provider=gemini&model="+model+"&prompt=test", nil))
			if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "partial answer") || !reflect.DeepEqual(methods, []string{"POST", "DELETE"}) {
				t.Fatalf("status=%d lifecycle=%v body=%s", response.Code, methods, response.Body)
			}
		})
	}
}

func TestGeminiQualifiedModelsManagement(t *testing.T) {
	for _, model := range qualifiedGeminiModels {
		t.Run(model, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPost:
					payload := decodeGeminiInteractionRequest(t, r)
					if payload["model"] != "gemini-3.5-flash" {
						t.Errorf("model=%v", payload["model"])
					}
					writeGeminiInteractionSnapshot(t, w, "qualified-key", "in_progress", "", nil)
				case http.MethodGet:
					writeGeminiInteractionSnapshot(t, w, "qualified-key", "completed", "verified", nil)
				case http.MethodDelete:
					writeGeminiInteractionDeleted(t, w)
				}
			}))
			defer upstream.Close()
			router := newOperationalProviderKeyVerificationRouter(t, providerKeyVerificationConfiguration(upstream.URL), zap.NewNop().Sugar(), t.TempDir()+"/managed.db", TestTimeout)
			cookie := managementSessionCookie(t, model)
			tenantID := managementDefaultTenantTestID(t, router, cookie)
			response := putManagementProviderKey(t, router, cookie, tenantID, "gemini", "qualified-test-key", model, "", context.Background())
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			profile := requestProviderKeyVerificationProfile(t, router, cookie, tenantID)
			provider := verificationProfileProvider(t, profile, "gemini")
			if !provider.Configured || provider.TextModel != model || profile.Tenant.Defaults.Provider != "" || profile.Tenant.Defaults.Model != "" {
				t.Fatalf("defaults=%+v provider=%+v", profile.Tenant.Defaults, provider)
			}
		})
	}
}

func TestGeminiQualifiedModelsSavedReasoning(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	cookie := managementSessionCookie(t, "qualified-gemini-reasoning")
	tenantPath := managementDefaultTenantTestPath(t, router, cookie, "")
	response := putManagementProviderKey(t, router, cookie, strings.TrimPrefix(tenantPath, "/api/management/tenants/"), "gemini", "qualified-test-key", qualifiedGeminiModels[0], "", context.Background())
	if response.Code != http.StatusOK {
		t.Fatalf("save connection: %d %s", response.Code, response.Body)
	}
	for _, model := range qualifiedGeminiModels {
		for _, effort := range append([]string{""}, currentGeminiEfforts(model)...) {
			request := authenticatedJSONRequest(http.MethodPut, tenantPath+"/defaults", managementDefaultsRequestBodyWithReasoningEffort(t, "gemini", model, "", "", "", effort), cookie)
			response = httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("model=%s effort=%s status=%d body=%s", model, effort, response.Code, response.Body)
			}
			read := httptest.NewRequest(http.MethodGet, tenantPath, nil)
			read.AddCookie(cookie)
			response = httptest.NewRecorder()
			router.ServeHTTP(response, read)
			var profile struct {
				Tenant struct {
					Defaults struct {
						Model  string `json:"model"`
						Effort string `json:"reasoning_effort"`
					} `json:"defaults"`
				} `json:"tenant"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || profile.Tenant.Defaults.Model != model || profile.Tenant.Defaults.Effort != effort {
				t.Fatalf("saved profile=%s", response.Body)
			}
		}
	}
}
