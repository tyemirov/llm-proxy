package proxy

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm/clause"
)

func TestHostedTextUsagePreservesCacheLifetimes(t *testing.T) {
	for _, scenario := range []struct {
		name, creation, details, five, hour string
		reason                              journalUnknownReason
	}{
		{"exact", "9007199254740994", `{"ephemeral_5m_input_tokens":9007199254740993,"ephemeral_1h_input_tokens":1}`, "9007199254740993", "1", ""},
		{"zero", "0", `{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}`, "0", "0", ""},
		{"missing", "12", `{}`, "", "", journalQuantityNotReported},
		{"partial", "12", `{"ephemeral_5m_input_tokens":10}`, "10", "", journalQuantityNotReported},
		{"sum_mismatch", "12", `{"ephemeral_5m_input_tokens":8,"ephemeral_1h_input_tokens":3}`, "", "", journalQuantityInvalid},
		{"child_exceeds_parent", "12", `{"ephemeral_5m_input_tokens":13,"ephemeral_1h_input_tokens":0}`, "", "", journalQuantityInvalid},
		{"invalid", "12", `{"ephemeral_5m_input_tokens":"private value","ephemeral_1h_input_tokens":-1}`, "", "", journalQuantityInvalid},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Method != http.MethodPost || request.Header.Get("x-api-key") != "hosted-text-secret" {
					t.Error("incorrect text authority")
				}
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(writer, `{"id":"private-message","type":"message","role":"assistant","content":[{"type":"text","text":"retained answer"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":50,"cache_creation_input_tokens":%s,"cache_creation":%s}}`, scenario.creation, scenario.details)
			}))
			t.Cleanup(upstream.Close)
			server := newHostedTextProviderFixture(t, database, upstream.URL, "anthropic", "claude-sonnet-4-6")
			for range 2 {
				request, err := http.NewRequest(http.MethodPost, server.URL+"/?provider=anthropic&model=claude-sonnet-4-6", strings.NewReader(`{"prompt":"private prompt"}`))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "cache-lifetimes")
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != http.StatusOK || string(body) != "retained answer" {
					t.Fatalf("text status=%d body=%s error=%v", response.StatusCode, body, err)
				}
				validateHostedIdentityResponse(t, request, response, body)
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil || len(pending) != 1 || calls.Load() != 1 {
				t.Fatalf("cache evidence=%v calls=%d error=%v", pending, calls.Load(), err)
			}
			var quantities []journalQuantity
			if err := json.Unmarshal(pending[0].Quantities, &quantities); err != nil {
				t.Fatal(err)
			}
			values := map[string]journalQuantity{}
			for _, quantity := range quantities {
				values[quantity.Dimension] = quantity
			}
			for dimension, expected := range map[string]string{"cache_write_5m_tokens": scenario.five, "cache_write_1h_tokens": scenario.hour} {
				quantity := values[dimension]
				if quantity.Value != expected || quantity.Unit != "token" || quantity.IncludedIn != "cache_write_tokens" || (expected == "" && quantity.UnknownReason != scenario.reason) {
					t.Fatalf("cache quantity=%+v expected=%q reason=%s", quantity, expected, scenario.reason)
				}
			}
			if values["cache_write_tokens"].IncludedIn != "" || values["cache_read_tokens"].IncludedIn != "" {
				t.Fatal("Anthropic cache is additional to ordinary input")
			}
			usage := journalUsageComplete
			if scenario.reason != "" {
				usage = journalUsageUnknown
			}
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if entry["usage_state"] != string(usage) {
				t.Fatalf("cache completeness=%v", entry)
			}
			if strings.Contains(string(pending[0].SourceFields), "private") {
				t.Fatal("retained private content")
			}
			if scenario.name == "exact" && !strings.Contains(string(pending[0].SourceFields), `"value":"9007199254740993"`) {
				t.Fatal("lost exact cache source")
			}
			if scenario.name == "sum_mismatch" && !strings.Contains(string(pending[0].SourceFields), `"value":"8"`) {
				t.Fatal("lost contradictory numeric source")
			}
		})
	}
}

func newHostedTextProviderFixture(t *testing.T, database *gormManagedTenantDatabase, endpoint, provider, model string) *httptest.Server {
	t.Helper()
	router, service, _ := newHostedIdentityHTTPHandler(t, database, endpoint, t.TempDir())
	for _, record := range []any{&managedHostedTenantAssignmentRecord{}, &managedProviderProfileRecord{}} {
		if err := database.database.Where("tenant_id = ? AND provider_id = ?", "managed-first", provider).Delete(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	definition := service.store.routingDefaults.definitions[providerID(provider)]
	for identifier, transport := range definition.transports {
		transport.endpointURLOverride = endpoint
		definition.transports[identifier] = transport
	}
	service.store.routingDefaults.definitions[providerID(provider)] = definition
	secret, err := service.store.providerKeyCipher.encryptConnection(rand.Reader, platformCredentialReference("platform-text", 1), provider, CatalogCredentialAPIKey, "hosted-text-secret")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := json.Marshal(map[string]string{CatalogCredentialAPIKey: secret})
	offerings, _ := json.Marshal([]hostedGrantOffering{{Model: model, Operations: []string{ModelOperationText}}})
	now := time.Now().UTC()
	for _, record := range []any{
		&managedPlatformConnectionRecord{ID: "platform-text", Provider: provider, Name: "Text", Version: 1, CreatedAt: now, UpdatedAt: now},
		&managedPlatformCredentialRecord{ConnectionID: "platform-text", Version: 1, Fields: fields, QualifiedAt: now, CreatedAt: now},
		&managedHostedGrantRecord{ID: "grant-text", BillingAccountID: "billing-journal", TenantID: "managed-first", PlatformConnectionID: "platform-text", Provider: provider, CatalogRevision: "journal-catalog", Offerings: offerings, State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
		&managedHostedGrantRevisionRecord{GrantID: "grant-text", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Text acceptance", CreatedAt: now},
		&managedHostedTenantAssignmentRecord{TenantID: "managed-first", ProviderID: provider, GrantID: "grant-text", CreatedAt: now},
		&managedProviderProfileRecord{TenantID: "managed-first", ProviderID: provider, TextModel: model, CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(router)
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	t.Cleanup(server.Close)
	return server
}
