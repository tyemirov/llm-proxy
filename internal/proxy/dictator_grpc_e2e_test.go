package proxy_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

const dictatorAcceptanceTimeout = 10 * time.Second

// Inject a lost assignment at the database read used by admission validation.
type dictatorAdmissionFailureDialector struct {
	gorm.Dialector
	fail *atomic.Bool
}

func (dialector dictatorAdmissionFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	return database.Callback().Query().After("gorm:query").Register("dictator_admission_assignment_failure", func(query *gorm.DB) {
		if _, bindingRead := query.Statement.Preloads["Connection.Fields"]; bindingRead && strings.Contains(query.Statement.SQL.String(), "tenant_id = ? AND provider_id = ?") && dialector.fail.Load() {
			query.AddError(gorm.ErrRecordNotFound)
		}
	})
}

type dictatorGRPCFixture struct {
	dictator.UnimplementedVoiceServiceServer
	dictator.UnimplementedTranscriptionServiceServer
	dictator.UnimplementedArtifactServiceServer
	dictator.UnimplementedAlignmentServiceServer
	dictator.UnimplementedSubtitleServiceServer
	token                string
	rotatedToken         atomic.Value
	pending              atomic.Bool
	observed             chan string
	cancelled            atomic.Int32
	submissions          atomic.Int32
	discoveryFailure     atomic.Int32
	invalidDiarization   atomic.Bool
	diarizationRequest   atomic.Pointer[dictator.DiarizeAudioRequest]
	transcriptionRequest atomic.Pointer[dictator.TranscribeRequest]
	subtitleRequest      atomic.Pointer[dictator.RenderSubtitlesRequest]
	extractionRequest    atomic.Pointer[dictator.ExtractReferenceSampleRequest]
	timelineOverride     atomic.Value
	stopped              sync.Map
	announced            sync.Map
}

func (fixture *dictatorGRPCFixture) UploadArtifact(stream grpc.ClientStreamingServer[dictator.UploadArtifactChunk, dictator.UploadArtifactResponse]) error {
	values, _ := metadata.FromIncomingContext(stream.Context())
	if strings.Join(values.Get("authorization"), "") != "Bearer "+fixture.token {
		return status.Error(codes.Unauthenticated, "invalid artifact credential")
	}
	var content []byte
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		content = append(content, chunk.GetContent()...)
	}
	return stream.SendAndClose(&dictator.UploadArtifactResponse{Artifact: &dictator.ArtifactRef{ArtifactId: "private-input", MediaType: "audio/wav", SizeBytes: int64(len(content)), Sha256: fmt.Sprintf("%x", sha256.Sum256(content))}})
}

