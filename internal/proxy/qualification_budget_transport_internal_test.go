package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type qualificationBoundaryReadFailure struct{}

func (qualificationBoundaryReadFailure) Read([]byte) (int, error) {
	return 0, errors.New("controlled_wire_read_failure")
}

type qualificationBoundaryHarness struct {
	fixture  *qualificationFaultFixture
	doer     *qualificationBudgetHTTPDoer
	request  *http.Request
	logical  *qualificationExecution
	identity qualificationTransportIdentity
	calls    atomic.Int64
}

func newQualificationBoundaryHarness(t *testing.T) *qualificationBoundaryHarness {
	t.Helper()
	fixture := newQualificationFaultFixture(t)
	fixture.initialize(t)
	harness := &qualificationBoundaryHarness{fixture: fixture}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		harness.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
	}))
	t.Cleanup(server.Close)
	harness.doer = &qualificationBudgetHTTPDoer{next: server.Client(), qualified: server.Client(), budget: fixture.budget}
	ctx := qualificationLogicalContext(requestContextWithIdentifier(context.Background(), "origin-fixture"))
	harness.logical = ctx.Value(qualificationExecutionKey{}).(*qualificationExecution)
	harness.identity = qualificationTransportIdentity{provider: "cerebras", scope: upstreamRequestScope{tenant: fixture.policy.TenantID, account: "fixture-connection", class: upstreamInteractive}, transport: providerTransportDefinition{responseCodec: CatalogProtocolOpenAIChatCompletions, lifecycle: textExecutionLifecycleSynchronousCompletion, protocolParameters: providerProtocolParameters{TokenField: CatalogProtocolVariationMaxCompletionTokens}}}
	ctx = context.WithValue(ctx, qualificationTransportKey{}, harness.identity)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/chat/completions", strings.NewReader(`{"model":"qwen-3.8-27b","max_completion_tokens":128,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	harness.request = request
	return harness
}

func TestQualificationBudgetBoundaryDispatchAdmission(t *testing.T) {
	for _, scenario := range []string{"missing-transport-attribution", "foreign-unscoped", "missing-connection", "missing-logical", "read-only-without-logical", "missing-origin", "delete", "metadata-get", "unmatched-observation", "unsupported-method", "unresolved-prior"} {
		t.Run(scenario, func(t *testing.T) {
			harness := newQualificationBoundaryHarness(t)
			request := harness.request
			identity := harness.identity
			ctx := request.Context()
			expectedCalls := int64(0)
			switch scenario {
			case "missing-transport-attribution":
				ctx = requestContextWithUpstreamScope(context.Background(), identity.scope)
			case "foreign-unscoped":
				ctx = context.Background()
				expectedCalls = 1
			case "missing-connection":
				identity.scope.account = ""
				ctx = context.WithValue(ctx, qualificationTransportKey{}, identity)
			case "missing-logical":
				ctx = context.WithValue(context.Background(), qualificationTransportKey{}, identity)
			case "read-only-without-logical":
				identity.transport.lifecycle = textExecutionLifecycle(CatalogExecutionReadOnly)
				ctx = context.WithValue(context.Background(), qualificationTransportKey{}, identity)
				request.Method = http.MethodGet
				expectedCalls = 1
			case "missing-origin":
				ctx = context.WithValue(ctx, qualificationExecutionKey{}, &qualificationExecution{id: "missing-origin"})
			case "delete":
				request.Method = http.MethodDelete
				expectedCalls = 1
			case "metadata-get":
				request.Method = http.MethodGet
				expectedCalls = 1
			case "unmatched-observation":
				request.Method = http.MethodGet
				harness.logical.active = &qualificationBudgetAttempt{ResourceID: "different-resource"}
			case "unsupported-method":
				request.Method = http.MethodPatch
			case "unresolved-prior":
				harness.logical.active = &qualificationBudgetAttempt{ID: 99}
			}
			response, err := harness.doer.Do(request.WithContext(ctx))
			if response != nil {
				_ = response.Body.Close()
			}
			if expectedCalls == 0 && !errors.Is(err, errQualificationBudget) {
				t.Fatalf("admission not terminal: %v", err)
			}
			if expectedCalls == 1 && err != nil {
				t.Fatal(err)
			}
			if harness.calls.Load() != expectedCalls {
				t.Fatalf("actual network calls=%d expected=%d", harness.calls.Load(), expectedCalls)
			}
		})
	}
}

