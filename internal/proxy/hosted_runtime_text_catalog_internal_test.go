package proxy

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This matrix derives its scope from enabled catalog offerings. Controlled
// protocol responses and rates qualify local integration, not vendor accounts.
func TestHostedRuntimeTextCatalogFinancialAcceptance(t *testing.T) {
	hostedTextCatalogFinancialAcceptance(t, nil)
}

type hostedTextCatalogAttachment struct {
	Type string `json:"type"`
	MIME string `json:"mime_type"`
	Data string `json:"data"`
}

func TestHostedRuntimeMultimodalCatalogFinancialAcceptance(t *testing.T) {
	firstImage := hostedTextCatalogAttachment{Type: "image", MIME: "image/png", Data: base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 2, 2))))}
	secondImage := hostedTextCatalogAttachment{Type: "image", MIME: "image/png", Data: base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 3, 3))))}
	audio := []byte("RIFF\x00\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00data\x02\x00\x00\x00\x00\x00")
	binary.LittleEndian.PutUint32(audio[4:8], uint32(len(audio)-8))
	voice := hostedTextCatalogAttachment{Type: "audio", MIME: "audio/wav", Data: base64.StdEncoding.EncodeToString(audio)}
	for _, scenario := range []struct {
		name        string
		attachments []hostedTextCatalogAttachment
	}{{"images", []hostedTextCatalogAttachment{firstImage, secondImage}}, {"audio", []hostedTextCatalogAttachment{voice}}, {"ordered-image-audio", []hostedTextCatalogAttachment{firstImage, voice, secondImage}}} {
		t.Run(scenario.name, func(t *testing.T) { hostedTextCatalogFinancialAcceptance(t, scenario.attachments) })
	}
}