func (fixture *dictatorGRPCFixture) SubmitTranscribeJob(ctx context.Context, request *dictator.TranscribeRequest) (*dictator.SubmitTranscribeJobResponse, error) {
	fixture.transcriptionRequest.Store(request)
	values, _ := metadata.FromIncomingContext(ctx)
	if strings.Join(values.Get("authorization"), "") != "Bearer "+fixture.token {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	if request.AudioArtifactId != "private-input" || request.LanguageCode != "en" || request.AutodetectLanguage {
		return nil, status.Error(codes.InvalidArgument, "wrong transcription request")
	}
	return &dictator.SubmitTranscribeJobResponse{JobId: "private-transcription", State: dictator.TranscriptionJobState_TRANSCRIPTION_JOB_STATE_QUEUED}, nil
}

func (fixture *dictatorGRPCFixture) GetTranscribeJob(_ context.Context, request *dictator.GetTranscribeJobRequest) (*dictator.GetTranscribeJobResponse, error) {
	return &dictator.GetTranscribeJobResponse{JobId: request.JobId, State: dictator.TranscriptionJobState(fixture.jobState(request.JobId)), Text: "A clear transcript.", LanguageCode: "en", SourceArtifactId: "private-input"}, nil
}

func (fixture *dictatorGRPCFixture) ListSynthesisVoices(ctx context.Context, _ *dictator.ListSynthesisVoicesRequest) (*dictator.ListSynthesisVoicesResponse, error) {
	if code := codes.Code(fixture.discoveryFailure.Load()); code != codes.OK {
		return nil, status.Error(code, "private upstream diagnostic")
	}
	values, _ := metadata.FromIncomingContext(ctx)
	token := fixture.token
	if rotated := fixture.rotatedToken.Load(); rotated != nil {
		token = rotated.(string)
	}
	if strings.Join(values.Get("authorization"), "") != "Bearer "+token {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	return &dictator.ListSynthesisVoicesResponse{Voices: []*dictator.SynthesisVoice{{
		SynthesisEngine: dictator.SynthesisEngine_SYNTHESIS_ENGINE_SILERO_RU,
		LanguageCode:    "ru", VoiceId: "baya", DisplayName: "Baya", IsDefault: true,
		NativeSampleRateHz: []int32{24000, 48000}, DefaultSampleRateHz: 48000,
	}}}, nil
}

func TestDictatorAccountConnectionUsesAuthenticatedGRPC(t *testing.T) {
	exerciseDictatorAccountConnection(t, proxy.ProviderNameDictator, proxy.ModelNameDictatorWhisperBase, testfixtures.ProviderCatalog(t))
}

func TestDictatorSecondProviderUsesCatalogRegistration(t *testing.T) {
	const providerID = "speech-fixture"
	const modelID = "speech-fixture-v1"
	schema := testfixtures.ProviderCatalog(t).Schema()
	for _, model := range schema.Models {
		if model.ID == proxy.ModelNameDictatorWhisperBase {
			model.ID = modelID
			schema.Models = append(schema.Models, model)
			break
		}
	}
	for _, provider := range schema.Providers {
		if provider.ID == proxy.ProviderNameDictator {
			provider.ID = providerID
			provider.Label = "Second speech provider"
			provider.APIServiceLabel = "Second speech service"
			provider.Fields = append([]proxy.ProviderCatalogField(nil), provider.Fields...)
			for index := range provider.Fields {
				switch provider.Fields[index].ID {
				case "grpc_address":
					provider.Fields[index].ID = "speech_endpoint"
				case "grpc_auth_token":
					provider.Fields[index].ID = "speech_bearer"
				}
			}
			provider.Transports = append([]proxy.ProviderCatalogTransport(nil), provider.Transports...)
			provider.Transports[0].Endpoint.SettingField = "speech_endpoint"
			provider.Transports[0].Components.Authentication.Field = "speech_bearer"
			provider.Offerings = append([]proxy.ProviderCatalogOffering(nil), provider.Offerings...)
			for index := range provider.Offerings {
				if provider.Offerings[index].Model == proxy.ModelNameDictatorWhisperBase {
					provider.Offerings[index].Model = modelID
					provider.Offerings[index].UpstreamModel = "private-speech-fixture"
				}
			}
			schema.Providers = append(schema.Providers, provider)
			break
		}
	}
	document, err := yaml.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	invalidDocument := strings.Replace(string(document), "id: dictator_speech_v1", "id: unknown_speech_codec", 1)
	if _, err := proxy.ParseProviderCatalog([]byte(invalidDocument)); err == nil {
		t.Fatal("invalid speech codec accepted")
	}
	catalog, err := proxy.ParseProviderCatalog(document)
	if err != nil {
		t.Fatal(err)
	}
	exerciseDictatorAccountConnection(t, providerID, modelID, catalog)
}

func waitForDictatorAcceptanceOperation(t *testing.T, client llmproxyclient.Client, operationID string) (llmproxyclient.MediaOperation, error) {
	// Earlier scenarios must not consume this operation's completion budget.
	ctx, cancel := context.WithTimeout(t.Context(), dictatorAcceptanceTimeout)
	defer cancel()
	return client.WaitMediaOperation(ctx, operationID, 10*time.Millisecond)
}

func exerciseDictatorAccountConnection(t *testing.T, provider, model string, catalog *proxy.ProviderCatalog) {
	t.Helper()
	var addressField, tokenField string
	for _, definition := range catalog.Schema().Providers {
		if definition.ID == provider {
			addressField = definition.Transports[0].Endpoint.SettingField
			tokenField = definition.Transports[0].Components.Authentication.Field
		}
	}
	fixture, listener := newDictatorAcceptanceUpstream(t, provider)
	var err error
	databasePath := filepath.Join(t.TempDir(), "management.sqlite")
	configuration := proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), MediaOperationClaimSeconds: 7200, MediaOperationClaimRenewalSeconds: 3600}
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	server.Client().Timeout = dictatorAcceptanceTimeout
	owner := managementSessionCookie(t, "dictator-account-owner")
	account := requestManagementAccount(t, router, owner)
	request := authenticatedJSONRequest(http.MethodPost, server.URL+"/api/management/connections",
		`{"name":"My speech server","provider":"`+provider+`","fields":{"`+addressField+`":"`+listener.Addr().String()+`","`+tokenField+`":"`+fixture.token+`","grpc_tls":"false"}}`, owner)
	request.RequestURI = ""
	request.Header.Set("Idempotency-Key", "dictator-account-create")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create Dictator connection: status=%d body=%s", response.StatusCode, body)
	}
	if strings.Contains(string(body), fixture.token) {
		t.Fatal("connection response exposed credential")
	}
	var connection struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &connection); err != nil {
		t.Fatal(err)
	}
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+account.Tenants[0].ID+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": connection.ID}, http.StatusOK)
	secret := generateManagementTenantSecret(t, router, owner, account.Tenants[0].ID)
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	voicesPage, err := client.GetMediaVoices(ctx, llmproxyclient.MediaVoiceQuery{Provider: provider})
	voices := voicesPage.Voices
	if err != nil || len(voices) != 1 {
		t.Fatalf("Dictator voices=%v error=%v", voices, err)
	}
	asset, err := client.UploadAsset(ctx, llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: []byte("fixture audio")})
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := client.GetMediaCapabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	advertised := 0
	advertisedSilero := 0
	advertisedQwen := 0
	for _, route := range capabilities.Routes {
		if route.Provider != provider {
			continue
		}
		switch route.Model {
		case model:
			advertised++
		case proxy.ModelNameDictatorSileroRU:
			advertisedSilero++
		case proxy.ModelNameDictatorQwen3TTS:
			advertisedQwen++
		}
	}
	if advertised != 5 {
		t.Fatalf("advertised transcription capabilities=%d", advertised)
	}
	if advertisedSilero != 1 || advertisedQwen != 1 {
		t.Fatalf("advertised speech capabilities silero=%d qwen=%d", advertisedSilero, advertisedQwen)
	}
	_, rejectedError := client.CreateMediaOperation(ctx, "unsupported-controls", llmproxyclient.MediaOperationInput{Provider: provider, Model: model, Capability: "audio.transcribe", Input: json.RawMessage(`{"audio_asset_id":"` + asset.AssetID + `"}`), Controls: json.RawMessage(`{"unknown_control":true}`)})
	if httpFailureStatus(rejectedError) != http.StatusBadRequest || fixture.submissions.Load() != 0 {
		t.Fatalf("unsupported request error=%v submissions=%d", rejectedError, fixture.submissions.Load())
	}
	_, durationError := client.CreateMediaOperation(ctx, "unsupported-duration", llmproxyclient.MediaOperationInput{Provider: provider, Model: model, Capability: "audio.transcribe", Input: json.RawMessage(`{"audio_asset_id":"` + asset.AssetID + `"}`), Controls: json.RawMessage(`{"language":"en","duration_seconds":0.5}`)})
	if httpFailureStatus(durationError) != http.StatusBadRequest || fixture.submissions.Load() != 0 {
		t.Fatalf("unsupported duration error=%v", durationError)
	}
	operation, err := client.CreateMediaOperation(ctx, "dictator-transcribe", llmproxyclient.MediaOperationInput{Provider: provider, Model: model, Capability: "audio.transcribe", Input: json.RawMessage(`{"audio_asset_id":"` + asset.AssetID + `"}`), Controls: json.RawMessage(`{"language":"en"}`)})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := client.CreateMediaOperation(ctx, "dictator-transcribe", llmproxyclient.MediaOperationInput{Provider: provider, Model: model, Capability: "audio.transcribe", Input: json.RawMessage(`{"audio_asset_id":"` + asset.AssetID + `"}`), Controls: json.RawMessage(`{"language":"en"}`)})
	if err != nil || duplicate.OperationID != operation.OperationID {
		t.Fatalf("duplicate operation=%s error=%v", duplicate.OperationID, err)
	}
	completed, err := waitForDictatorAcceptanceOperation(t, client, operation.OperationID)
	if err != nil || completed.State != "succeeded" || len(completed.Outputs) != 1 {
		t.Fatalf("transcription=%+v error=%v", completed, err)
	}
	output, err := client.GetAsset(ctx, completed.Outputs[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := client.DownloadAsset(ctx, output)
	if err != nil || !strings.Contains(string(content), "A clear transcript.") || strings.Contains(string(content), "private-input") {
		t.Fatalf("transcript=%s error=%v", content, err)
	}
	scenarios := []struct {
		capability, input, controls, scenarioModel string
		outputs                                    int
	}{
		{"audio.diarize", `{"audio_asset_id":"` + asset.AssetID + `"}`, `{"language":"en","model_size":"base","utterance_gap_seconds":0.5}`, model, 1},
		{"audio.align", `{"audio_asset_id":"` + asset.AssetID + `","transcript":"A clear transcript."}`, `{"language":"en","remove_punctuation":true}`, model, 2},
		{"subtitles.create", `{"audio_asset_id":"` + asset.AssetID + `","transcript":"A clear transcript."}`, `{"language":"en","granularity":"sentence","group_size":2}`, model, 1},
		{"audio.speech.generate", `{"text":"<speak>Привет</speak>","voice_id":"` + voices[0].VoiceID + `"}`, `{"language":"ru","text_format":"ssml","sample_rate_hz":48000,"include_timeline":true,"max_duration_seconds":5}`, proxy.ModelNameDictatorSileroRU, 2},
		{"audio.voice.extract", `{"audio_asset_id":"` + asset.AssetID + `","transcript":"A clear transcript.","display_name":"My voice","language":"en"}`, `{"model_size":"base","duration_seconds":0.5}`, model, 1},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.capability, func(t *testing.T) {
			operation, err := client.CreateMediaOperation(ctx, "dictator-"+scenario.capability, llmproxyclient.MediaOperationInput{Provider: provider, Model: scenario.scenarioModel, Capability: scenario.capability, Input: json.RawMessage(scenario.input), Controls: json.RawMessage(scenario.controls)})
			if err != nil {
				t.Fatal(err)
			}
			completed, err := waitForDictatorAcceptanceOperation(t, client, operation.OperationID)
			if err != nil || completed.State != "succeeded" || len(completed.Outputs) != scenario.outputs {
				t.Fatalf("result=%+v error=%v", completed, err)
			}
			for _, reference := range completed.Outputs {
				output, err := client.GetAsset(ctx, reference.AssetID)
				if err != nil {
					t.Fatal(err)
				}
				content, err := client.DownloadAsset(ctx, output)
				if err != nil {
					t.Fatal(err)
				}
				if scenario.capability == "audio.align" && reference.Ordinal == 1 && (output.MIMEType != "application/x-subrip" || string(content) != "1\n00:00:00,000 --> 00:00:00,200\nA\n") {
					t.Fatalf("alignment SRT changed: mime=%s content=%q", output.MIMEType, content)
				}
				if scenario.capability == "audio.speech.generate" && reference.Ordinal == 1 && string(content) != `{"textSegments":[{"content":"Hello","start":0,"end":1}]}` {
					t.Fatalf("unexpected public timeline: %s", content)
				}
				if strings.Contains(string(content), "private-") || strings.Contains(string(content), "binding") {
					t.Fatalf("native identity in output: %s", content)
				}
			}
		})
	}
	if request := fixture.extractionRequest.Load(); request == nil || request.DurationSeconds != 0.5 {
		t.Fatalf("extraction duration was not preserved: %v", request)
	}
	for index, controls := range []string{`{"model_size":"base","duration_seconds":2.75}`, `{"model_size":"base"}`} {
		accepted, err := client.CreateMediaOperation(ctx, fmt.Sprintf("extraction-duration-%d", index), llmproxyclient.MediaOperationInput{Provider: provider, Model: model, Capability: "audio.voice.extract", Input: json.RawMessage(scenarios[4].input), Controls: json.RawMessage(controls)})
		if err != nil {
			t.Fatal(err)
		}
		completed, err := waitForDictatorAcceptanceOperation(t, client, accepted.OperationID)
		if err != nil || completed.State != "succeeded" {
			t.Fatalf("extraction state=%s error=%v", completed.State, err)
		}
		expected := []float64{2.75, 20}[index]
		if got := fixture.extractionRequest.Load().DurationSeconds; got != expected {
			t.Fatalf("extraction duration=%g want=%g", got, expected)
		}
	}
	for index, controls := range []string{`{"model_size":"base","duration_seconds":0}`, `{"model_size":"base","duration_seconds":-1}`, `{"model_size":"base","duration_seconds":null}`} {
		submitted := fixture.submissions.Load()
		_, err := client.CreateMediaOperation(ctx, fmt.Sprintf("invalid-extraction-duration-%d", index), llmproxyclient.MediaOperationInput{Provider: provider, Model: model, Capability: "audio.voice.extract", Input: json.RawMessage(scenarios[4].input), Controls: json.RawMessage(controls)})
		if httpFailureStatus(err) != http.StatusBadRequest || fixture.submissions.Load() != submitted {
			t.Fatalf("invalid duration error=%v", err)
		}
	}
	for index, controls := range []string{`{"language":"en","model_size":"base","utterance_gap_seconds":0}`, `{"language":"en","model_size":"base"}`, `{"language":"en","model_size":"base","utterance_gap_seconds":0.5}`} {
		accepted, err := client.CreateMediaOperation(ctx, fmt.Sprintf("diarization-presence-%d", index), llmproxyclient.MediaOperationInput{Provider: provider, Model: model, Capability: "audio.diarize", Input: json.RawMessage(scenarios[0].input), Controls: json.RawMessage(controls)})
		if err != nil {
			t.Fatal(err)
		}
		accepted, err = waitForDictatorAcceptanceOperation(t, client, accepted.OperationID)
		if err != nil || accepted.State != "succeeded" {
			t.Fatalf("diarization presence operation=%+v error=%v", accepted, err)
		}
		native := fixture.diarizationRequest.Load()
		var expected struct {
			Gap *float64 `json:"utterance_gap_seconds"`
		}
		if err := json.Unmarshal([]byte(controls), &expected); err != nil {
			t.Fatal(err)
		}
		if (native.UtteranceGapSeconds == nil) != (expected.Gap == nil) || (expected.Gap != nil && *native.UtteranceGapSeconds != *expected.Gap) {
			t.Errorf("diarization gap presence lost: controls=%s native=%v", controls, native.UtteranceGapSeconds)
		}
	}
	discoveredPage, err := client.GetMediaVoices(ctx, llmproxyclient.MediaVoiceQuery{Provider: provider})
	discovered := discoveredPage.Voices
	if err != nil {
		t.Fatal(err)
	}
	extractedID := ""
	for _, voice := range discovered {
		if voice.Mode == "extracted" {
			extractedID = voice.VoiceID
		}
	}
	if extractedID == "" {
		t.Fatal("extracted voice absent")
	}
	_, err = client.CreateMediaOperation(ctx, "extracted-voice-wrong-engine", llmproxyclient.MediaOperationInput{Provider: provider, Model: proxy.ModelNameDictatorSileroRU, Capability: "audio.speech.generate", Input: json.RawMessage(`{"text":"Hello","voice_id":"` + extractedID + `"}`), Controls: json.RawMessage(`{"language":"ru","text_format":"plain","sample_rate_hz":48000}`)})
	if httpFailureStatus(err) != http.StatusBadRequest {
		t.Fatalf("Silero accepted an extracted Qwen3 voice: %v", err)
	}
	generated, err := client.CreateMediaOperation(ctx, "extracted-synthesis", llmproxyclient.MediaOperationInput{Provider: provider, Model: proxy.ModelNameDictatorQwen3TTS, Capability: "audio.speech.generate", Input: json.RawMessage(`{"text":"Hello","voice_id":"` + extractedID + `"}`), Controls: json.RawMessage(`{"language":"en","text_format":"plain","sample_rate_hz":24000}`)})
	if err != nil {
		t.Fatal(err)
	}
	generated, err = waitForDictatorAcceptanceOperation(t, client, generated.OperationID)
	if err != nil || generated.State != "succeeded" {
		t.Fatalf("extracted synthesis=%+v error=%v", generated, err)
	}

	// Restore the durable dispatch state left by a stopped process. The real
	// startup path must recover native handles without another paid submission.
	database, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDatabase.Close()
	var dispatched []struct{ OperationID string }
	if err := database.Table("media_operation_records").Select("operation_id").Find(&dispatched).Error; err != nil {
		t.Fatal(err)
	}
	submitted := fixture.submissions.Load()
	if err := database.Exec("UPDATE media_operation_records SET public_state = 'running', provider_execution_state = 'dispatched', terminal_at = NULL").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("UPDATE media_operation_claim_records SET expires_at = ?", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("DELETE FROM media_operation_asset_reference_records WHERE role = 'output'").Error; err != nil {
		t.Fatal(err)
	}
	recoveryConfiguration := managementConfigurationWithDatabasePath(configuration, databasePath)
	recoveryConfiguration.UpstreamCapacity = testfixtures.UpstreamCapacity(1, 32)
	var admissionFailure atomic.Bool
	recoveryConfiguration.Management.DatabaseDialector = dictatorAdmissionFailureDialector{Dialector: recoveryConfiguration.Management.DatabaseDialector, fail: &admissionFailure}
	recoveredRouter, err := buildRouterWithCatalogs(t, recoveryConfiguration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	restarted := httptest.NewServer(recoveredRouter)
	defer restarted.Close()
	restarted.Client().Timeout = dictatorAcceptanceTimeout
	restartedConfig, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: restarted.URL, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	recoveredClient, err := llmproxyclient.NewClient(restartedConfig, restarted.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range dispatched {
		recovered, err := waitForDictatorAcceptanceOperation(t, recoveredClient, record.OperationID)
		if err != nil || recovered.State != "succeeded" {
			t.Fatalf("recovered=%+v error=%v", recovered, err)
		}
	}
	if fixture.submissions.Load() != submitted {
		t.Fatalf("restart submitted %d jobs", fixture.submissions.Load()-submitted)
	}
	usage := waitForManagementValue(t, func() managementTenantUsageTestResponse {
		return requestManagementTenantUsage(t, recoveredRouter, owner, account.Tenants[0].ID)
	}, func(value managementTenantUsageTestResponse) bool { return value.Totals.Requests == len(dispatched) })
	if usage.Totals.Requests != len(dispatched) {
		t.Fatalf("restart usage=%d operations=%d", usage.Totals.Requests, len(dispatched))
	}
	client = recoveredClient

	scenarios = append(scenarios, struct {
		capability, input, controls, scenarioModel string
		outputs                                    int
	}{"audio.transcribe", `{"audio_asset_id":"` + asset.AssetID + `"}`, `{"language":"en"}`, model, 1})
	fixture.pending.Store(true)
	for _, scenario := range scenarios {
		t.Run("cancel-"+scenario.capability, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), dictatorAcceptanceTimeout)
			defer cancel()
			accepted, err := client.CreateMediaOperation(ctx, "cancel-"+scenario.capability, llmproxyclient.MediaOperationInput{Provider: provider, Model: scenario.scenarioModel, Capability: scenario.capability, Input: json.RawMessage(scenario.input), Controls: json.RawMessage(scenario.controls)})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-fixture.observed:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			stopped, err := client.CancelMediaOperation(ctx, accepted.OperationID)
			if err != nil || stopped.State != "cancelled" || stopped.CancellationState != "confirmed" {
				t.Fatalf("cancel=%+v error=%v", stopped, err)
			}
		})
	}
	fixture.pending.Store(false)
	if fixture.cancelled.Load() != 6 {
		t.Fatalf("native cancellations=%d", fixture.cancelled.Load())
	}
	otherListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	otherUpstream := grpc.NewServer()
	otherFixture := &dictatorGRPCFixture{token: "second-account-token"}
	dictator.RegisterVoiceServiceServer(otherUpstream, otherFixture)
	dictator.RegisterArtifactServiceServer(otherUpstream, otherFixture)
	go func() { _ = otherUpstream.Serve(otherListener) }()
	defer otherUpstream.Stop()
	otherOwner := managementSessionCookie(t, "other-dictator-owner")
	otherAccount := requestManagementAccount(t, router, otherOwner)
	created := accountConnectionExchange(t, router, otherOwner, http.MethodPost, "/connections", map[string]any{"name": "Other speech server", "provider": provider, "fields": map[string]string{addressField: otherListener.Addr().String(), tokenField: "second-account-token", "grpc_tls": "false"}}, http.StatusCreated)
	otherConnectionID := created["id"].(string)
	accountConnectionExchange(t, router, otherOwner, http.MethodPut, "/tenants/"+otherAccount.Tenants[0].ID+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": otherConnectionID}, http.StatusOK)
	otherSecret := generateManagementTenantSecret(t, router, otherOwner, otherAccount.Tenants[0].ID)
	otherConfig, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: otherSecret})
	if err != nil {
		t.Fatal(err)
	}
	otherClient, err := llmproxyclient.NewClient(otherConfig, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	otherVoicesPage, err := otherClient.GetMediaVoices(ctx, llmproxyclient.MediaVoiceQuery{Provider: provider})
	otherVoices := otherVoicesPage.Voices
	if err != nil || len(otherVoices) != 1 || otherVoices[0].VoiceID == voices[0].VoiceID {
		t.Fatalf("other voices=%+v error=%v", otherVoices, err)
	}
	if _, err := otherClient.GetMediaVoice(ctx, extractedID); httpFailureStatus(err) != http.StatusNotFound {
		t.Fatalf("cross-account voice error=%v", err)
	}
	if _, err := otherClient.GetMediaOperation(ctx, operation.OperationID); httpFailureStatus(err) != http.StatusNotFound {
		t.Fatalf("cross-account operation error=%v", err)
	}
	if _, err := otherClient.GetAsset(ctx, asset.AssetID); httpFailureStatus(err) != http.StatusNotFound {
		t.Fatalf("cross-account asset error=%v", err)
	}

	for _, scenario := range []struct {
		code   codes.Code
		status int
	}{{codes.Unauthenticated, 422}, {codes.PermissionDenied, 422}, {codes.ResourceExhausted, 429}, {codes.DeadlineExceeded, 504}, {codes.Canceled, 504}, {codes.Unavailable, 503}} {
		fixture.discoveryFailure.Store(int32(scenario.code))
		rejected := httptest.NewRecorder()
		router.ServeHTTP(rejected, authenticatedJSONRequest(http.MethodPost, "/api/management/connections", fmt.Sprintf(`{"name":"Rejected %d","provider":%q,"fields":{%q:%q,%q:%q,"grpc_tls":"false"}}`, scenario.code, provider, addressField, listener.Addr().String(), tokenField, fixture.token), owner))
		if rejected.Code != scenario.status || strings.Contains(rejected.Body.String(), "private") {
			t.Fatalf("verification status=%d body=%s", rejected.Code, rejected.Body.String())
		}
	}
	fixture.discoveryFailure.Store(0)
	inventory := accountConnectionExchange(t, router, owner, http.MethodGet, "/connections", nil, http.StatusOK)
	if len(inventory["connections"].([]any)) != 1 {
		t.Fatalf("failed verification stored a connection: %v", inventory)
	}
	fixture.invalidDiarization.Store(true)
	fixture.stopped.Delete("private-diarization")
	invalidResult, err := client.CreateMediaOperation(ctx, "invalid-native-diarization", llmproxyclient.MediaOperationInput{Provider: provider, Model: model, Capability: "audio.diarize", Input: json.RawMessage(scenarios[0].input), Controls: json.RawMessage(scenarios[0].controls)})
	if err != nil {
		t.Fatal(err)
	}
	invalidResult, err = waitForDictatorAcceptanceOperation(t, client, invalidResult.OperationID)
	if err != nil || invalidResult.State != "uncertain" || len(invalidResult.Outputs) != 0 {
		t.Fatalf("invalid provider result=%+v error=%v", invalidResult, err)
	}
	fixture.stopped.Delete("private-synthesis")
	for index, document := range []string{`{`, `null`, `[]`, `{"textSegments":[]}`, `{"textSegments":[{"content":"Hello","end":1}]}`, `{"textSegments":[{"content":"Hello","start":-1,"end":1}]}`, `{"textSegments":[{"content":"Hello","start":2,"end":1}]}`, `{"textSegments":[{"content":"Hello","start":0,"end":1,"file":"/private/path"}]}`} {
		fixture.timelineOverride.Store(document)
		invalid, err := client.CreateMediaOperation(ctx, fmt.Sprintf("invalid-timeline-%d", index), llmproxyclient.MediaOperationInput{Provider: provider, Model: scenarios[3].scenarioModel, Capability: scenarios[3].capability, Input: json.RawMessage(scenarios[3].input), Controls: json.RawMessage(scenarios[3].controls)})
		if err != nil {
			t.Fatal(err)
		}
		invalid, err = waitForDictatorAcceptanceOperation(t, client, invalid.OperationID)
		if err != nil || invalid.State != "uncertain" || len(invalid.Outputs) != 0 {
			t.Fatalf("invalid timeline %s: result=%+v error=%v", document, invalid, err)
		}
	}

	for index, fields := range []map[string]string{
		{addressField: otherListener.Addr().String(), tokenField: "second-account-token", "grpc_tls": "false"},
		{addressField: otherListener.Addr().String(), tokenField: "rotated-account-token", "grpc_tls": "false"},
	} {
		otherFixture.rotatedToken.Store(fields[tokenField])
		current := accountConnectionExchange(t, router, owner, http.MethodGet, "/connections/"+connection.ID, nil, http.StatusOK)
		accountConnectionExchange(t, router, owner, http.MethodPut, "/connections/"+connection.ID, map[string]any{"name": "Changed speech server", "provider": provider, "version": current["version"], "fields": fields}, http.StatusOK)
		for _, staleID := range []string{voices[0].VoiceID, extractedID} {
			stale, err := client.CreateMediaOperation(ctx, fmt.Sprintf("stale-voice-%d-%s", index, staleID), llmproxyclient.MediaOperationInput{Provider: provider, Model: proxy.ModelNameDictatorSileroRU, Capability: "audio.speech.generate", Input: json.RawMessage(`{"text":"Hello","voice_id":"` + staleID + `"}`), Controls: json.RawMessage(`{"language":"en","text_format":"plain","sample_rate_hz":24000}`)})
			var failure *llmproxyclient.HTTPFailure
			if !errors.As(err, &failure) || failure.StatusCode() != http.StatusBadRequest {
				t.Errorf("obsolete voice was not rejected before discovery: operation=%+v error=%v", stale, err)
			}
		}
		currentVoicesPage, err := client.GetMediaVoices(ctx, llmproxyclient.MediaVoiceQuery{Provider: provider})
		currentVoices := currentVoicesPage.Voices
		if err != nil || len(currentVoices) != 1 || currentVoices[0].VoiceID == voices[0].VoiceID {
			t.Fatalf("voices after connection change: %+v error=%v", currentVoices, err)
		}
		currentInput := llmproxyclient.MediaOperationInput{Provider: provider, Model: scenarios[3].scenarioModel, Capability: scenarios[3].capability, Input: json.RawMessage(`{"text":"<speak>Привет</speak>","voice_id":"` + currentVoices[0].VoiceID + `"}`), Controls: json.RawMessage(scenarios[3].controls)}
		currentOperation, err := client.CreateMediaOperation(ctx, fmt.Sprintf("current-voice-%d", index), currentInput)
		if err != nil {
			t.Fatal(err)
		}
		currentOperation, err = waitForDictatorAcceptanceOperation(t, client, currentOperation.OperationID)
		if err != nil || currentOperation.State != "succeeded" {
			t.Fatalf("current voice execution: %+v error=%v", currentOperation, err)
		}
		voices = currentVoices
	}

	validSpeech := llmproxyclient.MediaOperationInput{Provider: provider, Model: scenarios[3].scenarioModel, Capability: scenarios[3].capability, Input: json.RawMessage(`{"text":"<speak>Привет</speak>","voice_id":"` + voices[0].VoiceID + `"}`), Controls: json.RawMessage(scenarios[3].controls)}
	admissionFailure.Store(true)
	_, err = client.CreateMediaOperation(ctx, "lost-admission-assignment", validSpeech)
	admissionFailure.Store(false)
	var failure *llmproxyclient.HTTPFailure
	if !errors.As(err, &failure) || failure.StatusCode() != http.StatusBadRequest {
		t.Fatalf("lost admission assignment: %v", err)
	}
	if err := database.Table("media_voice_records").Where("voice_id = ?", voices[0].VoiceID).Update("provider_voice_reference", `{`).Error; err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateMediaOperation(ctx, "malformed-admission-voice", validSpeech)
	if !errors.As(err, &failure) || failure.StatusCode() != http.StatusBadRequest {
		t.Fatalf("malformed admission voice: %v", err)
	}

}

