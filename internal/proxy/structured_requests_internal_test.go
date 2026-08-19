package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

type structuredRouteAdapter func(context.Context, *providerRouter, chatRequestParameters, *zap.SugaredLogger) (textGenerationResult, error)

func (adapter structuredRouteAdapter) generateText(requestContext context.Context, router *providerRouter, request chatRequestParameters, logger *zap.SugaredLogger) (textGenerationResult, error) {
	return adapter(requestContext, router, request, logger)
}

func TestStructuredRequestStoreLifecycle(testingInstance *testing.T) {
	assetRoot := testingInstance.TempDir()
	store, storeError := newStructuredRequestStore(assetRoot, 3600)
	if storeError != nil {
		testingInstance.Fatal(storeError)
	}
	requestTenant := structuredTestTenant(testingInstance, "tenant-one")
	otherTenant := structuredTestTenant(testingInstance, "tenant-two")
	intent := structuredRequestIntent("openai", "gpt-5.5", []byte(`{"messages":[{"content":"review","role":"user"}]}`))
	if intent != structuredRequestIntent("openai", "gpt-5.5", []byte(`{"messages":[{"content":"review","role":"user"}]}`)) || !digestPattern.MatchString(intent) {
		testingInstance.Fatalf("intent=%q", intent)
	}

	record, created, beginError := store.begin(requestTenant, "review:key-1", intent, "openai", "gpt-5.5", "proxy-1")
	if beginError != nil || !created || record.State != structuredRequestStateNotDispatched {
		testingInstance.Fatalf("record=%+v created=%t error=%v", record, created, beginError)
	}
	if duplicate, duplicateCreated, duplicateError := store.begin(requestTenant, "review:key-1", intent, "openai", "gpt-5.5", "proxy-2"); duplicateError != nil || duplicateCreated || duplicate.ProxyRequestID != "proxy-1" {
		testingInstance.Fatalf("duplicate=%+v created=%t error=%v", duplicate, duplicateCreated, duplicateError)
	}
	if _, _, conflictError := store.begin(requestTenant, "review:key-1", strings.Repeat("a", 64), "openai", "gpt-5.5", "proxy-3"); !errors.Is(conflictError, errStructuredRequestConflict) {
		testingInstance.Fatalf("conflict error=%v", conflictError)
	}
	if _, lookupError := store.lookup(otherTenant, "review:key-1"); !errors.Is(lookupError, errStructuredRequestNotFound) {
		testingInstance.Fatalf("other tenant lookup error=%v", lookupError)
	}
	claimed, claimError := store.claimDispatch(requestTenant, "review:key-1", intent)
	if claimError != nil || !claimed {
		testingInstance.Fatalf("claimed=%t error=%v", claimed, claimError)
	}
	claimed, claimError = store.claimDispatch(requestTenant, "review:key-1", intent)
	if claimError != nil || claimed {
		testingInstance.Fatalf("second claim=%t error=%v", claimed, claimError)
	}
	if succeedError := store.succeed(requestTenant, "review:key-1", intent, `not-json`); succeedError == nil {
		testingInstance.Fatal("invalid result must fail")
	}
	if succeedError := store.succeed(requestTenant, "review:key-1", intent, `{"decision":"pass"}`); succeedError != nil {
		testingInstance.Fatal(succeedError)
	}
	succeeded, lookupError := store.lookup(requestTenant, "review:key-1")
	if lookupError != nil || succeeded.State != structuredRequestStateSucceeded || !jsonEqual(succeeded.Result, []byte(`{"decision":"pass"}`)) || succeeded.StatusCode != http.StatusOK {
		testingInstance.Fatalf("succeeded=%+v error=%v", succeeded, lookupError)
	}

	failureIntent := structuredRequestIntent("anthropic", "claude", []byte(`{"messages":[]}`))
	_, _, _ = store.begin(requestTenant, "review:key-2", failureIntent, "anthropic", "claude", "proxy-2")
	if failError := store.fail(requestTenant, "review:key-2", failureIntent, http.StatusTooManyRequests, "provider_rate_limited"); failError != nil {
		testingInstance.Fatal(failError)
	}
	failed, _ := store.lookup(requestTenant, "review:key-2")
	if failed.State != structuredRequestStateFailed || failed.FailureCode != "provider_rate_limited" {
		testingInstance.Fatalf("failed=%+v", failed)
	}
	retried, retryCreated, retryError := store.begin(requestTenant, "review:key-2", failureIntent, "anthropic", "claude", "proxy-2-retry")
	if retryError != nil || !retryCreated || retried.State != structuredRequestStateNotDispatched || retried.ProxyRequestID != "proxy-2-retry" || retried.StatusCode != 0 || retried.CompletedAt != "" {
		testingInstance.Fatalf("retried=%+v created=%t error=%v", retried, retryCreated, retryError)
	}

	unknownIntent := structuredRequestIntent("gemini", "gemini-3", []byte(`{"messages":[]}`))
	_, _, _ = store.begin(requestTenant, "review:key-3", unknownIntent, "gemini", "gemini-3", "proxy-3")
	if unknownError := store.uncertain(requestTenant, "review:key-3", unknownIntent); unknownError != nil {
		testingInstance.Fatal(unknownError)
	}
	unknown, _ := store.lookup(requestTenant, "review:key-3")
	if unknown.State != structuredRequestStateUncertain || unknown.StatusCode != http.StatusConflict {
		testingInstance.Fatalf("unknown=%+v", unknown)
	}

	rootInfo, rootError := os.Stat(filepath.Join(assetRoot, structuredRequestDirectoryName))
	if rootError != nil || rootInfo.Mode().Perm() != 0o700 {
		testingInstance.Fatalf("root mode=%v error=%v", rootInfo.Mode(), rootError)
	}
	recordPath, _ := store.recordPath(sha256Hex(requestTenant.identifier.string()), sha256Hex("review:key-1"), false)
	recordInfo, recordError := os.Stat(recordPath)
	if recordError != nil || recordInfo.Mode().Perm() != 0o600 {
		testingInstance.Fatalf("record mode=%v error=%v", recordInfo.Mode(), recordError)
	}
}

