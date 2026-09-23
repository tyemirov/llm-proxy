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
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"gorm.io/gorm/clause"
)

type hostedVoiceGRPCFixture struct {
	dictator.UnimplementedVoiceServiceServer
	calls       atomic.Int64
	submissions atomic.Int64
}

func (upstream *hostedVoiceGRPCFixture) ListSynthesisVoices(ctx context.Context, _ *dictator.ListSynthesisVoicesRequest) (*dictator.ListSynthesisVoicesResponse, error) {
	upstream.calls.Add(1)
	values, _ := metadata.FromIncomingContext(ctx)
	if strings.Join(values.Get("authorization"), "") != "Bearer hosted-voice-secret" {
		return nil, fmt.Errorf("incorrect hosted metadata credential")
	}
	return &dictator.ListSynthesisVoicesResponse{Voices: []*dictator.SynthesisVoice{
		{VoiceId: "private-native-preset", DisplayName: "Preset voice", SynthesisEngine: dictator.SynthesisEngine_SYNTHESIS_ENGINE_QWEN3, NativeSampleRateHz: []int32{24000}, DefaultSampleRateHz: 24000},
		{VoiceId: "private-native-reference", DisplayName: "Private reference", SynthesisEngine: dictator.SynthesisEngine_SYNTHESIS_ENGINE_QWEN3, RequiresReferenceAudio: true},
	}}, nil
}

func (upstream *hostedVoiceGRPCFixture) SubmitSynthesizeSpeechJob(context.Context, *dictator.SynthesizeSpeechRequest) (*dictator.SubmitSynthesizeSpeechJobResponse, error) {
	upstream.submissions.Add(1)
	return &dictator.SubmitSynthesizeSpeechJobResponse{JobId: "unwanted-job"}, nil
}

type hostedVoiceSubmissionProbe struct{ *accountDictatorAdapter }

func (adapter hostedVoiceSubmissionProbe) DiscoverMediaVoices(ctx context.Context, tenant string, _ MediaVoiceQuery) (MediaVoiceDiscovery, error) {
	reference, err := adapter.store.credentialReference(ctx, tenant, providerID(adapter.provider))
	if err != nil {
		return MediaVoiceDiscovery{}, err
	}
	protocol, closeConnection, err := adapter.bindProtocol(ctx, tenant, reference)
	if err != nil {
		return MediaVoiceDiscovery{}, err
	}
	defer closeConnection()
	_, err = dictator.NewVoiceServiceClient(protocol.connection).SubmitSynthesizeSpeechJob(protocol.context(ctx), &dictator.SynthesizeSpeechRequest{})
	return MediaVoiceDiscovery{}, err
}

