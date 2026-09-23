package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

func TestHostedRuntimeFundsAdmissionAndSettlementThroughNormalHTTP(t *testing.T) {
	database, _, _, _ := newHostedRatingFixture(t)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	root := t.TempDir()
	_, management, _ := newHostedIdentityHTTPHandler(t, database, upstream.URL, root)
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	management.configuration.DatabasePath = source.File
	priced, conditions := hostedRatingTextCatalog()
	catalog := internalCanonicalProviderCatalog()
	catalog.modelCatalog.Revision = priced.Revision
	for index, offering := range catalog.modelCatalog.Offerings {
		if offering.Provider == "openai" && offering.Model == "gpt-4.1" {
			catalog.modelCatalog.Offerings[index].Limits = priced.Offerings[0].Limits
			catalog.modelCatalog.Offerings[index].OutputTokenLimit = priced.Offerings[0].OutputTokenLimit
		}
	}
	for index, price := range catalog.modelCatalog.Prices {
		if price.Provider == "openai" && price.Model == "gpt-4.1" && price.Operation == ModelOperationText {
			catalog.modelCatalog.Prices[index] = priced.Prices[0]
		}
	}
	for _, schema := range []*ProviderCatalogSchema{&catalog.schema, &catalog.runtimeSchema} {
		for index := range schema.Providers {
			if schema.Providers[index].ID != "openai" {
				continue
			}
			for transport := range schema.Providers[index].Transports {
				if schema.Providers[index].Transports[transport].Endpoint.Protocol == CatalogEndpointProtocolHTTP {
					schema.Providers[index].Transports[transport].Endpoint.DefaultBaseURL = upstream.URL
				}
			}
		}
	}
	configuration := withInternalUpstreamCapacity(t, Configuration{Management: management.configuration, ProviderCatalog: catalog, AssetStorePath: root,
		Hosted: &HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "openai", Model: "gpt-4.1", Operation: ModelOperationText, MaximumAttempts: 1, Conditions: categoricalPriceConditions(conditions)}}}})
	openStore := func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
		return management.store, nil
	}
	excluded := configuration.Hosted.Offerings[0]
	for _, offering := range catalog.modelCatalog.Offerings {
		if offering.Model != "gpt-4.1" && slices.Contains(offering.Operations, ModelOperationText) {
			excluded.Provider, excluded.Model = offering.Provider, offering.Model
			break
		}
	}
	unpriced := configuration.Hosted.Offerings[0]
	unpriced.Conditions.BillingMode = "unqualified"
	for _, scenario := range []struct {
		name   string
		hosted *HostedConfiguration
		status int
	}{
		{"disabled", nil, http.StatusForbidden},
		{"excluded", &HostedConfiguration{Offerings: []HostedOfferingConfiguration{excluded}}, http.StatusForbidden},
		{"unpriced", &HostedConfiguration{Offerings: []HostedOfferingConfiguration{unpriced}}, http.StatusServiceUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			denied := configuration
			denied.Hosted = scenario.hosted
			application, err := buildProxyApplication(denied, zap.NewNop().Sugar(), openStore)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(application.router)
			defer server.Close()
			hostedRuntimeHTTP(t, server.URL, scenario.name, scenario.status)
			if calls.Load() != 0 {
				t.Fatal("unavailable offering dispatched")
			}
		})
	}
	application, err := buildProxyApplication(configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
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
			t.Errorf("normal runtime shutdown: %v", err)
		}
	})
	baseURL := "http://" + listener.Addr().String()
	request := func(key string, status int) string { return hostedRuntimeHTTP(t, baseURL, key, status) }
	request("unfunded", http.StatusPaymentRequired)
	if calls.Load() != 0 {
		t.Fatal("unfunded request dispatched")
	}
	seedHostedFunds(t, database, 5)
	request("funded", http.StatusOK)
	request("funded", http.StatusOK)
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		var count int64
		if err := database.database.Model(&managedFundsSettlementRecord{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("normal runtime did not settle accepted usage")
		case <-tick.C:
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("replay repeated provider work: %d", calls.Load())
	}
	assertHostedFundsBalance(t, database, 5, 5)
	assertFundsCreditRemainder(t, database, "91", "25000")
}

func hostedRuntimeHTTP(t *testing.T, baseURL, key string, status int) string {
	t.Helper()
	client := &http.Client{Transport: hostedIdentityTransport{next: http.DefaultTransport}}
	message, err := http.NewRequest(http.MethodPost, baseURL+"/?provider=openai&model=gpt-4.1&key="+hostedIdentityFixtureKey, strings.NewReader(`{"prompt":"funded prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	message.Header.Set("Content-Type", "application/json")
	message.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	response, err := client.Do(message)
	if err != nil {
		t.Fatal(err)
	}
	body, readError := io.ReadAll(response.Body)
	response.Body.Close()
	if readError != nil {
		t.Fatal(readError)
	}
	if response.StatusCode != status {
		t.Fatalf("normal runtime response=%d want=%d body=%s", response.StatusCode, status, body)
	}
	return string(body)
}
