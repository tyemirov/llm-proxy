package proxy_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"github.com/klauspost/compress/zstd"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestSystemOneEndpointExists(t *testing.T) {
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/systemone", strings.NewReader(`{"model":"jev-latest","state":"test","questions":{"ok":{"type":"noul"}}}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusUnauthorized || !strings.Contains(string(body), `"detail"`) {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

const systemOneRequest = `{"model":"jev-latest","state":{"message":"test","value":null},"unknown":true,"questions":{"ok":{"type":"noul","instructions":null,"criteria":null},"choice":{"type":"choice","criteria":{"yes":null}},"score":{"type":"score","criteria":[{"level":"one"}]},"box":{"type":"box"}}}`
const systemOneResponse = `{"model":"jev-1.14.0","answers":{"ok":{"type":"noul","noul":0.8},"choice":{"type":"choice","choice":"yes","confidence":0.8,"probabilities":{"yes":1}},"score":{"type":"score","score":0,"confidence":1,"legend":{"0":{"level":"one"}},"probabilities":{"0":1}},"box":{"type":"box","box":{"top":0,"bottom":1,"left":0,"right":1},"confidence":null}},"usage":{"input_tokens":4,"output_tokens":2}}`

func TestSystemOneWireAndIsolation(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/systemone" || r.Method != "POST" {
			t.Errorf("upstream route=%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Errorf("provider authentication incorrect")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != systemOneRequest {
			t.Errorf("wire body changed: %s", body)
		}
		if r.Header.Get("TypeSafe-Danger-Zone-Images") != "true" {
			t.Error("experimental header lost")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "upstream-id")
		io.WriteString(w, systemOneResponse)
	}))
	defer upstream.Close()
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.ProviderKeys["typesafe"] = "provider-secret"
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, "typesafe")}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, key := range []string{TestSecret, "wrong-key"} {
		request, _ := http.NewRequest("POST", server.URL+"/v1/systemone", strings.NewReader(systemOneRequest))
		request.Header.Set("Authorization", "Bearer "+key)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("TypeSafe-Danger-Zone-Images", "true")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if key == TestSecret {
			if response.StatusCode != 200 || string(body) != systemOneResponse || response.Header.Get("X-Request-ID") != "upstream-id" {
				t.Fatalf("status=%d body=%s headers=%v", response.StatusCode, body, response.Header)
			}
		} else if response.StatusCode != 401 {
			t.Fatalf("foreign key status=%d", response.StatusCode)
		}
	}
	if calls != 1 {
		t.Fatalf("provider calls=%d", calls)
	}
}