func TestQualificationBudgetBoundaryWireAdmission(t *testing.T) {
	for _, scenario := range []string{"absent-body", "unreadable-body", "invalid-json", "invalid-output-type", "negative-output", "billable-tool", "unsupported-codec", "max-tokens-variation"} {
		t.Run(scenario, func(t *testing.T) {
			harness := newQualificationBoundaryHarness(t)
			request := harness.request
			expectedCalls := int64(0)
			switch scenario {
			case "max-tokens-variation":
				expectedCalls = 1
				identity := harness.identity
				identity.transport.protocolParameters.TokenField = CatalogProtocolVariationMaxTokens
				request = request.WithContext(context.WithValue(request.Context(), qualificationTransportKey{}, identity))
				request.Body = io.NopCloser(strings.NewReader(`{"model":"qwen-3.8-27b","max_tokens":128}`))
			case "absent-body":
				request.Body = nil
			case "unreadable-body":
				request.Body = io.NopCloser(qualificationBoundaryReadFailure{})
			case "invalid-json":
				request.Body = io.NopCloser(strings.NewReader("{"))
			case "invalid-output-type":
				request.Body = io.NopCloser(strings.NewReader(`{"model":"qwen-3.8-27b","max_completion_tokens":"128"}`))
			case "negative-output":
				request.Body = io.NopCloser(strings.NewReader(`{"model":"qwen-3.8-27b","max_completion_tokens":-1}`))
			case "billable-tool":
				request.Body = io.NopCloser(strings.NewReader(`{"model":"qwen-3.8-27b","tools":[{"type":"function","name":"lookup"}]}`))
			case "unsupported-codec":
				harness.fixture.budget.frozen[0].Codec = CatalogProtocolVertexGenerateContent
				identity := harness.identity
				identity.transport.responseCodec = CatalogProtocolVertexGenerateContent
				request = request.WithContext(context.WithValue(request.Context(), qualificationTransportKey{}, identity))
			}
			response, err := harness.doer.Do(request)
			if response != nil {
				_ = response.Body.Close()
			}
			if expectedCalls == 0 && !errors.Is(err, errQualificationBudget) || expectedCalls == 1 && err != nil || harness.calls.Load() != expectedCalls {
				t.Fatalf("wire admission error=%v calls=%d expected=%d", err, harness.calls.Load(), expectedCalls)
			}
		})
	}
}

