package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Invalid public requests must not create a hold or invoke a provider.
func TestHostedQuotePublicInputBoundaries(t *testing.T) {
	fixture := newQuotedFixture(t)
	valid, _ := json.Marshal(quoteNativeInput())
	cases := []struct {
		name, method, path, key, contentType, body string
		status                                     int
	}{
		{"missing key", "POST", "", "", "application/json", string(valid), 400},
		{"query", "POST", "?extra=1", "key", "application/json", string(valid), 400},
		{"media type", "POST", "", "key", "text/plain", string(valid), 400},
		{"malformed", "POST", "", "key", "application/json", "{", 400},
		{"unknown field", "POST", "", "key", "application/json", `{"unknown":1}`, 400},
		{"trailing object", "POST", "", "key", "application/json", string(valid) + ` {}`, 400},
		{"digest", "POST", "", "key", "application/json", strings.Replace(string(valid), strings.Repeat("a", 64), "bad", 1), 400},
		{"unknown provider", "POST", "", "key", "application/json", strings.ReplaceAll(string(valid), "openai", "missing"), 400},
		{"provider casing", "POST", "", "key", "application/json", strings.ReplaceAll(string(valid), "openai", "OpenAI"), 400},
		{"kind", "POST", "", "key", "application/json", strings.Replace(string(valid), "native_v2", "unknown", 1), 400},
		{"no model", "POST", "", "key", "application/json", strings.Replace(string(valid), `"gpt-4.1"`, `""`, 1), 400},
		{"no schema", "POST", "", "key", "application/json", `{"processing_digest":"` + strings.Repeat("a", 64) + `","intent":{"kind":"native_v2","provider":"openai","request":{"model":"gpt-4.1"}}}`, 400},
		{"invalid messages", "POST", "", "key", "application/json", strings.Replace(string(valid), `"user"`, `"unknown"`, 1), 400},
		{"read missing key", "GET", "", "", "", "", 400},
		{"read bad id", "GET", "/bad", "", "", "", 400},
		{"read unknown id", "GET", "/quote-" + strings.Repeat("f", 32), "", "", "", 404},
		{"read query", "GET", "?extra=1", "key", "", "", 400},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			request, err := http.NewRequest(scenario.method, fixture.server.URL+llmproxycontract.FinancialQuotesPath+scenario.path, strings.NewReader(scenario.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
			request.Header.Set("Content-Type", scenario.contentType)
			request.Header.Set(llmproxycontract.HeaderIdempotencyKey, scenario.key)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != scenario.status {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
		})
	}
	var count int64
	if err := fixture.database.database.Model(&managedFinancialQuoteRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("quotes=%d err=%v", count, err)
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("invalid request dispatched")
	}
}

func TestHostedQuotePublicStateBoundaries(t *testing.T) {
	fixture := newQuotedFixture(t)
	input := quoteNativeInput()
	q := fixture.exchange(t, "POST", "", "states", input, 201)
	path := "/" + q["id"].(string)
	fixture.exchange(t, "GET", path+"/executions", "", nil, 409)
	for _, body := range []any{map[string]any{"revision": 0, "state": "released"}, map[string]any{"revision": 1, "state": "cancelled"}} {
		fixture.exchange(t, "PATCH", path, "", body, 400)
	}
	fixture.exchange(t, "PATCH", path, "", map[string]any{"revision": 99, "state": "released"}, 409)
	fixture.exchange(t, "POST", path+"/executions", "", map[string]any{}, 409)
	changed := quoteNativeInput()
	changed["intent"].(map[string]any)["request"].(map[string]any)["messages"] = []map[string]string{{"role": "user", "content": "different"}}
	fixture.exchange(t, "POST", path+"/executions", "", changed, 409)
	fixture.exchange(t, "POST", path+"/executions", "", map[string]any{"processing_digest": strings.Repeat("a", 64), "intent": map[string]any{"kind": "native_v2", "provider": "openai", "request": nil}}, 400)
	fixture.exchange(t, "PATCH", path, "", map[string]any{"revision": 1, "state": "released"}, 200)
	fixture.exchange(t, "PATCH", path, "", map[string]any{"revision": 1, "state": "released"}, 200)
	fixture.exchange(t, "POST", path+"/executions", "", input, 409)
	if fixture.calls.Load() != 0 {
		t.Fatal("rejected state dispatched")
	}
}

// Inject one actual persistence boundary failure while retaining real transactions,
// authorization, HTTP handlers, and SQLite rollback behavior.
func TestHostedQuotePersistenceRollbackBoundaries(t *testing.T) {
	cases := []struct {
		name, operation, table, method string
		nth                            int
	}{
		{"assignment lock", "update", "managed_tenant_records", "POST", 1},
		{"existing quote read", "query", "managed_financial_quote_records", "POST", 1},
		{"journal create", "create", "managed_journal_request_records", "POST", 1},
		{"quote create", "create", "managed_financial_quote_records", "POST", 1},
		{"authority binding", "query", "managed_hosted_grant_records", "POST", 1},
		{"response journal", "query", "managed_journal_request_records", "POST", 1},
		{"response hold", "query", "managed_funds_reservation_records", "POST", 1},
		{"response price", "query", "managed_price_snapshot_records", "POST", 1},
		{"read quote", "query", "managed_financial_quote_records", "GET", 1},
		{"read journal", "query", "managed_journal_request_records", "GET", 1},
		{"read hold", "query", "managed_funds_reservation_records", "GET", 1},
		{"read price", "query", "managed_price_snapshot_records", "GET", 1},
		{"release lock", "update", "managed_journal_request_records", "PATCH", 1},
		{"release inner lock", "update", "managed_journal_request_records", "PATCH", 2},
		{"release reread", "query", "managed_financial_quote_records", "PATCH", 2},
		{"release journal", "query", "managed_journal_request_records", "PATCH", 1},
		{"release hold", "query", "managed_funds_reservation_records", "PATCH", 1},
		{"release attempts", "query", "managed_journal_attempt_records", "PATCH", 1},
		{"release reservation", "update", "managed_funds_reservation_records", "PATCH", 1},
		{"release journal update", "update", "managed_journal_request_records", "PATCH", 3},
		{"release quote update", "update", "managed_financial_quote_records", "PATCH", 1},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newQuotedFixture(t)
			path := ""
			var body any = quoteNativeInput()
			key := "fault"
			if scenario.method != "POST" {
				q := fixture.exchange(t, "POST", "", key, body, 201)
				path = "/" + q["id"].(string)
				body = nil
				if scenario.method == "PATCH" {
					body = map[string]any{"revision": 1, "state": "released"}
				}
			}
			hits := 0
			admissionStarted := false
			fixture.database.database.Callback().Query().After("gorm:query").Register("quote:admission-phase", func(tx *gorm.DB) {
				if tx.Statement.Table == "managed_financial_quote_records" {
					admissionStarted = true
				}
			})
			defer fixture.database.database.Callback().Query().Remove("quote:admission-phase")
			sentinel := errors.New("controlled quoted persistence interruption")
			fault := func(tx *gorm.DB) {
				if tx.Statement.Table == scenario.table {
					if (scenario.table == "managed_hosted_grant_records" || scenario.table == "managed_billing_account_records") && !admissionStarted {
						return
					}
					hits++
					if hits == scenario.nth {
						tx.AddError(sentinel)
					}
				}
			}
			callback := fixture.database.database.Callback()
			name := "quoted:io-failure"
			switch scenario.operation {
			case "query":
				callback.Query().Before("gorm:query").Register(name, fault)
				defer callback.Query().Remove(name)
			case "create":
				callback.Create().Before("gorm:create").Register(name, fault)
				defer callback.Create().Remove(name)
			case "update":
				callback.Update().Before("gorm:update").Register(name, fault)
				defer callback.Update().Remove(name)
			}
			// Do not decode success as an injected failure. The public endpoint must expose503.
			data, _ := json.Marshal(body)
			request, _ := http.NewRequest(scenario.method, fixture.server.URL+llmproxycontract.FinancialQuotesPath+path, bytes.NewReader(data))
			request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			payload, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if hits < scenario.nth || response.StatusCode != 503 {
				t.Fatalf("hits=%d status=%d body=%s", hits, response.StatusCode, payload)
			}
			switch scenario.operation {
			case "query":
				callback.Query().Remove(name)
			case "create":
				callback.Create().Remove(name)
			case "update":
				callback.Update().Remove(name)
			}
			if fixture.calls.Load() != 0 {
				t.Fatal("persistence failure dispatched")
			}
			if scenario.method == "POST" {
				fixture.exchange(t, "POST", "", key, quoteNativeInput(), 201)
			} else {
				q := fixture.exchange(t, "GET", path, "", nil, 200)
				if q["state"] != "reserved" {
					t.Fatal("failed write changed quote", q)
				}
			}
		})
	}
}

