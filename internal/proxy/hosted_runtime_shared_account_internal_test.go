package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm/clause"
)

const sharedFundsTenantKey = "shared-funds-second-tenant-key"

func seedHostedSharedFundsTenant(t *testing.T, database *gormManagedTenantDatabase) {
	t.Helper()
	now := ratingTestAcceptanceTime()
	tenant := fakeTenantRecord("owner", "shared-funds-second", "Second funded tenant", now)
	digest := sha256Hex(sharedFundsTenantKey)
	tenant.SecretDigest = &digest
	const grantID = "grant-44444444444444444444444444444444"
	for _, record := range []any{
		&tenant,
		&managedHostedGrantRecord{ID: grantID, BillingAccountID: "billing-journal", TenantID: tenant.TenantID, PlatformConnectionID: "platform-journal", Provider: "openai", CatalogRevision: "journal-catalog", Offerings: []byte(`[{"model":"gpt-4.1","operations":["text"]}]`), State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
		&managedHostedGrantRevisionRecord{GrantID: grantID, Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Shared funds acceptance", CreatedAt: now},
		&managedHostedTenantAssignmentRecord{TenantID: tenant.TenantID, ProviderID: "openai", GrantID: grantID, CreatedAt: now},
		&managedProviderProfileRecord{TenantID: tenant.TenantID, ProviderID: "openai", TextModel: "gpt-4.1", CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestHostedRuntimeConcurrentTenantsConserveSharedFunds(t *testing.T) {
	for _, funds := range []int64{5, 10} {
		t.Run(fmt.Sprintf("%d-cents", funds), func(t *testing.T) {
			fixture := newHostedRuntimeTextFixture(t)
			seedHostedSharedFundsTenant(t, fixture.database)
			seedHostedFunds(t, fixture.database, funds)
			configuration := fixture.configuration
			configuration.Hosted.Offerings[0].MaximumAttempts = 2
			entered := make(chan struct{}, 3)
			released := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(released) }) }
			defer release()
			var forward http.Handler
			provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodDelete {
					select {
					case entered <- struct{}{}:
					case <-request.Context().Done():
						return
					}
					select {
					case <-released:
					case <-request.Context().Done():
						return
					}
				}
				forward.ServeHTTP(writer, request)
			}))
			t.Cleanup(provider.Close)
			for _, schema := range []*ProviderCatalogSchema{&configuration.ProviderCatalog.schema, &configuration.ProviderCatalog.runtimeSchema} {
				for index := range schema.Providers {
					if schema.Providers[index].ID != "openai" {
						continue
					}
					for route := range schema.Providers[index].Transports {
						endpoint := &schema.Providers[index].Transports[route].Endpoint
						if endpoint.Protocol == CatalogEndpointProtocolHTTP {
							if forward == nil {
								target, err := url.Parse(endpoint.DefaultBaseURL)
								if err != nil {
									t.Fatal(err)
								}
								forward = httputil.NewSingleHostReverseProxy(target)
							}
							endpoint.DefaultBaseURL = provider.URL
						}
					}
				}
			}
			configuration.UpstreamCapacity.Origins = nil
			configuration = withInternalUpstreamCapacity(t, configuration)
			baseURL, _, stop := startHostedAlignmentRuntime(t, configuration)
			client := &http.Client{Timeout: 5 * time.Second}
			secrets := []string{hostedIdentityFixtureKey, sharedFundsTenantKey}
			type response struct {
				tenant int
				status int
				body   string
				err    error
			}
			exchange := func(tenant int, key string) response {
				request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/?provider=openai&model=gpt-4.1&key="+secrets[tenant], strings.NewReader(`{"prompt":"funded prompt"}`))
				if err != nil {
					return response{tenant: tenant, err: err}
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
				result, err := client.Do(request)
				if err != nil {
					return response{tenant: tenant, err: err}
				}
				defer result.Body.Close()
				body, err := io.ReadAll(result.Body)
				return response{tenant, result.StatusCode, string(body), err}
			}
			assertSuccess := func(result response) {
				t.Helper()
				if result.err != nil || result.status != http.StatusOK || result.body != "funded result" {
					t.Fatalf("tenant response=%+v", result)
				}
			}
			results := make(chan response, 2)
			start := make(chan struct{})
			for tenant := range secrets {
				go func() {
					<-start
					results <- exchange(tenant, "shared-idempotency-key")
				}()
			}
			close(start)
			accepted := 2
			rejected := -1
			if funds == 5 {
				accepted = 1
				select {
				case result := <-results:
					if result.err != nil || result.status != http.StatusPaymentRequired || !strings.Contains(result.body, `"code":"insufficient_funds"`) {
						t.Fatalf("concurrent spending rejection=%+v", result)
					}
					rejected = result.tenant
				case <-time.After(5 * time.Second):
					t.Fatal("shared account did not reject excess concurrent spending")
				}
			}
			for range accepted {
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("accepted tenant did not reach the provider")
				}
			}
			assertHostedFundsBalance(t, fixture.database, funds, funds-int64(3*accepted))
			release()
			for range accepted {
				select {
				case result := <-results:
					assertSuccess(result)
				case <-time.After(5 * time.Second):
					t.Fatal("accepted tenant did not receive its result")
				}
			}
			waitSettlements := func(want int64) {
				t.Helper()
				deadline := time.After(5 * time.Second)
				tick := time.NewTicker(10 * time.Millisecond)
				defer tick.Stop()
				for {
					var count int64
					if err := fixture.database.database.Model(&managedFundsSettlementRecord{}).Count(&count).Error; err != nil {
						t.Fatal(err)
					}
					if count == want {
						return
					}
					select {
					case <-deadline:
						t.Fatalf("settlements=%d want=%d", count, want)
					case <-tick.C:
					}
				}
			}
			waitSettlements(int64(accepted))
			if rejected >= 0 {
				assertSuccess(exchange(rejected, "shared-idempotency-key"))
				waitSettlements(2)
			}
			for tenant := range secrets {
				assertSuccess(exchange(tenant, "shared-idempotency-key"))
			}
			assertSuccess(exchange(0, "third-funded-request"))
			waitSettlements(3)
			assertHostedFundsBalance(t, fixture.database, funds-1, funds-1)
			assertFundsCreditRemainder(t, fixture.database, "23", "25000")
			var requests []managedJournalRequestRecord
			if err := fixture.database.database.Find(&requests).Error; err != nil || len(requests) != 3 {
				t.Fatalf("tenant requests=%v error=%v", requests, err)
			}
			tenants := map[string]int{}
			for _, request := range requests {
				tenants[request.TenantID]++
				if request.BillingAccountID != "billing-journal" {
					t.Fatalf("request used another account: %+v", request)
				}
			}
			if !reflect.DeepEqual(tenants, map[string]int{"managed-first": 2, "shared-funds-second": 1}) || fixture.calls.Load() != 3 {
				t.Fatalf("tenant idempotency or dispatch mismatch: tenants=%v calls=%d", tenants, fixture.calls.Load())
			}
			before := fixture.state(t)
			for range 2 {
				stop()
				baseURL, _, stop = startHostedAlignmentRuntime(t, configuration)
				for tenant := range secrets {
					assertSuccess(exchange(tenant, "shared-idempotency-key"))
				}
				assertSuccess(exchange(0, "third-funded-request"))
				if fixture.calls.Load() != 3 || !reflect.DeepEqual(before, fixture.state(t)) {
					t.Fatal("restart changed shared funds, charges, or provider calls")
				}
			}
		})
	}
}
