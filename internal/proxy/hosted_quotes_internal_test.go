package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type quotedFixture struct {
	server      *httptest.Server
	database    *gormManagedTenantDatabase
	calls       *atomic.Int64
	runtime     *applicationQuoteRuntime
	output      *atomic.Value
	providerURL string
	root        string
	prices      journalReservation
}

func newQuotedFixture(t *testing.T) quotedFixture {
	t.Helper()
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	calls := &atomic.Int64{}
	output := &atomic.Value{}
	output.Store(`{"id":"quoted-provider-result","status":"completed","output_text":"{\"answer\":\"quoted\"}","usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, output.Load().(string))
	}))
	t.Cleanup(provider.Close)
	root := t.TempDir()
	router, management, coordinator := newHostedIdentityHTTPHandler(t, database, provider.URL, root, fundsDependencies(prices))
	runtime := &applicationQuoteRuntime{database: database, text: coordinator.hostedText, providers: management.providers, coordinator: coordinator, assets: newTenantAssetStore(root, 4096, 60), store: management.store, logger: zap.NewNop().Sugar(), now: ratingTestAcceptanceTime}
	policy, err := newRequestTimeoutPolicy(30, 30)
	if err != nil {
		t.Fatal(err)
	}
	registerApplicationQuoteRoutes(router, newTenantAuthenticator(management.store), policy, runtime)
	registerApplicationFinancialRoutes(router, newTenantAuthenticator(management.store), zap.NewNop().Sugar(), database)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return quotedFixture{server: server, database: database, calls: calls, runtime: runtime, output: output, providerURL: provider.URL, root: root, prices: prices}
}
func quoteNativeInput() map[string]any {
	return map[string]any{"processing_digest": strings.Repeat("a", 64), "intent": map[string]any{"kind": "native_v2", "provider": "openai", "request": map[string]any{"model": "gpt-4.1", "messages": []map[string]string{{"role": "user", "content": "quoted private input"}}, "structured_output": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}}, "required": []string{"answer"}, "additionalProperties": false}}}}}
}
func (fixture quotedFixture) exchange(t *testing.T, method, path, key string, body any, want int) map[string]any {
	t.Helper()
	var content []byte
	if body != nil {
		var err error
		content, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequest(method, fixture.server.URL+llmproxycontract.FinancialQuotesPath+path, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
	if key != "" {
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if want != 0 && response.StatusCode != want {
		t.Fatalf("%s%s status=%d want=%d body=%s", method, path, response.StatusCode, want, data)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("financial quote response cacheable")
	}
	contract, contractErr := internalCanonicalOpenAPIContract()
	if contractErr != nil {
		t.Fatal(contractErr)
	}
	template := llmproxycontract.FinancialQuotesPath
	if path != "" {
		template += "/{quote_id}"
		if strings.HasSuffix(path, "/executions") {
			template += "/executions"
		}
	}
	if err := contract.ValidateResponse(template, method, response.StatusCode, response.Header, data); err != nil {
		t.Fatalf("quoted response contract: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func TestHostedQuoteNativeBoundExecutionAndRead(t *testing.T) {
	fixture := newQuotedFixture(t)
	input := quoteNativeInput()
	quote := fixture.exchange(t, "POST", "", "quote-one", input, 201)
	if quote["state"] != "reserved" || quote["maximum_cents"] != "3" || quote["currency"] != "USD" || fixture.calls.Load() != 0 {
		t.Fatalf("quote=%v calls=%d", quote, fixture.calls.Load())
	}
	fixture.exchange(t, "GET", "", "quote-one", nil, 200)
	replay := fixture.exchange(t, "POST", "", "quote-one", input, 200)
	if replay["id"] != quote["id"] {
		t.Fatal("lost quote response created second quote")
	}
	path := "/" + quote["id"].(string)
	fixture.exchange(t, "GET", path, "", nil, 200)
	if fixture.calls.Load() != 0 {
		t.Fatal("quote/read dispatched provider")
	}
	result := fixture.exchange(t, "POST", path+"/executions", "", input, 200)
	if result["answer"] != "quoted" {
		t.Fatal(result)
	}
	fixture.exchange(t, "GET", path+"/executions", "", nil, 200)
	fixture.exchange(t, "POST", path+"/executions", "", input, 200)
	if fixture.calls.Load() != 1 {
		t.Fatal("quoted replay repeated provider execution")
	}
	consumed := fixture.exchange(t, "GET", path, "", nil, 200)
	if consumed["state"] != "consumed" {
		t.Fatal(consumed)
	}
	fixture.exchange(t, "PATCH", path, "", map[string]any{"state": "released", "revision": consumed["revision"]}, 409)
}
func TestHostedQuoteReleaseExpiryAndChangedIntent(t *testing.T) {
	fixture := newQuotedFixture(t)
	input := quoteNativeInput()
	quote := fixture.exchange(t, "POST", "", "release-one", input, 201)
	path := "/" + quote["id"].(string)
	changed := quoteNativeInput()
	changed["processing_digest"] = strings.Repeat("b", 64)
	fixture.exchange(t, "POST", path+"/executions", "", changed, 409)
	released := fixture.exchange(t, "PATCH", path, "", map[string]any{"state": "released", "revision": quote["revision"]}, 200)
	if released["state"] != "released" {
		t.Fatal(released)
	}
	fixture.exchange(t, "PATCH", path, "", map[string]any{"state": "released", "revision": quote["revision"]}, 200)
	fixture.exchange(t, "POST", path+"/executions", "", input, 409)
	fixture.exchange(t, "POST", "", "release-one", input, 200)
	quote = fixture.exchange(t, "POST", "", "expiry-one", input, 201)
	path = "/" + quote["id"].(string)
	fixture.runtime.now = func() time.Time { return ratingTestAcceptanceTime().Add(time.Hour) }
	// A pure GET does not process expiration or release a hold.
	if read := fixture.exchange(t, "GET", path, "", nil, 200); read["state"] != "reserved" {
		t.Fatal(read)
	}
	if err := fixture.database.reconcileHostedFunds(t.Context(), fixture.runtime.now()); err != nil {
		t.Fatal(err)
	}
	if read := fixture.exchange(t, "GET", path, "", nil, 200); read["state"] != "expired" {
		t.Fatal(read)
	}
	fixture.exchange(t, "POST", path+"/executions", "", input, 409)
	if fixture.calls.Load() != 0 {
		t.Fatal("unconsumed quote called provider")
	}
	assertHostedFundsBalance(t, fixture.database, 5, 5)
}

func TestHostedQuoteKnownProcessingFailureSettlesWithoutResult(t *testing.T) {
	fixture := newQuotedFixture(t)
	fixture.output.Store(`{"id":"paid-invalid-result","status":"completed","output_text":"not structured JSON","usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	input := quoteNativeInput()
	quote := fixture.exchange(t, "POST", "", "paid-failure", input, 201)
	path := "/" + quote["id"].(string)
	fixture.exchange(t, "POST", path+"/executions", "", input, 502)
	if err := deliverFundsFixture(t, fixture.database); err != nil {
		t.Fatal(err)
	}
	var request managedJournalRequestRecord
	if err := fixture.database.database.First(&request, "id = ?", quote["request_id"]).Error; err != nil {
		t.Fatal(err)
	}
	if request.State != journalRequestFailed {
		t.Fatalf("failed processing state=%s", request.State)
	}
	var reservation managedFundsReservationRecord
	if err := fixture.database.database.First(&reservation, "request_id = ?", quote["request_id"]).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.State != fundsReservationSettled {
		t.Fatalf("known paid failure retained hold: %s", reservation.State)
	}
	var settlement managedFundsSettlementRecord
	if err := fixture.database.database.First(&settlement, "request_id = ?", quote["request_id"]).Error; err != nil {
		t.Fatal(err)
	}
	if settlement.ChargeNumerator == "0" {
		t.Fatal("known processing failure invented zero cost")
	}
	fixture.exchange(t, "GET", path+"/executions", "", nil, 502)
	if fixture.calls.Load() == 0 {
		t.Fatal("fixture did not execute actual provider protocol")
	}
}

type previousQuotedJournalConstraint struct {
	State journalRequestState `gorm:"check:chk_managed_journal_request_records_state,state IN ('accepted','executing','completed','failed','uncertain')"`
}

func (previousQuotedJournalConstraint) TableName() string { return "managed_journal_request_records" }
func TestHostedQuoteSchemaMigratesExactPredecessorWithFinancialHistory(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	hostedIdentityHTTP(t, fixture.generation, "quote-migration-history", "funded prompt", http.StatusOK)
	if err := deliverFundsFixture(t, fixture.database); err != nil {
		t.Fatal(err)
	}
	before := fixture.state(t)

	makePreviousQuoteSchema(t, fixture.database)
	_ = openJournalTransactionInstance(t, fixture.database)

	if !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatal("quote schema transition changed retained financial history")
	}
	if err := fixture.database.database.Transaction(initializeHostedFinancialQuoteSchema); err != nil {
		t.Fatal(err)
	}
}

func makePreviousQuoteSchema(t *testing.T, database *gormManagedTenantDatabase) {
	t.Helper()
	err := database.database.Connection(func(connection *gorm.DB) error {
		connection = connection.Session(&gorm.Session{NewDB: true})
		if err := connection.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		defer connection.Exec("PRAGMA foreign_keys = ON")
		return connection.Transaction(func(tx *gorm.DB) error {
			objects, err := journalSchemaObjects(tx)
			if err != nil {
				return err
			}
			if err := tx.Migrator().DropTable(&managedFinancialQuoteRecord{}); err != nil {
				return err
			}
			if err := tx.Migrator().DropConstraint(&managedJournalRequestRecord{}, journalQuoteConstraint); err != nil {
				return err
			}
			if err := tx.Migrator().CreateConstraint(&previousQuotedJournalConstraint{}, journalQuoteConstraint); err != nil {
				return err
			}
			for _, object := range objects {
				if err := tx.Exec(object.SQL).Error; err != nil {
					return err
				}
			}
			return requireNoForeignKeyViolations(tx)
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHostedQuoteMigrationRollbackRestoresConnectionAndHistory(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	hostedIdentityHTTP(t, fixture.generation, "quote-rollback-history", "funded prompt", 200)
	if err := deliverFundsFixture(t, fixture.database); err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.database.Exec("CREATE TRIGGER quote_migration_history_guard BEFORE UPDATE ON managed_journal_request_records WHEN NEW.id <> OLD.id BEGIN SELECT RAISE(ABORT, 'immutable journal identity'); END").Error; err != nil {
		t.Fatal(err)
	}
	before := fixture.state(t)
	makePreviousQuoteSchema(t, fixture.database)
	objects, err := journalSchemaObjects(fixture.database.database)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("controlled quote migration rollback")
	err = initializeWithHostedQuoteBoundary(fixture.database.database, func(tx *gorm.DB) error {
		if err := initializeHostedFinancialQuoteSchema(tx); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error=%v", err)
	}
	var enabled int
	if err := fixture.database.database.Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil || enabled != 1 {
		t.Fatalf("foreign keys=%d error=%v", enabled, err)
	}
	predecessor, err := quoteSchemaPredecessor(fixture.database.database)
	if err != nil || !predecessor {
		t.Fatalf("rollback lost predecessor: %v", err)
	}
	restored, err := journalSchemaObjects(fixture.database.database)
	if err != nil || !reflect.DeepEqual(objects, restored) || !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatal("rollback changed financial rows or schema objects")
	}
	_ = openJournalTransactionInstance(t, fixture.database)
	restored, err = journalSchemaObjects(fixture.database.database)
	if err != nil || !reflect.DeepEqual(objects, restored) {
		t.Fatal("migration dropped retained trigger/index objects")
	}
}
func TestHostedQuoteConcurrentMigrationAndRestart(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	hostedIdentityHTTP(t, fixture.generation, "quote-concurrent-history", "funded prompt", 200)
	if err := deliverFundsFixture(t, fixture.database); err != nil {
		t.Fatal(err)
	}
	before := fixture.state(t)
	makePreviousQuoteSchema(t, fixture.database)
	var group sync.WaitGroup
	start := make(chan struct{})
	failures := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			instance, err := newGORMManagedTenantDatabase(ManagementConfiguration{DatabaseDialector: fixture.database.database.Dialector}, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
			if err == nil {
				err = instance.close()
			}
			failures <- err
		}()
	}
	close(start)
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatal("concurrent startup changed retained financial evidence")
	}
	_ = openJournalTransactionInstance(t, fixture.database)
}

func newQuoteReplica(t *testing.T, first quotedFixture) quotedFixture {
	t.Helper()
	database := openJournalTransactionInstance(t, first.database)
	router, management, coordinator := newHostedIdentityHTTPHandler(t, database, first.providerURL, first.root, fundsDependencies(first.prices))
	runtime := &applicationQuoteRuntime{database: database, text: coordinator.hostedText, providers: management.providers, coordinator: coordinator, assets: newTenantAssetStore(first.root, 4096, 60), store: management.store, logger: zap.NewNop().Sugar(), now: ratingTestAcceptanceTime}
	policy, err := newRequestTimeoutPolicy(30, 30)
	if err != nil {
		t.Fatal(err)
	}
	registerApplicationQuoteRoutes(router, newTenantAuthenticator(management.store), policy, runtime)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return quotedFixture{server: server, database: database, calls: first.calls, runtime: runtime, output: first.output, providerURL: first.providerURL, root: first.root, prices: first.prices}
}
func TestHostedQuoteTwoReplicasReserveAndExecuteOnce(t *testing.T) {
	first := newQuotedFixture(t)
	second := newQuoteReplica(t, first)
	replicas := []quotedFixture{first, second}
	input := quoteNativeInput()
	results := make([]map[string]any, 2)
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index] = replicas[index].exchange(t, "POST", "", "replica-one", input, 0)
		}(index)
	}
	group.Wait()
	if results[0]["id"] != results[1]["id"] {
		t.Fatal("concurrent quotes created different holds")
	}
	assertHostedFundsBalance(t, first.database, 5, 2)
	if first.calls.Load() != 0 {
		t.Fatal("quote called provider")
	}
	path := "/" + results[0]["id"].(string)
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index] = replicas[index].exchange(t, "POST", path+"/executions", "", input, 0)
		}(index)
	}
	group.Wait()
	if first.calls.Load() != 1 {
		t.Fatalf("replica execution calls=%d", first.calls.Load())
	}
	for _, replica := range replicas {
		result := replica.exchange(t, "GET", path+"/executions", "", nil, 200)
		if result["answer"] != "quoted" {
			t.Fatal(result)
		}
	}
	assertHostedFundsBalance(t, first.database, 5, 2)
}
func TestHostedQuoteOrdinaryKeyCannotSpendQuotedHold(t *testing.T) {
	fixture := newQuotedFixture(t)
	input := quoteNativeInput()
	quote := fixture.exchange(t, "POST", "", "same-key", input, 201)
	fixture.server.Client().Transport = hostedIdentityTransport{next: http.DefaultTransport}
	hostedIdentityHTTP(t, fixture.server, "same-key", "ordinary different request", http.StatusPaymentRequired)
	if fixture.calls.Load() != 0 {
		t.Fatal("ordinary route consumed quoted hold")
	}
	read := fixture.exchange(t, "GET", "/"+quote["id"].(string), "", nil, 200)
	if read["state"] != "reserved" {
		t.Fatal(read)
	}
	assertHostedFundsBalance(t, fixture.database, 5, 2)
}