func TestQualificationBudgetBoundarySchemaToolNamesAreData(t *testing.T) {
	harness := newQualificationBoundaryHarness(t)
	harness.request.Body = io.NopCloser(bytes.NewBufferString(`{"model":"qwen-3.8-27b","max_completion_tokens":128,"response_format":{"type":"json_schema","json_schema":{"schema":{"type":"object","properties":{"tools":{"type":"string"}}}}}}`))
	response, err := harness.doer.Do(harness.request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if harness.calls.Load() != 1 {
		t.Fatal("schema property names changed paid-feature admission")
	}
}

func TestQualificationBudgetFiveCanonicalOfferingsAndExplicitRegion(t *testing.T) {
	for _, scenario := range []string{"five-routes", "missing-region", "wrong-region"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			region := "singapore"
			if scenario == "missing-region" {
				region = ""
			}
			if scenario == "wrong-region" {
				region = "us-east"
			}
			fixture.policy.RemainingMicroUSD = 4355061
			fixture.policy.Offerings = []QualificationBudgetOffering{{Provider: "cerebras", Model: "qwen3.8-27b", MaxOutputTokens: 32768}, {Provider: "dashscope", Model: "qwen3.8-flash", PriceRegion: region, MaxOutputTokens: 32768}, {Provider: "openai", Model: "gpt-6-luna", MaxOutputTokens: 32768}, {Provider: "anthropic", Model: "claude-haiku-5-5", MaxOutputTokens: 32768}, {Provider: "gemini", Model: "gemini-3.5-flash-lite", MaxOutputTokens: 32768}}
			if err := validateQualificationBudget(&fixture.policy, fixture.managementPath); err != nil {
				t.Fatal(err)
			}
			budget, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
			if scenario != "five-routes" {
				requireQualificationFault(t, err, "unsupported price conditions")
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			fixture.budget = budget
			if len(budget.frozen) != 5 {
				t.Fatalf("frozen=%+v", budget.frozen)
			}
			var output bytes.Buffer
			if err := OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", &output); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), `"PriceRegion":"singapore"`) || !strings.Contains(output.String(), `"region":"singapore"`) {
				t.Fatalf("explicit region missing from receipt: %s", output.String())
			}
			for _, offering := range budget.frozen {
				if offering.Revision != fixture.catalog.Revision || offering.MaxOutput != 32768 || offering.Source == "" || offering.LastVerified == "" {
					t.Fatalf("unfrozen route: %+v", offering)
				}
			}
		})
	}
}

func TestQualificationBudgetRejectsSQLitePathSyntaxAndInvalidRegion(t *testing.T) {
	for _, scenario := range []string{"query-path", "fragment-path", "invalid-region"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			switch scenario {
			case "query-path":
				fixture.policy.ReceiptDatabasePath = fixture.managementPath + "?different"
			case "fragment-path":
				fixture.policy.ReceiptDatabasePath += "#fragment"
			case "invalid-region":
				fixture.policy.Offerings[0].PriceRegion = "Singapore?"
			}
			configuration := withInternalUpstreamCapacity(t, Configuration{Management: managedRouterTestManagementConfiguration(), ProviderCatalog: internalCanonicalProviderCatalog(), QualificationBudget: &fixture.policy})
			configuration.Management.DatabasePath = fixture.managementPath
			if _, err := NewConfiguration(configuration); err == nil {
				t.Fatal("invalid operator path/region accepted")
			}
		})
	}
}

func TestQualificationBudgetListEstimateRejectsIncompletePriceAuthority(t *testing.T) {
	policy := QualificationBudgetConfiguration{Offerings: []QualificationBudgetOffering{{Provider: "cerebras", Model: "qwen3.8-27b", MaxOutputTokens: 128}}}
	offerings, err := freezeQualificationOfferings(policy, internalCanonicalProviderCatalog().ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"unsupported-unit", "unresolved-price", "expired-prices", "unknown-component", "missing-output", "input-tier-gap", "output-tier-gap", "overflow", "ambiguous-conditions", "ambiguous-input-dimensions", "ambiguous-output-dimensions"} {
		t.Run(scenario, func(t *testing.T) {
			offering := offerings[0]
			offering.Rates = slices.Clone(offering.Rates)
			at := time.Now().UTC()
			switch scenario {
			case "unsupported-unit":
				offering.Rates[0].Unit = "USD/minute"
			case "unresolved-price":
				offering.Rates[0].Conditions.InputTokens = CatalogTokenRange{UnresolvedReason: "published boundary unresolved"}
			case "expired-prices":
				for i := range offering.Rates {
					offering.Rates[i].Conditions.EffectiveUntil = at.Add(time.Hour).Truncate(time.Second).Format(time.RFC3339)
				}
				at = at.Add(2 * time.Hour)
			case "unknown-component":
				offering.Rates[0].Component = "provider_tool_calls"
			case "missing-output":
				offering.Rates = offering.Rates[:1]
			case "input-tier-gap":
				offering.Rates[0].Conditions.InputTokens = CatalogTokenRange{Minimum: 1}
			case "output-tier-gap":
				offering.Rates[1].Conditions.InputTokens = CatalogTokenRange{Minimum: 1}
			case "overflow":
				offering.Rates[0].Rate = CatalogDecimal("999999999999999999999999999999999999")
			case "ambiguous-conditions":
				offering.Rates[1].Conditions.BillingMode = "standard"
			case "ambiguous-input-dimensions":
				rate := offering.Rates[0]
				rate.Component = "input_text"
				offering.Rates = append(offering.Rates, rate)
			case "ambiguous-output-dimensions":
				rate := offering.Rates[1]
				rate.Component = "output_text"
				offering.Rates = append(offering.Rates, rate)
			}
			if _, err := offering.estimate(offering.InputCeiling, uint64(offering.MaxOutput), true, at); err == nil {
				t.Fatal("incomplete list authority admitted")
			}
		})
	}
}