func TestStructuredRequestStoreRecoveryAndSafety(testingInstance *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	activeRoot := testingInstance.TempDir()
	active, activeError := newStructuredRequestStore(activeRoot, 3600)
	if activeError != nil {
		testingInstance.Fatal(activeError)
	}
	active.now = func() time.Time { return now }
	requestTenant := structuredTestTenant(testingInstance, "recovery")
	intent := structuredRequestIntent("openai", "gpt", []byte(`{}`))
	_, _, _ = active.begin(requestTenant, "active", intent, "openai", "gpt", "proxy-active")
	_, _ = active.claimDispatch(requestTenant, "active", intent)
	_, _, _ = active.begin(requestTenant, "not-dispatched", intent, "openai", "gpt", "proxy-not-dispatched")
	_, _, _ = active.begin(requestTenant, "terminal", intent, "openai", "gpt", "proxy-terminal")
	_ = active.succeed(requestTenant, "terminal", intent, `{"ok":true}`)
	recovered, recoveredError := newStructuredRequestStore(activeRoot, 3600)
	if recoveredError != nil {
		testingInstance.Fatal(recoveredError)
	}
	activeRecord, _ := recovered.lookup(requestTenant, "active")
	notDispatchedRecord, _ := recovered.lookup(requestTenant, "not-dispatched")
	terminalRecord, _ := recovered.lookup(requestTenant, "terminal")
	if activeRecord.State != structuredRequestStateUncertain || activeRecord.FailureCode != "structured_request_interrupted" || notDispatchedRecord.State != structuredRequestStateNotDispatched || terminalRecord.State != structuredRequestStateSucceeded {
		testingInstance.Fatalf("active=%+v not_dispatched=%+v terminal=%+v", activeRecord, notDispatchedRecord, terminalRecord)
	}

	expiredRoot := testingInstance.TempDir()
	expired, _ := newStructuredRequestStore(expiredRoot, 1)
	expired.now = func() time.Time { return now.Add(-time.Hour) }
	_, _, _ = expired.begin(requestTenant, "expired", intent, "openai", "gpt", "proxy-expired")
	_, _, _ = expired.begin(requestTenant, "expired-dispatched", intent, "openai", "gpt", "proxy-dispatched")
	_, _ = expired.claimDispatch(requestTenant, "expired-dispatched", intent)
	_, _, _ = expired.begin(requestTenant, "expired-uncertain", intent, "openai", "gpt", "proxy-uncertain")
	_ = expired.uncertain(requestTenant, "expired-uncertain", intent)
	reloaded, reloadError := newStructuredRequestStore(expiredRoot, 1)
	if reloadError != nil {
		testingInstance.Fatal(reloadError)
	}
	if _, lookupError := reloaded.lookup(requestTenant, "expired"); !errors.Is(lookupError, errStructuredRequestNotFound) {
		testingInstance.Fatalf("expired lookup=%v", lookupError)
	}
	for _, key := range []string{"expired-dispatched", "expired-uncertain"} {
		record, lookupError := reloaded.lookup(requestTenant, key)
		if lookupError != nil || record.State != structuredRequestStateUncertain {
			testingInstance.Fatalf("retained uncertain key=%s record=%+v error=%v", key, record, lookupError)
		}
	}

	for _, unsafe := range []struct {
		name  string
		setup func(string) error
	}{
		{name: "file root", setup: func(root string) error {
			return os.WriteFile(filepath.Join(root, structuredRequestDirectoryName), []byte("file"), 0o600)
		}},
		{name: "symlink root", setup: func(root string) error { return os.Symlink(root, filepath.Join(root, structuredRequestDirectoryName)) }},
	} {
		testingInstance.Run(unsafe.name, func(subtest *testing.T) {
			root := subtest.TempDir()
			if setupError := unsafe.setup(root); setupError != nil {
				subtest.Fatal(setupError)
			}
			if _, storeError := newStructuredRequestStore(root, 1); storeError == nil {
				subtest.Fatal("unsafe root must fail")
			}
		})
	}

	unexpectedRoot := testingInstance.TempDir()
	untracked := filepath.Join(unexpectedRoot, structuredRequestDirectoryName)
	if makeError := os.MkdirAll(untracked, 0o700); makeError != nil {
		testingInstance.Fatal(makeError)
	}
	if writeError := os.WriteFile(filepath.Join(untracked, "unexpected.txt"), []byte("x"), 0o600); writeError != nil {
		testingInstance.Fatal(writeError)
	}
	if _, storeError := newStructuredRequestStore(unexpectedRoot, 1); storeError == nil {
		testingInstance.Fatal("unexpected file must fail recovery")
	}
}