func TestHostedVoicesGRPCPreservesTenantResourcesAndMetadataAuthority(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	upstream := &hostedVoiceGRPCFixture{}
	grpcServer := grpc.NewServer()
	dictator.RegisterVoiceServiceServer(grpcServer, upstream)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Error(err)
		}
	}()
	t.Cleanup(grpcServer.Stop)
	server, service := newHostedVoiceHTTPFixture(t, database, ProviderNameDictator, ModelNameDictatorQwen3TTS, map[string]string{dictatorAddressField: listener.Addr().String(), dictatorTokenField: "hosted-voice-secret", dictatorTLSField: "false"})
	imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
	provider := service.providers.definitions[ProviderNameDictator]
	adapter := &accountDictatorAdapter{provider: ProviderNameDictator, transport: provider.transports["speech"], tenants: imageAdapter.tenants, store: service.store, assets: service.assets}
	service.voiceProviders = map[string]MediaVoiceProvider{ProviderNameDictator: adapter}
	authority := "platform-voices:" + mediaSHA256Hex([]byte(listener.Addr().String()+"\x00hosted-voice-secret\x00false"))
	var foreignID string
	for _, tenant := range []string{"managed-first", "managed-second"} {
		reference, _ := json.Marshal(dictatorVoiceReference{Binding: authority, Engine: dictator.SynthesisEngine_SYNTHESIS_ENGINE_QWEN3, Artifact: "private-native-" + tenant, Transcript: "private transcript"})
		voice, err := persistMediaVoice(database.database, tenant, ProviderNameDictator, MediaVoiceProviderRecord{Authority: authority, Provider: ProviderNameDictator, Model: ModelNameDictatorQwen3TTS, Mode: MediaVoiceModeExtracted, Language: "en", DisplayName: tenant, SampleRates: []int{24000}, DefaultSampleRate: 24000, ProviderVoiceReference: string(reference)}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if tenant == "managed-second" {
			foreignID = voice.VoiceID
		}
	}
	page := hostedVoiceHTTP(t, server, "/model/v1/voices?provider=dictator&include_total_count=true", http.StatusOK)
	if len(page["voices"].([]any)) != 2 || page["total_count"] != float64(2) || upstream.calls.Load() != 1 {
		t.Fatalf("hosted Dictator page=%v calls=%d", page, upstream.calls.Load())
	}
	for _, item := range page["voices"].([]any) {
		voice := item.(map[string]any)
		if voice["display_name"] == "managed-second" || voice["display_name"] == "Private reference" {
			t.Fatalf("foreign voice=%v", voice)
		}
		hostedVoiceHTTP(t, server, "/model/v1/voices/"+voice["voice_id"].(string), http.StatusOK)
	}
	hostedVoiceHTTP(t, server, "/model/v1/voices/"+foreignID, http.StatusNotFound)
	paged := hostedVoiceHTTP(t, server, "/model/v1/voices?provider=dictator&page_size=1", http.StatusOK)
	secret, err := imageAdapter.tenants.providerKeyCipher.encryptConnection(rand.Reader, platformCredentialReference("platform-voices", 2), ProviderNameDictator, dictatorTokenField, "hosted-voice-secret")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := json.Marshal(map[string]string{dictatorAddressField: listener.Addr().String(), dictatorTokenField: secret, dictatorTLSField: "false"})
	if err := database.database.Create(&managedPlatformCredentialRecord{ConnectionID: "platform-voices", Version: 2, Fields: fields, QualifiedAt: time.Now(), CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&managedPlatformConnectionRecord{}).Where("id = ?", "platform-voices").Update("version", 2).Error; err != nil {
		t.Fatal(err)
	}
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=dictator&cursor="+url.QueryEscape(paged["next_cursor"].(string)), http.StatusBadRequest)
	service.voiceProviders[ProviderNameDictator] = hostedVoiceSubmissionProbe{adapter}
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=dictator", http.StatusForbidden)
	if upstream.submissions.Load() != 0 {
		t.Fatal("metadata authorization submitted paid work")
	}
	service.voiceProviders[ProviderNameDictator] = adapter
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-voices").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=dictator", http.StatusForbidden)
	if upstream.calls.Load() != 2 {
		t.Fatal("revoked Dictator discovery reached provider")
	}
}

