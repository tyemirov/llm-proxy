package proxy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	health "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type dictatorDiagnosticsFixture struct {
	*dictatorGRPCFixture
	dictator.UnimplementedRuntimeServiceServer
	health.UnimplementedHealthServer
	metrics       *dictator.GetMetricsResponse
	metricCalls   atomic.Int32
	healthFailure atomic.Int32
	metricFailure atomic.Int32
	unavailable   atomic.Bool
	stall         atomic.Bool
}

func (fixture *dictatorDiagnosticsFixture) Check(ctx context.Context, _ *health.HealthCheckRequest) (*health.HealthCheckResponse, error) {
	if fixture.stall.Load() {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if code := codes.Code(fixture.healthFailure.Load()); code != codes.OK {
		return nil, status.Error(code, "private server detail")
	}
	state := health.HealthCheckResponse_SERVING
	if fixture.unavailable.Load() {
		state = health.HealthCheckResponse_NOT_SERVING
	}
	return &health.HealthCheckResponse{Status: state}, nil
}

func (fixture *dictatorDiagnosticsFixture) GetMetrics(context.Context, *dictator.GetMetricsRequest) (*dictator.GetMetricsResponse, error) {
	fixture.metricCalls.Add(1)
	if code := codes.Code(fixture.metricFailure.Load()); code != codes.OK {
		return nil, status.Error(code, "private server metrics")
	}
	return fixture.metrics, nil
}

func startDictatorDiagnosticsFixture(t *testing.T, total int64) (*dictatorDiagnosticsFixture, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &dictatorDiagnosticsFixture{dictatorGRPCFixture: &dictatorGRPCFixture{token: "diagnostic-secret"}, metrics: &dictator.GetMetricsResponse{RequestsTotal: total, RequestsSucceeded: 9, RequestsFailed: 2, Inflight: 1, UptimeSeconds: 3600, AverageLatencySeconds: 0.25, MaxLatencySeconds: 1.5}}
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		values, _ := metadata.FromIncomingContext(ctx)
		if strings.Join(values.Get("authorization"), "") != "Bearer "+fixture.token {
			return nil, status.Error(codes.Unauthenticated, "private credential")
		}
		if strings.Contains(info.FullMethod, "/Submit") {
			fixture.submissions.Add(1)
		}
		return handler(ctx, request)
	}))
	dictator.RegisterVoiceServiceServer(server, fixture)
	dictator.RegisterRuntimeServiceServer(server, fixture)
	health.RegisterHealthServer(server, fixture)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return fixture, listener.Addr().String()
}

