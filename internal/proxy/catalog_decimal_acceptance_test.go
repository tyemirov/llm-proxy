package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"gopkg.in/yaml.v3"
)

func TestProviderCatalogExactAmountsSurvivePublicHTTP(t *testing.T) {
	document, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	rate := regexp.MustCompile(`(?m)^([ \t]+rate: )[^\n]+`)
	location := rate.FindSubmatchIndex(document)
	if location == nil {
		t.Fatal("catalog has no rate")
	}
	for _, amount := range []string{"9007199254740993.123456789012345678", "0.000000000000000001", "0"} {
		t.Run(amount, func(t *testing.T) {
			candidate := string(document[:location[0]]) + string(document[location[2]:location[3]]) + "'" + amount + "'" + string(document[location[1]:])
			catalog, err := proxy.ParseProviderCatalog([]byte(candidate))
			if err != nil {
				t.Fatal(err)
			}
			minimum, err := proxy.NewCatalogDecimal(amount)
			if err != nil {
				t.Fatal(err)
			}
			schema := catalog.Schema()
			schema.Providers[0].Offerings[0].Prices[0].MinimumCharge = &proxy.CatalogMinimumCharge{Currency: "USD", Amount: minimum, Unit: "USD/request"}
			encoded, err := yaml.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			catalog, err = proxy.ParseProviderCatalog(encoded)
			if err != nil {
				t.Fatal(err)
			}
			public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: catalog})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(proxy.BuildPublicCapabilityRouter(public, "error"))
			defer server.Close()
			response, err := server.Client().Get(server.URL + proxy.PublicCapabilitiesPath)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != http.StatusOK {
				t.Fatalf("status=%d error=%v", response.StatusCode, err)
			}
			if !strings.Contains(string(body), `"rate":"`+amount+`"`) {
				t.Fatalf("exact amount %q missing from public catalog", amount)
			}
			if !strings.Contains(string(body), `"amount":"`+amount+`"`) {
				t.Fatalf("exact minimum %q missing from public catalog", amount)
			}
		})
	}
}

func TestProviderCatalogRejectsInvalidMonetaryAmounts(t *testing.T) {
	document, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	rate := regexp.MustCompile(`(?m)^([ \t]+rate: )[^\n]+`)
	location := rate.FindSubmatchIndex(document)
	for _, input := range []string{"1.25", "null", "true", "[]", "'-1'", "'01'", "'1.0'", "'1e3'", "'.1'", "'NaN'", "''", "'" + strings.Repeat("9", 129) + "'"} {
		t.Run(input, func(t *testing.T) {
			candidate := string(document[:location[0]]) + string(document[location[2]:location[3]]) + input + string(document[location[1]:])
			if _, err := proxy.ParseProviderCatalog([]byte(candidate)); err == nil {
				t.Fatalf("invalid monetary input accepted: %s", input)
			}
		})
	}
	for _, input := range []string{"1.25", "null", "true", "[]", `"-1"`, `"01"`, `"1.0"`, `"1e3"`, `".1"`, `"NaN"`, `""`} {
		var rate proxy.CatalogPriceRate
		if err := json.Unmarshal([]byte(`{"rate":`+input+`}`), &rate); err == nil {
			t.Fatalf("invalid JSON rate accepted: %s", input)
		}
	}
}