func TestHostedQuoteOfficialSDKNativeRoundTrip(t *testing.T) {
	fixture := newQuotedFixture(t)
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: fixture.server.URL, Secret: hostedIdentityFixtureKey, Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: "gpt-4.1", Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Answer the fictional question"}}, StructuredOutput: &llmproxyclient.StructuredOutputInput{JSONSchema: []byte(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`)}, IdempotencyKey: "constructor-only-unused"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := llmproxyclient.NewNativeFinancialQuoteInput(strings.Repeat("a", 64), "openai", request)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := client.CreateFinancialQuote(t.Context(), "go-native-quote", input)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := quote.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetFinancialQuote(t.Context(), "go-native-quote"); err != nil {
		t.Fatal(err)
	}
	restored, err := client.GetFinancialQuoteByID(t.Context(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	financial, err := client.GetFinancialRequestByID(t.Context(), snapshot.RequestID)
	if err != nil || financial.Quote == nil || financial.Quote.ID != snapshot.ID || financial.Quote.ProcessingDigest != snapshot.ProcessingDigest || financial.RequestDigest != snapshot.RequestDigest {
		t.Fatalf("financial quote binding=%v error=%v", financial, err)
	}
	execution, err := client.CreateQuotedExecution(t.Context(), restored, input)
	if err != nil {
		t.Fatal(err)
	}
	native, ok := execution.NativeResult()
	if !ok || native.State != "succeeded" || native.Output != `{"answer":"quoted"}` {
		t.Fatalf("SDK result=%v", native)
	}
	if _, err := client.GetQuotedExecution(t.Context(), restored); err != nil {
		t.Fatal(err)
	}
	if err := deliverFundsFixture(t, fixture.database); err != nil {
		t.Fatal(err)
	}
	script := `import sys
from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest, ClientStructuredOutput, QuotedClient, FinancialQuoteInput
client=QuotedClient(Client(ClientConfig(base_url=sys.argv[1],secret=sys.argv[2],provider="openai")))
request=ClientMessagesRequest(messages=(ClientMessage(role='user',content='Answer the fictional question'),),model='gpt-4.1',structured_output=ClientStructuredOutput(schema={'type':'object','properties':{'answer':{'type':'string'}},'required':['answer'],'additionalProperties':False}),idempotency_key='constructor-only-unused')
intent=FinancialQuoteInput.native('a'*64,'openai',request)
quote=client.create_financial_quote('python-native-quote',intent)
assert client.get_financial_quote('python-native-quote').id==quote.id
assert client.get_financial_quote_by_id(quote.id).id==quote.id
result=client.create_quoted_execution(quote,intent)
assert result.native.state=='succeeded' and result.native.output=='{"answer":"quoted"}'
assert client.get_quoted_execution(quote).native.output==result.native.output
`
	if output, err := hostedClientPythonCommand(t, script, fixture.server.URL, hostedIdentityFixtureKey).CombinedOutput(); err != nil {
		t.Fatalf("Python real quoted SDK: %s error=%v", output, err)
	}
	if fixture.calls.Load() != 2 {
		t.Fatalf("SDK provider calls=%d", fixture.calls.Load())
	}
}

func newQuotedImageFixture(t *testing.T) (quotedFixture, map[string]any, *mediaOperationService) {
	t.Helper()
	database, _, _, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 500)
	png := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))
	encoded := base64.StdEncoding.EncodeToString(png)
	calls := &atomic.Int64{}
	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":{"total_tokens":22,"input_tokens":13,"output_tokens":9,"input_tokens_details":{"text_tokens":10,"image_tokens":3},"output_tokens_details":{"text_tokens":2,"image_tokens":7}}}`, encoded)
	}))
	t.Cleanup(provider.Close)
	server, worker := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(provider.URL))
	settings := hostedImageFinancialSettings(t, ModelOperationImageGeneration)
	worker.catalog = settings.catalog
	worker.hostedAdmission = settings.mediaAdmission(worker.providers)
	manager := newInternalManagementService(t, newFakeManagedTenantDatabase(), worker.providers)
	manager.store.database = database
	policy, err := newRequestTimeoutPolicy(30, 30)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &applicationQuoteRuntime{database: database, media: worker, providers: worker.providers, store: manager.store, logger: zap.NewNop().Sugar(), now: worker.store.now}
	registerApplicationQuoteRoutes(server.Config.Handler.(*gin.Engine), newTenantAuthenticator(manager.store), policy, runtime)
	fixture := quotedFixture{server: server, database: database, calls: calls, runtime: runtime}
	input := map[string]any{"processing_digest": strings.Repeat("c", 64), "intent": map[string]any{"kind": "media_operation", "provider": "openai", "request": map[string]any{"capability": "image.generate", "provider": "openai", "model": "gpt-image-2", "input": map[string]any{"prompt": "Fictional diagram"}, "controls": map[string]any{"surface": "images", "quality": "low", "size": "1024x1024", "background": "opaque", "output_format": "png", "output_count": 1}}}}
	return fixture, input, worker
}

