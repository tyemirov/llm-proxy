package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHostedSpeechUsageCoversCatalogOfferings(t *testing.T) {
	registry := internalManagementProviderRegistry()
	count := 0
	for _, offering := range internalCanonicalProviderCatalog().ModelCatalog().Offerings {
		provider := registry.definitions[providerID(offering.Provider)]
		codec := provider.transports[offering.Transport].responseCodec
		if codec != CatalogProtocolElevenLabsSpeech && codec != CatalogProtocolElevenLabsConversion {
			continue
		}
		count++
		t.Run(offering.Provider+"/"+offering.Model, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				var payload struct {
					Model string `json:"model_id"`
				}
				if codec == CatalogProtocolElevenLabsConversion {
					if err := request.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						writer.WriteHeader(http.StatusBadRequest)
						return
					}
					defer request.MultipartForm.RemoveAll()
					payload.Model = request.FormValue("model_id")
					file, _, err := request.FormFile("audio")
					if err != nil {
						t.Error(err)
						writer.WriteHeader(http.StatusBadRequest)
						return
					}
					data, err := io.ReadAll(file)
					file.Close()
					if err != nil || string(data) != "1234" || request.FormValue("file_format") != "pcm_s16le_16" {
						t.Errorf("conversion input=%q error=%v", data, err)
					}
				} else if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload.Model != offering.ProviderModel {
					t.Errorf("speech model=%q", payload.Model)
				}
				writer.Header().Set("request-id", "catalog-speech")
				writer.Header().Set("character-cost", "12.5")
				if strings.HasSuffix(request.URL.Path, "/with-timestamps") {
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(writer, `{"audio_base64":%q}`, base64.StdEncoding.EncodeToString([]byte("controlled audio")))
				} else {
					writer.Header().Set("Content-Type", "audio/mpeg")
					fmt.Fprint(writer, "controlled audio")
				}
			}))
			t.Cleanup(upstream.Close)
			server, service, intent := newHostedSpeechUsageFixture(t, database, offering.Provider, offering.Model, upstream.URL)
			modes := []string{"plain", "timed"}
			if codec == CatalogProtocolElevenLabsConversion {
				modes = modes[:1]
			}
			for _, mode := range modes {
				body := intent
				if mode == "timed" {
					body = strings.Replace(body, `"timestamps":false`, `"timestamps":true`, 1)
				}
				accepted := hostedSpeechHTTP(t, server, mode, body, http.StatusAccepted)
				id := accepted["operation_id"].(string)
				service.runOperation("catalog-speech-worker", id)
				if state := hostedMediaWorkerStatus(t, server, id); state["state"] != MediaOperationStateSucceeded {
					t.Fatalf("speech output=%v", state)
				}
				hostedSpeechHTTP(t, server, mode, body, http.StatusOK)
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil || len(pending) != len(modes) || calls.Load() != int64(len(modes)) {
				t.Fatalf("speech evidence=%v calls=%d error=%v", pending, calls.Load(), err)
			}
			for _, observation := range pending {
				if !strings.Contains(string(observation.Quantities), `"value":"12.5"`) || observation.Completeness != journalUsageComplete {
					t.Fatalf("speech evidence=%+v", observation)
				}
			}
			for _, value := range read("")["requests"].([]any) {
				entry := value.(map[string]any)
				if entry["usage_state"] != string(journalUsageComplete) {
					t.Fatalf("speech journal=%v", entry)
				}
			}
		})
	}
	if count == 0 {
		t.Fatal("catalog has no speech protocol offerings")
	}
}

