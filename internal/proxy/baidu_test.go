package proxy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

var baiduModels = []string{"ernie-5.0", "deepseek-v4-pro", "deepseek-v4-flash", "deepseek-v3.2"}
var baiduOutputLimits = []int{65536, 131072, 131072, 32768}

func baiduResponse(text, reason, flag string) string {
	flagField := ""
	if flag != "" {
		flagField = `,"flag":` + flag
	}
	return fmt.Sprintf(`{"choices":[{"message":{"role":"assistant","content":%q},"finish_reason":%q%s}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`, text, reason, flagField)
}

func TestBaiduTextRoutes(t *testing.T) {
	for index, model := range baiduModels {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-baidu" || r.Header.Get("appid") != "" || payload["model"] != model || payload["max_tokens"] != float64(baiduOutputLimits[index]) {
					t.Errorf("unexpected Baidu request path=%s payload=%v", r.URL.Path, payload)
				}
				for _, field := range []string{"max_completion_tokens", "max_output_tokens", "thinking", "reasoning_effort", "tools", "stream", "response_format"} {
					if _, ok := payload[field]; ok {
						t.Errorf("unexpected field=%s", field)
					}
				}
				messages := payload["messages"].([]any)
				if messages[0].(map[string]any)["content"] != "test" {
					t.Errorf("messages=%v", messages)
				}
				w.Header().Set("Content-Type", "application/json")
				if calls%2 == 1 {
					if len(messages) != 1 {
						t.Errorf("initial messages=%v", messages)
					}
					io.WriteString(w, baiduResponse("visible ", "length", "1"))
				} else {
					if len(messages) != 3 || messages[1].(map[string]any)["role"] != "assistant" || messages[1].(map[string]any)["content"] != "visible " || messages[2].(map[string]any)["role"] != "user" {
						t.Errorf("continuation=%v", messages)
					}
					io.WriteString(w, baiduResponse("answer", "stop", "0"))
				}
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"baidu": upstream.URL + "/v1"}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			for _, route := range []string{"GET", "POST", "v2"} {
				requestURL := server.URL + "/?key=" + TestSecret + "&provider=baidu"
				var request *http.Request
				if route == "GET" {
					request, err = http.NewRequest("GET", requestURL+fmt.Sprintf("&model=%s&prompt=test&max_tokens=%d", model, baiduOutputLimits[index]), nil)
				} else {
					payload := map[string]any{"model": model, "max_tokens": baiduOutputLimits[index], "prompt": "test"}
					if route == "v2" {
						requestURL = server.URL + "/v2?key=" + TestSecret + "&provider=baidu"
						delete(payload, "prompt")
						payload["messages"] = []any{map[string]string{"role": "user", "content": "test"}}
					}
					body, _ := json.Marshal(payload)
					request, err = http.NewRequest("POST", requestURL, strings.NewReader(string(body)))
					if err == nil {
						request.Header.Set("Content-Type", "application/json")
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != 200 || string(body) != "visible answer" || response.Header.Get("X-LLM-Proxy-Total-Tokens") != "10" {
					t.Fatalf("route=%s status=%d body=%s headers=%v", route, response.StatusCode, body, response.Header)
				}
			}
			for _, route := range []string{"GET", "POST", "v2"} {
				path := "/?key=" + TestSecret + "&provider=baidu&model=" + model + "&prompt=test&reasoning_effort=high"
				method := "GET"
				body := ""
				if route != "GET" {
					method = "POST"
					body = fmt.Sprintf(`{"model":%q,"prompt":"test","reasoning_effort":"high"}`, model)
				}
				if route == "v2" {
					path = "/v2?key=" + TestSecret + "&provider=baidu"
					body = fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"test"}],"reasoning_effort":"high"}`, model)
				}
				request := httptest.NewRequest(method, path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != 400 || calls != 6 {
					t.Fatalf("unsupported effort route=%s status=%d calls=%d", route, response.Code, calls)
				}
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/?key=%s&provider=baidu&model=%s&prompt=test&max_tokens=%d", TestSecret, model, baiduOutputLimits[index]+1), nil))
			if response.Code != 400 || calls != 6 {
				t.Fatalf("output limit status=%d calls=%d", response.Code, calls)
			}
			for _, kind := range []string{"image", "audio"} {
				mime := "image/png"
				if kind == "audio" {
					mime = "audio/wav"
				}
				body := mediaV2RequestBody(t, model, "test", []map[string]any{messageMediaPayload(kind, mime, []byte("input"))})
				request := httptest.NewRequest("POST", "/v2?key="+TestSecret+"&provider=baidu", strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != 400 || calls != 6 {
					t.Fatalf("unsupported media=%s status=%d calls=%d", kind, response.Code, calls)
				}
			}
		})
	}
}

func TestBaiduResponsePolicy(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
	}{
		{"omitted flag", baiduResponse("safe", "stop", ""), 200},
		{"low risk", baiduResponse("safe", "stop", "1"), 200},
		{"banned", baiduResponse("private", "stop", "2"), 502},
		{"hidden", baiduResponse("private", "stop", "3"), 502},
		{"remove", baiduResponse("private", "stop", "4"), 502},
		{"unknown flag", baiduResponse("private", "stop", "5"), 502},
		{"negative flag", baiduResponse("private", "stop", "-1"), 502},
		{"string flag", baiduResponse("private", "stop", `"0"`), 502},
		{"null flag", baiduResponse("private", "stop", "null"), 502},
		{"fraction flag", baiduResponse("private", "stop", "0.5"), 502},
		{"blocked length", baiduResponse("private", "length", "2"), 502},
		{"filter", baiduResponse("private", "content_filter", "0"), 502},
		{"tools", baiduResponse("private", "tool_calls", "0"), 502},
		{"unknown finish", baiduResponse("private", "unknown", "0"), 502},
		{"missing finish", baiduResponse("private", "", "0"), 502},
		{"malformed", `{"choices":`, 502},
		{"missing choices", `{}`, 502},
		{"empty choices", `{"choices":[]}`, 502},
		{"empty content", baiduResponse("", "stop", "0"), 502},
		{"invalid usage", strings.Replace(baiduResponse("private", "stop", "0"), `"prompt_tokens":2`, `"prompt_tokens":-1`, 1), 502},
		{"second blocked choice", `{"choices":[{"message":{"content":"private"},"finish_reason":"stop","flag":0},{"message":{"content":"private"},"finish_reason":"stop","flag":2}]}`, 502},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, scenario.body)
			}))
			defer upstream.Close()
			core, logs := observer.New(zap.DebugLevel)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"baidu": upstream.URL}, nil)}, zap.New(core).Sugar())
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", "/?key="+TestSecret+"&provider=baidu&prompt=test", nil))
			if response.Code != scenario.status || calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body)
			}
			if strings.Contains(response.Body.String(), "private") {
				t.Fatal("unsafe response text exposed")
			}
			assertProviderKeyVerificationLogsAreSafe(t, logs, "private", "sk-baidu", scenario.body)
		})
	}
}

func TestBaiduProviderErrors(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		status   int
		timeout  bool
		expected int
	}{{"unauthorized", 401, false, 502}, {"rate limited", 429, false, 429}, {"upstream error", 500, false, 502}, {"timeout", 0, true, 504}} {
		t.Run(scenario.name, func(t *testing.T) {
			previous := proxy.HTTPClient
			proxy.HTTPClient = coverageHTTPDoer(func(r *http.Request) (*http.Response, error) {
				if scenario.timeout {
					return nil, context.DeadlineExceeded
				}
				return coverageHTTPResponse(scenario.status, "private Baidu response"), nil
			})
			defer func() { proxy.HTTPClient = previous }()
			core, logs := observer.New(zap.DebugLevel)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.New(core).Sugar())
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", "/?key="+TestSecret+"&provider=baidu&prompt=test", nil))
			if response.Code != scenario.expected || strings.Contains(response.Body.String(), "private") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			assertProviderKeyVerificationLogsAreSafe(t, logs, "private Baidu response", "sk-baidu")
		})
	}
}

func TestBaiduCatalog(t *testing.T) {
	catalog := testfixtures.ProviderCatalog(t)
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", proxy.PublicCapabilitiesPath, nil))
	var public proxy.PublicCapabilityCatalog
	if response.Code != 200 {
		t.Fatal(response.Code)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	for index, model := range baiduModels {
		i := slices.IndexFunc(public.Offerings, func(o proxy.PublicProviderOffering) bool { return o.Provider == "baidu" && o.Model == model })
		if i < 0 {
			t.Fatal(model)
		}
		offering := public.Offerings[i]
		if offering.OutputTokenLimit != baiduOutputLimits[index] || !reflect.DeepEqual(offering.Capabilities, []string{"text"}) || offering.WireContract != "openai_chat_completions" || offering.ExecutionLifecycle != "synchronous_completion" || len(offering.ReasoningEfforts) != 0 {
			t.Fatalf("offering=%+v", offering)
		}
		count := 0
		for _, exact := range public.Models {
			if exact.Identifier == model {
				count++
				if index == 1 || index == 2 {
					if !slices.Contains(exact.ProviderOfferings, "baidu:"+model) || !slices.Contains(exact.ProviderOfferings, "deepseek:"+model) {
						t.Fatalf("shared model=%+v", exact)
					}
				}
			}
		}
		if count != 1 {
			t.Fatalf("model=%s count=%d", model, count)
		}
	}
	for _, provider := range catalog.Schema().Providers {
		if provider.ID == "baidu" {
			if provider.Label != "Baidu Qianfan" || len(provider.Aliases) != 0 || provider.Offerings[0].Model != "ernie-5.0" || !slices.Contains(provider.Offerings[0].DefaultOperations, "text") {
				t.Fatalf("provider=%+v", provider)
			}
		}
	}
	for _, policy := range []string{"unknown", ""} {
		schema := catalog.Schema()
		for i := range schema.Providers {
			if schema.Providers[i].ID == "baidu" {
				schema.Providers[i].Transports[0].ProtocolParameters.ResponsePolicy = policy
			}
		}
		if _, err := proxy.NewProviderCatalog(schema); err == nil {
			t.Fatalf("inconsistent response policy=%q accepted", policy)
		}
	}
}
