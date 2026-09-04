package proxy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestClientProtocolsBackgroundPartialFunctionCalls(t *testing.T) {
	for _, protocol := range []struct{ path, body string }{
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":"read","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]}`},
		{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"read"}],"tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object"}}}]}`},
	} {
		for _, terminal := range []struct {
			name, arguments string
			status          int
		}{
			{"complete", `{"path":"hello.txt"}`, http.StatusOK},
			{"malformed", `{"path":`, http.StatusBadGateway},
		} {
			t.Run(protocol.path+"/"+terminal.name, func(t *testing.T) {
				var polls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodPost {
						var payload struct {
							Background bool `json:"background"`
						}
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || !payload.Background {
							t.Errorf("expected background request: payload=%+v err=%v", payload, err)
						}
						io.WriteString(w, `{"id":"pending-call","status":"queued","output":[]}`)
						return
					}
					if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/responses/pending-call") {
						t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
					}
					status, arguments := "in_progress", ""
					switch polls.Add(1) {
					case 1:
						status = "queued"
					case 2:
					case 3:
						arguments = `{"path":`
					default:
						status, arguments = "completed", terminal.arguments
					}
					if err := json.NewEncoder(w).Encode(map[string]any{
						"id": "pending-call", "status": status,
						"output": []any{map[string]string{"type": "function_call", "status": status, "call_id": "call_read", "name": "read", "arguments": arguments}},
					}); err != nil {
						t.Error(err)
					}
				}))
				defer upstream.Close()
				router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, nil)}, zap.NewNop().Sugar())
				if err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(router)
				defer server.Close()
				request, err := http.NewRequest(http.MethodPost, server.URL+protocol.path, strings.NewReader(protocol.body))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer "+TestSecret)
				request.Header.Set("Content-Type", "application/json")
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != terminal.status || polls.Load() != 4 {
					t.Fatalf("status=%d want=%d polls=%d want=4 body=%s", response.StatusCode, terminal.status, polls.Load(), body)
				}
				if terminal.status == http.StatusOK {
					arguments, _ := json.Marshal(terminal.arguments)
					if !strings.Contains(string(body), string(arguments)) || !strings.Contains(string(body), `"call_read"`) {
						t.Fatalf("completed function call missing: %s", body)
					}
				}
			})
		}
	}
}

func TestClientProtocolsRegistryIsAtomic(t *testing.T) {
	handler := func(c *gin.Context) { c.String(200, "registered") }
	for _, routes := range [][]proxy.ClientProtocolRoute{
		{{Method: "POST", Path: "/duplicate", Handler: handler}, {Method: "POST", Path: "/duplicate", Handler: handler}},
		{{Method: "POST", Path: "/resource", Handler: handler}},
		{{Method: "PATCH", Path: "/invalid", Handler: handler}},
		{{Method: "POST", Path: "invalid", Handler: handler}},
		{{Method: "POST", Path: "/invalid"}},
	} {
		router := gin.New()
		router.POST("/resource", handler)
		err := proxy.RegisterClientProtocols(router, []proxy.ClientProtocolAdapter{{Name: "test", Routes: routes}})
		if err == nil || len(router.Routes()) != 1 {
			t.Fatalf("partial registration: err=%v routes=%v", err, router.Routes())
		}
	}
	router := gin.New()
	if err := proxy.RegisterClientProtocols(router, []proxy.ClientProtocolAdapter{{Routes: []proxy.ClientProtocolRoute{{Method: "POST", Path: "/invalid", Handler: handler}}}}); err == nil {
		t.Fatal("unnamed adapter accepted")
	}
}