func (fixture *dictatorGRPCFixture) jobState(jobID string) int32 {
	if _, stopped := fixture.stopped.Load(jobID); stopped {
		return 5
	}
	if fixture.pending.Load() {
		if _, announced := fixture.announced.LoadOrStore(jobID, true); !announced {
			fixture.observed <- jobID
		}
		return 2
	}
	return 3
}

func (fixture *dictatorGRPCFixture) SubmitDiarizeAudioJob(_ context.Context, request *dictator.DiarizeAudioRequest) (*dictator.SubmitDiarizeAudioJobResponse, error) {
	if request.AudioArtifactId != "private-input" || request.ModelSize != "base" || !request.IncludeWords || !request.IncludeSpeakers || !request.IncludeUtterances || !request.IncludeSpeakerSegments {
		return nil, status.Error(codes.InvalidArgument, "wrong diarization controls")
	}
	fixture.diarizationRequest.Store(request)
	return &dictator.SubmitDiarizeAudioJobResponse{JobId: "private-diarization", State: dictator.DiarizationJobState_DIARIZATION_JOB_STATE_QUEUED}, nil
}
func (fixture *dictatorGRPCFixture) GetDiarizeAudioJob(_ context.Context, request *dictator.GetDiarizeAudioJobRequest) (*dictator.GetDiarizeAudioJobResponse, error) {
	result, _ := structpb.NewStruct(map[string]any{"text": "Speaker one.", "languageCode": "en", "words": []any{map[string]any{"word": "Speaker", "speaker": "speaker_1", "start": 0.0, "end": 1.0}}, "utterances": []any{map[string]any{"text": "Speaker one.", "speaker": "speaker_1", "start": 0.0, "end": 1.0, "words": []any{}}}, "speakers": []any{map[string]any{"speaker": "speaker_1", "wordCount": 2, "utteranceCount": 1, "totalDurationSeconds": 1.0}}, "speakerSegments": []any{map[string]any{"speaker": "speaker_1", "start": 0.0, "end": 1.0}}})
	if fixture.invalidDiarization.Load() {
		result.Fields["source_artifact_id"] = structpb.NewStringValue("private-artifact")
	}
	return &dictator.GetDiarizeAudioJobResponse{JobId: request.JobId, State: dictator.DiarizationJobState(fixture.jobState(request.JobId)), Diarization: result}, nil
}
func (fixture *dictatorGRPCFixture) SubmitAlignTranscriptJob(_ context.Context, request *dictator.AlignTranscriptRequest) (*dictator.SubmitAlignTranscriptJobResponse, error) {
	if request.GetTranscriptText() != "A clear transcript." || !request.RemovePunctuation || request.LanguageCode != "en" {
		return nil, status.Error(codes.InvalidArgument, "wrong alignment controls")
	}
	return &dictator.SubmitAlignTranscriptJobResponse{JobId: "private-alignment", State: dictator.AlignmentJobState_ALIGNMENT_JOB_STATE_QUEUED}, nil
}
func (fixture *dictatorGRPCFixture) GetAlignTranscriptJob(_ context.Context, request *dictator.GetAlignTranscriptJobRequest) (*dictator.GetAlignTranscriptJobResponse, error) {
	return &dictator.GetAlignTranscriptJobResponse{JobId: request.JobId, State: dictator.AlignmentJobState(fixture.jobState(request.JobId)), LanguageCode: "en", Words: []*dictator.WordSegment{{Content: "A", StartSeconds: 0, EndSeconds: 0.2}}, SrtArtifactId: "private-aligned-subtitles"}, nil
}
func (fixture *dictatorGRPCFixture) SubmitRenderSubtitlesJob(_ context.Context, request *dictator.RenderSubtitlesRequest) (*dictator.SubmitRenderSubtitlesJobResponse, error) {
	fixture.subtitleRequest.Store(request)
	if request.AudioArtifactId != "private-input" || request.Granularity != dictator.SubtitleGranularity_SUBTITLE_GRANULARITY_SENTENCES || request.GroupSize != 2 || request.GetSourceText() != "A clear transcript." || request.OutputFormat != dictator.SubtitleFormat_SUBTITLE_FORMAT_SRT {
		return nil, status.Error(codes.InvalidArgument, "wrong subtitle controls")
	}
	return &dictator.SubmitRenderSubtitlesJobResponse{JobId: "private-subtitles", State: dictator.SubtitleJobState_SUBTITLE_JOB_STATE_QUEUED}, nil
}
func (fixture *dictatorGRPCFixture) GetRenderSubtitlesJob(_ context.Context, request *dictator.GetRenderSubtitlesJobRequest) (*dictator.GetRenderSubtitlesJobResponse, error) {
	return &dictator.GetRenderSubtitlesJobResponse{JobId: request.JobId, State: dictator.SubtitleJobState(fixture.jobState(request.JobId)), SrtArtifactId: "private-subtitle-artifact"}, nil
}
func (fixture *dictatorGRPCFixture) SubmitSynthesizeSpeechJob(_ context.Context, request *dictator.SynthesizeSpeechRequest) (*dictator.SubmitSynthesizeSpeechJobResponse, error) {
	if request.SynthesisEngine == dictator.SynthesisEngine_SYNTHESIS_ENGINE_SILERO_RU {
		if request.PresetSpeaker != "baya" || request.TextFormat != dictator.SynthesisTextFormat_SYNTHESIS_TEXT_FORMAT_SSML || request.GetText() != "<speak>Привет</speak>" || request.AudioFormat.SampleRateHz != 48000 || !request.IncludeTimeline || request.MaxDurationSeconds != 5 {
			return nil, status.Error(codes.InvalidArgument, "wrong preset synthesis controls")
		}
	} else if request.SynthesisEngine == dictator.SynthesisEngine_SYNTHESIS_ENGINE_QWEN3 {
		if request.SpeakerArtifactId != "private-extracted-sample" || request.SpeakerTranscriptText != "A clear transcript." || request.TextFormat != dictator.SynthesisTextFormat_SYNTHESIS_TEXT_FORMAT_PLAIN_TEXT {
			return nil, status.Error(codes.InvalidArgument, "wrong extracted synthesis controls")
		}
	} else {
		return nil, status.Error(codes.InvalidArgument, "unknown engine")
	}
	return &dictator.SubmitSynthesizeSpeechJobResponse{JobId: "private-synthesis", State: dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_QUEUED}, nil
}
func (fixture *dictatorGRPCFixture) GetSynthesizeSpeechJob(_ context.Context, request *dictator.GetSynthesizeSpeechJobRequest) (*dictator.GetSynthesizeSpeechJobResponse, error) {
	return &dictator.GetSynthesizeSpeechJobResponse{JobId: request.JobId, State: dictator.SynthesisJobState(fixture.jobState(request.JobId)), AudioArtifact: &dictator.ArtifactRef{ArtifactId: "private-audio"}, TimelineArtifactId: "private-timeline"}, nil
}
func (fixture *dictatorGRPCFixture) SubmitExtractReferenceSampleJob(_ context.Context, request *dictator.ExtractReferenceSampleRequest) (*dictator.SubmitExtractReferenceSampleJobResponse, error) {
	fixture.extractionRequest.Store(request)
	if request.SourceArtifactId != "private-input" || request.ModelSize != "base" || request.LanguageCode != "en" {
		return nil, status.Error(codes.InvalidArgument, "wrong extraction controls")
	}
	return &dictator.SubmitExtractReferenceSampleJobResponse{JobId: "private-extraction", State: dictator.ExtractReferenceSampleJobState_EXTRACT_REFERENCE_SAMPLE_JOB_STATE_QUEUED}, nil
}
func (fixture *dictatorGRPCFixture) GetExtractReferenceSampleJob(_ context.Context, request *dictator.GetExtractReferenceSampleJobRequest) (*dictator.GetExtractReferenceSampleJobResponse, error) {
	return &dictator.GetExtractReferenceSampleJobResponse{JobId: request.JobId, State: dictator.ExtractReferenceSampleJobState(fixture.jobState(request.JobId)), SampleArtifact: &dictator.ArtifactRef{ArtifactId: "private-extracted-sample", MediaType: "audio/wav"}}, nil
}
func (fixture *dictatorGRPCFixture) DownloadArtifact(request *dictator.DownloadArtifactRequest, stream grpc.ServerStreamingServer[dictator.DownloadArtifactChunk]) error {
	outputs := map[string]struct{ mime, data string }{
		"private-aligned-subtitles": {"application/x-subrip", "1\n00:00:00,000 --> 00:00:00,200\nA\n"},
		"private-subtitle-artifact": {"application/x-subrip", "1\n00:00:00,000 --> 00:00:01,000\nA clear transcript.\n"},
		"private-audio":             {"audio/wav", "synthesized audio"},
		"private-timeline":          {"application/json", `{"textSegments":[{"content":"Hello","start":0,"end":1}],"imageCues":[],"voices":[{"id":"private-extracted-sample","label":"private-voice","engine":"qwen3","file":"/srv/private-voices/sample.wav","speaker":"baya"}]}`},
	}
	output, ok := outputs[request.ArtifactId]
	if !ok {
		return status.Error(codes.NotFound, "artifact missing")
	}
	if request.ArtifactId == "private-timeline" {
		if override := fixture.timelineOverride.Load(); override != nil {
			output.data = override.(string)
		}
	}
	content := []byte(output.data)
	return stream.Send(&dictator.DownloadArtifactChunk{Artifact: &dictator.ArtifactRef{ArtifactId: request.ArtifactId, MediaType: output.mime, SizeBytes: int64(len(content)), Sha256: fmt.Sprintf("%x", sha256.Sum256(content))}, Content: content, Eof: true})
}

