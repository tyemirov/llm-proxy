package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHostedRuntimeMediaFundsAdmissionAndSettlementThroughNormalHTTP(t *testing.T) {
	database, _, _, _ := newHostedRatingFixture(t)
	var calls atomic.Int64
	encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
			t.Error("normal media runtime did not use the platform credential")
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":{"total_tokens":22,"input_tokens":13,"output_tokens":9,"input_tokens_details":{"text_tokens":10,"image_tokens":3},"output_tokens_details":{"text_tokens":2,"image_tokens":7}}}`, encoded)
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
	for index := range catalog.modelCatalog.Offerings {
		offering := &catalog.modelCatalog.Offerings[index]
		if offering.Provider == "openai" && offering.Model == "gpt-image-2" {
			for _, dimension := range []string{"input_text_tokens", "input_image_tokens", "output_text_tokens", "output_image_tokens"} {
				maximum := 100
				offering.Limits = append(offering.Limits, CatalogLimit{ID: dimension, Value: &maximum, Unit: "tokens"})
			}
		}
	}
	for index := range catalog.modelCatalog.Prices {
		price := &catalog.modelCatalog.Prices[index]
		if price.Provider == "openai" && price.Model == "gpt-image-2" && price.Operation == ModelOperationImageGeneration {
			*price = CatalogPriceDescriptor{Provider: price.Provider, Model: price.Model, Operation: price.Operation, Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22"}
			for rate, component := range []string{"input_text", "input_image", "output_text", "output_image"} {
				price.Rates = append(price.Rates, CatalogPriceRate{Component: component, Currency: "USD", Rate: CatalogDecimal(fmt.Sprint(2 << rate)), Unit: "USD/1M_tokens", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z", Quality: "low", Resolution: "1024x1024"}})
			}
		}
	}
	for _, schema := range []*ProviderCatalogSchema{&catalog.schema, &catalog.runtimeSchema} {
		for index := range schema.Providers {
			if schema.Providers[index].ID == "openai" {
				for transport := range schema.Providers[index].Transports {
					if schema.Providers[index].Transports[transport].Endpoint.Protocol == CatalogEndpointProtocolHTTP {
						schema.Providers[index].Transports[transport].Endpoint.DefaultBaseURL = upstream.URL
					}
				}
			}
		}
	}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Updates(map[string]any{
		"catalog_revision": catalog.modelCatalog.Revision, "offerings": []byte(`[{"model":"gpt-image-2","operations":["image_generation"]}]`),
	}).Error; err != nil {
		t.Fatal(err)
	}
	configuration := withInternalUpstreamCapacity(t, Configuration{Management: management.configuration, ProviderCatalog: catalog, AssetStorePath: root,
		Hosted: &HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "openai", Model: "gpt-image-2", Operation: ModelOperationImageGeneration, MaximumAttempts: 1, Conditions: CatalogPriceConditions{Quality: "low", Resolution: "1024x1024"}}}}})
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
			t.Errorf("normal media runtime shutdown: %v", err)
		}
	})
	baseURL := "http://" + listener.Addr().String()
	client := &http.Client{Transport: hostedMCPBearerTransport{token: hostedIdentityFixtureKey}}
	admit := func(status int) map[string]any {
		return hostedMediaAdmissionClientHTTP(t, client, baseURL, "normal-media", "funded image", status)
	}
	admit(http.StatusPaymentRequired)
	if calls.Load() != 0 {
		t.Fatal("unfunded media dispatched")
	}
	seedHostedFunds(t, database, 5)
	hostedMediaAdmissionClientHTTP(t, client, baseURL, "unpriced-quality", "funded image", http.StatusServiceUnavailable, json.RawMessage(`{"surface":"images","quality":"high","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}`))
	if calls.Load() != 0 {
		t.Fatal("media with unmatched price conditions dispatched")
	}
	accepted := admit(http.StatusAccepted)
	replay := admit(http.StatusOK)
	if accepted["operation_id"] != replay["operation_id"] {
		t.Fatal("media replay changed operation identity")
	}
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
			t.Fatalf("normal media runtime did not settle: calls=%d", calls.Load())
		case <-tick.C:
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("media dispatched %d times", calls.Load())
	}
	assertHostedFundsBalance(t, database, 5, 5)
	assertFundsCreditRemainder(t, database, "13", "62500")
}
