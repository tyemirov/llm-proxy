package proxy_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
)

const metaTranscriptionModel = "muse-voice-transcribe-1.0"

func TestMetaTranscriptionDiscovery(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		catalog := testfixtures.ProviderCatalog(t)
		if enabled {
			catalog = metaTranscriptionCatalog(t)
		}
		router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog}, zap.NewNop().Sugar())
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(router)
		defer server.Close()
		config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL + "/v2", Secret: TestSecret})
		if err != nil {
			t.Fatal(err)
		}
		client, err := llmproxyclient.NewClient(config, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		capabilities, err := client.GetPublicCapabilities(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, offering := range capabilities.Offerings {
			if offering.Model == metaTranscriptionModel {
				found = true
				if offering.WireContract != "meta_transcription" || offering.ExecutionLifecycle != "synchronous_completion" {
					t.Fatalf("offering=%+v", offering)
				}
			}
		}
		if found != enabled {
			t.Fatalf("enabled=%t found=%t", enabled, found)
		}
		request, err := http.NewRequest(http.MethodGet, server.URL+"/v1/models", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+TestSecret)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || bytes.Contains(body, []byte(metaTranscriptionModel)) != enabled {
			t.Fatalf("model discovery enabled=%t status=%d error=%v", enabled, response.StatusCode, err)
		}
		if !enabled {
			status, _ := requestMetaTranscription(t, server.URL, "/v1/audio/transcriptions", metaTranscriptionWAV(16000, 1))
			if status != 400 {
				t.Fatalf("disabled model status=%d", status)
			}
		}
	}
}

func TestMetaTranscriptionPreservesPublicUploadLimit(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: metaTranscriptionCatalog(t), MaxInputAudioBytes: 1000, Endpoints: providerEndpointOverrides(map[string]string{"meta": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, endpoint := range []string{"/dictate?key=" + TestSecret + "&provider=meta", "/v1/audio/transcriptions"} {
		status, body := requestMetaTranscription(t, server.URL, endpoint, metaTranscriptionWAV(16000, 1))
		if status != 413 || calls.Load() != 0 {
			t.Fatalf("status=%d calls=%d body=%s", status, calls.Load(), body)
		}
	}
}

func TestMetaTranscriptionAudioValidation(t *testing.T) {
	mutate := func(change func([]byte) []byte) []byte { return change(metaTranscriptionWAV(16000, 1)) }
	set16 := func(offset int, value uint16) []byte {
		return mutate(func(data []byte) []byte { binary.LittleEndian.PutUint16(data[offset:], value); return data })
	}
	set32 := func(offset int, value uint32) []byte {
		return mutate(func(data []byte) []byte { binary.LittleEndian.PutUint32(data[offset:], value); return data })
	}
	riff := func(chunks []byte) []byte {
		data := append([]byte("RIFF\x00\x00\x00\x00WAVE"), chunks...)
		binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
		return data
	}
	wav := metaTranscriptionWAV(16000, 1)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"transcript":"Accepted."}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: metaTranscriptionCatalog(t), MaxInputAudioBytes: 40_000_000, Endpoints: providerEndpointOverrides(map[string]string{"meta": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	metadataWAV := func(size int) []byte {
		chunk := make([]byte, size-len(wav))
		copy(chunk, "JUNK")
		binary.LittleEndian.PutUint32(chunk[4:], uint32(len(chunk)-8))
		return riff(append(append([]byte(nil), wav[12:]...), chunk...))
	}
	for _, scenario := range []struct {
		name  string
		audio []byte
		want  int
	}{
		{"empty", nil, 400}, {"not WAV", []byte("not WAV"), 400},
		{"wrong container", mutate(func(data []byte) []byte { copy(data, "RF64"); return data }), 400},
		{"wrong format", mutate(func(data []byte) []byte { copy(data[8:], "AVI "); return data }), 400},
		{"wrong RIFF size", set32(4, 12), 400},
		{"truncated chunk header", riff([]byte("fmt")), 400},
		{"truncated chunk", set32(16, 0xffffffff), 400},
		{"short fmt", riff([]byte("fmt \x00\x00\x00\x00")), 400},
		{"duplicate fmt", riff(append(append([]byte(nil), wav[12:36]...), wav[12:]...)), 400},
		{"duplicate audio", riff(append(append([]byte(nil), wav[12:]...), wav[36:]...)), 400},
		{"missing fmt", riff(wav[36:]), 400}, {"missing audio", riff(wav[12:36]), 400},
		{"compressed", set16(20, 3), 400}, {"stereo", set16(22, 2), 400},
		{"unsupported sample rate", set32(24, 44100), 400}, {"wrong byte rate", set32(28, 100), 400},
		{"wrong block alignment", set16(32, 4), 400}, {"eight bit", set16(34, 8), 400},
		{"empty audio", metaTranscriptionWAV(16000, 0), 400},
		{"odd audio size", set32(40, uint32(len(wav)-45)), 400},
		{"over ten minutes", metaTranscriptionWAV(24000, 601), 400},
		{"ten minutes", metaTranscriptionWAV(24000, 600), 200},
		{"padded metadata", riff(append(append([]byte(nil), wav[12:]...), []byte("JUNK\x01\x00\x00\x00a\x00")...)), 200},
		{"missing padding", riff(append(append([]byte(nil), wav[12:]...), []byte("JUNK\x01\x00\x00\x00a")...)), 400},
		{"audio before fmt", riff(append(append([]byte(nil), wav[36:]...), wav[12:36]...)), 200},
		{"encoded request bound", metadataWAV(32_000_000), 400},
		{"input read bound", make([]byte, 32_000_001), 400},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := calls.Load()
			status, body := requestMetaTranscription(t, server.URL, "/dictate?key="+TestSecret+"&provider=meta", scenario.audio)
			wantCalls := before
			if scenario.want == 200 {
				wantCalls++
			}
			if status != scenario.want || calls.Load() != wantCalls {
				t.Fatalf("status=%d calls=%d body=%s", status, calls.Load()-before, body)
			}
		})
	}
}

func TestMetaTranscriptionFailures(t *testing.T) {
	for _, endpoint := range []string{"/dictate?key=" + TestSecret + "&provider=meta", "/v1/audio/transcriptions"} {
		for _, scenario := range []struct {
			name, body     string
			upstream, want int
		}{
			{"empty result", `{}`, 200, 502}, {"empty transcript", `{"transcript":" "}`, 200, 502},
			{"wrong field", `{"text":"private"}`, 200, 502}, {"plain text", `private`, 200, 502},
			{"wrong type", `{"transcript":true}`, 200, 502}, {"malformed", `{"transcript":`, 200, 502},
			{"rejected audio", `{"message":"private"}`, 400, 502}, {"provider size", `private`, 413, 413},
			{"rate limit", `private`, 429, 429}, {"provider error", `private`, 500, 502},
		} {
			t.Run(endpoint+scenario.name, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(scenario.upstream)
					_, _ = io.WriteString(w, scenario.body)
				}))
				defer upstream.Close()
				router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: metaTranscriptionCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"meta": upstream.URL}, nil)}, zap.NewNop().Sugar())
				if err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(router)
				defer server.Close()
				status, body := requestMetaTranscription(t, server.URL, endpoint, metaTranscriptionWAV(16000, 1))
				if status != scenario.want || strings.Contains(string(body), "private") {
					t.Fatalf("status=%d body=%s", status, body)
				}
			})
		}
	}
}