func systemOneServer(t *testing.T, upstream *httptest.Server) *httptest.Server {
	t.Helper()
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.ProviderKeys["typesafe"] = "provider-secret"
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, "typesafe"), RequestTimeoutSeconds: 1}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}
func systemOneCall(t *testing.T, server *httptest.Server, body []byte, contentType, encoding string) (int, []byte, http.Header) {
	t.Helper()
	request, _ := http.NewRequest("POST", server.URL+"/v1/systemone", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+TestSecret)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("TypeSafe-Danger-Zone-Images", "true")
	if encoding != "" {
		request.Header.Set("Content-Encoding", encoding)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data, response.Header
}
func TestSystemOneCompressedAndMultipartWire(t *testing.T) {
	for _, encoding := range []string{"gzip", "zstd", "multipart"} {
		t.Run(encoding, func(t *testing.T) {
			original := []byte(systemOneRequest)
			contentType := "application/json"
			wire := original
			headerEncoding := encoding
			switch encoding {
			case "gzip":
				var buffer bytes.Buffer
				writer := gzip.NewWriter(&buffer)
				writer.Write(original)
				writer.Close()
				wire = buffer.Bytes()
			case "zstd":
				writer, _ := zstd.NewWriter(nil)
				wire = writer.EncodeAll(original, nil)
				writer.Close()
			case "multipart":
				headerEncoding = ""
				var buffer bytes.Buffer
				writer := multipart.NewWriter(&buffer)
				part, _ := writer.CreateFormField("request")
				part.Write([]byte(`{"model":"jev-latest","state":"photo","questions":{"ok":{"type":"noul"}},"assets":{"photo":{"type":"image","part":"image"}}}`))
				part, _ = writer.CreateFormFile("image", "photo.png")
				part.Write([]byte("image-bytes"))
				writer.Close()
				wire = buffer.Bytes()
				contentType = writer.FormDataContentType()
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if !bytes.Equal(body, wire) || r.Header.Get("Content-Encoding") != headerEncoding {
					t.Error("wire was not preserved")
				}
				w.Header().Set("Content-Type", "application/json")
				if encoding == "multipart" {
					io.WriteString(w, `{"model":"jev-preview","answers":{"ok":{"type":"noul","noul":0.5}},"usage":{"input_tokens":null,"output_tokens":null}}`)
				} else {
					io.WriteString(w, systemOneResponse)
				}
			}))
			defer upstream.Close()
			server := systemOneServer(t, upstream)
			status, body, _ := systemOneCall(t, server, wire, contentType, headerEncoding)
			if status != 200 {
				t.Fatalf("status=%d body=%s", status, body)
			}
		})
	}
}
func TestSystemOneValidationNoDispatch(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	server := systemOneServer(t, upstream)
	for _, body := range []string{`{}`, `{"model":"jev-latest","state":null,"questions":{"ok":{"type":"noul"}}}`, `{"model":"jev-latest","state":"x","questions":{"score":{"type":"score","criteria":[]}}}`, `{"model":"jev-latest","state":"x","questions":{"ok":{"type":"noul","instructions":4}}}`, `{"model":"jev-latest","state":"x","questions":{"ok":{"type":"noul"}},"assets":null}`, `{"model":"jev-latest","state":1e999,"questions":{"ok":{"type":"noul"}}}`, `{"model":"jev-latest","state":` + strings.Repeat("[", 65) + `"x"` + strings.Repeat("]", 65) + `,"questions":{"ok":{"type":"noul"}}}`} {
		status, data, _ := systemOneCall(t, server, []byte(body), "application/json", "")
		if (status != 422 && status != 400) || !bytes.Contains(data, []byte(`"detail"`)) {
			t.Fatalf("status=%d body=%s", status, data)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("provider calls=%d", calls.Load())
	}
}
func TestSystemOneProviderErrorsAndMalformedAnswers(t *testing.T) {
	for _, test := range []struct {
		name     string
		status   int
		response string
		want     int
	}{
		{"rate-limit", 429, `{"detail":{"message":"provider-secret rate limited","code":"rate_limit"}}`, 429},
		{"validation", 422, `{"detail":[{"loc":["body","state"],"msg":"bad provider-secret","type":"value_error"}]}`, 422},
		{"malformed-json", 200, `not json`, 502},
		{"missing-answer", 200, `{"model":"jev","answers":{"other":{"type":"noul","noul":1}},"usage":{"input_tokens":0,"output_tokens":0}}`, 502},
		{"wrong-type", 200, strings.Replace(systemOneResponse, `"type":"noul","noul":0.8`, `"type":"box","box":{"top":0,"bottom":1,"left":0,"right":1},"confidence":null`, 1), 502},
		{"invalid-probability-type", 200, strings.Replace(systemOneResponse, `"noul":0.8`, `"noul":"yes"`, 1), 502},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "9")
				w.WriteHeader(test.status)
				io.WriteString(w, test.response)
			}))
			defer upstream.Close()
			server := systemOneServer(t, upstream)
			status, body, headers := systemOneCall(t, server, []byte(systemOneRequest), "application/json", "")
			if status != test.want || bytes.Contains(body, []byte("provider-secret")) || headers.Get("Retry-After") != "9" {
				t.Fatalf("status=%d body=%s headers=%v", status, body, headers)
			}
			if calls.Load() != 1 {
				t.Fatalf("calls=%d", calls.Load())
			}
		})
	}
}
func TestSystemOneDeadlineAndCancellation(t *testing.T) {
	for _, cancelCaller := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelCaller), func(t *testing.T) {
			started := make(chan struct{})
			ended := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				close(started)
				<-r.Context().Done()
				close(ended)
			}))
			defer upstream.Close()
			server := systemOneServer(t, upstream)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/v1/systemone", strings.NewReader(systemOneRequest))
			request.Header.Set("Authorization", "Bearer "+TestSecret)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("TypeSafe-Danger-Zone-Images", "true")
			done := make(chan error, 1)
			go func() {
				response, err := server.Client().Do(request)
				if response != nil {
					body, _ := io.ReadAll(response.Body)
					response.Body.Close()
					if response.StatusCode != 504 || !bytes.Contains(body, []byte(`"detail"`)) {
						t.Errorf("status=%d body=%s", response.StatusCode, body)
					}
				}
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("provider never started")
			}
			if cancelCaller {
				cancel()
			}
			select {
			case err := <-done:
				if cancelCaller && err == nil {
					t.Error("caller cancellation did not fail client")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("request did not finish")
			}
			select {
			case <-ended:
			case <-time.After(3 * time.Second):
				t.Fatal("provider work was not cancelled")
			}
		})
	}
}