func TestHostedQuoteImageCapacityRejection(t *testing.T) {
	for _, scope := range []string{"tenant", "global"} {
		t.Run(scope, func(t *testing.T) {
			fixture, input, worker := newQuotedImageFixture(t)
			if scope == "tenant" {
				worker.tenantCapacity = 1
			} else {
				worker.globalCapacity = 1
			}
			first := fixture.exchange(t, "POST", "", "capacity-first", input, http.StatusCreated)
			firstPath := "/" + first["id"].(string)
			operation := fixture.exchange(t, "POST", firstPath+"/executions", "", input, http.StatusAccepted)
			second := fixture.exchange(t, "POST", "", "capacity-second", input, http.StatusCreated)
			secondPath := "/" + second["id"].(string)
			failure := fixture.exchange(t, "POST", secondPath+"/executions", "", input, http.StatusTooManyRequests)
			if failure["error"].(map[string]any)["code"] != llmproxycontract.ErrorCodeMediaOperationCapacity {
				t.Fatal(failure)
			}
			if quote := fixture.exchange(t, "GET", secondPath, "", nil, http.StatusOK); quote["state"] != quoteReserved {
				t.Fatal(quote)
			}
			if fixture.calls.Load() != 0 {
				t.Fatal("capacity rejection dispatched provider work")
			}
			worker.runOperation("quoted-capacity-worker", operation["operation_id"].(string))
			fixture.exchange(t, "POST", secondPath+"/executions", "", input, http.StatusAccepted)
			fixture.exchange(t, "POST", secondPath+"/executions", "", input, http.StatusOK)
			if fixture.calls.Load() != 1 {
				t.Fatal("retry dispatched provider work before the worker ran")
			}
		})
	}
}

