package proxy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"
)

func TestHostedRuntimeDictionaryFundsAndSettlement(t *testing.T) {
	database, _, managementHTTP, _ := newHostedRatingFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.Header.Get("xi-api-key") != "hosted-service-secret" {
			t.Error("dictionary request lost its platform authority")
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("request-id", "dictionary-priced-call")
		fmt.Fprint(writer, `{"id":"native-dictionary","version_id":"native-version","name":"Names","description":"private dictionary","created_by":"provider-user","creation_time_unix":1,"version_rules_num":1}`)
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	_, management, _ := newHostedIdentityHTTPHandler(t, database, upstream.URL, root)
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	management.configuration.DatabasePath = source.File
	catalog := internalCanonicalProviderCatalog()
	for index := range catalog.modelCatalog.Providers {
		provider := &catalog.modelCatalog.Providers[index]
		if provider.ID != "elevenlabs" {
			continue
		}
		for serviceIndex := range provider.Services {
			service := &provider.Services[serviceIndex]
			if service.Operation == ModelOperationPronunciationDictionaryCreation {
				// This fixture price is not a supplier rate or a production activation.
				service.Price = ProviderCatalogPrice{Operation: service.Operation, Available: true, Source: "https://example.com/controlled-service-rates", LastVerified: "2026-09-23", Rates: []CatalogPriceRate{{Component: "service_calls", Currency: "USD", Rate: "0.5", Unit: "USD/call", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}}}}
			}
		}
	}
	for _, schema := range []*ProviderCatalogSchema{&catalog.schema, &catalog.runtimeSchema} {
		for index := range schema.Providers {
			provider := &schema.Providers[index]
			if provider.ID != "elevenlabs" {
				continue
			}
			for transport := range provider.Transports {
				if provider.Transports[transport].Endpoint.Protocol == CatalogEndpointProtocolHTTP {
					provider.Transports[transport].Endpoint.DefaultBaseURL = upstream.URL
				}
			}
		}
	}
	key, err := management.store.providerKeyCipher.encryptConnection(rand.Reader, platformCredentialReference("platform-service", 1), "elevenlabs", CatalogCredentialAPIKey, "hosted-service-secret")
	if err != nil {
		t.Fatal(err)
	}
	fields, err := json.Marshal(map[string]string{CatalogCredentialAPIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, record := range []any{
		&managedPlatformConnectionRecord{ID: "platform-service", Provider: "elevenlabs", Name: "Service", Version: 1, CreatedAt: now, UpdatedAt: now},
		&managedPlatformCredentialRecord{ConnectionID: "platform-service", Version: 1, Fields: fields, QualifiedAt: now, CreatedAt: now},
		&managedHostedGrantRecord{ID: "grant-service", BillingAccountID: "billing-journal", TenantID: "managed-first", PlatformConnectionID: "platform-service", Provider: "elevenlabs", CatalogRevision: catalog.modelCatalog.Revision, Offerings: []byte(`[{"operations":["pronunciation_dictionary_creation"]}]`), State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
		&managedHostedGrantRevisionRecord{GrantID: "grant-service", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Service acceptance", CreatedAt: now},
		&managedHostedTenantAssignmentRecord{TenantID: "managed-first", ProviderID: "elevenlabs", GrantID: "grant-service", CreatedAt: now},
		&managedProviderProfileRecord{TenantID: "managed-first", ProviderID: "elevenlabs", CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	configuration := withInternalUpstreamCapacity(t, Configuration{Management: management.configuration, ProviderCatalog: catalog, AssetStorePath: root, Hosted: &HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "elevenlabs", Operation: ModelOperationPronunciationDictionaryCreation, MaximumAttempts: 1}}}})
	application, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
		return management.store, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- application.serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("dictionary runtime shutdown: %v", err)
		}
	})
	client := &http.Client{Transport: hostedMCPBearerTransport{token: hostedIdentityFixtureKey}}
	intent := `{"capability":"audio.dictionary.create","provider":"elevenlabs","input":{"name":"Names","description":"private dictionary","rules":[{"type":"alias","string_to_replace":"A","alias":"Alpha"}]},"controls":{}}`
	exchange := func(key string, status int) map[string]any {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String()+llmproxycontract.MediaOperationsPath, strings.NewReader(intent))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != status {
			t.Fatalf("dictionary status=%d want=%d body=%s", response.StatusCode, status, body)
		}
		var result map[string]any
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	exchange("unfunded", http.StatusPaymentRequired)
	if calls.Load() != 0 {
		t.Fatal("unfunded dictionary dispatched")
	}
	seedHostedFunds(t, database, 520)
	for index := 0; index < 8; index++ {
		identity := fmt.Sprintf("dictionary-%d", index)
		admitted := exchange(identity, http.StatusAccepted)
		if replay := exchange(identity, http.StatusOK); replay["operation_id"] != admitted["operation_id"] {
			t.Fatal("dictionary replay changed identity")
		}
		deadline := time.After(5 * time.Second)
		tick := time.NewTicker(20 * time.Millisecond)
		for {
			var count int64
			if err := database.database.Model(&managedFundsSettlementRecord{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count == int64(index+1) {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("dictionary did not settle: calls=%d settlements=%d", calls.Load(), count)
			case <-tick.C:
			}
		}
		tick.Stop()
		assertHostedFundsBalance(t, database, 520-int64(index+1)*65, 520-int64(index+1)*65)
	}
	exchange("exhausted", http.StatusPaymentRequired)
	if calls.Load() != 8 {
		t.Fatalf("dictionary calls=%d want=8", calls.Load())
	}
	charges := ratingHTTPExchange(t, managementHTTP, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
	if len(charges) != 8 {
		t.Fatalf("charges=%v", charges)
	}
	for _, item := range charges {
		charge := item.(map[string]any)
		if charge["state"] != chargeRated || !reflect.DeepEqual(charge["rating"].(map[string]any)["provider_cost"], map[string]any{"numerator": "1", "denominator": "2"}) || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "13", "denominator": "20"}) {
			t.Fatalf("dictionary charge=%v", charge)
		}
	}
	assertFundsCreditRemainder(t, database, "0", "1")
}