func TestHostedQuoteUnavailableServicesAndBodyLimits(t *testing.T) {
	for _, name := range []string{"database", "registry", "text", "media", "body limit"} {
		t.Run(name, func(t *testing.T) {
			fixture := newQuotedFixture(t)
			input := quoteNativeInput()
			want := 503
			switch name {
			case "database":
				fixture.runtime.database = nil
			case "registry":
				fixture.runtime.providers = nil
			case "text":
				fixture.runtime.text = nil
			case "media":
				input["intent"].(map[string]any)["kind"] = "media_operation"
			case "body limit":
				fixture.runtime.maxBytes = 1
				want = 400
			}
			fixture.exchange(t, "POST", "", "unavailable", input, want)
			if name == "database" {
				fixture.exchange(t, "GET", "", "unavailable", nil, 503)
			}
			if fixture.calls.Load() != 0 {
				t.Fatal("unavailable service dispatched")
			}
		})
	}
}

func TestHostedQuoteUnauthenticatedRequestsCannotReadOrReserve(t *testing.T) {
	fixture := newQuotedFixture(t)
	for _, method := range []string{"GET", "POST"} {
		request, _ := http.NewRequest(method, fixture.server.URL+llmproxycontract.FinancialQuotesPath, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 403 && response.StatusCode != 405 {
			t.Fatalf("%s status=%d", method, response.StatusCode)
		}
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("unauthenticated request dispatched")
	}
}

func TestHostedQuoteEntropyFailuresRetainNoHold(t *testing.T) {
	for _, failAt := range []int64{2, 3} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			fixture := newQuotedFixture(t)
			source := &mediaWorkerEntropyFailure{next: rand.Reader, failAt: failAt}
			func() {
				rand.Reader = source
				defer func() { rand.Reader = source.next }()
				fixture.exchange(t, "POST", "", "entropy", quoteNativeInput(), 503)
			}()
			if !source.failed.Load() || fixture.calls.Load() != 0 {
				t.Fatal("entropy rejection not exercised")
			}
			fixture.exchange(t, "POST", "", "entropy", quoteNativeInput(), 201)
		})
	}
}