func TestHostedQuoteImageExpiredExecutionReplay(t *testing.T) {
	fixture, input, worker := newQuotedImageFixture(t)
	quote := fixture.exchange(t, "POST", "", "expired-image", input, http.StatusCreated)
	path := "/" + quote["id"].(string)
	operation := fixture.exchange(t, "POST", path+"/executions", "", input, http.StatusAccepted)
	id := operation["operation_id"].(string)
	worker.runOperation("quoted-expiry-worker", id)
	if result := fixture.exchange(t, "GET", path+"/executions", "", nil, http.StatusOK); result["state"] != MediaOperationStateSucceeded {
		t.Fatal(result)
	}
	now := worker.store.now().Add(worker.terminalRetention + time.Second)
	worker.store.now = func() time.Time { return now }
	worker.expireTerminalData()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		var body any
		if method == http.MethodPost {
			body = input
		}
		failure := fixture.exchange(t, method, path+"/executions", "", body, http.StatusGone)
		if failure["error"].(map[string]any)["code"] != llmproxycontract.ErrorCodeMediaOperationExpired || failure["operation_id"] != id || failure["state"] != MediaOperationStateSucceeded {
			t.Fatal(failure)
		}
	}
	if retained := fixture.exchange(t, "GET", path, "", nil, http.StatusOK); retained["state"] != quoteConsumed {
		t.Fatal(retained)
	}
	if fixture.calls.Load() != 1 {
		t.Fatal("expired replay repeated provider execution")
	}
}

