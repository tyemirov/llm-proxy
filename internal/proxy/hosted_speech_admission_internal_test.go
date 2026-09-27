package proxy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type hostedSpeechUpstream struct {
	dictator.UnimplementedVoiceServiceServer
	dictator.UnimplementedArtifactServiceServer
	submissions atomic.Int64
	cancelled   atomic.Bool
	beforePoll  atomic.Pointer[func(context.Context)]
}

func (upstream *hostedSpeechUpstream) SubmitSynthesizeSpeechJob(_ context.Context, request *dictator.SynthesizeSpeechRequest) (*dictator.SubmitSynthesizeSpeechJobResponse, error) {
	upstream.submissions.Add(1)
	if request.GetText() != "A short message." || request.PresetSpeaker != "native-preset" {
		return nil, fmt.Errorf("unexpected speech request")
	}
	return &dictator.SubmitSynthesizeSpeechJobResponse{JobId: "native-speech-job", State: dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_QUEUED}, nil
}
func (upstream *hostedSpeechUpstream) GetSynthesizeSpeechJob(ctx context.Context, request *dictator.GetSynthesizeSpeechJobRequest) (*dictator.GetSynthesizeSpeechJobResponse, error) {
	if beforePoll := upstream.beforePoll.Load(); beforePoll != nil {
		(*beforePoll)(ctx)
	}
	if upstream.cancelled.Load() {
		return &dictator.GetSynthesizeSpeechJobResponse{JobId: request.JobId, State: dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_CANCELED}, nil
	}
	return &dictator.GetSynthesizeSpeechJobResponse{JobId: request.JobId, State: dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_SUCCEEDED, AudioArtifact: &dictator.ArtifactRef{ArtifactId: "native-audio"}}, nil
}
func (upstream *hostedSpeechUpstream) CancelSynthesizeSpeechJob(_ context.Context, request *dictator.CancelSynthesizeSpeechJobRequest) (*dictator.CancelSynthesizeSpeechJobResponse, error) {
	upstream.cancelled.Store(true)
	return &dictator.CancelSynthesizeSpeechJobResponse{JobId: request.JobId, State: dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_CANCELED}, nil
}
func (*hostedSpeechUpstream) DownloadArtifact(_ *dictator.DownloadArtifactRequest, stream grpc.ServerStreamingServer[dictator.DownloadArtifactChunk]) error {
	audio := []byte("controlled audio")
	return stream.Send(&dictator.DownloadArtifactChunk{Artifact: &dictator.ArtifactRef{ArtifactId: "native-audio", MediaType: "audio/wav", SizeBytes: int64(len(audio)), Sha256: mediaSHA256Hex(audio)}, Content: audio, Eof: true})
}

func TestHostedSpeechAdmissionUsesPinnedAuthority(t *testing.T) {
	testHostedSpeechAuthority(t, "")
}

func TestHostedSpeechDispatchRejectsLostAuthority(t *testing.T) {
	for _, transition := range []string{"revoked", "expired"} {
		t.Run(transition, func(t *testing.T) { testHostedSpeechAuthority(t, transition) })
	}
}

func TestHostedSpeechRecoveryAndCancellationAfterRevocation(t *testing.T) {
	for _, transition := range []string{"recover", "cancel", "resubmit"} {
		t.Run(transition, func(t *testing.T) { testHostedSpeechAuthority(t, transition) })
	}
}

type hostedSpeechInvalidRecovery struct{ MediaOperationAdapter }

func (adapter hostedSpeechInvalidRecovery) Recover(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	return adapter.MediaOperationAdapter.Execute(ctx, request)
}

type hostedSpeechExecutionHook struct {
	MediaOperationAdapter
	before func(MediaOperationExecutionRequest)
}

func (adapter hostedSpeechExecutionHook) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	adapter.before(request)
	return adapter.MediaOperationAdapter.Execute(ctx, request)
}

