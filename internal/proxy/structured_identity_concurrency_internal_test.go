package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

const identityConcurrencyBody = `{"model":"gpt-5.5","messages":[{"role":"user","content":"review"}],"structured_output":{"schema":{"type":"object","additionalProperties":false,"required":["decision"],"properties":{"decision":{"type":"string","enum":["pass"]}}}}}`

func TestStructuredIdentityIndependentHTTPPersistence(t *testing.T) {
	for _, state := range []string{structuredRequestStateNotDispatched, structuredRequestStateDispatched, structuredRequestStateSucceeded} {
		for _, otherTenant := range []bool{false, true} {
			label := state + "/different-key"
			if otherTenant {
				label = state + "/different-tenant-same-key"
			}
			t.Run(label, func(t *testing.T) {
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"identity","status":"completed","output_text":"{\"decision\":\"pass\"}"}`)
				}))
				defer upstream.Close()
				endpoints := NewEndpoints()
				endpoints.SetResponsesURL(upstream.URL)
				tenants := []ManagedTenantTestConfiguration{StandardManagedTenantTestConfiguration("identity-secret-one"), StandardManagedTenantTestConfiguration("identity-secret-two")}
				tenants[0].ID = "identity-one"
				tenants[1].ID = "identity-two"
				configuration := Configuration{Endpoints: endpoints, AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600, RequestTimeoutSeconds: 10}
				router, err := BuildRouterWithManagedTenantsForTest(t, configuration, zap.NewNop().Sugar(), tenants)
				if err != nil {
					t.Fatal(err)
				}
				blocked, release := blockIdentityPublication(t, tenants[0].ID, "identity-a", state)
				first := identityHTTPRequestAsync(router, tenants[0].Secret, "identity-a", identityConcurrencyBody)
				select {
				case <-blocked:
				case <-time.After(3 * time.Second):
					t.Fatal("first publication did not reach gate")
				}
				// Blocking publication must not expose the new state or result.
				path := filepath.Join(configuration.AssetStorePath, structuredRequestDirectoryName, sha256Hex(tenants[0].ID), sha256Hex("identity-a")+".json")
				saved, readError := os.ReadFile(path)
				if state == structuredRequestStateNotDispatched {
					if !os.IsNotExist(readError) {
						t.Fatalf("unpublished initial record: error=%v", readError)
					}
				} else {
					if readError != nil {
						t.Fatal(readError)
					}
					var retained structuredRequestRecord
					if err := json.Unmarshal(saved, &retained); err != nil {
						t.Fatal(err)
					}
					expected := structuredRequestStateNotDispatched
					if state == structuredRequestStateSucceeded {
						expected = structuredRequestStateDispatched
					}
					if retained.State != expected || len(retained.Result) != 0 {
						t.Fatalf("unpublished state/result exposed: state=%s result_bytes=%d", retained.State, len(retained.Result))
					}
				}
				secondKey, secondSecret := "identity-b", tenants[0].Secret
				if otherTenant {
					secondKey = "identity-a"
					secondSecret = tenants[1].Secret
				}
				sameStarted := make(chan struct{})
				same := make(chan *httptest.ResponseRecorder, 1)
				go func() {
					request := httptest.NewRequest(http.MethodPost, "/v2?key="+tenants[0].Secret, &identitySignalingReader{Reader: strings.NewReader(identityConcurrencyBody), started: sameStarted})
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "identity-a")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					same <- response
				}()
				<-sameStarted
				second := identityHTTPRequestAsync(router, secondSecret, secondKey, identityConcurrencyBody)
				select {
				case response := <-second:
					assertIdentitySuccess(t, response)
				case <-time.After(2 * time.Second):
					release()
					<-first
					<-second
					t.Fatal("independent identity blocked behind another publication")
				}
				select {
				case response := <-same:
					release()
					<-first
					t.Fatalf("same identity returned during uncommitted publication: status=%d body=%s", response.Code, response.Body.String())
				default:
				}
				expectedCalls := int64(1)
				if state == structuredRequestStateSucceeded {
					expectedCalls = 2
				}
				if calls.Load() != expectedCalls {
					release()
					<-first
					<-same
					t.Fatalf("same identity dispatched while publication blocked: calls=%d want=%d", calls.Load(), expectedCalls)
				}
				status := identityHTTPRequest(router, http.MethodGet, secondSecret, secondKey, "")
				assertIdentitySuccess(t, status)
				release()
				firstResponse, sameResponse := <-first, <-same
				for _, response := range []*httptest.ResponseRecorder{firstResponse, sameResponse} {
					if response.Code != http.StatusOK && response.Code != http.StatusAccepted {
						t.Fatalf("same identity status=%d body=%s", response.Code, response.Body.String())
					}
				}
				if firstResponse.Code != http.StatusOK && sameResponse.Code != http.StatusOK {
					t.Fatal("no same-identity submit completed")
				}
				assertIdentitySuccess(t, identityHTTPRequest(router, http.MethodGet, tenants[0].Secret, "identity-a", ""))
				// Same identity replay and changed intent retain their canonical semantics.
				assertIdentitySuccess(t, identityHTTPRequest(router, http.MethodPost, tenants[0].Secret, "identity-a", identityConcurrencyBody))
				conflict := identityHTTPRequest(router, http.MethodPost, tenants[0].Secret, "identity-a", strings.Replace(identityConcurrencyBody, "review", "changed", 1))
				if conflict.Code != http.StatusConflict {
					t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
				}
				reopened, err := BuildRouterWithManagedTenantsForTest(t, configuration, zap.NewNop().Sugar(), tenants)
				if err != nil {
					t.Fatal(err)
				}
				assertIdentitySuccess(t, identityHTTPRequest(reopened, http.MethodPost, tenants[0].Secret, "identity-a", identityConcurrencyBody))
				assertIdentitySuccess(t, identityHTTPRequest(reopened, http.MethodGet, secondSecret, secondKey, ""))
				if calls.Load() != 2 {
					t.Fatalf("provider calls=%d, want exactly two independent executions", calls.Load())
				}
			})
		}
	}
}

func blockIdentityPublication(t *testing.T, tenantID, key, state string) (<-chan struct{}, func()) {
	t.Helper()
	original := structuredRequestFileWrite
	blocked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var gate sync.Once
	structuredRequestFileWrite = func(file *os.File, data []byte) (int, error) {
		var record structuredRequestRecord
		if json.Unmarshal(data, &record) == nil && record.TenantSHA256 == sha256Hex(tenantID) && record.IdempotencySHA256 == sha256Hex(key) && record.State == state {
			gate.Do(func() { close(blocked); <-release })
		}
		return original(file, data)
	}
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); structuredRequestFileWrite = original })
	return blocked, unblock
}
func identityHTTPRequestAsync(router *gin.Engine, secret, key, body string) <-chan *httptest.ResponseRecorder {
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- identityHTTPRequest(router, http.MethodPost, secret, key, body) }()
	return result
}
func identityHTTPRequest(router *gin.Engine, method, secret, key, body string) *httptest.ResponseRecorder {
	path := "/v2?key=" + secret
	if method == http.MethodGet {
		path = "/v2/requests?key=" + secret
	}
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
func assertIdentitySuccess(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK || !jsonEqual(response.Body.Bytes(), []byte(`{"decision":"pass"}`)) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStructuredIdentitySameKeyHTTPDispatch(t *testing.T) {
	var calls atomic.Int64
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		_, _ = io.WriteString(w, `{"id":"identity","status":"completed","output_text":"{\"decision\":\"pass\"}"}`)
	}))
	t.Cleanup(upstream.Close)
	endpoints := NewEndpoints()
	endpoints.SetResponsesURL(upstream.URL)
	tenant := StandardManagedTenantTestConfiguration("same-identity-secret")
	router, err := BuildRouterWithManagedTenantForTest(t, Configuration{Endpoints: endpoints, AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600, RequestTimeoutSeconds: 10}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	first := identityHTTPRequestAsync(router, tenant.Secret, "same-key", identityConcurrencyBody)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("provider was not entered")
	}
	duplicate := identityHTTPRequest(router, http.MethodPost, tenant.Secret, "same-key", identityConcurrencyBody)
	if duplicate.Code != http.StatusAccepted {
		t.Fatalf("active duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	conflict := identityHTTPRequest(router, http.MethodPost, tenant.Secret, "same-key", strings.Replace(identityConcurrencyBody, "review", "different", 1))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("active conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	unblock()
	assertIdentitySuccess(t, <-first)
	assertIdentitySuccess(t, identityHTTPRequest(router, http.MethodPost, tenant.Secret, "same-key", identityConcurrencyBody))
	if calls.Load() != 1 {
		t.Fatalf("duplicate provider calls=%d", calls.Load())
	}
}

// This diagnostic reports local journal costs without a latency acceptance gate.
func TestStructuredIdentityLocalCohortDiagnostic(t *testing.T) {
	const preseedCount = 128
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"id":"diagnostic","status":"completed","output_text":"{\"decision\":\"pass\"}"}`)
	}))
	defer upstream.Close()
	endpoints := NewEndpoints()
	endpoints.SetResponsesURL(upstream.URL)
	tenant := StandardManagedTenantTestConfiguration("local-cohort-secret")
	router, err := BuildRouterWithManagedTenantForTest(t, Configuration{Endpoints: endpoints, AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600, RequestTimeoutSeconds: 10}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < preseedCount; index++ {
		assertIdentitySuccess(t, identityHTTPRequest(router, http.MethodPost, tenant.Secret, fmt.Sprintf("preseed-%d", index), identityConcurrencyBody))
	}
	for _, count := range []int{1, 32} {
		type observed struct {
			response *httptest.ResponseRecorder
			elapsed  time.Duration
		}
		results := make(chan observed, count)
		start := make(chan struct{})
		for index := 0; index < count; index++ {
			go func(index int) {
				<-start
				started := time.Now()
				response := identityHTTPRequest(router, http.MethodPost, tenant.Secret, fmt.Sprintf("cohort-%d-%d", count, index), identityConcurrencyBody)
				results <- observed{response, time.Since(started)}
			}(index)
		}
		started := time.Now()
		close(start)
		latencies := make([]time.Duration, 0, count)
		for index := 0; index < count; index++ {
			result := <-results
			assertIdentitySuccess(t, result.response)
			latencies = append(latencies, result.elapsed)
		}
		elapsed := time.Since(started)
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		median := latencies[len(latencies)/2]
		if len(latencies)%2 == 0 {
			median = (latencies[len(latencies)/2-1] + median) / 2
		}
		t.Logf("local_cohort preseed_records=%d cohort=%d wall=%s min=%s median=%s max=%s", preseedCount, count, elapsed, latencies[0], median, latencies[len(latencies)-1])
		for index := 0; index < count; index++ {
			assertIdentitySuccess(t, identityHTTPRequest(router, http.MethodGet, tenant.Secret, fmt.Sprintf("cohort-%d-%d", count, index), ""))
		}
	}
	if calls.Load() != preseedCount+33 {
		t.Fatalf("provider calls=%d", calls.Load())
	}
}

type identitySignalingReader struct {
	io.Reader
	started chan struct{}
	once    sync.Once
}

func (reader *identitySignalingReader) Read(data []byte) (int, error) {
	reader.once.Do(func() { close(reader.started) })
	return reader.Reader.Read(data)
}