func TestHostedQuoteConsumptionPersistenceFailures(t *testing.T) {
	for _, scenario := range []struct {
		name, operation, table string
		nth                    int
	}{
		{"request lock", "update", "managed_journal_request_records", 1},
		{"quote reread", "query", "managed_financial_quote_records", 2},
		{"request read", "query", "managed_journal_request_records", 1},
		{"authority read", "query", "managed_hosted_grant_records", 1},
		{"request transition", "update", "managed_journal_request_records", 2},
		{"quote transition", "update", "managed_financial_quote_records", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newQuotedFixture(t)
			q := fixture.exchange(t, "POST", "", "consume-fault", quoteNativeInput(), 201)
			path := "/" + q["id"].(string)
			hits := 0
			name := "quote:consume-failure"
			fault := func(tx *gorm.DB) {
				if tx.Statement.Table == scenario.table {
					hits++
					if hits == scenario.nth {
						tx.AddError(errors.New("controlled consume IO failure"))
					}
				}
			}
			callbacks := fixture.database.database.Callback()
			if scenario.operation == "query" {
				callbacks.Query().Before("gorm:query").Register(name, fault)
			} else {
				callbacks.Update().Before("gorm:update").Register(name, fault)
			}
			data, _ := json.Marshal(quoteNativeInput())
			request, _ := http.NewRequest("POST", fixture.server.URL+llmproxycontract.FinancialQuotesPath+path+"/executions", bytes.NewReader(data))
			request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			payload, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if scenario.operation == "query" {
				callbacks.Query().Remove(name)
			} else {
				callbacks.Update().Remove(name)
			}
			if hits < scenario.nth || response.StatusCode < 400 {
				t.Fatalf("hits=%d status=%d body=%s", hits, response.StatusCode, payload)
			}
			if fixture.calls.Load() != 0 {
				t.Fatal("failure dispatched")
			}
			retained := fixture.exchange(t, "GET", path, "", nil, 200)
			if retained["state"] != "reserved" {
				t.Fatal("failed consume changed quote", retained)
			}
			fixture.exchange(t, "POST", path+"/executions", "", quoteNativeInput(), 200)
		})
	}
}

