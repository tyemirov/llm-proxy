package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

func TestHostedAlignmentDelayedUsageSettlesExactFunds(t *testing.T) {
	testHostedAlignmentFinancial(t, "delayed", "")
}

func TestHostedAlignmentPendingUsageSurvivesShutdown(t *testing.T) {
	testHostedAlignmentFinancial(t, "restart", "")
}

func TestHostedAlignmentSubmissionShutdownRetainsUncertainty(t *testing.T) {
	testHostedAlignmentFinancial(t, "submission-shutdown", "")
}

func TestHostedAlignmentAccountIsolation(t *testing.T) {
	testHostedAlignmentFinancial(t, "isolation", "")
}

type alignmentCredentialFailureDialector struct {
	*sqlite.Dialector
	armed    *atomic.Bool
	failures *atomic.Int64
}

func (dialector alignmentCredentialFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	return database.Callback().Query().Before("gorm:query").Register("test:alignment_credential_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "managed_platform_credential_records" && dialector.armed.CompareAndSwap(true, false) {
			dialector.failures.Add(1)
			tx.AddError(errors.New("controlled_alignment_credential_read_failure"))
		}
	})
}

func alignmentUsageTestResponse(rows string) string {
	return `{"columns":["total_minutes","usage_count","total_cost"],"column_types":["Float","Int","Float"],"column_units":["min",null,"usd"],"rows":` + rows + `}`
}

func TestHostedAlignmentNativeUsageBoundaries(t *testing.T) {
	for _, scenario := range []struct{ name, body string }{
		{"zero", alignmentUsageTestResponse(`[[0,1,0]]`)},
		{"zero-buckets", ""},
		{"scientific", alignmentUsageTestResponse(`[[2.99375e-2,1,1.0977083333333333e-4]]`)},
		{"missing-trace", ""}, {"http-failure", ""}, {"truncated", ""}, {"observation-failure", ""},
		{"missing-trace-observation-failure", ""}, {"credential-read-failure", ""},
	} {
		t.Run(scenario.name, func(t *testing.T) { testHostedAlignmentFinancial(t, scenario.name, scenario.body) })
	}
	for name, body := range map[string]string{
		"invalid-json":          `{`,
		"oversized-response":    strings.Repeat(" ", providerMetadataMaximumBytes+1),
		"null-rows":             alignmentUsageTestResponse(`null`),
		"negative-minutes":      alignmentUsageTestResponse(`[[-1,1,0]]`),
		"null-minutes":          alignmentUsageTestResponse(`[[null,1,0]]`),
		"string-minutes":        alignmentUsageTestResponse(`[["0.0299375",1,0]]`),
		"negative-cost":         alignmentUsageTestResponse(`[[0.0299375,1,-1]]`),
		"fractional-count":      alignmentUsageTestResponse(`[[0.0299375,0.5,0]]`),
		"multiple-requests":     alignmentUsageTestResponse(`[[0.0299375,2,0]]`),
		"duplicate-requests":    alignmentUsageTestResponse(`[[0.0299375,1,0],[0.0299375,1,0]]`),
		"usage-without-request": alignmentUsageTestResponse(`[[0.0299375,0,0]]`),
		"cost-without-request":  alignmentUsageTestResponse(`[[0,0,1]]`),
		"over-limit":            alignmentUsageTestResponse(`[[600.1,1,3]]`),
		"short-row":             alignmentUsageTestResponse(`[[0,1]]`),
		"wrong-unit":            strings.Replace(alignmentUsageTestResponse(`[[0,1,0]]`), `"min"`, `"s"`, 1),
		"wrong-currency":        strings.Replace(alignmentUsageTestResponse(`[[0,1,0]]`), `"usd"`, `"eur"`, 1),
		"missing-column":        strings.Replace(alignmentUsageTestResponse(`[[0,1,0]]`), `"total_minutes"`, `"unknown"`, 1),
		"duplicate-column":      strings.Replace(alignmentUsageTestResponse(`[[0,1,0]]`), `"total_minutes"`, `"usage_count"`, 1),
		"wrong-type":            strings.Replace(alignmentUsageTestResponse(`[[0,1,0]]`), `"Float"`, `"String"`, 1),
	} {
		t.Run(name, func(t *testing.T) { testHostedAlignmentFinancial(t, "invalid", body) })
	}
}