func TestDictatorDiagnosticsUseAssignedConnection(t *testing.T) {
	fixture, address := startDictatorDiagnosticsFixture(t, 12)
	databasePath := filepath.Join(t.TempDir(), "management.sqlite")
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	server := httptest.NewServer(router)
	defer server.Close()
	owner := managementSessionCookie(t, "diagnostic-owner")
	account := requestManagementAccount(t, router, owner)
	connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Diagnostics", "provider": "dictator", "fields": map[string]string{"grpc_address": address, "grpc_auth_token": fixture.token, "grpc_tls": "false"}}, http.StatusCreated)
	tenantID := account.Tenants[0].ID
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/dictator", map[string]string{"connection_id": connection["id"].(string)}, http.StatusOK)
	secret := generateManagementTenantSecret(t, router, owner, tenantID)
	request, err := http.NewRequest(http.MethodGet, server.URL+"/model/v1/provider-diagnostics/dictator", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("diagnostics status=%d body=%s", response.StatusCode, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result["scope"] != "tenant" || result["provider"] != "dictator" || result["operations_total"] != float64(0) || len(result) != 9 {
		t.Fatalf("diagnostic result=%s", body)
	}
	if response.Header.Get("Cache-Control") != "no-store" || fixture.metricCalls.Load() != 0 || fixture.submissions.Load() != 0 {
		t.Fatalf("diagnostic read changed state or cache contract: %s", body)
	}
	contract, err := openapitest.Load("../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contractPath := "/model/v1/provider-diagnostics/{provider}"
	if err := contract.ValidateResponse(contractPath, http.MethodGet, response.StatusCode, response.Header, body); err != nil {
		t.Fatal(err)
	}
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL + "/v2?key=obsolete", Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := client.GetProviderDiagnostics(context.Background(), "dictator")
	if err != nil || diagnostics.OperationsTotal != 0 || diagnostics.Scope != "tenant" {
		t.Fatalf("official client result=%+v error=%v", diagnostics, err)
	}

	exchange := func(method, path, key string, expected int, timeout string) []byte {
		t.Helper()
		request, err := http.NewRequest(method, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if key != "" {
			request.Header.Set("Authorization", "Bearer "+key)
		}
		if timeout != "" {
			request.Header.Set("X-LLM-Proxy-Request-Timeout-Seconds", timeout)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != expected {
			t.Fatalf("%s %s status=%d body=%s", method, path, response.StatusCode, body)
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("diagnostic response permits caching")
		}
		if strings.Contains(string(body), "private") || strings.Contains(string(body), address) || strings.Contains(string(body), fixture.token) {
			t.Fatalf("private diagnostics exposed: %s", body)
		}
		if method == http.MethodGet {
			if err := contract.ValidateResponse(contractPath, method, response.StatusCode, response.Header, body); err != nil {
				t.Fatal(err)
			}
		}
		if expected == http.StatusMethodNotAllowed && response.Header.Get("Allow") != "GET, HEAD" {
			t.Fatal("missing allowed methods")
		}
		return body
	}
	path := "/model/v1/provider-diagnostics/dictator"
	exchange(http.MethodGet, path, "", 403, "")
	exchange(http.MethodGet, path+"?key=obsolete", secret, 403, "")
	exchange(http.MethodGet, "/model/v1/provider-diagnostics/unknown", secret, 404, "")
	exchange(http.MethodPost, path, secret, 405, "")
	if body := exchange(http.MethodHead, path, secret, 200, ""); len(body) != 0 {
		t.Fatal("HEAD returned a body")
	}
	exchange(http.MethodGet, path, secret, 400, "0")
	fixture.stall.Store(true)
	exchange(http.MethodGet, path, secret, 200, "1")

	otherOwner := managementSessionCookie(t, "diagnostic-other-owner")
	otherAccount := requestManagementAccount(t, router, otherOwner)
	otherSecret := generateManagementTenantSecret(t, router, otherOwner, otherAccount.Tenants[0].ID)
	exchange(http.MethodGet, path, otherSecret, 404, "")
	otherFixture, otherAddress := startDictatorDiagnosticsFixture(t, 64)
	otherConnection := accountConnectionExchange(t, router, otherOwner, http.MethodPost, "/connections", map[string]any{"name": "Other diagnostics", "provider": "dictator", "fields": map[string]string{"grpc_address": otherAddress, "grpc_auth_token": otherFixture.token, "grpc_tls": "false"}}, http.StatusCreated)
	accountConnectionExchange(t, router, otherOwner, http.MethodPut, "/tenants/"+otherAccount.Tenants[0].ID+"/connections/dictator", map[string]string{"connection_id": otherConnection["id"].(string)}, http.StatusOK)
	if body := exchange(http.MethodGet, path, otherSecret, 200, ""); !strings.Contains(string(body), `"operations_total":0`) {
		t.Fatalf("wrong account server: %s", body)
	}
	sharedTenant := accountConnectionExchange(t, router, owner, http.MethodPost, "/tenants", map[string]string{"name": "Shared metrics"}, http.StatusCreated)
	sharedID := sharedTenant["tenant"].(map[string]any)["id"].(string)
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+sharedID+"/connections/dictator", map[string]string{"connection_id": connection["id"].(string)}, http.StatusOK)
	sharedSecret := generateManagementTenantSecret(t, router, owner, sharedID)
	if body := exchange(http.MethodGet, path, sharedSecret, 200, ""); !strings.Contains(string(body), `"operations_total":0`) {
		t.Fatalf("shared tenant received foreign totals: %s", body)
	}
	database, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDatabase.Close()
	// Seed retained states at the persistence boundary. No provider RPC is involved.
	for index, state := range []string{"queued", "running", "succeeded", "failed", "cancelled", "uncertain"} {
		identifier := fmt.Sprintf("retained-%d", index)
		record := map[string]any{"operation_id": identifier, "tenant_id": tenantID, "idempotency_key_digest": identifier,
			"intent_digest": identifier, "capability": "audio.transcribe", "catalog_operation": "audio.transcribe", "provider": "dictator",
			"model": "whisper-base", "catalog_revision": "test", "credential_reference": "test", "normalized_input": []byte(`{}`),
			"normalized_controls": []byte(`{}`), "public_state": state, "provider_execution_state": "not_dispatched",
			"cancellation_state": "not_requested", "accepted_at": time.Now(), "updated_at": time.Now(), "deadline_at": time.Now().Add(time.Hour)}
		if err := database.Transaction(func(transaction *gorm.DB) error {
			if err := transaction.Table("media_operation_records").Create(record).Error; err != nil {
				return err
			}
			return transaction.Exec("INSERT INTO media_operation_claim_records (operation_id, worker_id, generation, expires_at) VALUES (?, ?, ?, ?)", identifier, "fixture", 1, time.Now().Add(time.Hour)).Error
		}); err != nil {
			t.Fatal(err)
		}

	}
	counts, err := client.GetProviderDiagnostics(context.Background(), "dictator")
	if err != nil || counts.OperationsTotal != 6 || counts.Queued != 1 || counts.Running != 1 || counts.Succeeded != 1 || counts.Failed != 1 || counts.Cancelled != 1 || counts.Uncertain != 1 {
		t.Fatalf("tenant counts=%+v err=%v", counts, err)
	}
	if body := exchange(http.MethodGet, path, sharedSecret, 200, ""); !strings.Contains(string(body), `"operations_total":0`) {
		t.Fatalf("shared tenant leaked counts: %s", body)
	}
	if body := exchange(http.MethodGet, path, otherSecret, 200, ""); !strings.Contains(string(body), `"operations_total":0`) {
		t.Fatalf("other account leaked counts: %s", body)
	}
	if err := database.Exec("UPDATE media_operation_records SET provider = ? WHERE operation_id = ?", "xai", "retained-0").Error; err != nil {
		t.Fatal(err)
	}
	if body := exchange(http.MethodGet, path, secret, 200, ""); !strings.Contains(string(body), `"operations_total":5`) {
		t.Fatalf("provider filter failed: %s", body)
	}
	if err := database.Exec("DELETE FROM media_operation_records WHERE operation_id = ?", "retained-2").Error; err != nil {
		t.Fatal(err)
	}
	if body := exchange(http.MethodGet, path, secret, 200, ""); !strings.Contains(string(body), `"operations_total":4`) {
		t.Fatalf("retention filter failed: %s", body)
	}
	accountConnectionExchange(t, router, owner, http.MethodDelete, "/tenants/"+tenantID+"/connections/dictator", nil, http.StatusNoContent)
	before := fixture.metricCalls.Load()
	exchange(http.MethodGet, path, secret, 404, "")
	if fixture.metricCalls.Load() != before {
		t.Fatal("detached tenant reached the provider")
	}
	exchange(http.MethodGet, path, sharedSecret, 200, "")
	replacement := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Replacement diagnostics", "provider": "dictator", "fields": map[string]string{"grpc_address": otherAddress, "grpc_auth_token": otherFixture.token, "grpc_tls": "false"}}, http.StatusCreated)
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/dictator", map[string]string{"connection_id": replacement["id"].(string)}, http.StatusOK)
	if body := exchange(http.MethodGet, path, secret, 200, ""); !strings.Contains(string(body), `"operations_total":4`) {
		t.Fatalf("changed assignment lost tenant history: %s", body)
	}
	if fixture.submissions.Load() != 0 || otherFixture.submissions.Load() != 0 || fixture.metricCalls.Load() != 0 || otherFixture.metricCalls.Load() != 0 {
		t.Fatal("diagnostics submitted speech work")
	}
	if err := database.Exec("DROP TABLE media_operation_records").Error; err != nil {
		t.Fatal(err)
	}
	exchange(http.MethodGet, path, secret, 500, "")
	if err := database.Exec("UPDATE managed_account_connection_records SET version = 0").Error; err != nil {
		t.Fatal(err)
	}
	exchange(http.MethodGet, path, secret, 500, "")

}