func TestHostedQuoteReleaseRequiresConsistentUndispatchedProof(t *testing.T) {
	for _, scenario := range []string{"journal state", "reservation state", "attempt proof", "quote revision"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newQuotedFixture(t)
			q := fixture.exchange(t, "POST", "", "release-proof", quoteNativeInput(), 201)
			path := "/" + q["id"].(string)
			db := fixture.database.database
			switch scenario {
			case "journal state":
				if err := db.Model(&managedJournalRequestRecord{}).Where("id = ?", q["request_id"]).Update("state", journalRequestAccepted).Error; err != nil {
					t.Fatal(err)
				}
			case "reservation state":
				if err := db.Model(&managedFundsReservationRecord{}).Where("request_id = ?", q["request_id"]).Update("state", fundsReservationReconciliation).Error; err != nil {
					t.Fatal(err)
				}
			case "attempt proof":
				db.Callback().Query().After("gorm:query").Register("quote:attempt-proof", func(tx *gorm.DB) {
					if tx.Statement.Table == "managed_journal_attempt_records" {
						if count, ok := tx.Statement.Dest.(*int64); ok {
							*count = 1
						}
					}
				})
				defer db.Callback().Query().Remove("quote:attempt-proof")
			case "quote revision":
				db.Callback().Update().Before("gorm:update").Register("quote:revision-race", func(tx *gorm.DB) {
					if tx.Statement.Table == "managed_financial_quote_records" {
						tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "1=0"}}})
					}
				})
				defer db.Callback().Update().Remove("quote:revision-race")
			}
			fixture.exchange(t, "PATCH", path, "", map[string]any{"state": "released", "revision": 1}, 409)
			if fixture.calls.Load() != 0 {
				t.Fatal("proof failure dispatched")
			}
			retained := fixture.exchange(t, "GET", path, "", nil, 200)
			if retained["state"] != "reserved" {
				t.Fatal("proof failure released quote", retained)
			}
		})
	}
}

func TestHostedQuoteSavedExecutionReadsNeverStartWork(t *testing.T) {
	fixture := newQuotedFixture(t)
	q := fixture.exchange(t, "POST", "", "pending-read", quoteNativeInput(), 201)
	path := "/" + q["id"].(string)
	// Model a restart immediately after the durable consume transaction, before dispatch.
	if err := fixture.database.database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&managedFinancialQuoteRecord{}).Where("id = ?", q["id"]).Update("state", quoteConsumed).Error; err != nil {
			return err
		}
		return tx.Model(&managedJournalRequestRecord{}).Where("id = ?", q["request_id"]).Update("state", journalRequestAccepted).Error
	}); err != nil {
		t.Fatal(err)
	}
	fixture.exchange(t, "GET", path+"/executions", "", nil, 202)
	fixture.runtime.text = nil
	fixture.exchange(t, "GET", path+"/executions", "", nil, 503)
	if fixture.calls.Load() != 0 {
		t.Fatal("pending read started work")
	}
}

