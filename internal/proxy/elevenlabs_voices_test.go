package proxy_test

import (
	"context"
	"encoding/json"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
)

func TestElevenLabsVoicesPreservePagesAndPrivateAccountAuthority(t *testing.T) {
	for _, provider := range []string{"elevenlabs", "voice-resource-fixture"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			var previewOrigin string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/private-preview.mp3" {
					if r.Header.Get("xi-api-key") != "" || r.Header.Get("Authorization") != "" {
						t.Error("credential leaked to preview origin")
					}
					w.Header().Set("Content-Type", "audio/mpeg")
					_, _ = w.Write([]byte("ID3preview"))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/user/subscription" {
					_, _ = io.WriteString(w, elevenQuotaFixture)
					return
				}
				if r.URL.Path != "/v2/voices" || r.Method != "GET" || r.Header.Get("xi-api-key") == "" {
					t.Errorf("unexpected native voice request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				calls.Add(1)
				if r.URL.Query().Get("page_size") != "1" || r.URL.Query().Get("search") != "reader" || r.URL.Query().Get("sort") != "name" || r.URL.Query().Get("sort_direction") != "asc" || r.URL.Query().Get("voice_type") != "personal" || r.URL.Query().Get("category") != "cloned" || r.URL.Query().Get("include_total_count") != "true" {
					t.Errorf("lost source query fields: %s", r.URL.RawQuery)
				}
				if r.URL.Query().Get("next_page_token") == "native-page-secret" {
					_, _ = io.WriteString(w, `{"voices":[],"has_more":false,"total_count":1,"next_page_token":null}`)
					return
				}
				_, _ = io.WriteString(w, strings.ReplaceAll(`{"voices":[{"voice_id":"native-voice-secret","name":"Reader","category":"cloned","description":"A calm reader","labels":{"accent":"american"},"high_quality_base_model_ids":["eleven_v3"],"verified_languages":[{"language":"en","model_id":"eleven_v3","accent":"american","locale":"en-US","preview_url":"PREVIEW/private-preview.mp3"}],"preview_url":"PREVIEW/private-preview.mp3"}],"has_more":true,"total_count":1,"next_page_token":"native-page-secret"}`, "PREVIEW", previewOrigin))
			}))
			defer upstream.Close()
			previewOrigin = upstream.URL
			router := newManagementRouter(t, proxy.Configuration{UpstreamCapacity: elevenVoicePreviewCapacity(t), ProviderCatalog: elevenResourceCatalog(t, provider, upstream.URL)})
			owner := managementSessionCookie(t, "voice-owner")
			tenant := managementDefaultTenantTestID(t, router, owner)
			key := generateManagementTenantSecret(t, router, owner, tenant)
			conn := accountConnectionExchange(t, router, owner, "POST", "/connections", map[string]any{"name": "Voices", "provider": provider, "fields": map[string]string{"resource_token": "voice-secret"}}, 201)
			accountConnectionExchange(t, router, owner, "PUT", "/tenants/"+tenant+"/connections/"+provider, map[string]string{"connection_id": conn["id"].(string)}, 200)
			server := httptest.NewServer(router)
			defer server.Close()
			read := func(query string, want int) map[string]any {
				t.Helper()
				req, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/model/v1/voices?provider="+provider+query, nil)
				req.Header.Set("Authorization", "Bearer "+key)
				resp, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				if resp.StatusCode != want {
					t.Fatalf("voice discovery status=%d want=%d body=%s", resp.StatusCode, want, body)
				}
				if strings.Contains(string(body), "native-") || strings.Contains(string(body), "voice-secret") {
					t.Fatalf("private voice evidence leaked: %s", body)
				}
				var result map[string]any
				if err := json.Unmarshal(body, &result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			first := read("&page_size=1&search=reader&sort=name&sort_direction=asc&voice_type=personal&category=cloned&include_total_count=true", 200)
			voices := first["voices"].([]any)
			if len(voices) != 1 {
				t.Fatalf("voices=%v", voices)
			}
			voice := voices[0].(map[string]any)
			if voice["language"] != nil || voice["default_sample_rate"] != nil || len(voice["sample_rates"].([]any)) != 0 || voice["description"] != "A calm reader" || voice["category"] != "cloned" || voice["labels"].(map[string]any)["accent"] != "american" || len(voice["verified_languages"].([]any)) != 1 {
				t.Fatalf("source voice metadata lost or invented: %v", voice)
			}
			preview, ok := voice["preview"].(string)
			if !ok || !strings.HasPrefix(preview, "/model/v1/voices/"+voice["voice_id"].(string)+"/previews/") {
				t.Fatalf("missing gateway preview: %v", voice)
			}
			previewContext, cancelPreview := context.WithTimeout(t.Context(), time.Second)
			defer cancelPreview()
			previewRequest, _ := http.NewRequestWithContext(previewContext, "GET", server.URL+preview, nil)
			previewRequest.Header.Set("Authorization", "Bearer "+key)
			previewResponse, err := server.Client().Do(previewRequest)
			if err != nil {
				t.Fatal(err)
			}
			previewBody, _ := io.ReadAll(previewResponse.Body)
			_ = previewResponse.Body.Close()
			if previewResponse.StatusCode != 200 || string(previewBody) != "ID3preview" || previewResponse.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("preview status=%d body=%s", previewResponse.StatusCode, previewBody)
			}
			cursor, ok := first["next_cursor"].(string)
			if !ok || cursor == "" || first["has_more"] != true || first["total_count"] != float64(1) {
				t.Fatalf("page metadata=%v", first)
			}
			next := read("&cursor="+url.QueryEscape(cursor), 200)
			if next["has_more"] != false || next["next_cursor"] != nil || len(next["voices"].([]any)) != 0 {
				t.Fatalf("next page=%v", next)
			}
			otherTenant := createManagementTenant(t, router, owner, "Other voices").Tenant.ID
			otherKey := generateManagementTenantSecret(t, router, owner, otherTenant)
			accountConnectionExchange(t, router, owner, "PUT", "/tenants/"+otherTenant+"/connections/"+provider, map[string]string{"connection_id": conn["id"].(string)}, 200)
			foreignRead := func(path, key string, want int) {
				t.Helper()
				request, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+path, nil)
				request.Header.Set("Authorization", "Bearer "+key)
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != want {
					t.Fatalf("voice authority path=%s status=%d want=%d", path, response.StatusCode, want)
				}
			}
			before := calls.Load()
			foreignRead("/model/v1/voices?provider="+provider+"&cursor="+url.QueryEscape(cursor), otherKey, 400)
			foreignRead("/model/v1/voices/"+voice["voice_id"].(string), otherKey, 404)
			foreignRead(preview, otherKey, 404)
			read("&cursor=invalid", 400)
			read("&cursor="+url.QueryEscape(cursor)+"&search=changed", 400)
			current := accountConnectionExchange(t, router, owner, "GET", "/connections/"+conn["id"].(string), nil, 200)
			accountConnectionExchange(t, router, owner, "PUT", "/connections/"+conn["id"].(string), map[string]any{"name": "Voices", "provider": provider, "version": current["version"], "fields": map[string]string{"resource_token": "replacement"}}, 200)
			read("&cursor="+url.QueryEscape(cursor), 400)
			foreignRead("/model/v1/voices/"+voice["voice_id"].(string), key, 404)
			foreignRead(preview, key, 404)
			if calls.Load() != before {
				t.Fatal("invalid or stale cursor reached provider")
			}
			fresh := read("&page_size=1&search=reader&sort=name&sort_direction=asc&voice_type=personal&category=cloned&include_total_count=true", 200)
			if fresh["voices"].([]any)[0].(map[string]any)["voice_id"] == voice["voice_id"] {
				t.Fatal("old voice ID was rebound to replacement credentials")
			}

		})
	}
}