func (fixture *dictatorGRPCFixture) CancelTranscribeJob(_ context.Context, request *dictator.CancelTranscribeJobRequest) (*dictator.CancelTranscribeJobResponse, error) {
	fixture.cancelled.Add(1)
	fixture.stopped.Store(request.JobId, true)
	return &dictator.CancelTranscribeJobResponse{JobId: request.JobId, State: dictator.TranscriptionJobState_TRANSCRIPTION_JOB_STATE_CANCELED}, nil
}

func (fixture *dictatorGRPCFixture) CancelDiarizeAudioJob(_ context.Context, request *dictator.CancelDiarizeAudioJobRequest) (*dictator.CancelDiarizeAudioJobResponse, error) {
	fixture.cancelled.Add(1)
	fixture.stopped.Store(request.JobId, true)
	return &dictator.CancelDiarizeAudioJobResponse{JobId: request.JobId, State: dictator.DiarizationJobState_DIARIZATION_JOB_STATE_CANCELED}, nil
}

func (fixture *dictatorGRPCFixture) CancelAlignTranscriptJob(_ context.Context, request *dictator.CancelAlignTranscriptJobRequest) (*dictator.CancelAlignTranscriptJobResponse, error) {
	fixture.cancelled.Add(1)
	fixture.stopped.Store(request.JobId, true)
	return &dictator.CancelAlignTranscriptJobResponse{JobId: request.JobId, State: dictator.AlignmentJobState_ALIGNMENT_JOB_STATE_CANCELED}, nil
}