func TestHostedQuoteImageInputAndReadBoundaries(t *testing.T) {
	fixture, original, worker := newQuotedImageFixture(t)
	for _, scenario := range []string{"invalid request", "invalid input", "invalid controls", "invalid operation"} {
		t.Run(scenario, func(t *testing.T) {
			serialized, _ := json.Marshal(original)
			var input map[string]any
			json.Unmarshal(serialized, &input)
			request := input["intent"].(map[string]any)["request"].(map[string]any)
			switch scenario {
			case "invalid request":
				request["model"] = ""
			case "invalid input":
				request["input"] = []any{}
			case "invalid controls":
				request["controls"] = []any{}
			case "invalid operation":
				request["input"] = map[string]any{}
			}
			fixture.exchange(t, "POST", "", "invalid-image", input, 400)
		})
	}
	q := fixture.exchange(t, "POST", "", "image-read", original, 201)
	path := "/" + q["id"].(string)
	fixture.exchange(t, "POST", path+"/executions", "", original, 202)
	fixture.runtime.media = nil
	fixture.exchange(t, "GET", path+"/executions", "", nil, 503)
	fixture.runtime.media = worker
	name := "quote:image-read-failure"
	worker.store.database.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "media_operation_asset_reference_records" {
			tx.AddError(errors.New("controlled image read failure"))
		}
	})
	fixture.exchange(t, "GET", path+"/executions", "", nil, 503)
	worker.store.database.Callback().Query().Remove(name)
	if fixture.calls.Load() != 0 {
		t.Fatal("quote/read invoked image provider")
	}
}

func TestHostedQuoteConsumptionRechecksStoredQuote(t *testing.T) {
	for _, scenario := range []string{"digest changed", "state changed", "expired", "quote revision", "authorization IO"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newQuotedFixture(t)
			q := fixture.exchange(t, "POST", "", "consume-race", quoteNativeInput(), 201)
			path := "/" + q["id"].(string)
			db := fixture.database.database
			hits := 0
			name := "quote:concurrent-boundary"
			if scenario == "quote revision" {
				db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
					if tx.Statement.Table == "managed_financial_quote_records" {
						tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "1=0"}}})
					}
				})
				defer db.Callback().Update().Remove(name)
			} else if scenario == "authorization IO" {
				fixture.runtime.text.authorize = func(*gorm.DB, managedJournalRequestRecord, hostedCompletionIntent) error {
					return errors.New("controlled reservation reauthorization failure")
				}
			} else {
				db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
					if tx.Statement.Table != "managed_financial_quote_records" {
						return
					}
					hits++
					if hits != 2 {
						return
					}
					changes := map[string]any{}
					switch scenario {
					case "digest changed":
						changes["processing_digest"] = strings.Repeat("f", 64)
					case "state changed":
						changes["state"] = quoteReleased
					case "expired":
						changes["expires_at"] = ratingTestAcceptanceTime().Add(-time.Hour)
					}
					if err := tx.Session(&gorm.Session{NewDB: true}).Model(&managedFinancialQuoteRecord{}).Where("id = ?", q["id"]).Updates(changes).Error; err != nil {
						tx.AddError(err)
					}
				})
				defer db.Callback().Query().Remove(name)
			}
			data, _ := json.Marshal(quoteNativeInput())
			request, _ := http.NewRequest("POST", fixture.server.URL+llmproxycontract.FinancialQuotesPath+path+"/executions", bytes.NewReader(data))
			request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			payload, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode < 400 || fixture.calls.Load() != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.StatusCode, fixture.calls.Load(), payload)
			}
			retained := fixture.exchange(t, "GET", path, "", nil, 200)
			if retained["state"] != "reserved" {
				t.Fatal("failed consume did not roll back", retained)
			}
		})
	}
}