func TestMetaTranscriptionCancellation(t *testing.T) {
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: metaTranscriptionCatalog(t), RequestTimeoutSeconds: 1, Endpoints: providerEndpointOverrides(map[string]string{"meta": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	status, body := requestMetaTranscription(t, server.URL, "/dictate?key="+TestSecret+"&provider=meta", metaTranscriptionWAV(16000, 1))
	if status != 504 {
		t.Fatalf("status=%d body=%s", status, body)
	}
	<-cancelled
}

func metaTranscriptionCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for index := range schema.Models {
		if schema.Models[index].ID == metaTranscriptionModel {
			schema.Models[index].Enabled = proxy.ModelEnabled
		}
	}
	for index := range schema.Providers {
		for offeringIndex := range schema.Providers[index].Offerings {
			if schema.Providers[index].Offerings[offeringIndex].Model == metaTranscriptionModel {
				schema.Providers[index].Offerings[offeringIndex].DefaultOperations = []string{proxy.ModelOperationDictation}
			}
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func metaTranscriptionWAV(rate, seconds int) []byte {
	data := make([]byte, 44+rate*2*seconds)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], uint32(rate))
	binary.LittleEndian.PutUint32(data[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	return data
}

func requestMetaTranscription(t *testing.T, origin, endpoint string, audio []byte) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	field := "audio"
	if endpoint == "/v1/audio/transcriptions" {
		field = "file"
		if err := form.WriteField("model", "meta/"+metaTranscriptionModel); err != nil {
			t.Fatal(err)
		}
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

func TestMetaTranscriptionHTTP(t *testing.T) {
	var calls atomic.Int32
	var expectedAudio atomic.Value
	expectedAudio.Store(metaTranscriptionWAV(16000, 1))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/asr/transcribe" || r.Header.Get("Authorization") != "Bearer "+testMetaKey || r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected Meta request: %s %s", r.Method, r.URL.Path)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		settings, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		var payload map[string]string
		if err := json.NewDecoder(settings).Decode(&payload); err != nil {
			t.Error(err)
		}
		if settings.FormName() != "request" || settings.Header.Get("Content-Type") != "application/json" || len(payload) != 3 || payload["model"] != metaTranscriptionModel || payload["audioEncoding"] != "WAV" || payload["mode"] != "PUSH_TO_TALK" {
			t.Errorf("invalid settings: %v", payload)
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		audio, err := io.ReadAll(part)
		if err != nil || part.FormName() != "audio" || part.Header.Get("Content-Type") != "audio/wav" || !bytes.Equal(audio, expectedAudio.Load().([]byte)) {
			t.Error("audio was not preserved")
		}
		if _, err := reader.NextPart(); err != io.EOF {
			t.Error("unexpected multipart field")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"sessionId":"private-session","transcript":"A clear transcript.","audioDurationMs":1000,"turns":[]}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: metaTranscriptionCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"meta": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, rate := range []int{16000, 24000} {
		wantAudio := metaTranscriptionWAV(rate, 1)
		expectedAudio.Store(wantAudio)
		for _, endpoint := range []string{"/dictate?key=" + TestSecret + "&provider=meta&model=" + metaTranscriptionModel, "/dictate?key=" + TestSecret + "&provider=meta", "/v1/audio/transcriptions"} {
			status, data := requestMetaTranscription(t, server.URL, endpoint, wantAudio)
			if status != 200 || decodeTextResponse(t, data) != "A clear transcript." || strings.Contains(string(data), "private") {
				t.Fatalf("status=%d body=%s", status, data)
			}
		}
	}
	if calls.Load() != 6 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestMetaTranscriptionManagement(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"transcript":"Managed transcript."}`)
	}))
	defer upstream.Close()
	router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: metaTranscriptionCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"meta": upstream.URL}, nil)})
	cookie := managementSessionCookie(t, "meta-transcription")
	tenantID := managementDefaultTenantTestID(t, router, cookie)
	tenantPath := managementDefaultTenantTestPath(t, router, cookie, "")
	connection := putManagementProviderKey(t, router, cookie, tenantID, "meta", "test-meta-transcription-key", "muse-spark-1.1", metaTranscriptionModel, context.Background())
	if connection.Code != 200 {
		t.Fatalf("connection: %d %s", connection.Code, connection.Body)
	}
	before := requestProviderKeyVerificationProfile(t, router, cookie, tenantID)
	defaults := managementDefaultsRequestBody(t, before.Tenant.Defaults.Provider, before.Tenant.Defaults.Model, "meta", metaTranscriptionModel, "")
	request := authenticatedJSONRequest(http.MethodPut, tenantPath+"/defaults", defaults, cookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("defaults: %d %s", response.Code, response.Body)
	}
	profile := requestProviderKeyVerificationProfile(t, router, cookie, tenantID)
	if profile.Tenant.Defaults.Provider != before.Tenant.Defaults.Provider || profile.Tenant.Defaults.Model != before.Tenant.Defaults.Model || profile.Tenant.Defaults.DictationProvider != "meta" || profile.Tenant.Defaults.DictationModel != metaTranscriptionModel {
		t.Fatalf("saved defaults=%+v", profile.Tenant.Defaults)
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, cookie)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, secretRequest)
	var secret struct {
		Secret string `json:"secret"`
	}
	if response.Code != 200 {
		t.Fatalf("secret status=%d", response.Code)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &secret); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	status, body := requestMetaTranscription(t, server.URL, "/dictate?key="+secret.Secret, metaTranscriptionWAV(16000, 1))
	if status != 200 || decodeTextResponse(t, body) != "Managed transcript." {
		t.Fatalf("managed dictation status=%d body=%s", status, body)
	}
	usage := waitForManagementValue(t, func() managementTenantUsageTestResponse {
		return requestManagementTenantUsage(t, router, cookie, tenantID)
	}, func(value managementTenantUsageTestResponse) bool { return value.Totals.Requests == 1 })
	if usage.Totals.Requests != 1 {
		t.Fatalf("usage requests=%d", usage.Totals.Requests)
	}
	status, _ = requestMetaTranscription(t, server.URL, "/dictate?key="+secret.Secret, []byte("invalid WAV"))
	if status != 400 {
		t.Fatalf("unsupported audio status=%d", status)
	}
	rejectedUsage := waitForManagementValue(t, func() managementUsageTestResponse { return requestManagementUsage(t, router, cookie, "30d") }, func(value managementUsageTestResponse) bool {
		return value.Totals.Requests == 2 || value.RejectedRequests == 1
	})
	if rejectedUsage.Totals.Requests != 1 || rejectedUsage.RejectedRequests != 1 {
		t.Fatalf("unsupported audio accounting: accepted=%d rejected=%d", rejectedUsage.Totals.Requests, rejectedUsage.RejectedRequests)
	}
	rejections := requestManagementUsageRejections(t, router, cookie, tenantPath, "30d", 10, "")
	if len(rejections.Rejections) != 1 || rejections.Rejections[0].OutcomeCode != "invalid_request" {
		t.Fatalf("audio rejections=%+v", rejections)
	}
}