func (fixture *dictatorGRPCFixture) CancelRenderSubtitlesJob(_ context.Context, request *dictator.CancelRenderSubtitlesJobRequest) (*dictator.CancelRenderSubtitlesJobResponse, error) {
	fixture.cancelled.Add(1)
	fixture.stopped.Store(request.JobId, true)
	return &dictator.CancelRenderSubtitlesJobResponse{JobId: request.JobId, State: dictator.SubtitleJobState_SUBTITLE_JOB_STATE_CANCELED}, nil
}

func (fixture *dictatorGRPCFixture) CancelSynthesizeSpeechJob(_ context.Context, request *dictator.CancelSynthesizeSpeechJobRequest) (*dictator.CancelSynthesizeSpeechJobResponse, error) {
	fixture.cancelled.Add(1)
	fixture.stopped.Store(request.JobId, true)
	return &dictator.CancelSynthesizeSpeechJobResponse{JobId: request.JobId, State: dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_CANCELED}, nil
}

func (fixture *dictatorGRPCFixture) CancelExtractReferenceSampleJob(_ context.Context, request *dictator.CancelExtractReferenceSampleJobRequest) (*dictator.CancelExtractReferenceSampleJobResponse, error) {
	fixture.cancelled.Add(1)
	fixture.stopped.Store(request.JobId, true)
	return &dictator.CancelExtractReferenceSampleJobResponse{JobId: request.JobId, State: dictator.ExtractReferenceSampleJobState_EXTRACT_REFERENCE_SAMPLE_JOB_STATE_CANCELED}, nil
}

