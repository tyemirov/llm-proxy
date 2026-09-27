package proxy_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestCatalogPriceGeminiPublishedDiscountExpires(t *testing.T) {
	catalog := currentGeminiCandidateCatalog(t)
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy.BuildPublicCapabilityRouter(public, "error"))
	t.Cleanup(server.Close)
	response, err := server.Client().Get(server.URL + proxy.PublicCapabilitiesPath)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var decoded proxy.PublicCapabilityCatalog
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("catalog status=%d error=%v", response.StatusCode, err)
	}
	service, err := proxy.NewCatalogService(catalog.ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	boundary := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, model := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash"} {
		found := false
		for _, price := range decoded.Prices {
			if price.Provider != "gemini" || price.Model != model || price.Operation != "text" {
				continue
			}
			found = true
			for _, rate := range price.Rates {
				if rate.Conditions.EffectiveUntil != boundary.Format(time.RFC3339) {
					t.Fatalf("%s/%s retains an unbounded discount: %+v", model, rate.Component, rate.Conditions)
				}
				conditions := rate.Conditions
				conditions.EffectiveFrom, conditions.EffectiveUntil = "", ""
				before := service.SelectApplicablePrice("gemini", model, "text", rate.Component, conditions, 1, boundary.Add(-time.Nanosecond))
				after := service.SelectApplicablePrice("gemini", model, "text", rate.Component, conditions, 1, boundary)
				if !before.Available || before.Rate.Rate != rate.Rate || after.Available {
					t.Fatalf("%s/%s discount boundary: before=%+v after=%+v", model, rate.Component, before, after)
				}
			}
		}
		if !found {
			t.Fatalf("missing price for %s", model)
		}
	}
}

func TestCatalogRatingTypedTokenTiersAndEffectiveIntervals(t *testing.T) {
	base := proxy.CatalogPriceConditions{ServiceTier: "standard", Region: "us-central1", EffectiveFrom: "2026-09-01T00:00:00Z", EffectiveUntil: "2026-10-01T00:00:00Z"}
	low := base
	low.InputTokens = proxy.CatalogTokenRange{MaximumExclusive: 200000}
	high := base
	high.InputTokens = proxy.CatalogTokenRange{Minimum: 200000}
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "1", Unit: "USD/1M_tokens", Conditions: low},
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: high},
	}, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	request := proxy.CatalogPriceConditions{ServiceTier: "standard", Region: "us-central1"}
	for _, test := range []struct {
		tokens uint64
		at     string
		want   proxy.CatalogDecimal
	}{
		{199999, "2026-09-01T00:00:00Z", "1"},
		{200000, "2026-09-30T23:59:59Z", "2"},
		{0, "2026-08-31T23:59:59Z", ""},
		{200000, "2026-10-01T00:00:00Z", ""},
	} {
		at, err := time.Parse(time.RFC3339, test.at)
		if err != nil {
			t.Fatal(err)
		}
		selection := service.SelectApplicablePrice(provider, model, operation, "input_tokens", request, test.tokens, at)
		if test.want == "" {
			if selection.Available {
				t.Fatalf("inactive price selected: %+v", selection)
			}
			continue
		}
		if !selection.Available || selection.Rate.Rate != test.want {
			t.Fatalf("tier %d selected=%+v", test.tokens, selection)
		}
	}
	catalog.Prices[0].Rates[1].Conditions.InputTokens.Minimum = 199999
	if _, err := proxy.NewCatalogService(catalog); !errors.Is(err, proxy.ErrInvalidModelCatalog) {
		t.Fatalf("overlapping token tiers accepted: %v", err)
	}
	catalog.Prices[0].Rates[1].Conditions.InputTokens.Minimum = 200000
	catalog.Prices[0].Rates[0].Conditions.EffectiveUntil = "2026-09-01T00:00:00Z"
	if _, err := proxy.NewCatalogService(catalog); !errors.Is(err, proxy.ErrInvalidModelCatalog) {
		t.Fatalf("empty effective interval accepted: %v", err)
	}
}

func TestCatalogRatingUnresolvedPublishedBoundaryIsNotPaidEligible(t *testing.T) {
	conditions := proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{UnresolvedReason: "Published boundary uses 512k without an exact token count."}}
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{{Component: "input_tokens", Currency: "USD", Rate: "1", Unit: "USD/1M_tokens", Conditions: conditions}}, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "input_tokens", Dimension: "input_tokens"}}); !errors.Is(err, proxy.ErrCatalogRatingUnavailable) {
		t.Fatalf("unresolved boundary became paid eligible: %v", err)
	}
}