func testHostedSpeechAuthority(t *testing.T, transition string) {
	t.Helper()
	database, _, read := newJournalTransactionFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	upstream := &hostedSpeechUpstream{}
	checkToken := func(ctx context.Context) {
		values, _ := metadata.FromIncomingContext(ctx)
		if strings.Join(values.Get("authorization"), "") != "Bearer hosted-speech-secret" {
			t.Error("speech lost pinned credential")
		}
	}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		checkToken(ctx)
		return handler(ctx, request)
	}), grpc.StreamInterceptor(func(server any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		checkToken(stream.Context())
		return handler(server, stream)
	}))
	dictator.RegisterVoiceServiceServer(grpcServer, upstream)
	dictator.RegisterArtifactServiceServer(grpcServer, upstream)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Error(err)
		}
	}()
	t.Cleanup(grpcServer.Stop)
	server, service := newHostedMediaAdmissionHTTPServer(t, database)
	imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
	provider := service.providers.definitions[ProviderNameDictator]
	offering, err := service.catalog.ResolveOffering(ProviderNameDictator, ModelNameDictatorQwen3TTS)
	if err != nil {
		t.Fatal(err)
	}
	service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioSpeechGenerate, ProviderNameDictator, ModelNameDictatorQwen3TTS)] = &accountDictatorAdapter{provider: ProviderNameDictator, model: ModelNameDictatorQwen3TTS, transport: provider.transports[offering.Transport], tenants: imageAdapter.tenants, store: service.store, assets: service.assets}
	fields := map[string]string{dictatorAddressField: listener.Addr().String(), dictatorTokenField: "hosted-speech-secret", dictatorTLSField: "false"}
	encrypted, err := imageAdapter.tenants.providerKeyCipher.encryptConnection(rand.Reader, platformCredentialReference("platform-speech", 1), ProviderNameDictator, dictatorTokenField, fields[dictatorTokenField])
	if err != nil {
		t.Fatal(err)
	}
	fields[dictatorTokenField] = encrypted
	encodedFields, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, record := range []any{
		&managedPlatformConnectionRecord{ID: "platform-speech", Provider: ProviderNameDictator, Name: "Speech", Version: 1, CreatedAt: now, UpdatedAt: now},
		&managedPlatformCredentialRecord{ConnectionID: "platform-speech", Version: 1, Fields: encodedFields, QualifiedAt: now, CreatedAt: now},
		&managedHostedGrantRecord{ID: "grant-speech", BillingAccountID: "billing-journal", TenantID: "managed-first", PlatformConnectionID: "platform-speech", Provider: ProviderNameDictator, CatalogRevision: service.catalog.Revision(), Offerings: []byte(`[{"model":"qwen3-tts","operations":["speech_generation"]}]`), State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
		&managedHostedGrantRevisionRecord{GrantID: "grant-speech", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Speech acceptance", CreatedAt: now},
		&managedHostedTenantAssignmentRecord{TenantID: "managed-first", ProviderID: ProviderNameDictator, GrantID: "grant-speech", CreatedAt: now},
		&managedProviderProfileRecord{TenantID: "managed-first", ProviderID: ProviderNameDictator, CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	authority := "platform-speech:" + mediaSHA256Hex([]byte(listener.Addr().String()+"\x00hosted-speech-secret\x00false"))
	reference, err := json.Marshal(dictatorVoiceReference{Binding: authority, Engine: dictator.SynthesisEngine_SYNTHESIS_ENGINE_QWEN3, Preset: "native-preset"})
	if err != nil {
		t.Fatal(err)
	}
	voice, err := persistMediaVoice(database.database, "managed-first", ProviderNameDictator, MediaVoiceProviderRecord{Authority: authority, Provider: ProviderNameDictator, Mode: MediaVoiceModePreset, DisplayName: "Speech voice", SampleRates: []int{24000}, DefaultSampleRate: 24000, ProviderVoiceReference: string(reference)}, now)
	if err != nil {
		t.Fatal(err)
	}
	var reservations atomic.Int64
	service.hostedAdmission = func(_ *gorm.DB, record managedJournalRequestRecord, _ mediaOperationRecord) error {
		if record.CredentialVersion != 1 || record.PlatformConnectionID != "platform-speech" {
			t.Error("speech admission changed authority")
		}
		reservations.Add(1)
		return nil
	}
	intent := fmt.Sprintf(`{"capability":"audio.speech.generate","provider":"dictator","model":"qwen3-tts","input":{"text":"A short message.","voice_id":%q},"controls":{"language":"en","text_format":"plain","sample_rate_hz":24000}}`, voice.VoiceID)
	for _, invalid := range []struct{ field, value string }{
		{"tenant_id", "managed-second"}, {"provider", "elevenlabs"}, {"authority", "other-platform"},
	} {
		if err := database.database.Model(&mediaVoiceRecord{}).Where("voice_id = ?", voice.VoiceID).Update(invalid.field, invalid.value).Error; err != nil {
			t.Fatal(err)
		}
		hostedSpeechHTTP(t, server, "invalid-voice-"+invalid.field, intent, http.StatusBadRequest)
		if err := database.database.Model(&mediaVoiceRecord{}).Where("voice_id = ?", voice.VoiceID).Updates(map[string]any{"tenant_id": "managed-first", "provider": ProviderNameDictator, "authority": authority}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if reservations.Load() != 0 || upstream.submissions.Load() != 0 {
		t.Fatal("invalid voice created financial or provider effects")
	}
	accepted := hostedSpeechHTTP(t, server, "speech-intent", intent, http.StatusAccepted)
	id := accepted["operation_id"].(string)
	if upstream.submissions.Load() != 0 {
		t.Fatal("validation submitted paid work")
	}
	if transition == "recover" || transition == "cancel" || transition == "resubmit" {
		firstPoll, releasePoll, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var polls atomic.Int64
		var release sync.Once
		beforePoll := func(ctx context.Context) {
			if polls.Add(1) == 1 {
				close(firstPoll)
				select {
				case <-releasePoll:
				case <-ctx.Done():
				}
			}
		}
		upstream.beforePoll.Store(&beforePoll)
		go func() { defer close(done); service.runOperation("first-speech-worker", id) }()
		t.Cleanup(func() { release.Do(func() { close(releasePoll) }); waitHostedMediaWorker(t, done) })
		select {
		case <-firstPoll:
		case <-time.After(5 * time.Second):
			t.Fatal("speech did not reach provider status")
		}
		if transition != "resubmit" {
			if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-speech").Update("state", hostedGrantRevoked).Error; err != nil {
				t.Fatal(err)
			}
		}
		wantState, wantJournal := MediaOperationStateSucceeded, string(journalRequestCompleted)
		if transition != "cancel" {
			if err := database.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ?", id).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
				t.Fatal(err)
			}
			if transition == "resubmit" {
				key := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioSpeechGenerate, ProviderNameDictator, ModelNameDictatorQwen3TTS)
				service.adapters[key] = hostedSpeechInvalidRecovery{MediaOperationAdapter: service.adapters[key]}
				wantState, wantJournal = MediaOperationStateUncertain, string(journalRequestUncertain)
			}
			service.runOperation("recovered-speech-worker", id)
		} else {
			request, err := http.NewRequest(http.MethodPut, server.URL+llmproxycontract.MediaOperationsPath+"/"+id+"/cancellation", strings.NewReader(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || response.StatusCode != http.StatusOK {
				t.Fatalf("speech cancellation status=%d body=%s error=%v", response.StatusCode, body, err)
			}
			template := request.Clone(t.Context())
			template.URL.Path = llmproxycontract.MediaOperationsPath + "/{operation_id}/cancellation"
			validateHostedIdentityResponse(t, template, response, body)
			wantState, wantJournal = MediaOperationStateCancelled, string(journalRequestFailed)
		}
		release.Do(func() { close(releasePoll) })
		waitHostedMediaWorker(t, done)
		result := hostedMediaWorkerStatus(t, server, id)
		entry := read("")["requests"].([]any)[0].(map[string]any)
		if result["state"] != wantState || entry["state"] != wantJournal || upstream.submissions.Load() != 1 || reservations.Load() != 2 {
			t.Fatalf("speech %s result=%v journal=%v submissions=%d reservations=%d", transition, result, entry, upstream.submissions.Load(), reservations.Load())
		}
		return
	}
	if transition != "" {
		key := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioSpeechGenerate, ProviderNameDictator, ModelNameDictatorQwen3TTS)
		service.adapters[key] = hostedSpeechExecutionHook{MediaOperationAdapter: service.adapters[key], before: func(request MediaOperationExecutionRequest) {
			var err error
			if transition == "revoked" {
				err = database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-speech").Update("state", hostedGrantRevoked).Error
			} else {
				err = database.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ?", request.OperationID).Update("expires_at", time.Now().Add(-time.Second)).Error
			}
			if err != nil {
				t.Error(err)
			}
		}}
		service.runOperation("obsolete-speech-worker", id)
		if upstream.submissions.Load() != 0 {
			t.Fatalf("gRPC submitted work after authority became %s: %d", transition, upstream.submissions.Load())
		}
		replay := hostedSpeechHTTP(t, server, "speech-intent", intent, http.StatusOK)
		if replay["operation_id"] != id || reservations.Load() != 2 {
			t.Fatal("rejected dispatch changed accepted identity or reservation")
		}
		return
	}
	// Rotation after acceptance must not change the accepted voice or credential.
	fields[dictatorTokenField], err = imageAdapter.tenants.providerKeyCipher.encryptConnection(rand.Reader, platformCredentialReference("platform-speech", 2), ProviderNameDictator, dictatorTokenField, "rotated-speech-secret")
	if err != nil {
		t.Fatal(err)
	}
	encodedFields, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.database.Omit(clause.Associations).Create(&managedPlatformCredentialRecord{ConnectionID: "platform-speech", Version: 2, Fields: encodedFields, QualifiedAt: now, CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&managedPlatformConnectionRecord{}).Where("id = ?", "platform-speech").Update("version", 2).Error; err != nil {
		t.Fatal(err)
	}
	service.runOperation("speech-worker", id)
	result := hostedMediaWorkerStatus(t, server, id)
	entry := read("")["requests"].([]any)[0].(map[string]any)
	if result["state"] != MediaOperationStateSucceeded || entry["state"] != string(journalRequestCompleted) || entry["usage_state"] != string(journalUsageUnknown) {
		t.Fatalf("speech result=%v journal=%v", result, entry)
	}
	var attempt managedJournalAttemptRecord
	if err := database.database.Where("request_id = ?", entry["id"]).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.ProviderRequestID != "native-speech-job" {
		t.Fatalf("speech journal identity=%q", attempt.ProviderRequestID)
	}
	replay := hostedSpeechHTTP(t, server, "speech-intent", intent, http.StatusOK)
	if replay["operation_id"] != id {
		t.Fatal("speech replay changed identity")
	}
	hostedSpeechHTTP(t, server, "speech-intent", strings.Replace(intent, "A short message.", "Changed.", 1), http.StatusConflict)
	hostedSpeechHTTP(t, server, "rotated-voice", intent, http.StatusBadRequest)
	adapterKey := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioSpeechGenerate, ProviderNameDictator, ModelNameDictatorQwen3TTS)
	adapter := service.adapters[adapterKey]
	for _, change := range []string{"rotate", "revoke"} {
		if err := database.database.Model(&managedPlatformConnectionRecord{}).Where("id = ?", "platform-speech").Update("version", 1).Error; err != nil {
			t.Fatal(err)
		}
		service.adapters[adapterKey] = hostedMediaValidationHook{MediaOperationAdapter: adapter, after: func() error {
			if change == "rotate" {
				return database.database.Model(&managedPlatformConnectionRecord{}).Where("id = ?", "platform-speech").Update("version", 2).Error
			}
			return database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-speech").Update("state", hostedGrantRevoked).Error
		}}
		hostedSpeechHTTP(t, server, "validation-"+change, intent, http.StatusUnprocessableEntity)
	}
	if count := len(read("")["requests"].([]any)); count != 1 {
		t.Fatalf("rejected speech produced %d journal requests", count)
	}
	hostedSpeechHTTP(t, server, "speech-intent", intent, http.StatusOK)
	if upstream.submissions.Load() != 1 || reservations.Load() != 2 {
		t.Fatalf("speech effects: posts=%d reservations=%d", upstream.submissions.Load(), reservations.Load())
	}
}

func hostedSpeechHTTP(t *testing.T, server *httptest.Server, key, intent string, want int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+llmproxycontract.MediaOperationsPath, strings.NewReader(intent))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("speech status=%d want=%d body=%s", response.StatusCode, want, body)
	}
	validateHostedIdentityResponse(t, request, response, body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