func newDictatorAcceptanceUpstream(t *testing.T, provider string) (*dictatorGRPCFixture, net.Listener) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &dictatorGRPCFixture{token: provider + "-private-token", observed: make(chan string, 32)}
	upstream := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		values, _ := metadata.FromIncomingContext(ctx)
		if strings.Join(values.Get("authorization"), "") != "Bearer "+fixture.token {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
		if strings.Contains(info.FullMethod, "/Submit") {
			fixture.submissions.Add(1)
		}
		return handler(ctx, request)
	}))
	dictator.RegisterVoiceServiceServer(upstream, fixture)
	dictator.RegisterTranscriptionServiceServer(upstream, fixture)
	dictator.RegisterArtifactServiceServer(upstream, fixture)
	dictator.RegisterAlignmentServiceServer(upstream, fixture)
	dictator.RegisterSubtitleServiceServer(upstream, fixture)
	go func() { _ = upstream.Serve(listener) }()
	t.Cleanup(upstream.Stop)
	return fixture, listener
}

func TestDictatorGatewayAcceptanceHarness(t *testing.T) {
	fixture, listener := newDictatorAcceptanceUpstream(t, proxy.ProviderNameDictator)
	client := newDictatorGatewayAcceptanceClient(t, listener.Addr().String(), fixture.token, "false")
	exerciseDictatorGatewayAcceptance(t, client, []byte("fixture audio"), dictatorAcceptanceText{
		transcript: "A clear transcript.", transcription: "A clear transcript.", diarization: "Speaker one.", alignment: "A", subtitles: "A clear transcript.",
	})
}