func TestHostedQuoteImageExecutionUsesSameHold(t *testing.T) {
	fixture, input, worker := newQuotedImageFixture(t)
	database, calls := fixture.database, fixture.calls
	quote := fixture.exchange(t, "POST", "", "image-one", input, 201)
	if calls.Load() != 0 {
		t.Fatal("image quote invoked provider")
	}
	assertHostedFundsBalance(t, database, 500, 461)
	var operations int64
	if err := database.database.Model(&mediaOperationRecord{}).Count(&operations).Error; err != nil || operations != 0 {
		t.Fatal("quote enqueued image work")
	}
	path := "/" + quote["id"].(string)
	operation := fixture.exchange(t, "POST", path+"/executions", "", input, 202)
	id := operation["operation_id"].(string)
	assertHostedFundsBalance(t, database, 500, 461)
	worker.runOperation("quoted-image-worker", id)
	done := fixture.exchange(t, "GET", path+"/executions", "", nil, 200)
	if done["state"] != MediaOperationStateSucceeded {
		t.Fatal(done)
	}
	fixture.exchange(t, "POST", path+"/executions", "", input, 200)
	worker.runOperation("quoted-image-repeat", id)
	if calls.Load() != 1 {
		t.Fatalf("image calls=%d", calls.Load())
	}
	if err := database.reconcileHostedFunds(t.Context(), worker.store.now()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 498, 498)
	var reservation managedFundsReservationRecord
	if err := database.database.First(&reservation, "request_id = ?", quote["request_id"]).Error; err != nil || reservation.State != fundsReservationSettled {
		t.Fatalf("image reservation=%v error=%v", reservation, err)
	}
}