func TestClientProtocolsCancellationAndTimeout(t *testing.T) {
	for _, protocol := range []struct{ path, body string }{{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"hello"}],"stream":true}`}, {"/v1/responses", `{"model":"openai/gpt-5.6","input":"hello","stream":true}`}} {
		t.Run(protocol.path, func(t *testing.T) {
			started := make(chan struct{}, 2)
			cancelled := make(chan struct{}, 2)
			release := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				started <- struct{}{}
				select {
				case <-r.Context().Done():
					cancelled <- struct{}{}
				case <-release:
				}
			}))
			defer upstream.Close()
			defer close(release)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{RequestTimeoutSeconds: 1, MaxRequestTimeoutSeconds: 2, Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			request, _ := http.NewRequest("POST", server.URL+protocol.path, strings.NewReader(protocol.body))
			request.Header.Set("Authorization", "Bearer "+TestSecret)
			request.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 504 || !strings.Contains(string(data), "request_timeout") {
				t.Fatalf("timeout status=%d body=%s", response.StatusCode, data)
			}
			select {
			case <-cancelled:
			case <-time.After(3 * time.Second):
				t.Fatal("timeout left provider active")
			}
			<-started
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request = request.Clone(ctx)
			request.Body = io.NopCloser(strings.NewReader(protocol.body))
			finished := make(chan error, 1)
			go func() {
				response, err := server.Client().Do(request)
				if response != nil {
					response.Body.Close()
				}
				finished <- err
			}()
			<-started
			cancel()
			if err := <-finished; err == nil {
				t.Fatal("cancelled request succeeded")
			}
			select {
			case <-cancelled:
			case <-time.After(3 * time.Second):
				t.Fatal("caller cancellation left provider active")
			}
		})
	}
}

func TestClientProtocolsProviderErrors(t *testing.T) {
	for _, status := range []int{400, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if status == 429 {
					w.Header().Set("Retry-After", "2")
				}
				w.WriteHeader(status)
				io.WriteString(w, `{"error":{"message":"protected provider body","code":"invalid_request_error"}}`)
			}))
			defer upstream.Close()
			core, entries := observer.New(zap.DebugLevel)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameDeepSeek: upstream.URL}, nil)}, zap.New(core).Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			request, _ := http.NewRequest("POST", server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"deepseek/deepseek-chat","messages":[{"role":"user","content":"private prompt"}]}`))
			request.Header.Set("Authorization", "Bearer "+TestSecret)
			request.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			var body map[string]any
			if json.Unmarshal(data, &body) != nil || strings.Contains(string(data), "protected provider body") || strings.Contains(string(data), "private prompt") {
				t.Fatalf("unsafe error=%s", data)
			}
			expected := 502
			if status == 429 {
				expected = 429
			}
			if response.StatusCode != expected {
				t.Fatalf("status=%d body=%s", response.StatusCode, data)
			}
			logged, _ := json.Marshal(entries.AllUntimed())
			if len(entries.AllUntimed()) == 0 {
				t.Fatal("request log evidence missing")
			}
			for _, protected := range []string{"protected provider body", "private prompt", TestSecret} {
				if strings.Contains(string(logged), protected) {
					t.Fatal("protected data in request logs")
				}
			}
			if status == 429 && response.Header.Get("Retry-After") != "2" {
				t.Fatal("missing retry header")
			}
		})
	}
}

func TestClientProtocolsCapacity(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output_text":"done"}`)
	}))
	defer upstream.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{WorkerCount: 1, QueueSize: 1, Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	results := make(chan int, 3)
	send := func() {
		req, _ := http.NewRequest("POST", server.URL+"/v1/responses", strings.NewReader(`{"model":"openai/gpt-5.6","input":"hi"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+TestSecret)
		res, err := server.Client().Do(req)
		if err != nil {
			results <- 0
			return
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		results <- res.StatusCode
	}
	go send()
	<-started
	go send()
	go send()
	select {
	case status := <-results:
		if status != 503 {
			t.Errorf("capacity status=%d want=503", status)
		}
	case <-time.After(5 * time.Second):
		t.Error("capacity response missing")
	}
	close(release)
	for i := 0; i < 2; i++ {
		if status := <-results; status != 200 {
			t.Errorf("accepted status=%d", status)
		}
	}
}

type observedClientBody struct {
	io.ReadCloser
	reading chan struct{}
}

func (body *observedClientBody) Read(data []byte) (int, error) {
	select {
	case body.reading <- struct{}{}:
	default:
	}
	return body.ReadCloser.Read(data)
}
func TestClientProtocolsCancelledBody(t *testing.T) {
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	reading := make(chan struct{}, 1)
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = &observedClientBody{ReadCloser: r.Body, reading: reading}
		router.ServeHTTP(w, r)
		close(finished)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer writer.Close()
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/v1/responses", reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+TestSecret)
	clientDone := make(chan error, 1)
	go func() {
		res, err := server.Client().Do(req)
		if res != nil {
			res.Body.Close()
		}
		clientDone <- err
	}()
	writer.Write([]byte("{"))
	<-reading
	cancel()
	writer.Close()
	if err := <-clientDone; err == nil {
		t.Fatal("cancelled body succeeded")
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("body cancellation did not finish handler")
	}
}

func TestClientProtocolsBodyCancellationResponse(t *testing.T) {
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer writer.Close()
	reading := make(chan struct{}, 1)
	req := httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
	req.Body = &observedClientBody{ReadCloser: reader, reading: reading}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+TestSecret)
	finished := make(chan struct{})
	res := httptest.NewRecorder()
	go func() { router.ServeHTTP(res, req); close(finished) }()
	<-reading
	cancel()
	<-finished
	if res.Code != 499 || !strings.Contains(res.Body.String(), "request_cancelled") {
		t.Fatalf("body cancellation status=%d body=%s", res.Code, res.Body)
	}
}