func TestQualificationBudgetTokenOnlyRejectsMinimumCharge(t *testing.T) {
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	catalog.Prices = slices.Clone(catalog.Prices)
	for index := range catalog.Prices {
		if catalog.Prices[index].Provider == "cerebras" && catalog.Prices[index].Model == "qwen3.8-27b" {
			catalog.Prices[index].MinimumCharge = &CatalogMinimumCharge{Currency: "USD", Amount: "1", Unit: "USD/request"}
		}
	}
	policy := QualificationBudgetConfiguration{Offerings: []QualificationBudgetOffering{{Provider: "cerebras", Model: "qwen3.8-27b", MaxOutputTokens: 128}}}
	if _, err := freezeQualificationOfferings(policy, catalog); err == nil {
		t.Fatal("unbounded minimum charge admitted")
	}
}

func TestQualificationBudgetMeasurementBoundaryRetainsReservation(t *testing.T) {
	for _, scenario := range []string{"missing-input", "oversized-integer", "missing-canonical-quantities", "missing-reasoning", "oversized-reasoning-integer", "excessive-reasoning", "missing-cache", "oversized-cache-integer", "excessive-cache", "excessive-input", "unpriced-acceptance"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			provider, model := "cerebras", "qwen3.8-27b"
			switch scenario {
			case "missing-reasoning", "oversized-reasoning-integer", "excessive-reasoning":
				provider, model = "gemini", "gemini-3.5-flash-lite"
			case "missing-cache", "oversized-cache-integer", "excessive-cache":
				provider, model = "anthropic", "claude-haiku-5-5"
			}
			fixture.policy.Offerings = []QualificationBudgetOffering{{Provider: provider, Model: model, MaxOutputTokens: 128}}
			fixture.initialize(t)
			offering := fixture.budget.frozen[0]
			logical := &qualificationExecution{id: "measurement", requestID: "origin-fixture"}
			attempt, err := fixture.budget.reserve(logical, offering, "fixture-connection")
			if err != nil {
				t.Fatal(err)
			}
			body := `{"usage":{"prompt_tokens":2,"completion_tokens":3}}`
			switch scenario {
			case "missing-input":
				body = `{"usage":{"completion_tokens":3}}`
			case "oversized-integer":
				body = `{"usage":{"prompt_tokens":18446744073709551616,"completion_tokens":0}}`
			case "missing-reasoning":
				body = `{"usage":{"total_input_tokens":2,"total_output_tokens":3}}`
			case "oversized-reasoning-integer":
				body = `{"usage":{"total_input_tokens":2,"total_output_tokens":3,"total_thought_tokens":18446744073709551616}}`
			case "excessive-reasoning":
				body = `{"usage":{"total_input_tokens":2,"total_output_tokens":3,"total_thought_tokens":129}}`
			case "missing-cache":
				body = `{"usage":{"input_tokens":2,"output_tokens":3}}`
			case "oversized-cache-integer":
				body = `{"usage":{"input_tokens":2,"output_tokens":3,"cache_read_input_tokens":18446744073709551616,"cache_creation_input_tokens":0}}`
			case "excessive-cache":
				body = `{"usage":{"input_tokens":2,"output_tokens":3,"cache_read_input_tokens":1000001,"cache_creation_input_tokens":0}}`
			case "excessive-input":
				body = `{"usage":{"prompt_tokens":65537,"completion_tokens":0}}`
			case "unpriced-acceptance":
				attempt.CreatedAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			evidence, _ := newTextJournalMeter(offering.Codec).observe([]byte(body), http.StatusOK, "attempt", time.Now())
			if scenario == "missing-canonical-quantities" {
				evidence = journalUsageEvidenceInput{}
			}
			if err := fixture.budget.settle(attempt, offering, evidence); err == nil {
				t.Fatal("unmeasured/unpriced attempt released reservation")
			}
			var state qualificationBudgetState
			if err := fixture.budget.database.First(&state, "id = ?", fixture.policy.BudgetID).Error; err != nil {
				t.Fatal(err)
			}
			if state.Remaining != fixture.policy.RemainingMicroUSD-attempt.Reserved {
				t.Fatalf("invalid observation refunded funds: %+v", state)
			}
		})
	}
}