func testHostedAlignmentFinancial(t *testing.T, scenario, responseBody string) {
	t.Helper()
	database, _, management, _ := newHostedRatingFixture(t)
	var submissions, observations atomic.Int64
	var credentialReadArmed atomic.Bool
	var credentialReadFailures atomic.Int64
	var shutdownReadArmed atomic.Bool
	var receiptFailureArmed atomic.Bool
	var receiptFailures atomic.Int64
	var clockOffset atomic.Int64
	runtimeNow := func() time.Time { return time.Now().Add(time.Duration(clockOffset.Load())) }
	shutdownFailure := strings.HasPrefix(scenario, "shutdown-")
	receiptFailure := strings.HasPrefix(scenario, "receipt-")
	submissionShutdown := scenario == "submission-shutdown"
	missingTrace := scenario == "missing-trace" || scenario == "missing-trace-observation-failure"
	usagePolling := !missingTrace && scenario != "credential-read-failure" && !receiptFailure && !submissionShutdown
	failedObservation := scenario == "observation-failure" || scenario == "missing-trace-observation-failure"
	var queryIntent atomic.Value
	if responseBody == "" {
		responseBody = alignmentUsageTestResponse(`[[0,0,0],[0.0299375,1,0.00010977083333333333]]`)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseUsage := func() { releaseOnce.Do(func() { close(release) }) }
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("xi-api-key") != "hosted-service-secret" {
			t.Error("alignment lost its pinned provider authority")
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/forced-alignment":
			submissions.Add(1)
			if submissionShutdown {
				if _, err := io.Copy(io.Discard, request.Body); err != nil {
					t.Error(err)
					return
				}
				close(entered)
				select {
				case <-request.Context().Done():
				case <-release:
				}
				return
			}
			if receiptFailure {
				receiptFailureArmed.Store(true)
			}
			if !missingTrace {
				writer.Header().Set("x-trace-id", "private-funded-alignment")
			}
			if scenario == "credential-read-failure" {
				credentialReadArmed.Store(true)
			}
			writer.Header().Set("character-cost", "1")
			fmt.Fprint(writer, `{"characters":[],"words":[{"text":"Names","start":0,"end":1}],"loss":0}`)
		case "/v1/workspace/analytics/query/usage-by-product-over-time":
			count := observations.Add(1)
			queryBody, err := io.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
				return
			}
			var query struct {
				StartTime       int64            `json:"start_time"`
				EndTime         int64            `json:"end_time"`
				IntervalSeconds int              `json:"interval_seconds"`
				Filters         []map[string]any `json:"filters"`
			}
			if err := json.Unmarshal(queryBody, &query); err != nil || query.StartTime >= query.EndTime || query.IntervalSeconds != 60 || !reflect.DeepEqual(query.Filters, []map[string]any{{"column": "trace_id", "operation": "eq", "values": []any{"private-funded-alignment"}}}) {
				t.Errorf("alignment usage lost its exact trace query: %s error=%v", queryBody, err)
			}
			if count == 1 {
				queryIntent.Store(string(queryBody))
			} else if queryIntent.Load() != string(queryBody) {
				t.Error("alignment recovery changed its saved query interval or trace")
			}
			if count == 1 {
				rows := `[]`
				if scenario == "zero-buckets" {
					rows = `[[0,0,0]]`
				}
				fmt.Fprint(writer, alignmentUsageTestResponse(rows))
				return
			}
			if count == 2 {
				close(entered)
			}
			select {
			case <-release:
				if scenario == "http-failure" {
					writer.WriteHeader(http.StatusServiceUnavailable)
				}
				if scenario == "truncated" {
					writer.Header().Set("Content-Length", "999999")
				}
				fmt.Fprint(writer, responseBody)
			case <-request.Context().Done():
			}
		default:
			t.Errorf("unexpected alignment request: %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(releaseUsage)
	configuration, _ := hostedRuntimeServiceConfiguration(t, database, ModelOperationAudioAlignment, upstream.URL, func(service *ProviderCatalogService) {
		service.Price = ProviderCatalogPrice{Operation: service.Operation, Available: true, Source: "https://elevenlabs.io/pricing/api", LastVerified: "2026-09-25", Rates: []CatalogPriceRate{{Component: "input_audio", Currency: "USD", Rate: "0.22", Unit: "USD/hour", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-25T00:00:00Z"}}}}
	})
	if scenario == "credential-read-failure" {
		configuration.Management.DatabaseDialector = alignmentCredentialFailureDialector{
			Dialector: sqlite.Open(configuration.Management.DatabasePath + managedSQLiteRuntimeQuery).(*sqlite.Dialector),
			armed:     &credentialReadArmed, failures: &credentialReadFailures,
		}
	}
	if scenario == "isolation" {
		seedHostedAlignmentOtherAccount(t, database, configuration.ProviderCatalog.modelCatalog.Revision)
	}
	if scenario == "shutdown-receipt-read" {
		configuration.Management.DatabaseDialector = alignmentShutdownReadFailureDialector{
			Dialector: sqlite.Open(configuration.Management.DatabasePath + managedSQLiteRuntimeQuery).(*sqlite.Dialector), armed: &shutdownReadArmed,
		}
	}
	if scenario == alignmentReceiptClaimLock || scenario == alignmentReceiptAttemptRead {
		configuration.Management.DatabaseDialector = alignmentReceiptFailureDialector{
			Dialector: sqlite.Open(configuration.Management.DatabasePath + managedSQLiteRuntimeQuery).(*sqlite.Dialector),
			boundary:  scenario, armed: &receiptFailureArmed, failures: &receiptFailures,
		}
	}
	baseURL, httpClient, stopRuntime, runtimeLogs := startHostedAlignmentRuntimeWithClock(t, configuration, runtimeNow)
	exchange := func(method, path, key, body string, status int) map[string]any {
		return hostedAlignmentHTTP(t, httpClient, baseURL, method, path, key, body, status)
	}
	post := func(key string, intent string, status int) map[string]any {
		return exchange(http.MethodPost, "/model/v1/operations", key, intent, status)
	}
	readOperation := func(id string) map[string]any {
		return exchange(http.MethodGet, "/model/v1/operations/"+id, "", "", http.StatusOK)
	}
	clientConfiguration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: baseURL, Secret: hostedIdentityFixtureKey})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(clientConfiguration, httpClient)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: []byte("controlled owned audio")})
	if err != nil {
		t.Fatal(err)
	}
	intent := fmt.Sprintf(`{"capability":"audio.align","provider":"elevenlabs","input":{"audio_asset_id":%q,"transcript":"Names"},"controls":{}}`, asset.AssetID)
	post("unfunded-alignment", intent, http.StatusPaymentRequired)
	if submissions.Load() != 0 || observations.Load() != 0 {
		t.Fatal("unfunded alignment reached the provider")
	}
	seedHostedFunds(t, database, 500)
	if scenario == alignmentReceiptIgnored {
		if err := database.database.Exec("CREATE TRIGGER ignore_alignment_receipt BEFORE UPDATE OF provider_handle ON media_operation_records WHEN NEW.provider_handle != '' BEGIN SELECT RAISE(IGNORE); END").Error; err != nil {
			t.Fatal(err)
		}
	}
	if failedObservation {
		if err := database.database.Exec("CREATE TRIGGER reject_alignment_usage BEFORE INSERT ON managed_journal_observation_records BEGIN SELECT RAISE(ABORT, 'controlled alignment observation failure'); END").Error; err != nil {
			t.Fatal(err)
		}
	}
	id := post("funded-alignment", intent, http.StatusAccepted)["operation_id"].(string)
	if usagePolling || submissionShutdown {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("alignment did not reach its provider checkpoint: submissions=%d observations=%d", submissions.Load(), observations.Load())
		}
	}
	chargesPath := "/billing-accounts/billing-journal/charges"
	charges := ratingHTTPExchange(t, management, http.MethodGet, chargesPath, "", http.StatusOK)["charges"].([]any)
	if (usagePolling || submissionShutdown) && (len(charges) != 0 || readOperation(id)["state"] != MediaOperationStateRunning) {
		t.Fatalf("missing usage settled or published alignment: charges=%v", charges)
	}
	if usagePolling || submissionShutdown {
		assertHostedFundsBalance(t, database, 500, 214)
	}
	if submissionShutdown {
		stopRuntime()
		assertNoAlignmentProviderReceipt(t, database, id)
		baseURL, httpClient, stopRuntime, _ = startHostedAlignmentRuntimeWithClock(t, configuration, runtimeNow)
	}
	if scenario == "isolation" {
		assertHostedAlignmentAccountIsolation(t, database, baseURL, readOperation(id), asset.AssetID)
		assertHostedFundsBalance(t, database, 500, 214)
	}
	if scenario == "restart" || shutdownFailure {
		var reason string
		var retainedState alignmentShutdownState
		if shutdownFailure {
			retainedState = readAlignmentShutdownState(t, database, id)
			if retainedState.operation.ProviderHandle == "" {
				t.Fatal("alignment receipt was not retained")
			}
			reason = rejectAlignmentShutdownWrite(t, database, scenario)
			shutdownReadArmed.Store(scenario == "shutdown-receipt-read")
		}
		stopRuntime()
		assertHostedFundsBalance(t, database, 500, 214)
		if shutdownFailure {
			reports := runtimeLogs.FilterMessage("media operation worker failed").All()
			if len(reports) != 1 || !strings.Contains(fmt.Sprint(reports[0].ContextMap()["error"]), reason) {
				t.Fatalf("shutdown did not report the selected storage failure: %v", reports)
			}
			if retained := readAlignmentShutdownState(t, database, id); !reflect.DeepEqual(retained, retainedState) {
				t.Fatal("failed shutdown changed the saved receipt or claims")
			}
			if scenario != "shutdown-receipt-read" {
				if err := database.database.Exec("DROP TRIGGER reject_alignment_shutdown").Error; err != nil {
					t.Fatal(err)
				}
			}
			clockOffset.Store(int64(2 * time.Minute))
		}
		baseURL, httpClient, stopRuntime, _ = startHostedAlignmentRuntimeWithClock(t, configuration, runtimeNow)
		if current := readOperation(id); current["state"] != MediaOperationStateRunning {
			t.Fatalf("pending usage lost recoverable state at shutdown: %v", current)
		}
	}
	wantUncertain := scenario == "invalid" || scenario == "http-failure" || scenario == "truncated" || failedObservation || scenario == "credential-read-failure" || receiptFailure || submissionShutdown
	releaseUsage()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		charges = ratingHTTPExchange(t, management, http.MethodGet, chargesPath, "", http.StatusOK)["charges"].([]any)
		state := readOperation(id)["state"]
		if wantUncertain && state == MediaOperationStateUncertain || !wantUncertain && len(charges) == 1 && state == MediaOperationStateSucceeded {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("alignment did not settle: charges=%v operation=%v calls=%d observations=%d", charges, readOperation(id), submissions.Load(), observations.Load())
		case <-tick.C:
		}
	}
	if wantUncertain {
		if len(charges) != 0 {
			t.Fatalf("invalid usage created a charge: %v", charges)
		}
		assertHostedFundsBalance(t, database, 500, 214)
		assertFundsCreditRemainder(t, database, "0", "1")
		if outputs := readOperation(id)["outputs"].([]any); len(outputs) != 0 {
			t.Fatalf("uncertain alignment published outputs: %v", outputs)
		}
	} else if scenario == "missing-trace" {
		if charges[0].(map[string]any)["state"] != chargeUsageUnresolved {
			t.Fatalf("missing trace was billed: %v", charges)
		}
		assertHostedFundsBalance(t, database, 500, 214)
	} else if scenario == "zero" {
		charge := charges[0].(map[string]any)
		zero := map[string]any{"numerator": "0", "denominator": "1"}
		if charge["state"] != chargeRated || !reflect.DeepEqual(charge["customer_charge"], zero) || !reflect.DeepEqual(charge["rating"].(map[string]any)["provider_cost"], zero) {
			t.Fatalf("measured zero alignment charge=%v", charge)
		}
		assertHostedFundsBalance(t, database, 500, 500)
		assertFundsCreditRemainder(t, database, "0", "1")
	} else {
		charge := charges[0].(map[string]any)
		if charge["state"] != chargeRated || !reflect.DeepEqual(charge["rating"].(map[string]any)["provider_cost"], map[string]any{"numerator": "5269", "denominator": "48000000"}) || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "68497", "denominator": "480000000"}) {
			t.Fatalf("alignment charge=%v", charge)
		}
		assertHostedFundsBalance(t, database, 500, 500)
		assertFundsCreditRemainder(t, database, "68497", "480000000")
	}
	wantObservations := int64(2)
	if scenario == "restart" || shutdownFailure {
		wantObservations = 3
	} else if !usagePolling {
		wantObservations = 0
	}
	if scenario == "credential-read-failure" && credentialReadFailures.Load() != 1 {
		t.Fatalf("credential read failures=%d want=1", credentialReadFailures.Load())
	}
	if receiptFailure {
		assertNoAlignmentProviderReceipt(t, database, id)
		reports := runtimeLogs.FilterMessage("media operation persistence failed").All()
		if len(reports) != 1 || reports[0].ContextMap()["phase"] != "persist_provider_handle" || reports[0].ContextMap()["operation_id"] != id {
			t.Fatalf("receipt failure was not reported at its persistence boundary: %v", reports)
		}
		if scenario != alignmentReceiptIgnored && receiptFailures.Load() != 1 {
			t.Fatalf("receipt storage failures=%d want=1", receiptFailures.Load())
		}
	}
	if replay := post("funded-alignment", intent, http.StatusOK); replay["operation_id"] != id || submissions.Load() != 1 || observations.Load() != wantObservations {
		t.Fatalf("alignment replay repeated provider work: %v", replay)
	}
	if scenario == "isolation" {
		assertHostedAlignmentAccountIsolation(t, database, baseURL, readOperation(id), asset.AssetID)
	}
	if scenario == "missing-trace-observation-failure" || scenario == "credential-read-failure" || scenario == "invalid" && len(responseBody) > providerMetadataMaximumBytes || receiptFailure || submissionShutdown {
		retained := readOperation(id)
		stopRuntime()
		if failedObservation {
			if err := database.database.Exec("DROP TRIGGER reject_alignment_usage").Error; err != nil {
				t.Fatal(err)
			}
		}
		if scenario == alignmentReceiptIgnored {
			if err := database.database.Exec("DROP TRIGGER ignore_alignment_receipt").Error; err != nil {
				t.Fatal(err)
			}
		}
		for range 2 {
			baseURL, httpClient, stopRuntime = startHostedAlignmentRuntime(t, configuration)
			if replay := post("funded-alignment", intent, http.StatusOK); !reflect.DeepEqual(replay, retained) || submissions.Load() != 1 || observations.Load() != wantObservations {
				t.Fatalf("uncertain alignment restart changed retained outcome or repeated work: %v", replay)
			}
			assertHostedFundsBalance(t, database, 500, 214)
			assertFundsCreditRemainder(t, database, "0", "1")
			if after := ratingHTTPExchange(t, management, http.MethodGet, chargesPath, "", http.StatusOK)["charges"].([]any); len(after) != 0 {
				t.Fatalf("uncertain alignment restart created charges: %v", after)
			}
			if receiptFailure || submissionShutdown {
				assertNoAlignmentProviderReceipt(t, database, id)
			}
			stopRuntime()
		}
	}
	if scenario == "restart" || scenario == "delayed" || scenario == "isolation" || shutdownFailure {
		for range 2 {
			stopRuntime()
			baseURL, httpClient, stopRuntime, _ = startHostedAlignmentRuntimeWithClock(t, configuration, runtimeNow)
			if replay := post("funded-alignment", intent, http.StatusOK); replay["operation_id"] != id || replay["state"] != MediaOperationStateSucceeded || submissions.Load() != 1 || observations.Load() != wantObservations {
				t.Fatalf("settled alignment restart repeated provider work: %v", replay)
			}
			recoveredCharges := ratingHTTPExchange(t, management, http.MethodGet, chargesPath, "", http.StatusOK)["charges"].([]any)
			if !reflect.DeepEqual(recoveredCharges, charges) {
				t.Fatalf("settled alignment restart changed charges: %v", recoveredCharges)
			}
			assertHostedFundsBalance(t, database, 500, 500)
			assertFundsCreditRemainder(t, database, "68497", "480000000")
			if scenario == "isolation" {
				assertHostedAlignmentAccountIsolation(t, database, baseURL, readOperation(id), asset.AssetID)
			}
		}
	}
}