func TestDictatorGranularModelsReachUpstream(t *testing.T) {
	fixture, listener := newDictatorAcceptanceUpstream(t, proxy.ProviderNameDictator)
	client := newDictatorGatewayAcceptanceClient(t, listener.Addr().String(), fixture.token, "false")
	ctx := t.Context()
	asset, err := client.UploadAsset(ctx, llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: []byte("fixture audio")})
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []string{"tiny", "base", "small", "medium", "large-v3"} {
		for _, capability := range []string{"audio.transcribe", "subtitles.create"} {
			t.Run(size+"/"+capability, func(t *testing.T) {
				input := `{"audio_asset_id":"` + asset.AssetID + `"}`
				controls := `{"language":"en"}`
				if capability == "subtitles.create" {
					input = `{"audio_asset_id":"` + asset.AssetID + `","transcript":"A clear transcript."}`
					controls = `{"language":"en","granularity":"sentence","group_size":2}`
				}
				operation, err := client.CreateMediaOperation(ctx, size+"-"+capability, llmproxyclient.MediaOperationInput{Provider: proxy.ProviderNameDictator, Model: "whisper-" + size, Capability: capability, Input: json.RawMessage(input), Controls: json.RawMessage(controls)})
				if err != nil {
					t.Fatal(err)
				}
				completed, err := waitForDictatorAcceptanceOperation(t, client, operation.OperationID)
				if err != nil || completed.State != "succeeded" {
					t.Fatalf("operation=%+v error=%v", completed, err)
				}
				actual := fixture.transcriptionRequest.Load().GetModelSize()
				if capability == "subtitles.create" {
					actual = fixture.subtitleRequest.Load().GetModelSize()
				}
				if actual != size {
					t.Fatalf("upstream model size=%q want=%q", actual, size)
				}
			})
		}
	}
}