func TestQualificationBudgetAdmissionNonDispatch(t *testing.T) {
	for _, scenario := range []string{"queue-full", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			harness := newQualificationBoundaryHarness(t)
			origin := upstreamRequestOrigin(harness.request.URL)
			admission := admissionTestHTTPDoer(t, origin, admissionOriginB, upstreamRateLimits{})
			admission.next = harness.doer.qualified
			harness.doer.qualified = admission
			scope := harness.identity.scope
			blocker := admitTestRequest(t, admission.scheduler, origin, scope.tenant, scope.account, scope.class, time.Now())
			if err := admission.acquire(blocker, origin); err != nil {
				t.Fatal(err)
			}
			releaseBlocker := sync.OnceFunc(func() { admission.release(blocker) })
			t.Cleanup(releaseBlocker)
			var releaseQueued []func()
			if scenario == "queue-full" {
				for range 2 {
					queued := admitTestRequest(t, admission.scheduler, origin, scope.tenant, scope.account, scope.class, time.Now())
					release := sync.OnceFunc(func() { admission.release(queued) })
					releaseQueued = append(releaseQueued, release)
					t.Cleanup(release)
				}
			}
			ctx := requestContextWithUpstreamScope(harness.request.Context(), scope)
			expected := errQueueFull
			if scenario == "deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 30*time.Millisecond)
				defer cancel()
				expected = context.DeadlineExceeded
			}
			_, err := harness.doer.Do(harness.request.WithContext(ctx))
			if harness.calls.Load() != 0 {
				t.Fatal("rejected request reached provider")
			}
			if !errors.Is(err, expected) {
				t.Errorf("admission error lost: %v", err)
			}
			if err := finishQualificationExecution(ctx); err != nil {
				t.Errorf("non-dispatch finalizer: %v", err)
			}
			var output bytes.Buffer
			if err := OperateQualificationBudget(harness.fixture.managementPath, harness.fixture.policy.BudgetID, "inspect", &output); err != nil {
				t.Fatal(err)
			}
			var receipt struct {
				Remaining int64                        `json:"remaining_micro_usd"`
				Suspended bool                         `json:"suspended"`
				Attempts  []qualificationBudgetAttempt `json:"attempts"`
			}
			if err := json.Unmarshal(output.Bytes(), &receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.Remaining != harness.fixture.policy.RemainingMicroUSD || receipt.Suspended || len(receipt.Attempts) != 1 || receipt.Attempts[0].Active || receipt.Attempts[0].Cost != 0 {
				t.Fatalf("non-dispatch consumed allowance: %s", output.String())
			}
			releaseBlocker()
			for _, release := range releaseQueued {
				release()
			}
			harness.request.Body = io.NopCloser(strings.NewReader(`{"model":"qwen-3.8-27b","max_completion_tokens":128,"messages":[]}`))
			response, err := harness.doer.Do(harness.request.Clone(requestContextWithUpstreamScope(context.WithValue(harness.request.Context(), qualificationExecutionKey{}, &qualificationExecution{id: "next-logical", requestID: "next-origin"}), scope)))
			if err != nil {
				t.Fatalf("subsequent generation blocked: %v", err)
			}
			_ = response.Body.Close()
		})
	}
}