func TestHostedQuoteInputAuthorityAndPurgeBoundaries(t *testing.T) {
	fixture := newQuotedFixture(t)
	input := quoteNativeInput()
	quote := fixture.exchange(t, "POST", "", "authority-one", input, 201)
	path := "/" + quote["id"].(string)
	changed := quoteNativeInput()
	changed["intent"].(map[string]any)["request"].(map[string]any)["messages"] = []map[string]string{{"role": "user", "content": "different evidence"}}
	fixture.exchange(t, "POST", path+"/executions", "", changed, 409)
	fixture.exchange(t, "POST", "", "authority-one", changed, 409)
	if err := fixture.database.database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Create(&managedHostedGrantRevisionRecord{GrantID: hostedJournalFixtureGrantID, Revision: 2, State: hostedGrantActive, ActorUserID: "owner", Reason: "Controlled policy revision", CreatedAt: ratingTestAcceptanceTime()}).Error; err != nil {
			return err
		}
		return tx.Model(&managedHostedGrantRecord{}).Where("id = ?", hostedJournalFixtureGrantID).Update("revision", 2).Error
	}); err != nil {
		t.Fatal(err)
	}
	fixture.exchange(t, "POST", path+"/executions", "", input, 403)
	if fixture.calls.Load() != 0 {
		t.Fatal("revoked revision dispatched")
	}
	fixture.exchange(t, "PATCH", path, "", map[string]any{"state": "released", "revision": quote["revision"]}, 200)
	quote = fixture.exchange(t, "POST", "", "purge-one", input, 201)
	path = "/" + quote["id"].(string)
	fixture.exchange(t, "POST", path+"/executions", "", input, 200)
	if err := deliverFundsFixture(t, fixture.database); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(fixture.root); err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", hostedJournalFixtureGrantID).Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	fixture.exchange(t, "GET", path, "", nil, 200)
	fixture.exchange(t, "GET", "", "purge-one", nil, 200)
	fixture.exchange(t, "GET", path+"/executions", "", nil, 410)
	configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: fixture.server.URL, Secret: hostedIdentityFixtureKey, Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(configuration, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	financial, err := client.GetFinancialRequestByID(t.Context(), quote["request_id"].(string))
	if err != nil || financial.Receipt == nil || financial.Quote == nil || financial.Quote.ID != quote["id"] {
		t.Fatalf("post-purge receipt=%v error=%v", financial, err)
	}
}
func TestHostedQuoteExpiryConsumeRaceAndLateReleaseProof(t *testing.T) {
	fixture := newQuotedFixture(t)
	second := newQuoteReplica(t, fixture)
	input := quoteNativeInput()
	quote := fixture.exchange(t, "POST", "", "expiry-race", input, 201)
	path := "/" + quote["id"].(string)
	// Recovery and consumption use the same database writer fence. Exactly one
	// terminal quote transition can win, and release never follows consumption.
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		if err := fixture.database.expireFinancialQuotes(t.Context(), ratingTestAcceptanceTime().Add(time.Hour)); err != nil {
			t.Error(err)
		}
	}()
	go func() { defer group.Done(); second.exchange(t, "POST", path+"/executions", "", input, 0) }()
	group.Wait()
	read := fixture.exchange(t, "GET", path, "", nil, 200)
	switch read["state"] {
	case "expired":
		if fixture.calls.Load() != 0 {
			t.Fatal("expired quote dispatched")
		}
		assertHostedFundsBalance(t, fixture.database, 5, 5)
	case "consumed":
		if fixture.calls.Load() != 1 {
			t.Fatal("consumed quote did not execute exactly once")
		}
		fixture.exchange(t, "PATCH", path, "", map[string]any{"state": "released", "revision": read["revision"]}, 409)
	default:
		t.Fatal(read)
	}
}