func TestHostedQuoteLookupAndMalformedExecutionBoundaries(t *testing.T) {
	fixture := newQuotedFixture(t)
	for _, method := range []string{"PATCH", "POST", "GET"} {
		path := "/bad"
		if method != "PATCH" {
			path += "/executions"
		}
		fixture.exchange(t, method, path, "", nil, 400)
	}
	input := quoteNativeInput()
	input["intent"].(map[string]any)["provider"] = "deepseek"
	fixture.exchange(t, "POST", "", "unassigned", input, 403)
	q := fixture.exchange(t, "POST", "", "malformed-execution", quoteNativeInput(), 201)
	path := "/" + q["id"].(string)
	request, _ := http.NewRequest("POST", fixture.server.URL+llmproxycontract.FinancialQuotesPath+path+"/executions", strings.NewReader("{"))
	request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal(response.StatusCode)
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("invalid execution dispatched")
	}
}

func TestHostedQuoteFailedExecutionReadHasNoAssessment(t *testing.T) {
	fixture := newQuotedFixture(t)
	fixture.output.Store(`{"id":"paid-invalid-result","status":"completed","output_text":"not structured JSON","usage":{"input_tokens":1000,"output_tokens":100}}`)
	q := fixture.exchange(t, "POST", "", "failed-read", quoteNativeInput(), 201)
	path := "/" + q["id"].(string)
	fixture.exchange(t, "POST", path+"/executions", "", quoteNativeInput(), 502)
	fixture.exchange(t, "GET", path+"/executions", "", nil, 502)
	if fixture.calls.Load() != 1 {
		t.Fatal("failed read redispatched")
	}
}

type quotedAssetBoundaryAdapter struct {
	MediaOperationAdapter
	after func()
}

func (adapter quotedAssetBoundaryAdapter) Validate(ctx context.Context, input MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	result, err := adapter.MediaOperationAdapter.Validate(ctx, input)
	if err == nil && adapter.after != nil {
		adapter.after()
	}
	return result, err
}

func TestHostedQuoteImageEditPinsExactAssetAndRejectsMetadataLoss(t *testing.T) {
	fixture, input, worker := newQuotedImageFixture(t)
	settings := hostedImageFinancialSettings(t, ModelOperationImageEditing)
	worker.catalog = settings.catalog
	worker.hostedAdmission = settings.mediaAdmission(worker.providers)
	if err := fixture.database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", hostedJournalFixtureGrantID).Update("offerings", []byte(`[{"model":"gpt-image-2","operations":["image_editing"]}]`)).Error; err != nil {
		t.Fatal(err)
	}
	sourceKey := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")
	key := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageEdit, "openai", "gpt-image-2")
	worker.adapters[key] = worker.adapters[sourceKey]
	asset, err := worker.assets.upload(tenant{identifier: tenantID("managed-first")}, "image/png", bytes.NewReader(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))))
	if err != nil {
		t.Fatal(err)
	}
	request := input["intent"].(map[string]any)["request"].(map[string]any)
	request["capability"] = "image.edit"
	request["input"].(map[string]any)["image_asset_ids"] = []string{asset.AssetID}
	fixture.exchange(t, "POST", "", "edit-asset", input, 201)
	worker.adapters[key] = quotedAssetBoundaryAdapter{MediaOperationAdapter: worker.adapters[key], after: func() { worker.assets.cleanupError = errors.New("controlled source metadata loss") }}
	fixture.exchange(t, "POST", "", "edit-metadata-loss", input, 400)
	if fixture.calls.Load() != 0 {
		t.Fatal("quote validated by dispatch")
	}
}

func TestHostedQuoteReplayStorageFailureStaysReadOnly(t *testing.T) {
	fixture := newQuotedFixture(t)
	q := fixture.exchange(t, "POST", "", "read-io", quoteNativeInput(), 201)
	path := "/" + q["id"].(string)
	fixture.exchange(t, "POST", path+"/executions", "", quoteNativeInput(), 200)
	fixture.runtime.text.responses.read = func(string) (structuredRequestRecord, error) {
		return structuredRequestRecord{}, errors.New("controlled saved result read failure")
	}
	fixture.exchange(t, "GET", path+"/executions", "", nil, 503)
	if fixture.calls.Load() != 1 {
		t.Fatal("storage failure redispatched")
	}
}

