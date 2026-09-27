package proxy_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

// This test qualifies the current gateway against the selected live upstream.
// Account setup uses the real management API with the local test session issuer.
func TestDictatorGatewayLiveAcceptance(t *testing.T) {
	if os.Getenv("LLM_PROXY_DICTATOR_LIVE_ENABLED") != "1" {
		t.Skip("run make test-dictator-live to qualify the live speech provider")
	}
	required := func(name string) string {
		t.Helper()
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			t.Fatalf("live acceptance requires %s", name)
		}
		return value
	}
	address := net.JoinHostPort(required("DICTATOR_GRPC_HOST"), required("DICTATOR_GRPC_PORT"))
	token := required("DICTATOR_GRPC_AUTH_TOKEN")
	useTLS, err := strconv.ParseBool(required("DICTATOR_GRPC_USE_TLS"))
	if err != nil {
		t.Fatal("DICTATOR_GRPC_USE_TLS must be a boolean")
	}
	tls := strconv.FormatBool(useTLS)
	audio, err := os.ReadFile(required("LLM_PROXY_DICTATOR_LIVE_WAV_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	if len(audio) < 12 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		t.Fatal("live fixture must be a WAV recording of 'hello world'")
	}
	client := newDictatorGatewayAcceptanceClient(t, address, token, tls)
	exerciseDictatorGatewayAcceptance(t, client, audio, dictatorAcceptanceText{
		transcript: "hello world", transcription: "hello", diarization: "hello", alignment: "hello", subtitles: "hello world",
	})
}

func newDictatorGatewayAcceptanceClient(t *testing.T, address, token, tls string) llmproxyclient.Client {
	t.Helper()
	configuration := proxy.Configuration{AssetStorePath: t.TempDir(), MaxAssetBytes: 64 * 1024 * 1024, MediaOperationLifetimeSeconds: 900}
	router := newManagementRouterWithDatabasePath(t, configuration, filepath.Join(t.TempDir(), "acceptance.sqlite"))
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	owner := managementSessionCookie(t, "dictator-live-owner")
	account := requestManagementAccount(t, router, owner)
	fields := map[string]string{"grpc_address": address, "grpc_auth_token": "llm-proxy-live-invalid-token", "grpc_tls": tls}
	rejectedBody, err := json.Marshal(map[string]any{"name": "Rejected live connection", "provider": proxy.ProviderNameDictator, "fields": fields})
	if err != nil {
		t.Fatal(err)
	}
	rejected := httptest.NewRecorder()
	router.ServeHTTP(rejected, authenticatedJSONRequest(http.MethodPost, "/api/management/connections", string(rejectedBody), owner))
	if rejected.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid upstream credential: status=%d", rejected.Code)
	}

	t.Log("live upstream rejected an invalid credential through the management API")
	fields["grpc_auth_token"] = token
	connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{
		"name": "Live acceptance", "provider": proxy.ProviderNameDictator, "fields": fields,
	}, http.StatusCreated)
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+account.Tenants[0].ID+"/connections/"+proxy.ProviderNameDictator,
		map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
	secret := generateManagementTenantSecret(t, router, owner, account.Tenants[0].ID)
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

type dictatorAcceptanceText struct {
	transcript, transcription, diarization, alignment, subtitles string
}