func TestElevenLabsVoicesRejectInvalidNativePagesAndQueries(t *testing.T) {
	var body atomic.Value
	body.Store(`{"voices":[],"has_more":false,"total_count":null,"next_page_token":null}`)
	var mode atomic.Value
	mode.Store("")
	var calls atomic.Int32
	var voiceType atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/user/subscription" {
			_, _ = io.WriteString(w, elevenQuotaFixture)
			return
		}
		calls.Add(1)
		voiceType.Store(r.URL.Query().Get("voice_type"))
		if mode.Load().(string) == "disconnect" {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		if mode.Load().(string) == "truncated" {
			w.Header().Set("Content-Length", "99999999")
		}
		if mode.Load().(string) == "status" {
			w.WriteHeader(401)
		}
		_, _ = io.WriteString(w, body.Load().(string))
	}))
	defer upstream.Close()
	router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: elevenResourceCatalog(t, "elevenlabs", upstream.URL)})
	owner := managementSessionCookie(t, "voice-invalid-owner")
	tenant := managementDefaultTenantTestID(t, router, owner)
	key := generateManagementTenantSecret(t, router, owner, tenant)
	connection := accountConnectionExchange(t, router, owner, "POST", "/connections", map[string]any{"name": "Voices", "provider": "elevenlabs", "fields": map[string]string{"resource_token": "secret"}}, 201)
	accountConnectionExchange(t, router, owner, "PUT", "/tenants/"+tenant+"/connections/elevenlabs", map[string]string{"connection_id": connection["id"].(string)}, 200)
	server := httptest.NewServer(router)
	defer server.Close()
	read := func(query string, status int) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/model/v1/voices?provider=elevenlabs"+query, nil)
		request.Header.Set("Authorization", "Bearer "+key)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != status {
			t.Fatalf("query=%s status=%d want=%d body=%s", query, response.StatusCode, status, data)
		}
	}
	for _, query := range []string{"&unknown=x", "&search=", "&search=one&search=two", "&search=" + strings.Repeat("a", 1001), "&cursor=" + strings.Repeat("a", 16385), "&page_size=0", "&page_size=101", "&page_size=bad", "&include_total_count=1", "&sort=unknown", "&sort_direction=up", "&voice_type=unknown", "&voice_type=workspace", "&search=%ff", "&search=" + url.QueryEscape(strings.Repeat("声", 1001)), "&category=unknown", "&cursor=invalid", "&cursor=invalid&sort=name"} {
		before := calls.Load()
		read(query, 400)
		if calls.Load() != before {
			t.Fatal("invalid query reached provider")
		}
	}
	read("", 200)
	read("&include_total_count=false", 200)
	read("&search="+url.QueryEscape(strings.Repeat("声", 1000)), 200)
	read("&voice_type=account", 200)
	if voiceType.Load() != "workspace" {
		t.Fatalf("provider account filter=%v", voiceType.Load())
	}
	voice := `{"voice_id":"native","name":"Voice","category":"cloned","description":null,"labels":{},"high_quality_base_model_ids":[],"verified_languages":[],"preview_url":null}`
	wrap := func(voice string) string { return `{"voices":[` + voice + `],"has_more":false}` }
	for _, invalid := range []string{"null", "{}", "{", `{"voices":null,"has_more":false}`, `{"voices":[],"has_more":true}`, `{"voices":[],"has_more":false,"next_page_token":"unexpected"}`, `{"voices":[],"has_more":false,"total_count":-1}`, `{"voices":[],"has_more":true,"next_page_token":"` + strings.Repeat("a", 4097) + `"}`, wrap(strings.Replace(voice, `"native"`, `""`, 1)), wrap(strings.Replace(voice, `"Voice"`, `""`, 1)), wrap(voice + "," + voice), wrap(strings.Replace(voice, `"verified_languages":[]`, `"verified_languages":[{"language":"en","model_id":""}]`, 1)), wrap(strings.Replace(voice, `"high_quality_base_model_ids":[]`, `"high_quality_base_model_ids":[""]`, 1)), wrap(strings.Replace(voice, `"preview_url":null`, `"preview_url":"https://untrusted.invalid/audio"`, 1)), strings.Repeat(" ", (1<<20)+1)} {
		body.Store(invalid)
		read("", 502)
	}
	body.Store(wrap(voice))
	read("", 200)
	body.Store(`{"voices":[],"has_more":false}`)
	for _, failure := range []string{"status", "truncated", "disconnect"} {
		mode.Store(failure)
		read("", 502)
	}
}

func elevenVoicePreviewCapacity(t *testing.T) proxy.UpstreamCapacityConfiguration {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "configs", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Server struct {
			Capacity proxy.UpstreamCapacityConfiguration `yaml:"upstream_capacity"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(content, &config); err != nil {
		t.Fatal(err)
	}
	for _, origin := range config.Server.Capacity.Origins {
		if origin.Origin == "https://storage.googleapis.com" {
			return testfixtures.UpstreamCapacity(origin.Active, origin.Queued)
		}
	}
	t.Fatal("voice preview origin has no production admission allocation")
	return proxy.UpstreamCapacityConfiguration{}
}