func TestHostedQuoteExpiryIOFailuresKeepHoldUntilRetry(t *testing.T) {
	for _, scenario := range []struct {
		name, operation, table string
		nth                    int
	}{
		{"expired list", "query", "managed_financial_quote_records", 1},
		{"request lock", "update", "managed_journal_request_records", 1},
		{"locked reread", "query", "managed_financial_quote_records", 2},
		{"release write", "update", "managed_financial_quote_records", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newQuotedFixture(t)
			q := fixture.exchange(t, "POST", "", "expiry-io", quoteNativeInput(), 201)
			path := "/" + q["id"].(string)
			db := fixture.database.database
			hits := 0
			name := "quote:expiry-io"
			fault := func(tx *gorm.DB) {
				if tx.Statement.Table == scenario.table {
					hits++
					if hits == scenario.nth {
						tx.AddError(errors.New("controlled expiry IO interruption"))
					}
				}
			}
			if scenario.operation == "query" {
				db.Callback().Query().Before("gorm:query").Register(name, fault)
			} else {
				db.Callback().Update().Before("gorm:update").Register(name, fault)
			}
			err := fixture.database.expireFinancialQuotes(t.Context(), ratingTestAcceptanceTime().Add(time.Hour))
			if scenario.operation == "query" {
				db.Callback().Query().Remove(name)
			} else {
				db.Callback().Update().Remove(name)
			}
			if err == nil || hits < scenario.nth {
				t.Fatalf("hits=%d err=%v", hits, err)
			}
			retained := fixture.exchange(t, "GET", path, "", nil, 200)
			if retained["state"] != "reserved" {
				t.Fatal("interrupted expiry changed state", retained)
			}
			if err := fixture.database.expireFinancialQuotes(t.Context(), ratingTestAcceptanceTime().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			expired := fixture.exchange(t, "GET", path, "", nil, 200)
			if expired["state"] != "expired" {
				t.Fatal("retry failed", expired)
			}
			fixture.exchange(t, "PATCH", path, "", map[string]any{"revision": expired["revision"], "state": "released"}, 409)
			if fixture.calls.Load() != 0 {
				t.Fatal("expiry dispatched")
			}
		})
	}
}

func TestHostedQuoteMaximumIsEnforcedBeforeExecution(t *testing.T) {
	fixture := newQuotedFixture(t)
	fixture.exchange(t, "POST", "", "first-hold", quoteNativeInput(), 201)
	fixture.exchange(t, "POST", "", "second-hold", quoteNativeInput(), 402)
	if fixture.calls.Load() != 0 {
		t.Fatal("insufficient quote dispatched")
	}
}

func TestHostedQuoteExpirySkipsConcurrentConsumption(t *testing.T) {
	fixture := newQuotedFixture(t)
	q := fixture.exchange(t, "POST", "", "expiry-consumed", quoteNativeInput(), 201)
	db := fixture.database.database
	hits := 0
	name := "quote:expiry-consume"
	db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "managed_financial_quote_records" {
			return
		}
		hits++
		if hits != 2 {
			return
		}
		clean := tx.Session(&gorm.Session{NewDB: true})
		if err := clean.Model(&managedFinancialQuoteRecord{}).Where("id = ?", q["id"]).Update("state", quoteConsumed).Error; err != nil {
			tx.AddError(err)
			return
		}
		if err := clean.Model(&managedJournalRequestRecord{}).Where("id = ?", q["request_id"]).Update("state", journalRequestAccepted).Error; err != nil {
			tx.AddError(err)
		}
	})
	err := fixture.database.expireFinancialQuotes(t.Context(), ratingTestAcceptanceTime().Add(time.Hour))
	db.Callback().Query().Remove(name)
	if err != nil {
		t.Fatal(err)
	}
	retained := fixture.exchange(t, "GET", "/"+q["id"].(string), "", nil, 200)
	if retained["state"] != "consumed" {
		t.Fatal(retained)
	}
	assertHostedFundsBalance(t, fixture.database, 5, 2)
	if fixture.calls.Load() != 0 {
		t.Fatal("expiry executed consumed request")
	}
}