// The same assertions run against a local protocol fixture and the live provider.
func exerciseDictatorGatewayAcceptance(t *testing.T, client llmproxyclient.Client, audio []byte, expected dictatorAcceptanceText) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	capabilities, err := client.GetMediaCapabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	for _, route := range capabilities.Routes {
		if route.Provider != proxy.ProviderNameDictator {
			continue
		}
		switch route.Model {
		case proxy.ModelNameDictatorWhisperBase, proxy.ModelNameDictatorWhisperLargeV3, proxy.ModelNameDictatorQwen3TTS, proxy.ModelNameDictatorSileroRU:
			routes[route.Capability] = true
		}
	}
	voicesPage, err := client.GetMediaVoices(ctx, llmproxyclient.MediaVoiceQuery{Provider: proxy.ProviderNameDictator})
	voices := voicesPage.Voices
	if err != nil || len(voices) == 0 {
		t.Fatalf("voice discovery: count=%d error=%v", len(voices), err)
	}
	asset, err := client.UploadAsset(ctx, llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: audio})
	if err != nil {
		t.Fatal(err)
	}
	jsonObject := func(value map[string]string) json.RawMessage {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	dictatorModelForCapability := func(capability string) string {
		if capability == "audio.speech.generate" {
			return proxy.ModelNameDictatorQwen3TTS
		}
		return proxy.ModelNameDictatorWhisperBase
	}
	execute := func(capability string, input json.RawMessage, controls string) [][]byte {
		t.Helper()
		if !routes[capability] {
			t.Fatalf("missing advertised capability %s", capability)
		}
		request := llmproxyclient.MediaOperationInput{Provider: proxy.ProviderNameDictator, Model: dictatorModelForCapability(capability), Capability: capability, Input: input, Controls: json.RawMessage(controls)}
		operation, err := client.CreateMediaOperation(ctx, "gateway-acceptance-"+capability, request)
		if err != nil {
			t.Fatalf("%s admission: %v", capability, err)
		}
		duplicate, err := client.CreateMediaOperation(ctx, "gateway-acceptance-"+capability, request)
		if err != nil || duplicate.OperationID != operation.OperationID {
			t.Fatalf("%s duplicate changed operation: %v", capability, err)
		}
		result, err := client.WaitMediaOperation(ctx, operation.OperationID, 100*time.Millisecond)
		if err != nil || result.State != "succeeded" {
			t.Fatalf("%s state=%s error=%v provider_error=%v", capability, result.State, err, result.Error)
		}
		if result.Cost.Available || result.Cost.Reason == "" {
			t.Fatalf("%s lacks explicit unavailable cost evidence", capability)
		}
		content := make([][]byte, len(result.Outputs))
		for index, output := range result.Outputs {
			metadata, err := client.GetAsset(ctx, output.AssetID)
			if err != nil {
				t.Fatal(err)
			}
			content[index], err = client.DownloadAsset(ctx, metadata)
			if err != nil || len(content[index]) == 0 {
				t.Fatalf("%s empty output: %v", capability, err)
			}
		}
		t.Logf("%s succeeded; downloaded %d output assets; duplicate request retained operation identity", capability, len(content))
		return content
	}
	assertText := func(outputs [][]byte, count, ordinal int, text string) {
		t.Helper()
		if len(outputs) != count || !strings.Contains(strings.ToLower(string(outputs[ordinal])), strings.ToLower(text)) {
			t.Fatalf("expected %d outputs with %q at ordinal %d", count, text, ordinal)
		}
	}
	source := jsonObject(map[string]string{"audio_asset_id": asset.AssetID})
	transcription := execute("audio.transcribe", source, `{"language":"en"}`)
	assertText(transcription, 1, 0, expected.transcription)
	diarization := execute("audio.diarize", source, `{"language":"en","model_size":"base","utterance_gap_seconds":0.5}`)
	assertText(diarization, 1, 0, expected.diarization)
	var diarized struct {
		Words    []json.RawMessage `json:"words"`
		Segments []json.RawMessage `json:"speakerSegments"`
	}
	if err := json.Unmarshal(diarization[0], &diarized); err != nil || len(diarized.Words) == 0 || len(diarized.Segments) == 0 {
		t.Fatal("diarization lacks words or speaker segments")
	}
	alignedInput := jsonObject(map[string]string{"audio_asset_id": asset.AssetID, "transcript": expected.transcript})
	aligned := execute("audio.align", alignedInput, `{"language":"en","remove_punctuation":true}`)
	assertText(aligned, 2, 1, expected.alignment)
	assertText(aligned, 2, 1, "-->")
	subtitles := execute("subtitles.create", alignedInput, `{"language":"en","granularity":"sentence","group_size":2}`)
	assertText(subtitles, 1, 0, expected.subtitles)
	assertText(subtitles, 1, 0, "-->")
	extracted := execute("audio.voice.extract", jsonObject(map[string]string{"audio_asset_id": asset.AssetID, "transcript": expected.transcript, "display_name": "Acceptance voice", "language": "en"}), `{"model_size":"base","duration_seconds":0.5}`)
	if len(extracted) != 1 {
		t.Fatal("voice extraction lacks its public result")
	}
	voicesPage, err = client.GetMediaVoices(ctx, llmproxyclient.MediaVoiceQuery{Provider: proxy.ProviderNameDictator})
	voices = voicesPage.Voices
	if err != nil {
		t.Fatal(err)
	}
	var extractedID string
	for _, voice := range voices {
		if voice.Mode == "extracted" && voice.DisplayName == "Acceptance voice" {
			extractedID = voice.VoiceID
		}
	}
	if extractedID == "" {
		t.Fatal("extracted voice missing from tenant discovery")
	}
	generated := execute("audio.speech.generate", jsonObject(map[string]string{"voice_id": extractedID, "text": "Hello world"}), `{"language":"en","text_format":"plain","sample_rate_hz":24000,"include_timeline":true}`)
	if len(generated) != 2 {
		t.Fatal("synthesis lacks audio and timeline")
	}
	var timeline struct {
		Segments []json.RawMessage `json:"textSegments"`
	}
	if err := json.Unmarshal(generated[1], &timeline); err != nil || len(timeline.Segments) == 0 {
		t.Fatal("synthesis lacks a public text timeline")
	}
}
