package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"google.golang.org/grpc"
)

type hostedDictatorUsageUpstream struct {
	hostedSpeechUpstream
	duration     float64
	failDownload bool
	mode         string
	polls        atomic.Int64
	downloads    atomic.Int64
}

func (upstream *hostedDictatorUsageUpstream) GetSynthesizeSpeechJob(ctx context.Context, request *dictator.GetSynthesizeSpeechJobRequest) (*dictator.GetSynthesizeSpeechJobResponse, error) {
	response, err := upstream.hostedSpeechUpstream.GetSynthesizeSpeechJob(ctx, request)
	response.AudioDurationSeconds = upstream.duration
	if upstream.mode == "failed" {
		response.State = dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_FAILED
	}
	if upstream.mode == "running" && upstream.polls.Add(1) == 1 {
		response.State = dictator.SynthesisJobState_SYNTHESIS_JOB_STATE_RUNNING
		response.AudioDurationSeconds = 100
	}
	return response, err
}
func (upstream *hostedDictatorUsageUpstream) DownloadArtifact(request *dictator.DownloadArtifactRequest, stream grpc.ServerStreamingServer[dictator.DownloadArtifactChunk]) error {
	upstream.downloads.Add(1)
	if upstream.failDownload {
		return fmt.Errorf("controlled artifact loss")
	}
	return upstream.hostedSpeechUpstream.DownloadArtifact(request, stream)
}