func TestHostedQuoteImageExecutionIOBoundaries(t *testing.T) {
	for _, scenario := range []string{"operation creation", "response publication", "journal read", "missing operation"} {
		t.Run(scenario, func(t *testing.T) {
			fixture, input, worker := newQuotedImageFixture(t)
			q := fixture.exchange(t, "POST", "", "image-io", input, 201)
			path := "/" + q["id"].(string)
			db := fixture.database.database
			name := "quote:image-io"
			hits := 0
			if scenario == "journal read" || scenario == "missing operation" {
				fixture.exchange(t, "POST", path+"/executions", "", input, 202)
			}
			if scenario == "operation creation" {
				db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
					if tx.Statement.Table == "media_operation_records" {
						hits++
						tx.AddError(errors.New("controlled operation creation failure"))
					}
				})
			} else {
				worker.store.database.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
					table := "media_operation_asset_reference_records"
					if scenario == "journal read" {
						table = "managed_journal_request_records"
					}
					if scenario == "missing operation" {
						table = "media_operation_records"
					}
					if tx.Statement.Table == table {
						hits++
						if scenario == "missing operation" {
							tx.AddError(gorm.ErrRecordNotFound)
						} else {
							tx.AddError(errors.New("controlled result read failure"))
						}
					}
				})
			}
			method := "POST"
			var body any = input
			status := 503
			if scenario == "journal read" || scenario == "missing operation" {
				method = "GET"
				body = nil
			}
			if scenario == "missing operation" {
				status = 404
			}
			fixture.exchange(t, method, path+"/executions", "", body, status)
			db.Callback().Create().Remove(name)
			worker.store.database.Callback().Query().Remove(name)
			if hits == 0 || fixture.calls.Load() != 0 {
				t.Fatalf("hits=%d provider=%d", hits, fixture.calls.Load())
			}
			retained := fixture.exchange(t, "GET", path, "", nil, 200)
			want := "consumed"
			if scenario == "operation creation" {
				want = "reserved"
			}
			if retained["state"] != want {
				t.Fatalf("quote state=%v want=%s", retained, want)
			}
		})
	}
}

func TestHostedQuoteLedgerReleaseFailureRollsBack(t *testing.T) {
	fixture := newQuotedFixture(t)
	q := fixture.exchange(t, "POST", "", "ledger-release", quoteNativeInput(), 201)
	path := "/" + q["id"].(string)
	db := fixture.database.database
	if err := db.Exec("CREATE TRIGGER reject_quoted_ledger_release BEFORE UPDATE ON reservations BEGIN SELECT RAISE(ABORT, 'controlled_quote_ledger_release'); END").Error; err != nil {
		t.Fatal(err)
	}
	fixture.exchange(t, "PATCH", path, "", map[string]any{"state": "released", "revision": 1}, 503)
	if err := db.Exec("DROP TRIGGER reject_quoted_ledger_release").Error; err != nil {
		t.Fatal(err)
	}
	retained := fixture.exchange(t, "GET", path, "", nil, 200)
	if retained["state"] != "reserved" {
		t.Fatal("failed ledger release changed quote", retained)
	}
	assertHostedFundsBalance(t, fixture.database, 5, 2)
	fixture.exchange(t, "PATCH", path, "", map[string]any{"state": "released", "revision": 1}, 200)
	assertHostedFundsBalance(t, fixture.database, 5, 5)
}

func TestHostedQuoteExpiredReadDoesNotConsumeOrRelease(t *testing.T) {
	fixture := newQuotedFixture(t)
	q := fixture.exchange(t, "POST", "", "elapsed-before-dispatch", quoteNativeInput(), 201)
	path := "/" + q["id"].(string)
	fixture.runtime.now = func() time.Time { return ratingTestAcceptanceTime().Add(time.Hour) }
	fixture.exchange(t, "POST", path+"/executions", "", quoteNativeInput(), 409)
	retained := fixture.exchange(t, "GET", path, "", nil, 200)
	if retained["state"] != "reserved" {
		t.Fatal("elapsed read mutated quote", retained)
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("elapsed quote dispatched")
	}
}