func TestSystemOneHTTPBoundaryFailures(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, systemOneResponse)
	}))
	defer upstream.Close()
	server := systemOneServer(t, upstream)
	for _, test := range []struct {
		name, body, contentType, encoding string
		want                              int
	}{
		{"malformed", "{", "application/json", "", 400},
		{"encoding", systemOneRequest, "application/json", "br", 415},
		{"media", systemOneRequest, "text/plain", "", 415},
		{"bad-media", systemOneRequest, ";bad", "", 415},
		{"gzip-header", "invalid", "application/json", "gzip", 400},
		{"zstd-header", "invalid", "application/json", "zstd", 400},
		{"multipart-boundary", "invalid", "multipart/form-data; boundary=missing", "", 400},
		{"size", strings.Repeat("x", 32<<20+1), "application/json", "", 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, body, _ := systemOneCall(t, server, []byte(test.body), test.contentType, test.encoding)
			if status != test.want || !bytes.Contains(body, []byte(`"detail"`)) {
				t.Fatalf("status=%d body=%s", status, body)
			}
		})
	}
	for _, test := range []struct {
		method, auth, timeout, images string
		want                          int
	}{
		{"GET", "Bearer " + TestSecret, "", "true", 405},
		{"POST", "invalid", "", "true", 401},
		{"POST", "Bearer " + TestSecret + " extra", "", "true", 401},
		{"POST", "Bearer " + TestSecret, "bad", "true", 400},
		{"POST", "Bearer " + TestSecret, "", "", 400},
		{"POST", "Bearer " + TestSecret, "", "false", 400},
	} {
		t.Run(test.method+test.auth+test.timeout+test.images, func(t *testing.T) {
			request, _ := http.NewRequest(test.method, server.URL+"/v1/systemone", strings.NewReader(systemOneRequest))
			request.Header.Set("Authorization", test.auth)
			request.Header.Set("Content-Type", "application/json")
			if test.timeout != "" {
				request.Header.Set("X-LLM-Proxy-Request-Timeout-Seconds", test.timeout)
			}
			if test.images != "" {
				request.Header.Set("TypeSafe-Danger-Zone-Images", test.images)
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != test.want {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("provider calls=%d", calls.Load())
	}
}
func TestSystemOneStructuredDepthBoundary(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"model":"jev-1.14.0","answers":{"ok":{"type":"noul","noul":1}},"usage":{"input_tokens":0,"output_tokens":0}}`)
	}))
	defer upstream.Close()
	server := systemOneServer(t, upstream)
	for _, depth := range []int{64, 65} {
		body := `{"model":"jev-latest","state":` + strings.Repeat("[", depth) + `"x"` + strings.Repeat("]", depth) + `,"questions":{"ok":{"type":"noul"}}}`
		status, data, _ := systemOneCall(t, server, []byte(body), "application/json", "")
		want := 200
		if depth == 65 {
			want = 400
		}
		if status != want {
			t.Fatalf("depth=%d status=%d body=%s", depth, status, data)
		}
	}
}

func TestSystemOneMultipartValidation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid multipart reached provider") }))
	defer upstream.Close()
	server := systemOneServer(t, upstream)
	for _, scenario := range []string{"first", "duplicate", "unreferenced", "missing", "truncated", "request-truncated", "part-truncated"} {
		t.Run(scenario, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := multipart.NewWriter(&buffer)
			name := "request"
			if scenario == "first" {
				name = "image"
			}
			part, _ := writer.CreateFormField(name)
			body := `{"model":"jev-latest","state":"x","questions":{"ok":{"type":"noul"}}}`
			if scenario == "missing" {
				body = `{"model":"jev-latest","state":"x","questions":{"ok":{"type":"noul"}},"assets":{"x":{"part":"absent"}}}`
			}
			part.Write([]byte(body))
			if scenario == "unreferenced" || scenario == "duplicate" || scenario == "part-truncated" {
				part, _ = writer.CreateFormField("image")
				part.Write([]byte("image"))
			}
			if scenario == "duplicate" {
				part, _ = writer.CreateFormField("image")
				part.Write([]byte("image2"))
			}
			writer.Close()
			wire := buffer.Bytes()
			if scenario == "truncated" {
				wire = append(wire[:len(wire)-len(writer.Boundary())-8], []byte("\r\n--"+writer.Boundary()+"\r\ninvalid\r\n\r\n")...)
			}
			if scenario == "request-truncated" || scenario == "part-truncated" {
				wire = wire[:len(wire)-len(writer.Boundary())-8]
			}
			status, data, _ := systemOneCall(t, server, wire, writer.FormDataContentType(), "")
			if status != 400 {
				t.Fatalf("status=%d body=%s", status, data)
			}
		})
	}
}
func TestSystemOneCompressedBoundsAndHeaderValidation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid body reached provider") }))
	defer upstream.Close()
	server := systemOneServer(t, upstream)
	for _, scenario := range []string{"gzip-large", "zstd-large", "gzip-truncated", "experimental", "deep-instructions"} {
		t.Run(scenario, func(t *testing.T) {
			wire := []byte(systemOneRequest)
			encoding := ""
			want := 400
			switch scenario {
			case "gzip-large":
				var buffer bytes.Buffer
				writer := gzip.NewWriter(&buffer)
				writer.Write(bytes.Repeat([]byte("x"), 32<<20+1))
				writer.Close()
				wire = buffer.Bytes()
				encoding = "gzip"
				want = 413
			case "zstd-large":
				writer, _ := zstd.NewWriter(nil, zstd.WithWindowSize(1<<20))
				wire = writer.EncodeAll(bytes.Repeat([]byte("x"), 32<<20+1), nil)
				writer.Close()
				encoding = "zstd"
				want = 413
			case "gzip-truncated":
				var buffer bytes.Buffer
				writer := gzip.NewWriter(&buffer)
				writer.Write(wire)
				writer.Close()
				wire = buffer.Bytes()[:buffer.Len()-5]
				encoding = "gzip"
			case "deep-instructions":
				wire = []byte(`{"model":"jev-latest","state":"x","questions":{"ok":{"type":"noul","instructions":` + strings.Repeat(`{"x":`, 65) + `"x"` + strings.Repeat("}", 65) + `}}}`)
			}
			request, _ := http.NewRequest("POST", server.URL+"/v1/systemone", bytes.NewReader(wire))
			request.Header.Set("Authorization", "Bearer "+TestSecret)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("TypeSafe-Danger-Zone-Images", "true")
			request.Header.Set("Content-Encoding", encoding)
			if scenario == "experimental" {
				request.Header.Set("TypeSafe-Danger-Zone-Long-Context", "false")
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != want {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
		})
	}
}

func TestSystemOneUsageSurvivesRestart(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			io.WriteString(w, `{"models":[{"name":"jev-1.14.0","description":"Jev","release_date":"2026-11-11"}]}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		response := systemOneResponse
		if bytes.Contains(body, []byte(`"nullable"`)) {
			response = strings.Replace(response, `"input_tokens":4,"output_tokens":2`, `"input_tokens":null,"output_tokens":null`, 1)
		}
		io.WriteString(w, response)
	}))
	defer upstream.Close()
	path := t.TempDir() + "/usage.db"
	configuration := proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, "typesafe")}
	router := newManagementRouterWithDatabasePath(t, configuration, path)
	owner := managementSessionCookie(t, "system-one-usage-owner")
	tenant := requestManagementAccount(t, router, owner).Tenants[0].ID
	connection := accountConnectionExchange(t, router, owner, "POST", "/connections", map[string]any{"name": "TypeSafe", "provider": "typesafe", "fields": map[string]string{"api_key": "provider-secret"}}, 201)
	accountConnectionExchange(t, router, owner, "PUT", "/tenants/"+tenant+"/connections/typesafe", map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, 200)
	key := generateManagementTenantSecret(t, router, owner, tenant)
	server := httptest.NewServer(router)
	for _, state := range []string{"test", "nullable"} {
		request, _ := http.NewRequest("POST", server.URL+"/v1/systemone", strings.NewReader(strings.Replace(systemOneRequest, `"test"`, `"`+state+`"`, 1)))
		request.Header.Set("Authorization", "Bearer "+key)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("TypeSafe-Danger-Zone-Images", "true")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("status=%d body=%s", response.StatusCode, body)
		}
	}
	waitForManagementValue(t, func() managementTenantUsageTestResponse {
		return requestManagementTenantUsage(t, router, owner, tenant)
	}, func(value managementTenantUsageTestResponse) bool { return value.Totals.Requests == 2 })
	server.Close()
	restarted := newManagementRouterWithDatabasePath(t, configuration, path)
	request := httptest.NewRequest("GET", managementTenantTestPath(tenant, "/usage")+"?interval=30d", nil)
	request.AddCookie(owner)
	record := httptest.NewRecorder()
	restarted.ServeHTTP(record, request)
	if !bytes.Contains(record.Body.Bytes(), []byte(`"provider":"typesafe"`)) || !bytes.Contains(record.Body.Bytes(), []byte(`"model":"jev-1.14.0"`)) {
		t.Fatalf("usage route dimensions lost across restart: %s", record.Body.String())
	}
	if usage := requestManagementTenantUsage(t, restarted, owner, tenant); usage.Totals.Requests != 2 {
		t.Fatalf("retained usage=%+v", usage)
	}
}
func TestSystemOneStalledResponseReadDeadline(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer upstream.Close()
	server := systemOneServer(t, upstream)
	status, body, _ := systemOneCall(t, server, []byte(systemOneRequest), "application/json", "")
	if status != 504 {
		t.Fatalf("status=%d body=%s", status, body)
	}
}

