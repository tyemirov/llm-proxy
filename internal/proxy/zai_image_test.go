package proxy_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
)

func zaiImageBytes(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	var encoded bytes.Buffer
	source := image.NewGray(image.Rect(0, 0, width, height))
	var err error
	if format == "png" {
		err = png.Encode(&encoded, source)
	} else {
		err = jpeg.Encode(&encoded, source, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestZAIImageHTTP(t *testing.T) {
	first := zaiImageBytes(t, "png", 6000, 1)
	second := zaiImageBytes(t, "jpeg", 1, 6000)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if r.URL.Path != "/chat/completions" || payload["model"] != "glm-5.3-flash" || payload["reasoning_effort"] != "high" {
			t.Errorf("request=%v", payload)
		}
		messages := payload["messages"].([]any)
		blocks := messages[0].(map[string]any)["content"].([]any)
		if len(blocks) != 3 {
			t.Fatalf("blocks=%v", blocks)
		}
		for index, data := range [][]byte{first, second} {
			block := blocks[index].(map[string]any)
			want := "data:" + []string{"image/png", "image/jpeg"}[index] + ";base64," + base64.StdEncoding.EncodeToString(data)
			if block["type"] != "image_url" || block["image_url"].(map[string]any)["url"] != want {
				t.Errorf("image %d differs", index)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			io.WriteString(w, `{"choices":[{"message":{"content":"visible ","reasoning_content":"private"},"finish_reason":"length"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
		} else {
			if len(messages) != 3 || messages[1].(map[string]any)["reasoning_content"] != "private" {
				t.Errorf("continuation=%v", messages)
			}
			io.WriteString(w, `{"choices":[{"message":{"content":"answer","reasoning_content":"private final"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`)
		}
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentZAICatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"zai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, scenario := range []struct {
		name        string
		attachments []map[string]any
		status      int
	}{
		{"ordered boundary images", []map[string]any{messageMediaPayload("image", "image/png", first), messageMediaPayload("image", "image/jpeg", second)}, 200},
		{"width exceeded", []map[string]any{messageMediaPayload("image", "image/png", zaiImageBytes(t, "png", 6001, 1))}, 413},
		{"height exceeded", []map[string]any{messageMediaPayload("image", "image/jpeg", zaiImageBytes(t, "jpeg", 1, 6001))}, 413},
		{"invalid image", []map[string]any{messageMediaPayload("image", "image/png", []byte("invalid"))}, 400},
		{"wrong MIME", []map[string]any{messageMediaPayload("image", "image/jpeg", first)}, 400},
		{"unsupported WebP", []map[string]any{messageMediaPayload("image", "image/webp", []byte("webp"))}, 400},
	} {
		body := mediaV2RequestBody(t, "glm-5.3-flash", "inspect", scenario.attachments)
		var payload map[string]any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatal(err)
		}
		payload["reasoning_effort"] = "high"
		response, err := http.Post(server.URL+"/v2?key="+TestSecret+"&provider=zai", "application/json", bytes.NewReader(zaiCurrentJSON(t, payload)))
		if err != nil {
			t.Fatal(err)
		}
		bodyBytes, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != scenario.status || calls != 2 {
			t.Fatalf("case=%s status=%d calls=%d body=%s", scenario.name, response.StatusCode, calls, bodyBytes)
		}
		if scenario.status == 200 && (string(bodyBytes) != "visible answer" || strings.Contains(string(bodyBytes), "private") || response.Header.Get("X-LLM-Proxy-Total-Tokens") != "12") {
			t.Fatalf("output=%s headers=%v", bodyBytes, response.Header)
		}
	}
}

func TestZAIImageCatalogValidation(t *testing.T) {
	for _, scenario := range []string{"duplicate MIME", "unsupported MIME", "no image", "dimensions without formats", "unsupported decoder", "wrong dimension unit", "wrong pixel id"} {
		t.Run(scenario, func(t *testing.T) {
			schema := currentZAICatalog(t).Schema()
			for pi := range schema.Providers {
				if schema.Providers[pi].ID != "zai" {
					continue
				}
				for oi := range schema.Providers[pi].Offerings {
					offering := &schema.Providers[pi].Offerings[oi]
					if offering.Model != "glm-5.3-flash" {
						continue
					}
					switch scenario {
					case "duplicate MIME":
						offering.ImageMIMETypes = []string{"image/png", "image/png"}
					case "unsupported MIME":
						offering.ImageMIMETypes = []string{"image/gif"}
					case "no image":
						offering.MediaInputs = nil
						offering.MediaLimits = nil
					case "dimensions without formats":
						offering.ImageMIMETypes = nil
					case "unsupported decoder":
						offering.ImageMIMETypes = []string{"image/webp"}
					case "wrong dimension unit":
						offering.MediaLimits[3].Unit = "bytes"
					case "wrong pixel id":
						offering.MediaLimits[3].ID = "unhandled_dimension"
					}
				}
			}
			if _, err := proxy.NewProviderCatalog(schema); err == nil {
				t.Fatal("invalid image contract accepted")
			}
		})
	}
}

func TestZAIImagePublicCatalog(t *testing.T) {
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: currentZAICatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	var offering proxy.PublicProviderOffering
	for _, candidate := range public.Offerings {
		if candidate.Model == "glm-5.3-flash" {
			offering = candidate
		}
	}
	if !reflect.DeepEqual(offering.ImageMIMETypes, []string{"image/jpeg", "image/png"}) || len(offering.MediaLimits) != 5 {
		t.Fatalf("offering=%+v", offering)
	}
	for _, limit := range offering.MediaLimits {
		if limit.Source != "https://docs.z.ai/openapi.json" || limit.LastVerified != "2026-09-05" {
			t.Fatalf("source=%+v", limit)
		}
		if limit.Unit == "pixels" {
			if limit.Value == nil || *limit.Value != 6000 || limit.Status != "bounded" || limit.Scope != "attachment" {
				t.Fatalf("dimension=%+v", limit)
			}
		} else if limit.Status != "unknown" || limit.Value != nil {
			t.Fatalf("unverified bound=%+v", limit)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != proxy.PublicCapabilitiesPath {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"offerings": []proxy.PublicProviderOffering{offering}})
	}))
	defer server.Close()
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: TestSecret})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GetPublicCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Offerings[0].ImageMIMETypes, offering.ImageMIMETypes) || len(result.Offerings[0].MediaLimits) != 5 {
		t.Fatalf("client projection=%+v", result)
	}
	offering.ImageMIMETypes = []string{"image/gif"}
	if _, err := client.GetPublicCapabilities(context.Background()); err == nil {
		t.Fatal("client accepted invalid image formats")
	}
	offering.ImageMIMETypes = []string{"image/png"}
	offering.MediaLimits[3].Unit = "bytes"
	if _, err := client.GetPublicCapabilities(context.Background()); err == nil {
		t.Fatal("client accepted invalid dimensions")
	}
}

func TestZAIImageStoredAsset(t *testing.T) {
	imageBytes := zaiImageBytes(t, "png", 6000, 1)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		blocks := payload["messages"].([]any)[0].(map[string]any)["content"].([]any)
		uri := blocks[0].(map[string]any)["image_url"].(map[string]any)["url"]
		if uri != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageBytes) {
			t.Error("asset bytes changed after header inspection")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"accepted"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentZAICatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"zai": upstream.URL}, nil), AssetStorePath: t.TempDir(), MaxAssetBytes: 1000000, AssetRetentionSeconds: 60}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, scenario := range []struct {
		data   []byte
		status int
	}{{imageBytes, 200}, {zaiImageBytes(t, "png", 6001, 1), 413}} {
		asset := uploadTestAsset(t, router, TestSecret, "image/png", scenario.data)
		body := mediaV2RequestBody(t, "glm-5.3-flash", "inspect", []map[string]any{{"type": "image", "mime_type": "image/png", "asset_id": asset.AssetID}})
		response, err := http.Post(server.URL+"/v2?provider=zai&key="+TestSecret, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != scenario.status || calls != 1 {
			t.Fatalf("status=%d calls=%d", response.StatusCode, calls)
		}
	}
}