func TestQualificationBudgetNonDispatchReleaseFailure(t *testing.T) {
	for _, phase := range []string{"attempt-write", "allowance-write", "unavailable-attempt"} {
		t.Run(phase, func(t *testing.T) {
			harness := newQualificationBoundaryHarness(t)
			origin := upstreamRequestOrigin(harness.request.URL)
			harness.doer.qualified = admissionTestHTTPDoer(t, origin, admissionOriginB, upstreamRateLimits{})
			statement := `CREATE TRIGGER release_failure BEFORE UPDATE ON qualification_budget_attempts BEGIN SELECT RAISE(ABORT, 'controlled_release'); END`
			if phase == "allowance-write" {
				statement = `CREATE TRIGGER release_failure BEFORE UPDATE OF remaining ON qualification_budget_states WHEN NEW.remaining > OLD.remaining BEGIN SELECT RAISE(ABORT, 'controlled_release'); END`
			} else if phase == "unavailable-attempt" {
				statement = `CREATE TRIGGER release_failure BEFORE UPDATE ON qualification_budget_attempts WHEN NEW.state = 'settled' BEGIN SELECT RAISE(IGNORE); END`
			}
			qualificationFaultExec(t, harness.fixture.budget.database, statement)
			_, err := harness.doer.Do(harness.request)
			if !errors.Is(err, errQualificationBudget) || !errors.Is(err, errUpstreamNotDispatched) {
				t.Fatalf("release error: %v", err)
			}
			var state qualificationBudgetState
			var attempt qualificationBudgetAttempt
			if err := harness.fixture.budget.database.First(&state).Error; err != nil {
				t.Fatal(err)
			}
			if err := harness.fixture.budget.database.First(&attempt).Error; err != nil {
				t.Fatal(err)
			}
			if !state.Suspended || state.Remaining != harness.fixture.policy.RemainingMicroUSD-attempt.Reserved || attempt.State != "dispatched" || harness.logical.active == nil {
				t.Fatalf("failed release lost reservation: state=%+v attempt=%+v", state, attempt)
			}
			if err := OperateQualificationBudget(harness.fixture.managementPath, harness.fixture.policy.BudgetID, "inspect", io.Discard); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQualificationBudgetPollNonDispatchRetainsReservation(t *testing.T) {
	harness := newQualificationBoundaryHarness(t)
	budget := harness.fixture.budget
	offering := budget.frozen[0]
	attempt, err := budget.reserve(harness.logical, offering, harness.identity.scope.account)
	if err != nil {
		t.Fatal(err)
	}
	attempt.ResourceID = "existing-work"
	qualificationFaultExec(t, budget.database, `UPDATE qualification_budget_attempts SET state = 'pending', resource_id = ? WHERE id = ?`, attempt.ResourceID, attempt.ID)
	harness.logical.active = &attempt
	harness.logical.offering = offering
	harness.request.Method = http.MethodGet
	harness.request.URL.Path = "/responses/" + attempt.ResourceID
	harness.doer.qualified = admissionTestHTTPDoer(t, upstreamRequestOrigin(harness.request.URL), admissionOriginB, upstreamRateLimits{})
	if _, err := harness.doer.Do(harness.request); !errors.Is(err, errQualificationBudget) {
		t.Fatalf("poll failure: %v", err)
	}
	var state qualificationBudgetState
	if err := budget.database.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if !state.Suspended || state.Remaining != budget.policy.RemainingMicroUSD-attempt.Reserved || harness.logical.active == nil {
		t.Fatalf("poll failure refunded existing work: %+v", state)
	}
}