func TestStructuredRequestRecordValidationAndIOFailures(testingInstance *testing.T) {
	valid := structuredRequestRecord{
		Schema: structuredRequestRecordSchema, TenantSHA256: strings.Repeat("a", 64), IdempotencySHA256: strings.Repeat("b", 64),
		IntentSHA256: strings.Repeat("c", 64), Provider: "openai", Model: "gpt", ProxyRequestID: "proxy",
		State: structuredRequestStateNotDispatched, StartedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	invalidRecords := []structuredRequestRecord{
		{},
		withStructuredRecord(valid, func(record *structuredRequestRecord) { record.CompletedAt = "now" }),
		withStructuredRecord(valid, func(record *structuredRequestRecord) {
			record.State = structuredRequestStateSucceeded
			record.StatusCode = 200
			record.CompletedAt = "now"
			record.Result = []byte(`bad`)
		}),
		withStructuredRecord(valid, func(record *structuredRequestRecord) {
			record.State = structuredRequestStateFailed
			record.StatusCode = 0
			record.CompletedAt = "now"
			record.FailureCode = "failed"
		}),
		withStructuredRecord(valid, func(record *structuredRequestRecord) { record.State = "new" }),
	}
	for index, record := range invalidRecords {
		if validationError := validateStructuredRequestRecord(record); validationError == nil {
			testingInstance.Fatalf("invalid record case=%d", index)
		}
	}

	directory := testingInstance.TempDir()
	path := filepath.Join(directory, "record.json")
	if publishError := publishStructuredRequestRecord(path, valid); publishError != nil {
		testingInstance.Fatal(publishError)
	}
	read, readError := readStructuredRequestRecord(path)
	if readError != nil || read.State != structuredRequestStateNotDispatched {
		testingInstance.Fatalf("read=%+v error=%v", read, readError)
	}
	if publishError := publishStructuredRequestRecord(filepath.Join(directory, "invalid.json"), structuredRequestRecord{}); publishError == nil {
		testingInstance.Fatal("invalid publish must fail")
	}
	if publishError := publishStructuredRequestRecord(filepath.Join(directory, "missing", "record.json"), valid); publishError == nil {
		testingInstance.Fatal("missing directory publish must fail")
	}

	badPaths := []struct {
		name  string
		setup func(string) error
	}{
		{name: "directory", setup: func(path string) error { return os.Mkdir(path, 0o700) }},
		{name: "trailing", setup: func(path string) error { return os.WriteFile(path, []byte(`{"schema":"x"} {}`), 0o600) }},
		{name: "invalid json", setup: func(path string) error { return os.WriteFile(path, []byte(`{`), 0o600) }},
		{name: "invalid record", setup: func(path string) error { return os.WriteFile(path, []byte(`{}`), 0o600) }},
	}
	for _, badPath := range badPaths {
		testingInstance.Run(badPath.name, func(subtest *testing.T) {
			bad := filepath.Join(subtest.TempDir(), "record.json")
			if setupError := badPath.setup(bad); setupError != nil {
				subtest.Fatal(setupError)
			}
			if _, readError := readStructuredRequestRecord(bad); readError == nil {
				subtest.Fatal("unsafe record must fail")
			}
		})
	}
	if _, readError := readStructuredRequestRecord(filepath.Join(directory, "missing.json")); !os.IsNotExist(readError) {
		testingInstance.Fatalf("missing read error=%v", readError)
	}
}

func TestStructuredRequestStoreInjectedFailures(testingInstance *testing.T) {
	requestTenant := structuredTestTenant(testingInstance, "failures")
	intent := structuredRequestIntent("openai", "gpt", []byte(`{}`))
	store, _ := newStructuredRequestStore(testingInstance.TempDir(), 10)
	store.publish = func(string, structuredRequestRecord) error { return errors.New("publish failed") }
	if _, _, beginError := store.begin(requestTenant, "key", intent, "openai", "gpt", "proxy"); beginError == nil {
		testingInstance.Fatal("begin publish failure missing")
	}
	store, _ = newStructuredRequestStore(testingInstance.TempDir(), 10)
	_, _, _ = store.begin(requestTenant, "key", intent, "openai", "gpt", "proxy")
	store.publish = func(string, structuredRequestRecord) error { return errors.New("publish failed") }
	if _, claimError := store.claimDispatch(requestTenant, "key", intent); claimError == nil {
		testingInstance.Fatal("transition publish failure missing")
	}
	store.read = func(string) (structuredRequestRecord, error) {
		return structuredRequestRecord{}, errors.New("read failed")
	}
	if _, lookupError := store.lookup(requestTenant, "key"); lookupError == nil {
		testingInstance.Fatal("lookup read failure missing")
	}
	if _, transitionError := store.transition(requestTenant, "key", intent, func(*structuredRequestRecord, time.Time) {}); transitionError == nil {
		testingInstance.Fatal("transition read failure missing")
	}
	if _, pathError := store.recordPath("bad", strings.Repeat("b", 64), false); pathError == nil {
		testingInstance.Fatal("invalid digest must fail")
	}
}

func TestStructuredRequestStatusHandler(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	store, _ := newStructuredRequestStore(testingInstance.TempDir(), 10)
	requestTenant := structuredTestTenant(testingInstance, "handler")
	intent := structuredRequestIntent("openai", "gpt", []byte(`{}`))
	_, _, _ = store.begin(requestTenant, "pending", intent, "openai", "gpt", "proxy-pending")
	store.now = func() time.Time { return time.Now().UTC().Add(time.Second) }

	testCases := []struct {
		name       string
		key        string
		headerFunc func(http.Header)
		statusCode int
		contains   string
	}{
		{name: "invalid", headerFunc: func(http.Header) {}, statusCode: 400, contains: "invalid_idempotency_key"},
		{name: "duplicate header", headerFunc: func(header http.Header) {
			header.Add(llmproxycontract.HeaderIdempotencyKey, "one")
			header.Add(llmproxycontract.HeaderIdempotencyKey, "two")
		}, statusCode: 400, contains: "invalid_idempotency_key"},
		{name: "missing", key: "missing", statusCode: 404, contains: "structured_request_not_found"},
		{name: "pending", key: "pending", statusCode: 202, contains: `"state":"not_dispatched"`},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subtest *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/v2/requests", nil)
			if testCase.key != "" {
				request.Header.Set(llmproxycontract.HeaderIdempotencyKey, testCase.key)
			}
			if testCase.headerFunc != nil {
				testCase.headerFunc(request.Header)
			}
			response := httptest.NewRecorder()
			contextValue, _ := gin.CreateTestContext(response)
			contextValue.Request = request
			contextValue.Set(contextKeyTenant, requestTenant)
			structuredRequestStatusHandler(store)(contextValue)
			if response.Code != testCase.statusCode || !strings.Contains(response.Body.String(), testCase.contains) {
				subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	store.read = func(string) (structuredRequestRecord, error) {
		return structuredRequestRecord{}, errors.New("store failed")
	}
	request := httptest.NewRequest(http.MethodGet, "/v2/requests", nil)
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "pending")
	response := httptest.NewRecorder()
	contextValue, _ := gin.CreateTestContext(response)
	contextValue.Request = request
	contextValue.Set(contextKeyTenant, requestTenant)
	structuredRequestStatusHandler(store)(contextValue)
	if response.Code != 500 || !strings.Contains(response.Body.String(), "structured_request_store_error") {
		testingInstance.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStructuredRequestRecordResponses(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	base := structuredRequestRecord{ProxyRequestID: "proxy", StartedAt: now.Add(time.Second).Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	testCases := []struct {
		name       string
		record     structuredRequestRecord
		statusCode int
		contains   string
	}{
		{name: "succeeded", record: withStructuredRecord(base, func(record *structuredRequestRecord) {
			record.State = structuredRequestStateSucceeded
			record.Result = []byte(`{"ok":true}`)
		}), statusCode: 200, contains: `{"ok":true}`},
		{name: "not dispatched", record: withStructuredRecord(base, func(record *structuredRequestRecord) { record.State = structuredRequestStateNotDispatched }), statusCode: 202, contains: `"elapsed_seconds":0`},
		{name: "dispatched", record: withStructuredRecord(base, func(record *structuredRequestRecord) { record.State = structuredRequestStateDispatched }), statusCode: 202, contains: `"state":"dispatched"`},
		{name: "failed status", record: withStructuredRecord(base, func(record *structuredRequestRecord) {
			record.State = structuredRequestStateFailed
			record.StatusCode = 429
			record.FailureCode = "rate"
		}), statusCode: 429, contains: "structured_request_failed"},
		{name: "failed default status", record: withStructuredRecord(base, func(record *structuredRequestRecord) {
			record.State = structuredRequestStateFailed
			record.StatusCode = 200
			record.FailureCode = "provider"
		}), statusCode: 502, contains: "structured_request_failed"},
		{name: "uncertain", record: withStructuredRecord(base, func(record *structuredRequestRecord) {
			record.State = structuredRequestStateUncertain
			record.FailureCode = "unknown"
		}), statusCode: 409, contains: "structured_request_outcome_unknown"},
		{name: "invalid", record: withStructuredRecord(base, func(record *structuredRequestRecord) { record.State = "invalid" }), statusCode: 500, contains: "structured_request_store_error"},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subtest *testing.T) {
			response := httptest.NewRecorder()
			contextValue, _ := gin.CreateTestContext(response)
			writeStructuredRequestRecord(contextValue, testCase.record, now)
			if response.Code != testCase.statusCode || !strings.Contains(response.Body.String(), testCase.contains) {
				subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	for statusCode, expected := range map[int]string{429: llmproxycontract.ErrorCodeProviderRateLimited, 503: "service_unavailable", 504: llmproxycontract.ErrorCodeRequestTimeout, 502: llmproxycontract.ErrorCodeProviderError} {
		if actual := structuredRequestFailureCause(statusCode); actual != expected {
			testingInstance.Fatalf("status=%d cause=%s expected=%s", statusCode, actual, expected)
		}
	}
}

func structuredTestTenant(testingInstance *testing.T, identifier string) tenant {
	testingInstance.Helper()
	requestTenant, tenantError := newTenant(TenantConfiguration{ID: identifier, Secret: "secret-" + identifier})
	if tenantError != nil {
		testingInstance.Fatal(tenantError)
	}
	return requestTenant
}

func withStructuredRecord(record structuredRequestRecord, update func(*structuredRequestRecord)) structuredRequestRecord {
	update(&record)
	return record
}

func jsonEqual(first []byte, second []byte) bool {
	var firstValue any
	var secondValue any
	return json.Unmarshal(first, &firstValue) == nil && json.Unmarshal(second, &secondValue) == nil && reflect.DeepEqual(firstValue, secondValue)
}

func TestCanonicalStructuredJSON(testingInstance *testing.T) {
	canonical, canonicalError := canonicalJSON([]byte(`{"b":2,"a":1}`))
	if canonicalError != nil || !bytes.Equal(canonical, []byte(`{"a":1,"b":2}`)) {
		testingInstance.Fatalf("canonical=%s error=%v", canonical, canonicalError)
	}
	for _, invalid := range [][]byte{[]byte(`{`), []byte(`{} {}`)} {
		if _, canonicalError := canonicalJSON(invalid); canonicalError == nil {
			testingInstance.Fatalf("invalid JSON=%s", invalid)
		}
	}
}

func TestSubmitStructuredChatRequestLifecycle(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	requestTenant := structuredTestTenant(testingInstance, "submit")
	schema, schemaError := newStructuredOutputSchema(json.RawMessage(`{"schema":{"type":"object","required":["decision"],"properties":{"decision":{"type":"string"}}}}`))
	if schemaError != nil {
		testingInstance.Fatal(schemaError)
	}
	body := []byte(`{"messages":[{"role":"user","content":"review"}]}`)
	logger := zap.NewNop().Sugar()

	testingInstance.Run("success", func(subtest *testing.T) {
		store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
		chatRequest, providers := structuredSubmitRequest(schema, "success", func(context.Context, *providerRouter, chatRequestParameters, *zap.SugaredLogger) (textGenerationResult, error) {
			return textGenerationResult{text: `{"decision":"pass"}`}, nil
		})
		contextValue, response := structuredSubmitContext(subtest, context.Background())
		submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, body, store, nil, logger, time.Now())
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"decision":"pass"`) {
			subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	testingInstance.Run("invalid canonical body", func(subtest *testing.T) {
		store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
		chatRequest, providers := structuredSubmitRequest(schema, "invalid-body", nil)
		contextValue, response := structuredSubmitContext(subtest, context.Background())
		submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, []byte(`{`), store, nil, logger, time.Now())
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "structured_request_invalid") {
			subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	testingInstance.Run("intent conflict", func(subtest *testing.T) {
		store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
		chatRequest, providers := structuredSubmitRequest(schema, "conflict", nil)
		_, _, _ = store.begin(requestTenant, chatRequest.idempotencyKey, strings.Repeat("a", 64), "openai", "gpt", "old")
		contextValue, response := structuredSubmitContext(subtest, context.Background())
		submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, body, store, nil, logger, time.Now())
		if response.Code != http.StatusConflict {
			subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	testingInstance.Run("begin failure", func(subtest *testing.T) {
		store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
		store.read = func(string) (structuredRequestRecord, error) {
			return structuredRequestRecord{}, errors.New("read failed")
		}
		chatRequest, providers := structuredSubmitRequest(schema, "begin-failure", nil)
		contextValue, response := structuredSubmitContext(subtest, context.Background())
		submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, body, store, nil, logger, time.Now())
		if response.Code != http.StatusInternalServerError {
			subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	testingInstance.Run("claim failure", func(subtest *testing.T) {
		store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
		originalRead := store.read
		readCount := 0
		store.read = func(path string) (structuredRequestRecord, error) {
			readCount++
			if readCount == 2 {
				return structuredRequestRecord{}, errors.New("claim read failed")
			}
			return originalRead(path)
		}
		chatRequest, providers := structuredSubmitRequest(schema, "claim-failure", nil)
		contextValue, response := structuredSubmitContext(subtest, context.Background())
		submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, body, store, nil, logger, time.Now())
		if response.Code != http.StatusInternalServerError {
			subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	testingInstance.Run("replay lookup failure", func(subtest *testing.T) {
		store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
		chatRequest, providers := structuredSubmitRequest(schema, "lookup-failure", nil)
		canonicalBody, _ := canonicalJSON(body)
		intent := structuredRequestIntent("openai", "gpt", canonicalBody)
		_, _, _ = store.begin(requestTenant, chatRequest.idempotencyKey, intent, "openai", "gpt", "old")
		_, _ = store.claimDispatch(requestTenant, chatRequest.idempotencyKey, intent)
		originalRead := store.read
		readCount := 0
		store.read = func(path string) (structuredRequestRecord, error) {
			readCount++
			if readCount == 3 {
				return structuredRequestRecord{}, errors.New("lookup failed")
			}
			return originalRead(path)
		}
		contextValue, response := structuredSubmitContext(subtest, context.Background())
		submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, body, store, nil, logger, time.Now())
		if response.Code != http.StatusInternalServerError {
			subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	testingInstance.Run("provider failure", func(subtest *testing.T) {
		store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
		chatRequest, providers := structuredSubmitRequest(schema, "provider-failure", func(context.Context, *providerRouter, chatRequestParameters, *zap.SugaredLogger) (textGenerationResult, error) {
			return textGenerationResult{}, ErrProviderAPI
		})
		contextValue, response := structuredSubmitContext(subtest, context.Background())
		submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, body, store, nil, logger, time.Now())
		if response.Code != http.StatusBadGateway {
			subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	testingInstance.Run("provider failure persistence", func(subtest *testing.T) {
		store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
		chatRequest, providers := structuredSubmitRequest(schema, "failure-persist", func(context.Context, *providerRouter, chatRequestParameters, *zap.SugaredLogger) (textGenerationResult, error) {
			store.publish = func(string, structuredRequestRecord) error { return errors.New("publish failed") }
			return textGenerationResult{}, ErrProviderAPI
		})
		contextValue, response := structuredSubmitContext(subtest, context.Background())
		submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, body, store, nil, logger, time.Now())
		if response.Code != http.StatusInternalServerError {
			subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	testingInstance.Run("success persistence", func(subtest *testing.T) {
		store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
		chatRequest, providers := structuredSubmitRequest(schema, "success-persist", func(context.Context, *providerRouter, chatRequestParameters, *zap.SugaredLogger) (textGenerationResult, error) {
			store.publish = func(string, structuredRequestRecord) error { return errors.New("publish failed") }
			return textGenerationResult{text: `{"decision":"pass"}`}, nil
		})
		contextValue, response := structuredSubmitContext(subtest, context.Background())
		submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, body, store, nil, logger, time.Now())
		if response.Code != http.StatusInternalServerError {
			subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	for _, testCase := range []struct {
		name   string
		result textGenerationResult
		err    error
	}{
		{name: "canceled error", err: ErrProviderAPI},
		{name: "canceled success", result: textGenerationResult{text: `{"decision":"pass"}`}},
	} {
		testingInstance.Run(testCase.name, func(subtest *testing.T) {
			store, _ := newStructuredRequestStore(subtest.TempDir(), 10)
			requestContext, cancel := context.WithCancel(context.Background())
			chatRequest, providers := structuredSubmitRequest(schema, "cancel-"+testCase.name, func(context.Context, *providerRouter, chatRequestParameters, *zap.SugaredLogger) (textGenerationResult, error) {
				cancel()
				return testCase.result, testCase.err
			})
			contextValue, response := structuredSubmitContext(subtest, requestContext)
			submitStructuredChatRequest(contextValue, providers, chatRequest, requestTenant, body, store, nil, logger, time.Now())
			if contextValue.Writer.Status() != statusClientClosedRequest {
				subtest.Fatalf("status=%d recorder=%d body=%s", contextValue.Writer.Status(), response.Code, response.Body.String())
			}
		})
	}

	testingInstance.Run("structured output limit", func(subtest *testing.T) {
		chatRequest, providers := structuredSubmitRequest(schema, "output-limit", func(context.Context, *providerRouter, chatRequestParameters, *zap.SugaredLogger) (textGenerationResult, error) {
			return textGenerationResult{text: `{"decision":`}, errProviderOutputLimitReached
		})
		telemetryContext := requestContextWithTelemetry(context.Background(), newRequestTelemetry("output-limit", "/v2"))
		generation, generationError := providers.generateText(telemetryContext, chatRequest, logger)
		if !errors.Is(generationError, ErrProviderAPI) || generation.text != "" {
			subtest.Fatalf("generation=%+v error=%v", generation, generationError)
		}
	})
}

func structuredSubmitRequest(schema *structuredOutputSchema, key string, adapter structuredRouteAdapter) (chatRequestParameters, *providerRouter) {
	if adapter == nil {
		adapter = func(context.Context, *providerRouter, chatRequestParameters, *zap.SugaredLogger) (textGenerationResult, error) {
			return textGenerationResult{text: `{"decision":"pass"}`}, nil
		}
	}
	model := textModelDefinition{
		identifier: newModelID("gpt"), providerIdentifier: newModelID("gpt"),
		wireContract: textWireContractOpenAIResponses, executionLifecycle: textExecutionLifecycleSynchronousCompletion,
		routeAdapter: adapter,
	}
	request := chatRequestParameters{
		messages: chatMessages{{role: chatRoleUser, content: "review"}},
		provider: providerDefinition{identifier: providerID(ProviderNameOpenAI)}, model: model,
		structuredOutput: schema, idempotencyKey: key,
	}
	return request, &providerRouter{}
}

func structuredSubmitContext(testingInstance testing.TB, requestContext context.Context) (*gin.Context, *httptest.ResponseRecorder) {
	testingInstance.Helper()
	response := httptest.NewRecorder()
	contextValue, _ := gin.CreateTestContext(response)
	request := httptest.NewRequest(http.MethodPost, "/v2", nil).WithContext(requestContextWithTelemetry(requestContext, newRequestTelemetry("proxy-submit", "/v2")))
	contextValue.Request = request
	contextValue.Set(contextKeyRequestID, "proxy-submit")
	contextValue.Set(contextKeyRequestTimeoutState, &requestTimeoutState{budget: newRequestTimeoutBudget(10)})
	return contextValue, response
}