func hostedTextCatalogFinancialAcceptance(t *testing.T, attachments []hostedTextCatalogAttachment) {
	t.Helper()
	catalog := internalCanonicalProviderCatalog()
	offerings := []ProviderOffering{}
	grants := map[string][]hostedGrantOffering{}
	selectedOfferings := map[string]bool{}
	for _, offering := range catalog.modelCatalog.Offerings {
		if !slices.Contains(offering.Operations, ModelOperationText) {
			continue
		}
		if slices.ContainsFunc(attachments, func(attachment hostedTextCatalogAttachment) bool {
			return !slices.Contains(offering.MediaInputs, attachment.Type)
		}) {
			continue
		}
		selectedOfferings[offering.Provider+"/"+offering.Model] = true
		offerings = append(offerings, offering)
		grants[offering.Provider] = append(grants[offering.Provider], hostedGrantOffering{Model: offering.Model, Operations: []string{ModelOperationText}})
	}
	if len(offerings) == 0 {
		t.Fatal("text catalog is empty")
	}
	fixtures := hostedTextCatalogProtocolFixtures()
	if len(attachments) > 0 {
		for codec, fixture := range fixtures {
			fixtures[codec] = fixture.withInputMedia(t, codec, attachments)
		}
	}
	authentication := map[string]ProviderCatalogAuthentication{}
	endpoints := &Endpoints{}
	var activeOffering atomic.Pointer[ProviderOffering]
	var mutex sync.Mutex
	calls := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		parts := strings.Split(strings.TrimPrefix(request.URL.Path, "/"), "/")
		if len(parts) < 2 {
			t.Errorf("unexpected provider path %s", request.URL.Path)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		route := parts[0] + "/" + parts[1]
		auth, known := authentication[route]
		fixture, supported := fixtures[parts[1]]
		if !known || !supported {
			t.Errorf("unqualified protocol route %s", route)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Header.Get(auth.Header) != auth.Prefix+"catalog-platform-secret" {
			t.Errorf("platform authority absent for %s", route)
		}
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if request.Method != http.MethodPost {
			t.Errorf("unexpected provider method %s", request.Method)
		}
		selected := activeOffering.Load()
		if selected == nil || selected.Provider != parts[0] {
			t.Error("provider dispatch differs from accepted selection")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		var input struct {
			Model string `json:"model"`
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal(body, &input); err != nil {
			t.Error(err)
		}
		remaining := string(body)
		for _, attachment := range attachments {
			position := strings.Index(remaining, attachment.Data)
			if position < 0 {
				t.Errorf("%s lost ordered %s input", route, attachment.Type)
				break
			}
			remaining = remaining[position+len(attachment.Data):]
		}
		if parts[1] == CatalogProtocolVertexGenerateContent {
			if !strings.Contains(request.URL.Path, selected.ProviderModel) {
				t.Errorf("Vertex path lost selected model %s: %s", selected.ProviderModel, request.URL.Path)
			}
		} else if input.Model != selected.ProviderModel {
			t.Errorf("native model=%s want=%s", input.Model, selected.ProviderModel)
		}
		mutex.Lock()
		calls[parts[0]]++
		mutex.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, fixture.body)
	}))
	t.Cleanup(upstream.Close)
	for _, provider := range catalog.runtimeSchema.Providers {
		if _, selected := grants[provider.ID]; !selected {
			continue
		}
		for _, transport := range provider.Transports {
			if _, supported := fixtures[transport.Components.ResponseCodec.ID]; !supported {
				continue
			}
			route := provider.ID + "/" + transport.Components.ResponseCodec.ID
			authentication[route] = transport.Components.Authentication
			endpoints.SetProviderTransportBaseURL(provider.ID, transport.ID, upstream.URL+"/"+route)
		}
	}
	for index := range catalog.modelCatalog.Offerings {
		offering := &catalog.modelCatalog.Offerings[index]
		if !slices.Contains(offering.Operations, ModelOperationText) {
			continue
		}
		maximum := 1000
		offering.OutputTokenLimit = maximum
		offering.Limits = []CatalogLimit{{ID: "input_tokens", Value: &maximum, Unit: "tokens"}}
	}
	for index := range catalog.modelCatalog.Prices {
		price := &catalog.modelCatalog.Prices[index]
		if price.Operation != ModelOperationText || !selectedOfferings[price.Provider+"/"+price.Model] {
			continue
		}
		var codec string
		for _, offering := range offerings {
			if offering.Provider == price.Provider && offering.Model == price.Model {
				codec = offering.WireContract
				break
			}
		}
		fixture, exists := fixtures[codec]
		if !exists {
			t.Fatalf("missing financial protocol fixture %s/%s codec=%s", price.Provider, price.Model, codec)
		}
		*price = CatalogPriceDescriptor{Provider: price.Provider, Model: price.Model, Operation: price.Operation, Available: true, Source: "https://example.com/controlled-text-prices", LastVerified: "2026-09-23", Rates: fixture.rates()}
	}
	database, _, managementHTTP, _ := newHostedRatingFixture(t)
	root := t.TempDir()
	_, management, _ := newHostedIdentityHTTPHandler(t, database, upstream.URL, root)
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	management.configuration.DatabasePath = source.File
	now := time.Now().UTC()
	for provider, scope := range grants {
		for _, record := range []any{&managedHostedTenantAssignmentRecord{}, &managedProviderProfileRecord{}} {
			if err := database.database.Where("tenant_id = ? AND provider_id = ?", "managed-first", provider).Delete(record).Error; err != nil {
				t.Fatal(err)
			}
		}
		connection, grant := "matrix-platform-"+provider, "matrix-grant-"+provider
		fields := hostedTextCredentialFields(t, management.store.providerKeyCipher, connection, management.store.routingDefaults.definitions[providerID(provider)], "catalog-platform-secret")
		encoded, err := json.Marshal(scope)
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range []any{
			&managedPlatformConnectionRecord{ID: connection, Provider: provider, Name: "Catalog", Version: 1, CreatedAt: now, UpdatedAt: now},
			&managedPlatformCredentialRecord{ConnectionID: connection, Version: 1, Fields: fields, QualifiedAt: now, CreatedAt: now},
			&managedHostedGrantRecord{ID: grant, BillingAccountID: "billing-journal", TenantID: "managed-first", PlatformConnectionID: connection, Provider: provider, CatalogRevision: catalog.modelCatalog.Revision, Offerings: encoded, State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
			&managedHostedGrantRevisionRecord{GrantID: grant, Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Catalog acceptance", CreatedAt: now},
			&managedHostedTenantAssignmentRecord{TenantID: "managed-first", ProviderID: provider, GrantID: grant, CreatedAt: now},
			&managedProviderProfileRecord{TenantID: "managed-first", ProviderID: provider, TextModel: scope[0].Model, CreatedAt: now, UpdatedAt: now},
		} {
			if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	hosted := &HostedConfiguration{}
	for _, offering := range offerings {
		hosted.Offerings = append(hosted.Offerings, HostedOfferingConfiguration{Provider: offering.Provider, Model: offering.Model, Operation: ModelOperationText, MaximumAttempts: 1})
	}
	configuration := withInternalUpstreamCapacity(t, Configuration{Management: management.configuration, ProviderCatalog: catalog, Endpoints: endpoints, AssetStorePath: root, Hosted: hosted})
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
			t.Errorf("catalog runtime shutdown: %v", err)
		}
	})
	client := &http.Client{Transport: hostedIdentityTransport{next: http.DefaultTransport}}
	surfaces := []string{"/", v2Path, chatCompletionsPath, responsesPath}
	if len(attachments) > 0 {
		surfaces = []string{v2Path}
	}
	exchange := func(offering ProviderOffering, surface, key string, status int, media []hostedTextCatalogAttachment) {
		t.Helper()
		activeOffering.Store(&offering)
		query := url.Values{"provider": {offering.Provider}, "model": {offering.Model}}
		path, payload := surface+"?"+query.Encode(), `{"prompt":"funded catalog prompt"}`
		switch surface {
		case v2Path:
			payload = `{"messages":[{"role":"user","content":"funded catalog prompt"}]}`
		case chatCompletionsPath:
			path, payload = surface, fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"funded catalog prompt"}]}`, offering.Provider+"/"+offering.Model)
		case responsesPath:
			path, payload = surface, fmt.Sprintf(`{"model":%q,"input":"funded catalog prompt"}`, offering.Provider+"/"+offering.Model)
		}
		if len(media) > 0 {
			encoded, err := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": "funded catalog prompt", "attachments": media}}})
			if err != nil {
				t.Fatal(err)
			}
			payload = string(encoded)
		}
		request, err := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String()+path, strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		validateHostedIdentityResponse(t, request, response, body)
		for _, attachment := range media {
			if strings.Contains(string(body), attachment.Data) {
				t.Fatal("financial response exposed input media")
			}
		}
		if response.StatusCode != status || (status == http.StatusOK && !strings.Contains(string(body), "catalog answer")) {
			t.Fatalf("%s/%s status=%d want=%d body=%s", offering.Provider, offering.Model, response.StatusCode, status, body)
		}
	}
	for _, offering := range offerings {
		for _, surface := range surfaces {
			exchange(offering, surface, "unfunded-"+offering.Provider+"-"+offering.Model, http.StatusPaymentRequired, attachments)
		}
	}
	mutex.Lock()
	unfundedCalls := len(calls)
	mutex.Unlock()
	if unfundedCalls != 0 {
		t.Fatal("unfunded catalog request dispatched")
	}
	// Fixture funding uses the same write-before-read transaction boundary as
	// payment application, while request telemetry continues asynchronously.
	if err := database.database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&managedBillingAccountRecord{}).Where("id = ?", "billing-journal").UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		seedHostedFunds(t, &gormManagedTenantDatabase{database: tx}, 500)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expectedTotal := new(big.Rat)
	expected := map[string]hostedTextCatalogProtocolFixture{}
	for _, offering := range offerings {
		fixture := fixtures[offering.WireContract]
		for _, surface := range surfaces {
			key := "funded-" + offering.Provider + "-" + offering.Model + "-" + sha256Hex(surface)[:8]
			exchange(offering, surface, key, http.StatusOK, attachments)
			for _, replaySurface := range surfaces {
				exchange(offering, replaySurface, key, http.StatusOK, attachments)
			}
			if len(attachments) > 0 {
				exchange(offering, v2Path, key, http.StatusConflict, nil)
				changed := slices.Clone(attachments)
				data, err := base64.StdEncoding.DecodeString(changed[0].Data)
				if err != nil {
					t.Fatal(err)
				}
				if changed[0].Type == "image" {
					data = imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 4, 4)))
				} else {
					data[len(data)-1] = 1
				}
				changed[0].Data = base64.StdEncoding.EncodeToString(data)
				exchange(offering, v2Path, key, http.StatusConflict, changed)
				if len(attachments) > 1 {
					copy(changed, attachments)
					slices.Reverse(changed)
					exchange(offering, v2Path, key, http.StatusConflict, changed)
				}
			}
			value, ok := new(big.Rat).SetString(fixture.customer)
			if !ok {
				t.Fatal("invalid expected fixture charge")
			}
			expectedTotal.Add(expectedTotal, value)
		}
		expected[offering.Provider+"/"+offering.Model] = fixture
	}
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		var count int64
		if err := database.database.Model(&managedFundsSettlementRecord{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count == int64(len(offerings)*len(surfaces)) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("settled %d/%d catalog requests", count, len(offerings))
		case <-tick.C:
		}
	}
	mutex.Lock()
	for provider, scope := range grants {
		if calls[provider] != len(scope)*len(surfaces) {
			t.Errorf("%s calls=%d want=%d", provider, calls[provider], len(scope)*len(surfaces))
		}
	}
	mutex.Unlock()
	var requests []managedJournalRequestRecord
	if err := database.database.Find(&requests).Error; err != nil {
		t.Fatal(err)
	}
	if len(requests) != len(offerings)*len(surfaces) {
		t.Fatalf("retained %d requests for %d offerings", len(requests), len(offerings))
	}
	chargesByRequest := map[string][]any{}
	cursor := ""
	for {
		path := "/billing-accounts/billing-journal/charges?limit=100"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		page := ratingHTTPExchange(t, managementHTTP, http.MethodGet, path, "", http.StatusOK)
		for _, value := range page["charges"].([]any) {
			charge := value.(map[string]any)
			id := charge["request_id"].(string)
			chargesByRequest[id] = append(chargesByRequest[id], value)
		}
		cursor, _ = page["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	for _, request := range requests {
		fixture := expected[request.Provider+"/"+request.Model]
		charges := chargesByRequest[request.ID]
		if len(charges) != 1 {
			t.Fatalf("%s/%s charges=%v", request.Provider, request.Model, charges)
		}
		charge := charges[0].(map[string]any)
		money := func(value string) map[string]any {
			amount, _ := new(big.Rat).SetString(value)
			return map[string]any{"numerator": amount.Num().String(), "denominator": amount.Denom().String()}
		}
		if charge["state"] != chargeRated || !reflect.DeepEqual(charge["customer_charge"], money(fixture.customer)) || !reflect.DeepEqual(charge["rating"].(map[string]any)["provider_cost"], money(fixture.provider)) {
			t.Fatalf("%s/%s charge=%v", request.Provider, request.Model, charge)
		}
	}
	cents := new(big.Rat).Mul(expectedTotal, big.NewRat(100, 1))
	posted := new(big.Int).Quo(cents.Num(), cents.Denom()).Int64()
	assertHostedFundsBalance(t, database, 500-posted, 500-posted)
	remainder := new(big.Rat).Sub(expectedTotal, big.NewRat(posted, 100))
	assertFundsCreditRemainder(t, database, remainder.Num().String(), remainder.Denom().String())
	t.Logf("verified %d text offerings across %d providers and %d public HTTP surfaces", len(offerings), len(grants), len(surfaces))
}

type hostedTextCatalogProtocolFixture struct {
	body, provider, customer string
	anthropic                bool
}

func (fixture hostedTextCatalogProtocolFixture) rates() []CatalogPriceRate {
	conditions := CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}
	cache := conditions
	cache.CacheClass = "read"
	rates := []CatalogPriceRate{{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: conditions}, {Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens", Conditions: conditions}, {Component: "cache_read", Currency: "USD", Rate: "0.5", Unit: "USD/1M_tokens", Conditions: cache}}
	if fixture.anthropic {
		five, hour := conditions, conditions
		five.CacheClass = "write_5m"
		hour.CacheClass = "write_1h"
		rates = append(rates, CatalogPriceRate{Component: "prompt_cache_write_tokens", Currency: "USD", Rate: "2.5", Unit: "USD/1M_tokens", Conditions: five}, CatalogPriceRate{Component: "prompt_cache_write_tokens", Currency: "USD", Rate: "4", Unit: "USD/1M_tokens", Conditions: hour})
	}
	return rates
}
func hostedTextCatalogProtocolFixtures() map[string]hostedTextCatalogProtocolFixture {
	responses := hostedTextCatalogProtocolFixture{body: `{"id":"catalog-response","status":"completed","output_text":"catalog answer","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"catalog answer"}]}],"usage":{"input_tokens":1000,"output_tokens":100,"total_tokens":1100,"input_tokens_details":{"cached_tokens":200},"output_tokens_details":{"reasoning_tokens":40},"cost_in_usd_ticks":25000000}}`, provider: "1/400", customer: "13/4000"}
	return map[string]hostedTextCatalogProtocolFixture{
		CatalogProtocolOpenAIResponses: responses, CatalogProtocolDashScopeResponses: responses, CatalogProtocolXAIResponses: responses,
		CatalogProtocolOpenAIChatCompletions: {body: `{"id":"catalog-chat","choices":[{"message":{"role":"assistant","content":"catalog answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":100,"total_tokens":1100,"prompt_tokens_details":{"cached_tokens":200},"completion_tokens_details":{"reasoning_tokens":40}}}`, provider: "1/400", customer: "13/4000"},
		CatalogProtocolAnthropicMessages:     {body: `{"id":"catalog-message","type":"message","role":"assistant","content":[{"type":"text","text":"catalog answer"}],"stop_reason":"end_turn","usage":{"input_tokens":800,"output_tokens":100,"cache_read_input_tokens":200,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20}}}`, provider: "521/200000", customer: "6773/2000000", anthropic: true},
		CatalogProtocolGeminiInteractions:    {body: `{"id":"catalog-interaction","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"catalog answer"}]}],"usage":{"total_input_tokens":1000,"total_output_tokens":100,"total_tokens":1140,"total_cached_tokens":200,"total_thought_tokens":40,"total_tool_use_tokens":0,"input_tokens_by_modality":[{"modality":"text","tokens":1000}],"output_tokens_by_modality":[{"modality":"text","tokens":100}],"cached_tokens_by_modality":[{"modality":"text","tokens":200}]}}`, provider: "141/50000", customer: "1833/500000"},
		CatalogProtocolVertexGenerateContent: {body: `{"candidates":[{"content":{"parts":[{"text":"catalog answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":100,"totalTokenCount":1140,"cachedContentTokenCount":200,"thoughtsTokenCount":40,"toolUsePromptTokenCount":0,"promptTokensDetails":[{"modality":"TEXT","tokenCount":1000}],"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":100}],"cacheTokensDetails":[{"modality":"TEXT","tokenCount":200}]}}`, provider: "141/50000", customer: "1833/500000"},
	}
}

func (fixture hostedTextCatalogProtocolFixture) withInputMedia(t *testing.T, codec string, attachments []hostedTextCatalogAttachment) hostedTextCatalogProtocolFixture {
	t.Helper()
	var usageName, inputName, countName string
	modalityName := func(value string) string { return value }
	switch codec {
	case CatalogProtocolGeminiInteractions:
		usageName, inputName, countName = "usage", "input_tokens_by_modality", "tokens"
	case CatalogProtocolVertexGenerateContent:
		usageName, inputName, countName = "usageMetadata", "promptTokensDetails", "tokenCount"
		modalityName = strings.ToUpper
	default:
		return fixture
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(fixture.body), &payload); err != nil {
		t.Fatal(err)
	}
	modalities := []string{}
	for _, attachment := range attachments {
		if !slices.Contains(modalities, attachment.Type) {
			modalities = append(modalities, attachment.Type)
		}
	}
	perModality := 800 / len(modalities)
	input := []any{map[string]any{"modality": modalityName("text"), countName: 1000 - perModality*len(modalities)}}
	for _, modality := range modalities {
		input = append(input, map[string]any{"modality": modalityName(modality), countName: perModality})
	}
	payload[usageName].(map[string]any)[inputName] = input
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	fixture.body = string(encoded)
	return fixture
}
