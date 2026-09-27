package proxy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestUpstreamAdmissionSharesAccountCapacityAcrossTenants(t *testing.T) {
	started := make(chan string, 5)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	var active, maximum atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		prompt := payload.Messages[len(payload.Messages)-1].Content
		if prompt != "c0" {
			count := active.Add(1)
			for before := maximum.Load(); count > before; before = maximum.Load() {
				if maximum.CompareAndSwap(before, count) {
					break
				}
			}
			defer active.Add(-1)
		}
		started <- prompt
		if prompt != "c0" {
			select {
			case <-release:
			case <-request.Context().Done():
				return
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"completed"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	defer releaseAll()
	capacity := testfixtures.UpstreamCapacity(2, 8)
	capacity.Account.Active = 1
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{UpstreamCapacity: capacity, Endpoints: providerEndpoints(upstream.URL, proxy.ProviderNameDeepSeek)}, filepath.Join(t.TempDir(), "management.sqlite"))
	observedCore, logs := observer.New(zap.InfoLevel)
	previousHTTP := proxy.HTTPClient
	proxy.HTTPClient = managementProviderKeyVerificationHTTPDoer{next: previousHTTP}
	router, err := buildRouterWithCatalogs(t, configuration, zap.New(observedCore).Sugar())
	proxy.HTTPClient = previousHTTP
	if err != nil {
		t.Fatal(err)
	}
	owner := managementSessionCookie(t, "admission-account-owner")
	tenantA := requestManagementAccount(t, router, owner).Tenants[0].ID
	tenantB := createManagementTenant(t, router, owner, "Second").Tenant.ID
	tenantC := createManagementTenant(t, router, owner, "Independent").Tenant.ID
	connection := func(name string) string {
		t.Helper()
		result := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": name, "provider": "deepseek", "fields": map[string]string{"api_key": "test-" + name}}, http.StatusCreated)
		return result["id"].(string)
	}
	shared, independent := connection("shared"), connection("independent")
	secrets := map[string]string{}
	for tenant, id := range map[string]string{tenantA: shared, tenantB: shared, tenantC: independent} {
		accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenant+"/connections/deepseek", map[string]string{"kind": "account_connection", "resource_id": id}, http.StatusOK)
		secrets[tenant] = generateManagementTenantSecret(t, router, owner, tenant)
	}
	for _, event := range logs.FilterMessage("upstream HTTP admission").All() {
		if id, ok := event.ContextMap()["request_id"].(string); !ok || id == "" {
			t.Fatalf("management admission has no request correlation: %v", event.ContextMap())
		}
	}
	current := accountConnectionExchange(t, router, owner, http.MethodGet, "/connections/"+shared, nil, http.StatusOK)
	verificationBody, err := json.Marshal(map[string]any{"name": "shared", "provider": "deepseek", "version": current["version"], "fields": map[string]string{"api_key": "updated-shared"}})
	if err != nil {
		t.Fatal(err)
	}
	logs.TakeAll()
	server := httptest.NewServer(router)
	defer server.Close()
	defer releaseAll()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan error, 6)
	send := func(tenant, prompt string) {
		go func() {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/?key="+url.QueryEscape(secrets[tenant])+"&provider=deepseek&prompt="+prompt, nil)
			if err != nil {
				results <- err
				return
			}
			response, err := server.Client().Do(request)
			if err != nil {
				results <- err
				return
			}
			_, err = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if err == nil && response.StatusCode != http.StatusOK {
				err = fmt.Errorf("%s status=%d", prompt, response.StatusCode)
			}
			results <- err
		}()
	}
	receiveStart := func() string {
		t.Helper()
		select {
		case prompt := <-started:
			return prompt
		case <-time.After(time.Second):
			t.Fatal("admissible request did not start")
			return ""
		}
	}
	waitQueued := func(count int) {
		t.Helper()
		deadline := time.NewTimer(time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			if logs.FilterMessage("upstream HTTP admission").FilterField(zap.String("decision", "capacity_wait")).Len() >= count {
				return
			}
			select {
			case <-ticker.C:
			case <-deadline.C:
				t.Fatal("request did not enter bounded account queue")
			}
		}
	}
	send(tenantA, "a0")
	if got := receiveStart(); got != "a0" {
		t.Fatalf("first=%s", got)
	}
	send(tenantA, "a1")
	waitQueued(1)
	send(tenantA, "a2")
	waitQueued(2)
	send(tenantB, "b1")
	waitQueued(3)
	send(tenantC, "c0")
	if got := receiveStart(); got != "c0" {
		t.Fatalf("shared account exceeded active capacity: %s", got)
	}
	select {
	case err := <-results:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("independent account did not finish")
	}
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, authenticatedJSONRequest(http.MethodPut, "/api/management/connections/"+shared, string(verificationBody), owner))
		var err error
		if response.Code != http.StatusOK {
			err = fmt.Errorf("connection verification status=%d body=%s", response.Code, response.Body.String())
		}
		results <- err
	}()
	waitQueued(4)
	order := []string{}
	for range 3 {
		release <- struct{}{}
		order = append(order, receiveStart())
	}
	releaseAll()
	for range 5 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("queued request did not finish")
		}
	}
	if order[0] != "b1" && order[1] != "b1" {
		t.Fatalf("tenant B did not receive a fair turn: %v", order)
	}
	if maximum.Load() != 1 {
		t.Fatalf("shared account active=%d want=1", maximum.Load())
	}
	waitForManagementValue(t, func() managementTenantUsageTestResponse {
		return requestManagementAccountUsage(t, router, owner)
	}, func(usage managementTenantUsageTestResponse) bool {
		return usage.Totals.Requests == 5
	})
}