func TestHostedVoicesUseGrantedPlatformAndRejectPrivateVoices(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	var calls atomic.Int64
	var mode atomic.Value
	mode.Store("public")
	var origin string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path == "/preview" {
			if request.Header.Get("xi-api-key") != "" || request.Header.Get("Authorization") != "" {
				t.Error("preview leaked credentials")
			}
			writer.Header().Set("Content-Type", "audio/mpeg")
			if mode.Load() == "preview-revoke" {
				if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-voices").Update("state", hostedGrantRevoked).Error; err != nil {
					t.Error(err)
				}
			}
			fmt.Fprint(writer, "controlled preview")
			return
		}
		if request.Header.Get("xi-api-key") != "hosted-voice-secret" || request.URL.Query().Get("voice_type") != "default" || request.URL.Query().Get("category") != "premade" {
			t.Errorf("incorrect hosted voice request: path=%s query=%s", request.URL.Path, request.URL.RawQuery)
		}
		category := "premade"
		if mode.Load() == "private" {
			category = "cloned"
		}
		if mode.Load() == "revoke" {
			if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-voices").Update("state", hostedGrantRevoked).Error; err != nil {
				t.Error(err)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"voices":[{"voice_id":"private-native-voice","name":"Public voice","category":%q,"labels":{},"high_quality_base_model_ids":[],"verified_languages":[],"preview_url":%q}],"has_more":true,"total_count":1,"next_page_token":"private-native-page"}`, category, origin+"/preview")
	}))
	origin = upstream.URL
	t.Cleanup(upstream.Close)
	server, service := newHostedVoiceHTTPFixture(t, database, "elevenlabs", "eleven_v3", map[string]string{CatalogCredentialAPIKey: "hosted-voice-secret"})
	definition := service.providers.definitions[providerID("elevenlabs")]
	transport := definition.transports["voices"]
	transport.endpointURLOverride = upstream.URL + "/voices"
	transport.artifactOrigins = []string{upstream.URL}
	definition.transports["voices"] = transport
	service.providers.definitions[providerID("elevenlabs")] = definition
	imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
	service.voiceProviders = map[string]MediaVoiceProvider{"elevenlabs": &elevenLabsVoiceProvider{provider: definition, transport: "voices", tenants: imageAdapter.tenants, store: service.store, client: upstream.Client(), revision: service.catalog.Revision()}}
	admission := service.hostedAdmission
	service.hostedAdmission = nil
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=elevenlabs", http.StatusForbidden)
	service.hostedAdmission = admission
	var grant managedHostedGrantRecord
	if err := database.database.First(&grant, "id = ?", "grant-voices").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&grant).Update("offerings", []byte(`[{"operations":["audio_alignment"]}]`)).Error; err != nil {
		t.Fatal(err)
	}
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=elevenlabs", http.StatusForbidden)
	if err := database.database.Model(&grant).Update("offerings", []byte(`[{"model":"eleven_v3","operations":["speech_generation"]}]`)).Error; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("inactive or unrelated grant reached voice provider")
	}
	page := hostedVoiceHTTP(t, server, "/model/v1/voices?provider=elevenlabs", http.StatusOK)
	voices := page["voices"].([]any)
	if len(voices) != 1 {
		t.Fatalf("voices=%v", page)
	}
	voice := voices[0].(map[string]any)
	id := voice["voice_id"].(string)
	cursor := page["next_cursor"].(string)
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=elevenlabs&cursor="+url.QueryEscape(cursor), http.StatusOK)
	hostedVoiceHTTP(t, server, "/model/v1/voices/"+id, http.StatusOK)
	preview := voice["preview"].(string)
	request, _ := http.NewRequest(http.MethodGet, server.URL+preview, nil)
	request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "controlled preview" {
		t.Fatalf("preview=%s status=%d error=%v", body, response.StatusCode, err)
	}
	before := calls.Load()
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=elevenlabs&voice_type=personal", http.StatusBadRequest)
	if calls.Load() != before {
		t.Fatal("private filter reached platform")
	}
	mode.Store("private")
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=elevenlabs", http.StatusBadGateway)
	mode.Store("public")
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-voices").Update("revision", 2).Error; err != nil {
		t.Fatal(err)
	}
	before = calls.Load()
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=elevenlabs&cursor="+url.QueryEscape(cursor), http.StatusBadRequest)
	if calls.Load() != before {
		t.Fatal("obsolete grant cursor reached platform")
	}
	mode.Store("preview-revoke")
	hostedVoiceHTTP(t, server, preview, http.StatusNotFound)
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-voices").Update("state", hostedGrantActive).Error; err != nil {
		t.Fatal(err)
	}
	mode.Store("revoke")
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=elevenlabs", http.StatusForbidden)
	before = calls.Load()
	hostedVoiceHTTP(t, server, "/model/v1/voices?provider=elevenlabs", http.StatusForbidden)
	hostedVoiceHTTP(t, server, "/model/v1/voices/"+id, http.StatusNotFound)
	hostedVoiceHTTP(t, server, preview, http.StatusNotFound)
	if calls.Load() != before {
		t.Fatal("revoked voice read reached platform")
	}
	for _, table := range []string{"managed_journal_request_records", "managed_journal_attempt_records"} {
		var count int64
		if err := database.database.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("metadata created billing evidence: table=%s count=%d error=%v", table, count, err)
		}
	}
}

func newHostedVoiceHTTPFixture(t *testing.T, database *gormManagedTenantDatabase, provider, model string, fields map[string]string) (*httptest.Server, *mediaOperationService) {
	t.Helper()
	server, service := newHostedMediaAdmissionHTTPServer(t, database)
	imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
	definition := service.providers.definitions[providerID(provider)]
	encodedFields := map[string]string{}
	for name, value := range fields {
		if definition.fields[name].Kind == CatalogProviderFieldKindCredential {
			var err error
			value, err = imageAdapter.tenants.providerKeyCipher.encryptConnection(rand.Reader, platformCredentialReference("platform-voices", 1), provider, name, value)
			if err != nil {
				t.Fatal(err)
			}
		}
		encodedFields[name] = value
	}
	encoded, _ := json.Marshal(encodedFields)
	offerings, _ := json.Marshal([]hostedGrantOffering{{Model: model, Operations: []string{ModelOperationSpeechGeneration}}})
	now := time.Now().UTC()
	for _, record := range []any{
		&managedPlatformConnectionRecord{ID: "platform-voices", Provider: provider, Name: "Voices", Version: 1, CreatedAt: now, UpdatedAt: now},
		&managedPlatformCredentialRecord{ConnectionID: "platform-voices", Version: 1, Fields: encoded, QualifiedAt: now, CreatedAt: now},
		&managedHostedGrantRecord{ID: "grant-voices", BillingAccountID: "billing-journal", TenantID: "managed-first", PlatformConnectionID: "platform-voices", Provider: provider, CatalogRevision: service.catalog.Revision(), Offerings: offerings, State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
		&managedHostedTenantAssignmentRecord{TenantID: "managed-first", ProviderID: provider, GrantID: "grant-voices", CreatedAt: now},
		&managedProviderProfileRecord{TenantID: "managed-first", ProviderID: provider, CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	service.voiceCursorCipher = internalManagedProviderKeyCipher()
	service.voiceCursorRandom = rand.Reader
	router := server.Config.Handler.(*gin.Engine)
	auth := newTenantAuthenticator(imageAdapter.tenants)
	router.GET(llmproxycontract.MediaVoicesPath, mediaTenantAuthenticatedHandler(auth, zap.NewNop().Sugar(), service.mediaVoiceCollectionHandler()))
	router.GET(llmproxycontract.MediaVoicesPath+"/:voice_id", mediaTenantAuthenticatedHandler(auth, zap.NewNop().Sugar(), service.mediaVoiceHandler()))
	router.GET(llmproxycontract.MediaVoicesPath+"/:voice_id/previews/:preview", mediaTenantAuthenticatedHandler(auth, zap.NewNop().Sugar(), service.mediaVoicePreviewHandler()))
	return server, service
}

func hostedVoiceHTTP(t *testing.T, server *httptest.Server, path string, want int) map[string]any {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
	request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != want {
		t.Fatalf("voice read %s status=%d want=%d body=%s error=%v", path, response.StatusCode, want, body, err)
	}
	for _, private := range []string{"hosted-voice-secret", "private-native", "platform-voices"} {
		if strings.Contains(string(body), private) {
			t.Fatalf("voice read leaked %s", private)
		}
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("voice response may be cached")
	}
	contractRequest := request.Clone(request.Context())
	if strings.Contains(request.URL.Path, "/voices/voi_") {
		contractRequest.URL.Path = "/model/v1/voices/{voice_id}"
		if strings.Contains(request.URL.Path, "/previews/") {
			contractRequest.URL.Path += "/previews/{preview}"
		}
	}
	validateHostedIdentityResponse(t, contractRequest, response, body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