func TestHostedQuoteUnknownFailureRetainsFinancialUncertainty(t *testing.T) {
	fixture := newQuotedFixture(t)
	fixture.output.Store(`{"id":"unmetered-failure","status":"completed","output_text":"not structured JSON"}`)
	input := quoteNativeInput()
	quote := fixture.exchange(t, "POST", "", "unknown-failure", input, 201)
	path := "/" + quote["id"].(string)
	fixture.exchange(t, "POST", path+"/executions", "", input, 502)
	if err := deliverFundsFixture(t, fixture.database); err != nil {
		t.Fatal(err)
	}
	var reservation managedFundsReservationRecord
	if err := fixture.database.database.First(&reservation, "request_id = ?", quote["request_id"]).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.State != fundsReservationReconciliation {
		t.Fatal("unknown cost was settled or released")
	}
	var receipts int64
	if err := fixture.database.database.Model(&managedFundsSettlementRecord{}).Where("request_id = ?", quote["request_id"]).Count(&receipts).Error; err != nil || receipts != 0 {
		t.Fatal("unknown failure manufactured a cost receipt")
	}
	assertHostedFundsBalance(t, fixture.database, 5, 2)
}

type driftedQuotedJournalConstraint struct {
	State journalRequestState `gorm:"check:chk_managed_journal_request_records_state,state IN ('accepted','executing','completed','failed','uncertain','unreviewed')"`
}