func TestHostedSpeechUsagePreservesReportedCost(t *testing.T) {
	for _, route := range []struct{ model, codec string }{{"eleven_v3", CatalogProtocolElevenLabsSpeech}, {"eleven_english_sts_v2", CatalogProtocolElevenLabsConversion}, {"eleven_multilingual_sts_v2", CatalogProtocolElevenLabsConversion}} {
		for _, scenario := range []struct {
			name        string
			values      []string
			status      int
			contentType string
			value       string
			reason      journalUnknownReason
			state       string
		}{
			{"exact", []string{"9007199254740993.125"}, 200, "audio/mpeg", "9007199254740993.125", "", MediaOperationStateSucceeded},
			{"zero", []string{"0"}, 200, "audio/mpeg", "0", "", MediaOperationStateSucceeded},
			{"missing", nil, 200, "audio/mpeg", "", journalQuantityNotReported, MediaOperationStateSucceeded},
			{"negative", []string{"-1"}, 200, "audio/mpeg", "", journalQuantityInvalid, MediaOperationStateSucceeded},
			{"invalid", []string{"private header"}, 200, "audio/mpeg", "", journalQuantityInvalid, MediaOperationStateSucceeded},
			{"duplicate", []string{"1", "2"}, 200, "audio/mpeg", "", journalQuantityInvalid, MediaOperationStateSucceeded},
			{"provider_failure", []string{"2.5"}, 400, "application/json", "2.5", "", MediaOperationStateFailed},
			{"provider_unavailable", []string{"2.5"}, 503, "application/json", "2.5", "", MediaOperationStateUncertain},
			{"lost_result", []string{"3"}, 200, "application/json", "3", "", MediaOperationStateUncertain},
		} {
			t.Run(route.model+"/"+scenario.name, func(t *testing.T) {
				database, _, read := newJournalTransactionFixture(t)
				var posts atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					posts.Add(1)
					if request.Method != http.MethodPost || request.Header.Get("xi-api-key") != "hosted-voice-secret" {
						t.Error("speech lost pinned authority")
					}
					for _, value := range scenario.values {
						writer.Header().Add("character-cost", value)
					}
					writer.Header().Set("request-id", "private-speech-request")
					writer.Header().Set("Content-Type", scenario.contentType)
					writer.WriteHeader(scenario.status)
					fmt.Fprint(writer, "controlled audio")
				}))
				t.Cleanup(upstream.Close)
				server, service, intent := newHostedSpeechUsageFixture(t, database, "elevenlabs", route.model, upstream.URL)
				accepted := hostedSpeechHTTP(t, server, "speech-cost", intent, http.StatusAccepted)
				id := accepted["operation_id"].(string)
				service.runOperation("speech-cost-worker", id)
				if current := hostedMediaWorkerStatus(t, server, id); current["state"] != scenario.state {
					t.Fatalf("speech result=%v", current)
				}
				entry := read("")["requests"].([]any)[0].(map[string]any)
				usage := journalUsageComplete
				if scenario.reason != "" {
					usage = journalUsageUnknown
				}
				if entry["usage_state"] != string(usage) {
					t.Fatalf("speech completeness=%v", entry)
				}
				var attempt managedJournalAttemptRecord
				if err := database.database.Where("request_id = ?", entry["id"]).First(&attempt).Error; err != nil {
					t.Fatal(err)
				}
				var observation managedJournalObservationRecord
				if err := database.database.Where("attempt_id = ?", attempt.ID).First(&observation).Error; err != nil {
					t.Fatal(err)
				}
				var quantities []journalQuantity
				if err := json.Unmarshal(observation.Quantities, &quantities); err != nil {
					t.Fatal(err)
				}
				if len(quantities) != 1 || quantities[0].Dimension != "character_cost" || quantities[0].Unit != "provider_unit" || quantities[0].Value != scenario.value || quantities[0].UnknownReason != scenario.reason {
					t.Fatalf("speech usage=%s", observation.Quantities)
				}
				if observation.AdapterRevision != route.codec+":1" || attempt.ProviderRequestID != "private-speech-request" {
					t.Fatalf("speech evidence=%+v", observation)
				}
				var sources []journalSourceField
				if err := json.Unmarshal(observation.SourceFields, &sources); err != nil {
					t.Fatal(err)
				}
				if scenario.value != "" {
					if len(sources) != 1 || sources[0].Path != "headers.character_cost" || sources[0].Value != scenario.value {
						t.Fatalf("speech sources=%v", sources)
					}
				} else if len(sources) != 0 {
					t.Fatalf("invalid header retained: %v", sources)
				}
				if strings.Contains(string(observation.SourceFields), "controlled audio") || strings.Contains(string(observation.SourceFields), "private") {
					t.Fatal("private content in financial evidence")
				}
				replayed := hostedSpeechHTTP(t, server, "speech-cost", intent, http.StatusOK)
				if replayed["operation_id"] != id || posts.Load() != 1 {
					t.Fatal("speech replay repeated paid work")
				}
				service.runOperation("duplicate-worker", id)
				pending, err := database.pendingJournalDeliveries(t.Context(), 100)
				if err != nil || len(pending) != 1 || posts.Load() != 1 {
					t.Fatalf("speech accounting deliveries=%v posts=%d error=%v", pending, posts.Load(), err)
				}
			})
		}
	}
}