func hostedAlignmentHTTP(t *testing.T, client *http.Client, baseURL, method, path, key, body string, status int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("alignment HTTP status=%d want=%d body=%s", response.StatusCode, status, data)
	}
	if method == http.MethodGet {
		request.URL.Path = llmproxycontract.MediaOperationsPath + "/{operation_id}"
	}
	validateHostedIdentityResponse(t, request, response, data)
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func startHostedAlignmentRuntime(t *testing.T, configuration Configuration) (string, *http.Client, func()) {
	t.Helper()
	baseURL, client, stop, _ := startHostedAlignmentRuntimeWithClock(t, configuration, time.Now)
	return baseURL, client, stop
}

func startHostedAlignmentRuntimeWithClock(t *testing.T, configuration Configuration, now func() time.Time) (string, *http.Client, func(), *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.ErrorLevel)
	t.Cleanup(func() {
		if t.Failed() {
			for _, entry := range logs.All() {
				t.Logf("runtime error: %s %v", entry.Message, entry.ContextMap())
			}
		}
	})
	application, err := buildProxyApplication(configuration, zap.New(core).Sugar(), newManagedTenantStore)
	if err != nil {
		t.Fatal(err)
	}
	application.now = now
	application.media.store.now = now
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runtimeContext, cancelRuntime := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- application.serve(runtimeContext, listener) }()
	var stopOnce sync.Once
	stopRuntime := func() {
		stopOnce.Do(func() {
			cancelRuntime()
			if err := <-stopped; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(stopRuntime)
	baseURL := "http://" + listener.Addr().String()
	httpClient := &http.Client{Transport: hostedMCPBearerTransport{token: hostedIdentityFixtureKey}}
	return baseURL, httpClient, stopRuntime, logs
}