func (driftedQuotedJournalConstraint) TableName() string { return "managed_journal_request_records" }
func TestHostedQuoteMigrationRejectsMalformedPredecessor(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	makePreviousQuoteSchema(t, fixture.database)
	if err := fixture.database.database.Connection(func(connection *gorm.DB) error {
		connection = connection.Session(&gorm.Session{NewDB: true})
		if err := connection.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		defer connection.Exec("PRAGMA foreign_keys = ON")
		return connection.Transaction(func(tx *gorm.DB) error {
			objects, err := journalSchemaObjects(tx)
			if err != nil {
				return err
			}
			if err := tx.Migrator().DropConstraint(&managedJournalRequestRecord{}, journalQuoteConstraint); err != nil {
				return err
			}
			if err := tx.Migrator().CreateConstraint(&driftedQuotedJournalConstraint{}, journalQuoteConstraint); err != nil {
				return err
			}
			for _, object := range objects {
				if err := tx.Exec(object.SQL).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := initializeWithHostedQuoteBoundary(fixture.database.database, initializeHostedFinancialQuoteSchema); !errors.Is(err, errManagedTenantSchemaMigration) {
		t.Fatalf("malformed predecessor accepted: %v", err)
	}
	if fixture.database.database.Migrator().HasTable(&managedFinancialQuoteRecord{}) {
		t.Fatal("malformed predecessor changed schema")
	}
}
func TestHostedQuoteMigrationRestorationFailureDiscardsPool(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	makePreviousQuoteSchema(t, fixture.database)
	callback := "quote:restore-failure"
	if err := fixture.database.database.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "PRAGMA foreign_keys = ON" {
			tx.AddError(errors.New("controlled quote FK restoration failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := initializeWithHostedQuoteBoundary(fixture.database.database, initializeHostedFinancialQuoteSchema)
	if err == nil {
		t.Fatal("unverified FK restoration permitted startup")
	}
	pool, poolErr := fixture.database.database.DB()
	if poolErr != nil {
		t.Fatal(poolErr)
	}
	if pool.Ping() == nil {
		t.Fatal("failed restoration returned unsafe connection to pool")
	}
}
