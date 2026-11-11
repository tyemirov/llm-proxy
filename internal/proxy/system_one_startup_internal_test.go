package proxy

import (
	"bytes"
	"context"
	"errors"
	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.uber.org/zap"
)

func TestSystemOneStartupContractFailures(t *testing.T) {
	originalDocument := systemOneSchemaDocument
	originalFactory := systemOneSchemas
	originalAdd := addSystemOneSchemaResource
	defer func() {
		systemOneSchemaDocument = originalDocument
		systemOneSchemas = originalFactory
		addSystemOneSchemaResource = originalAdd
	}()
	for _, scenario := range []string{"decode", "resource", "compile"} {
		t.Run(scenario, func(t *testing.T) {
			systemOneSchemaDocument = originalDocument
			addSystemOneSchemaResource = originalAdd
			switch scenario {
			case "decode":
				systemOneSchemaDocument = []byte("invalid")
			case "resource":
				addSystemOneSchemaResource = func(*jsonschema.Compiler, string, any) error { return errors.New("injected schema resource failure") }
			case "compile":
				systemOneSchemaDocument = []byte(`{"components":{"schemas":{}}}`)
			}
			systemOneSchemas = sync.OnceValues(compileSystemOneSchemas)
			_, err := BuildRouterWithManagedTenantForTest(t, Configuration{}, zap.NewNop().Sugar(), StandardManagedTenantTestConfiguration("tenant"))
			if err == nil || !strings.Contains(err.Error(), "System One contract") {
				t.Fatalf("startup error=%v", err)
			}
		})
	}
}

type systemOneFaultDoer func(*http.Request) (*http.Response, error)

func (doer systemOneFaultDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type systemOneFaultReader struct{}

func (systemOneFaultReader) Read([]byte) (int, error) { return 0, errors.New("injected body failure") }
func (systemOneFaultReader) Close() error             { return nil }

func TestSystemOneTransportBoundaryFailures(t *testing.T) {
	originalClient := HTTPClient
	originalRequestFactory := newSystemOneUpstreamRequest
	defer func() { HTTPClient = originalClient; newSystemOneUpstreamRequest = originalRequestFactory }()
	for _, scenario := range []string{"request-create", "transport", "overload", "read", "ended", "bad-error-json", "bad-error-shape", "missing-validation-detail", "redact-keys"} {
		t.Run(scenario, func(t *testing.T) {
			newSystemOneUpstreamRequest = originalRequestFactory
			if scenario == "request-create" {
				newSystemOneUpstreamRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) {
					return nil, errors.New("injected request creation failure")
				}
			}
			HTTPClient = systemOneFaultDoer(func(request *http.Request) (*http.Response, error) {
				switch scenario {
				case "transport":
					return nil, errors.New(systemOneBadRequest("injected transport failure").Error())
				case "overload":
					return nil, errQueueFull
				case "read":
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: systemOneFaultReader{}}, nil
				case "ended":
					<-request.Context().Done()
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
				}
				status := 400
				body := `{"detail":{"provider-secret":"provider-secret"}}`
				switch scenario {
				case "bad-error-json":
					body = "{"
				case "bad-error-shape":
					body = "[]"
				case "missing-validation-detail":
					status = 422
					body = "{}"
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			tenant := StandardManagedTenantTestConfiguration("tenant")
			tenant.ProviderKeys[ProviderNameTypeSafe] = "provider-secret"
			router, err := BuildRouterWithManagedTenantForTest(t, Configuration{RequestTimeoutSeconds: 1}, zap.NewNop().Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			request, _ := http.NewRequest("POST", server.URL+llmproxycontract.SystemOnePath, strings.NewReader(`{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul"}}}`))
			request.Header.Set("Authorization", "Bearer tenant")
			request.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			want := 502
			if scenario == "overload" {
				want = 503
			}
			if scenario == "ended" {
				want = 504
			}
			if scenario == "redact-keys" {
				want = 400
			}
			if response.StatusCode != want || bytes.Contains(body, []byte("provider-secret")) {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
		})
	}
}

type systemOneBlockedBody struct {
	closed chan struct{}
	once   sync.Once
}

func (body *systemOneBlockedBody) Read([]byte) (int, error) {
	<-body.closed
	return 0, errors.New("closed request body")
}
func (body *systemOneBlockedBody) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}
func TestSystemOneRequestReadDeadline(t *testing.T) {
	router, err := BuildRouterWithManagedTenantForTest(t, Configuration{RequestTimeoutSeconds: 1}, zap.NewNop().Sugar(), StandardManagedTenantTestConfiguration("tenant"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		request.Body = &systemOneBlockedBody{closed: make(chan struct{})}
		router.ServeHTTP(writer, request)
	}))
	defer server.Close()
	request, _ := http.NewRequest("POST", server.URL+llmproxycontract.SystemOnePath, strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer tenant")
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 504 {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestSystemOnePersistedUsageRouteValidation(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(t.TempDir()+"/usage.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = database.AutoMigrate(&managedUsageEventRecord{}); err != nil {
		t.Fatal(err)
	}
	records := []managedUsageEventRecord{{ID: 1, Endpoint: usageEndpointSystemOne, ProviderID: ProviderNameTypeSafe, ModelID: ModelNameJev1140, StatusCode: 200, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess}, {ID: 2, Endpoint: usageEndpointSystemOne, ProviderID: ProviderNameTypeSafe, ModelID: ModelNameJev1140, StatusCode: 200, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess}}
	if err = database.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	providers := newProviderRegistry(Configuration{ProviderCatalog: internalCanonicalProviderCatalog()})
	if err = validateManagedResolvedUsageRoutes(database, providers); err != nil {
		t.Fatal(err)
	}
	if err = clearInvalidManagedUsageRoutes(database, providers); err != nil {
		t.Fatal(err)
	}
	var actual []managedUsageEventRecord
	if err = database.Order("id").Find(&actual).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range actual {
		if record.ProviderID != ProviderNameTypeSafe || record.ModelID != ModelNameJev1140 {
			t.Fatalf("typed decision usage identity lost: %+v", record)
		}
	}
}
