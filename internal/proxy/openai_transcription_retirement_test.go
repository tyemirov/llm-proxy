package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

const openAITranscriptionReplacement = "gpt-transcribe"

func TestOpenAITranscriptionRetirementCatalog(t *testing.T) {
	catalog, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{"gpt-4o-mini-transcribe", "gpt-4o-transcribe"} {
		if bytes.Contains(encoded, []byte(retired)) {
			t.Fatalf("retired route appears in public discovery: %s", retired)
		}
	}
	found := false
	for _, price := range catalog.Prices {
		if price.Provider == "openai" && price.Model == openAITranscriptionReplacement {
			found = true
			if !price.Available || len(price.Rates) != 1 || price.Rates[0].Rate != 0.0045 || price.Rates[0].Unit != "USD/minute" {
				t.Fatalf("replacement price=%+v", price)
			}
		}
	}
	if !found {
		t.Fatal("replacement price is absent")
	}
}

func requestOpenAITranscription(t *testing.T, origin, endpoint, model string, audio []byte) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	field := "audio"
	if endpoint == "/v1/audio/transcriptions" {
		field = "file"
		if err := form.WriteField("model", "openai/"+model); err != nil {
			t.Fatal(err)
		}
	} else if model != "" {
		endpoint += "&model=" + model
	}
	part, err := form.CreateFormFile(field, "speech.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(audio); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, origin+endpoint, &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+TestSecret)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

func TestOpenAITranscriptionRetirementHTTP(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/audio/transcriptions" || r.FormValue("model") != openAITranscriptionReplacement || len(r.MultipartForm.Value) != 1 {
			t.Errorf("unexpected transcription route: method=%s path=%s model=%s", r.Method, r.URL.Path, r.FormValue("model"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"A complete transcript.","languages":[{"code":"en"}],"usage":{"type":"duration","seconds":1}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"openai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, endpoint := range []string{"/dictate?key=" + TestSecret, "/v1/audio/transcriptions"} {
		status, body := requestOpenAITranscription(t, server.URL, endpoint, openAITranscriptionReplacement, metaTranscriptionWAV(16000, 1))
		if status != 200 || decodeTextResponse(t, body) != "A complete transcript." {
			t.Fatalf("status=%d body=%s", status, body)
		}
		for _, old := range []string{"gpt-4o-mini-transcribe", "gpt-4o-transcribe"} {
			before := calls.Load()
			status, body := requestOpenAITranscription(t, server.URL, endpoint, old, metaTranscriptionWAV(16000, 1))
			if status != 400 || calls.Load() != before {
				t.Fatalf("retired model=%s status=%d body=%s", old, status, body)
			}
		}
	}
	status, body := requestOpenAITranscription(t, server.URL, "/dictate?key="+TestSecret, "", metaTranscriptionWAV(16000, 1))
	if status != 200 || decodeTextResponse(t, body) != "A complete transcript." {
		t.Fatalf("default status=%d body=%s", status, body)
	}
}

func TestOpenAITranscriptionRetirementLive(t *testing.T) {
	if os.Getenv("LLM_PROXY_LIVE_OPENAI_TRANSCRIPTION") != "true" {
		t.Skip("explicit live acceptance required")
	}
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Fatal("OPENAI_API_KEY is required")
	}
	audio, err := os.ReadFile(os.Getenv("LIVE_TRANSCRIPTION_AUDIO"))
	if err != nil {
		t.Fatal("LIVE_TRANSCRIPTION_AUDIO must name the qualification WAV file")
	}
	catalog := testfixtures.ProviderCatalog(t)
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.ProviderKeys["openai"] = key
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: catalog}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal("build live qualification proxy failed")
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, endpoint := range []string{"/dictate?key=" + TestSecret, "/v1/audio/transcriptions"} {
		status, body := requestOpenAITranscription(t, server.URL, endpoint, openAITranscriptionReplacement, audio)
		if status != 200 {
			t.Fatalf("live transcription status=%d", status)
		}
		var result struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(body, &result); err != nil || !strings.Contains(strings.ToLower(result.Text), "quick brown fox") || !strings.Contains(strings.ToLower(result.Text), "lazy dog") {
			t.Fatal("live transcript did not preserve the known speech fixture")
		}
		t.Logf("complete transcription passed: model=%s endpoint=%s", openAITranscriptionReplacement, strings.Split(endpoint, "?")[0])
	}
}