func TestHostedSpeechUsageWriteFailurePreventsPublication(t *testing.T) {
	for _, model := range []string{"eleven_v3", "eleven_multilingual_sts_v2"} {
		for _, table := range []string{"managed_journal_observation_records", "managed_journal_delivery_records"} {
			t.Run(model+"/"+table, func(t *testing.T) {
				database, _, read := newJournalTransactionFixture(t)
				var posts atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					posts.Add(1)
					writer.Header().Set("request-id", "speech-write-failure")
					writer.Header().Set("character-cost", "4")
					writer.Header().Set("Content-Type", "audio/mpeg")
					fmt.Fprint(writer, "controlled audio")
				}))
				t.Cleanup(upstream.Close)
				server, service, intent := newHostedSpeechUsageFixture(t, database, "elevenlabs", model, upstream.URL)
				core, logs := observer.New(zap.InfoLevel)
				service.logger = zap.New(core).Sugar()
				accepted := hostedSpeechHTTP(t, server, "speech-write-failure", intent, http.StatusAccepted)
				id := accepted["operation_id"].(string)
				if err := database.database.Exec("CREATE TRIGGER reject_speech_usage BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT, 'controlled speech evidence failure'); END").Error; err != nil {
					t.Fatal(err)
				}
				service.runOperation("speech-worker", id)
				result := hostedMediaWorkerStatus(t, server, id)
				if result["state"] != MediaOperationStateUncertain || len(result["outputs"].([]any)) != 0 || result["error"].(map[string]any)["code"] != llmproxycontract.ErrorCodeUsageJournalUnavailable {
					t.Fatalf("failed speech observation result=%v", result)
				}
				reports := logs.FilterMessage("media operation persistence failed").All()
				if len(reports) != 1 || reports[0].ContextMap()["phase"] != "observe_usage" || reports[0].ContextMap()["operation_id"] != id {
					t.Fatalf("speech reports=%v", reports)
				}
				var count int64
				if err := database.database.Model(&managedJournalObservationRecord{}).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("partial speech evidence=%d error=%v", count, err)
				}
				if err := database.database.Exec("DROP TRIGGER reject_speech_usage").Error; err != nil {
					t.Fatal(err)
				}
				service.runOperation("replacement-worker", id)
				hostedSpeechHTTP(t, server, "speech-write-failure", intent, http.StatusOK)
				entry := read("")["requests"].([]any)[0].(map[string]any)
				if posts.Load() != 1 || entry["usage_state"] != string(journalUsageUnknown) || entry["state"] != string(journalRequestUncertain) {
					t.Fatalf("speech retry changed financial outcome: %v posts=%d", entry, posts.Load())
				}
			})
		}
	}
}

func newHostedSpeechUsageFixture(t *testing.T, database *gormManagedTenantDatabase, provider, model, endpoint string) (*httptest.Server, *mediaOperationService, string) {
	t.Helper()
	server, service := newHostedVoiceHTTPFixture(t, database, provider, model, map[string]string{CatalogCredentialAPIKey: "hosted-voice-secret"})
	if err := database.database.Create(&managedHostedGrantRevisionRecord{GrantID: "grant-voices", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Speech acceptance", CreatedAt: service.store.now()}).Error; err != nil {
		t.Fatal(err)
	}
	imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
	definition := service.providers.definitions[providerID(provider)]
	offering, err := service.catalog.ResolveOffering(provider, model)
	if err != nil {
		t.Fatal(err)
	}
	transport := definition.transports[offering.Transport]
	transport.endpointURLOverride = endpoint
	definition.transports[offering.Transport] = transport
	authority := "platform-voices:v1:" + service.catalog.Revision()
	reference, _ := json.Marshal(elevenLabsVoiceReference{Authority: authority, VoiceID: "native-voice"})
	voice, err := persistMediaVoice(database.database, "managed-first", provider, MediaVoiceProviderRecord{Authority: authority, Provider: provider, Mode: MediaVoiceModePreset, DisplayName: "Speech", ProviderVoiceReference: string(reference)}, service.store.now())
	if err != nil {
		t.Fatal(err)
	}
	if offeringSupportsOperation(offering, ModelOperationSpeechConversion) {
		offerings, _ := json.Marshal([]hostedGrantOffering{{Model: model, Operations: []string{ModelOperationSpeechConversion}}})
		if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-voices").Update("offerings", offerings).Error; err != nil {
			t.Fatal(err)
		}
		service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioSpeechConvert, provider, model)] = newProviderConversionAdapter(offering, definition, imageAdapter.tenants, service.store, service.assets, service.catalog.Revision())
		asset, err := service.assets.upload(tenant{identifier: tenantID("managed-first")}, "application/octet-stream", strings.NewReader("1234"))
		if err != nil {
			t.Fatal(err)
		}
		intent := fmt.Sprintf(`{"capability":"audio.speech.convert","provider":%q,"model":%q,"input":{"audio_asset_id":%q,"voice_id":%q},"controls":{"output_format":"mp3_44100_128","input_format":"pcm_s16le_16"}}`, provider, model, asset.AssetID, voice.VoiceID)
		return server, service, intent
	}
	service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioSpeechGenerate, provider, model)] = newProviderGenerationAdapter(offering, definition, imageAdapter.tenants, service.store, service.assets, service.catalog.Revision())
	intent := fmt.Sprintf(`{"capability":"audio.speech.generate","provider":%q,"model":%q,"input":{"text":"Private speech text.","voice_id":%q},"controls":{"output_format":"mp3_44100_128","timestamps":false}}`, provider, model, voice.VoiceID)
	return server, service, intent
}