func TestSystemOneBearerSchemeCase(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-TypeSafe-Request-ID", "typesafe-id")
		io.WriteString(w, systemOneResponse)
	}))
	defer upstream.Close()
	server := systemOneServer(t, upstream)
	for _, scheme := range []string{"bearer", "BEARER", "Bearer"} {
		request, _ := http.NewRequest("POST", server.URL+"/v1/systemone", strings.NewReader(systemOneRequest))
		request.Header.Set("Authorization", scheme+" "+TestSecret)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("TypeSafe-Danger-Zone-Images", "true")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || response.Header.Get("X-TypeSafe-Request-ID") != "typesafe-id" {
			t.Fatalf("scheme=%s status=%d headers=%v body=%s", scheme, response.StatusCode, response.Header, body)
		}
	}
}

func TestSystemOneErrorWirePreservation(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		body       string
		wantStatus int
		want       string
	}{
		{"clean400", 400, "{ \n \"detail\": {\"message\": \"rejected\", \"count\": 9007199254740993} }\n", 400, ""},
		{"clean429", 429, "{ \"detail\" : {\"retry_count\":9007199254740993} }\n", 429, ""},
		{"escaped-secret", 400, `{"detail":{"provider\u002dsecret":"provider\u002dsecret","items":["provider\u002dsecret",{"number":9007199254740993,"message":"provider\u002dsecret"}]}}`, 400, `{"detail":{"[redacted]":"[redacted]","items":["[redacted]",{"message":"[redacted]","number":9007199254740993}]}}`},
		{"trailing-json", 400, `{"detail":"rejected"} {"other":true}`, 502, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				io.WriteString(w, test.body)
			}))
			defer upstream.Close()
			server := systemOneServer(t, upstream)
			status, body, _ := systemOneCall(t, server, []byte(systemOneRequest), "application/json", "")
			if status != test.wantStatus {
				t.Fatalf("status=%d body=%s", status, body)
			}
			if test.wantStatus == 502 {
				return
			}
			want := test.want
			if want == "" {
				want = test.body
			}
			if string(body) != want {
				t.Fatalf("error wire changed:\n got %s\nwant %s", body, want)
			}
			if strings.Contains(string(body), "provider-secret") || strings.Contains(string(body), `provider\u002dsecret`) {
				t.Fatal("decoded credential leaked")
			}
		})
	}
}