func TestDictatorRejectsVoiceFromAnotherSynthesisEngine(t *testing.T) {
	fixture, listener := newDictatorAcceptanceUpstream(t, proxy.ProviderNameDictator)
	client := newDictatorGatewayAcceptanceClient(t, listener.Addr().String(), fixture.token, "false")
	voicesPage, err := client.GetMediaVoices(t.Context(), llmproxyclient.MediaVoiceQuery{Provider: proxy.ProviderNameDictator})
	voices := voicesPage.Voices
	if err != nil || len(voices) != 1 {
		t.Fatalf("voices=%v error=%v", voices, err)
	}
	_, err = client.CreateMediaOperation(t.Context(), "incompatible-voice", llmproxyclient.MediaOperationInput{Provider: proxy.ProviderNameDictator, Model: proxy.ModelNameDictatorQwen3TTS, Capability: "audio.speech.generate", Input: json.RawMessage(`{"text":"Hello","voice_id":"` + voices[0].VoiceID + `"}`), Controls: json.RawMessage(`{"language":"ru","text_format":"plain","sample_rate_hz":48000}`)})
	if httpFailureStatus(err) != http.StatusBadRequest || fixture.submissions.Load() != 0 {
		t.Fatalf("incompatible voice error=%v submissions=%d", err, fixture.submissions.Load())
	}
}