func TestHostedDictatorUsagePrecedesArtifactTransfer(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		duration float64
		value    string
		reason   journalUnknownReason
	}{
		{"exact", 1.125, "1.125", ""},
		{"silero", 1.125, "1.125", ""},
		{"running", 1.125, "1.125", ""}, {"failed", 1.125, "1.125", ""}, {"cancelled", 1.125, "1.125", ""}, {"precision", 0.12345678901234567, "0.12345678901234566", ""},
		{"absent", 0, "", journalQuantityNotReported}, {"negative", -1, "", journalQuantityInvalid},
		{"nan", math.NaN(), "", journalQuantityInvalid}, {"infinity", math.Inf(1), "", journalQuantityInvalid},
		{"artifact_loss", 1.125, "1.125", ""}, {"observation_failure", 1.125, "", ""}, {"delivery_failure", 1.125, "", ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			model := ModelNameDictatorQwen3TTS
			if scenario.name == "silero" {
				model = "silero-ru"
			}
			engine, _ := dictatorSynthesisEngineForModel(model)
			upstream := &hostedDictatorUsageUpstream{mode: scenario.name, duration: scenario.duration, failDownload: scenario.name == "artifact_loss"}
			if scenario.name == "cancelled" {
				upstream.cancelled.Store(true)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			grpcServer := grpc.NewServer()
			dictator.RegisterVoiceServiceServer(grpcServer, upstream)
			dictator.RegisterArtifactServiceServer(grpcServer, upstream)
			go func() {
				if err := grpcServer.Serve(listener); err != nil {
					t.Error(err)
				}
			}()
			t.Cleanup(grpcServer.Stop)
			server, service := newHostedVoiceHTTPFixture(t, database, ProviderNameDictator, model, map[string]string{dictatorAddressField: listener.Addr().String(), dictatorTokenField: "hosted-duration-secret", dictatorTLSField: "false"})
			if err := database.database.Create(&managedHostedGrantRevisionRecord{GrantID: "grant-voices", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Duration acceptance", CreatedAt: service.store.now()}).Error; err != nil {
				t.Fatal(err)
			}
			imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
			provider := service.providers.definitions[ProviderNameDictator]
			offering, err := service.catalog.ResolveOffering(ProviderNameDictator, model)
			if err != nil {
				t.Fatal(err)
			}
			service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityAudioSpeechGenerate, ProviderNameDictator, model)] = &accountDictatorAdapter{provider: ProviderNameDictator, model: model, transport: provider.transports[offering.Transport], tenants: imageAdapter.tenants, store: service.store, assets: service.assets}
			authority := "platform-voices:" + mediaSHA256Hex([]byte(listener.Addr().String()+"\x00hosted-duration-secret\x00false"))
			reference, _ := json.Marshal(dictatorVoiceReference{Binding: authority, Engine: engine, Preset: "native-preset"})
			voice, err := persistMediaVoice(database.database, "managed-first", ProviderNameDictator, MediaVoiceProviderRecord{Authority: authority, Provider: ProviderNameDictator, Mode: MediaVoiceModePreset, DisplayName: "Duration", SampleRates: []int{24000}, DefaultSampleRate: 24000, ProviderVoiceReference: string(reference)}, service.store.now())
			if err != nil {
				t.Fatal(err)
			}
			intent := fmt.Sprintf(`{"capability":"audio.speech.generate","provider":"dictator","model":%q,"input":{"text":"A short message.","voice_id":%q},"controls":{"language":"en","text_format":"plain","sample_rate_hz":24000}}`, model, voice.VoiceID)
			id := hostedSpeechHTTP(t, server, "duration", intent, http.StatusAccepted)["operation_id"].(string)
			failureTable := ""
			switch scenario.name {
			case "observation_failure":
				failureTable = "managed_journal_observation_records"
			case "delivery_failure":
				failureTable = "managed_journal_delivery_records"
			}
			if failureTable != "" {
				if err := database.database.Exec("CREATE TRIGGER reject_duration BEFORE INSERT ON " + failureTable + " BEGIN SELECT RAISE(ABORT, 'controlled duration write failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			service.runOperation("duration-worker", id)
			result := hostedMediaWorkerStatus(t, server, id)
			state := MediaOperationStateSucceeded
			if scenario.name == "artifact_loss" || failureTable != "" {
				state = MediaOperationStateUncertain
			}
			if scenario.name == "failed" {
				state = MediaOperationStateFailed
			}
			if scenario.name == "cancelled" {
				state = MediaOperationStateCancelled
			}
			if result["state"] != state {
				t.Fatalf("duration result=%v", result)
			}
			if failureTable != "" {
				if upstream.downloads.Load() != 0 || result["error"].(map[string]any)["code"] != llmproxycontract.ErrorCodeUsageJournalUnavailable {
					t.Fatalf("duration write failure=%v downloads=%d", result, upstream.downloads.Load())
				}
				if err := database.database.Exec("DROP TRIGGER reject_duration").Error; err != nil {
					t.Fatal(err)
				}
			}
			hostedSpeechHTTP(t, server, "duration", intent, http.StatusOK)
			service.runOperation("duplicate-worker", id)
			if upstream.submissions.Load() != 1 {
				t.Fatal("duration replay repeated synthesis")
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil {
				t.Fatal(err)
			}
			if failureTable != "" {
				if len(pending) != 0 {
					t.Fatal("partial duration evidence")
				}
				return
			}
			if len(pending) != 1 {
				t.Fatalf("duration observations=%d", len(pending))
			}
			var quantities []journalQuantity
			if err := json.Unmarshal(pending[0].Quantities, &quantities); err != nil {
				t.Fatal(err)
			}
			if len(quantities) != 1 || quantities[0].Dimension != "output_audio_seconds" || quantities[0].Unit != "second" || quantities[0].Value != scenario.value || quantities[0].UnknownReason != scenario.reason {
				t.Fatalf("duration quantities=%s", pending[0].Quantities)
			}
			if scenario.value != "" && !strings.Contains(string(pending[0].SourceFields), `"path":"audio_duration_seconds","value":"`+scenario.value+`"`) {
				t.Fatalf("duration sources=%s", pending[0].SourceFields)
			}
			usage := journalUsageComplete
			if scenario.reason != "" {
				usage = journalUsageUnknown
			}
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if entry["usage_state"] != string(usage) {
				t.Fatalf("duration journal=%v", entry)
			}
		})
	}
}
