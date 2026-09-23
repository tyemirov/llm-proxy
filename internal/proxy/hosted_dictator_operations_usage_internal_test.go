package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type hostedDictatorOperationsUpstream struct {
	dictator.UnimplementedTranscriptionServiceServer
	dictator.UnimplementedAlignmentServiceServer
	dictator.UnimplementedSubtitleServiceServer
	dictator.UnimplementedVoiceServiceServer
	dictator.UnimplementedArtifactServiceServer
	mode        string
	submissions atomic.Int64
	polls       atomic.Int64
	downloads   atomic.Int64
}

func (upstream *hostedDictatorOperationsUpstream) state() int32 {
	if upstream.mode == "running" && upstream.polls.Add(1) == 1 {
		return 2
	}
	switch upstream.mode {
	case "failed":
		return 4
	case "cancelled":
		return 5
	}
	return 3
}
func (*hostedDictatorOperationsUpstream) UploadArtifact(stream grpc.ClientStreamingServer[dictator.UploadArtifactChunk, dictator.UploadArtifactResponse]) error {
	var data []byte
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		data = append(data, chunk.GetContent()...)
	}
	return stream.SendAndClose(&dictator.UploadArtifactResponse{Artifact: &dictator.ArtifactRef{ArtifactId: "private-input", SizeBytes: int64(len(data)), Sha256: mediaSHA256Hex(data), AudioMetadata: &dictator.AudioMetadata{DurationSeconds: 123}}})
}
func (upstream *hostedDictatorOperationsUpstream) DownloadArtifact(request *dictator.DownloadArtifactRequest, stream grpc.ServerStreamingServer[dictator.DownloadArtifactChunk]) error {
	upstream.downloads.Add(1)
	if upstream.mode == "artifact_loss" {
		return fmt.Errorf("controlled artifact loss")
	}
	data := []byte("1\n00:00:00,000 --> 00:00:00,200\nA\n")
	return stream.Send(&dictator.DownloadArtifactChunk{Artifact: &dictator.ArtifactRef{ArtifactId: request.ArtifactId, SizeBytes: int64(len(data)), Sha256: mediaSHA256Hex(data), MediaType: "application/x-subrip"}, Content: data, Eof: true})
}

func (upstream *hostedDictatorOperationsUpstream) SubmitTranscribeJob(_ context.Context, _ *dictator.TranscribeRequest) (*dictator.SubmitTranscribeJobResponse, error) {
	upstream.submissions.Add(1)
	return &dictator.SubmitTranscribeJobResponse{JobId: "private-job", State: dictator.TranscriptionJobState(1)}, nil
}
func (upstream *hostedDictatorOperationsUpstream) GetTranscribeJob(_ context.Context, request *dictator.GetTranscribeJobRequest) (*dictator.GetTranscribeJobResponse, error) {

	return &dictator.GetTranscribeJobResponse{JobId: request.JobId, State: dictator.TranscriptionJobState(upstream.state()), Text: "Private transcript", LanguageCode: "en", SourceArtifactId: "private-input", Words: []*dictator.WordSegment{{Content: "Private", EndSeconds: 789}}}, nil
}

func (upstream *hostedDictatorOperationsUpstream) SubmitDiarizeAudioJob(_ context.Context, _ *dictator.DiarizeAudioRequest) (*dictator.SubmitDiarizeAudioJobResponse, error) {
	upstream.submissions.Add(1)
	return &dictator.SubmitDiarizeAudioJobResponse{JobId: "private-job", State: dictator.DiarizationJobState(1)}, nil
}
func (upstream *hostedDictatorOperationsUpstream) GetDiarizeAudioJob(_ context.Context, request *dictator.GetDiarizeAudioJobRequest) (*dictator.GetDiarizeAudioJobResponse, error) {
	document, _ := structpb.NewStruct(map[string]any{})
	if upstream.mode == "invalid_result" {
		document = nil
	}
	return &dictator.GetDiarizeAudioJobResponse{JobId: request.JobId, State: dictator.DiarizationJobState(upstream.state()), Diarization: document, SourceArtifactId: "private-input"}, nil
}

func (upstream *hostedDictatorOperationsUpstream) SubmitAlignTranscriptJob(_ context.Context, _ *dictator.AlignTranscriptRequest) (*dictator.SubmitAlignTranscriptJobResponse, error) {
	upstream.submissions.Add(1)
	return &dictator.SubmitAlignTranscriptJobResponse{JobId: "private-job", State: dictator.AlignmentJobState(1)}, nil
}
func (upstream *hostedDictatorOperationsUpstream) GetAlignTranscriptJob(_ context.Context, request *dictator.GetAlignTranscriptJobRequest) (*dictator.GetAlignTranscriptJobResponse, error) {

	return &dictator.GetAlignTranscriptJobResponse{JobId: request.JobId, State: dictator.AlignmentJobState(upstream.state()), LanguageCode: "en", SrtArtifactId: "private-srt", Words: []*dictator.WordSegment{{Content: "Private", EndSeconds: 789}}}, nil
}

func (upstream *hostedDictatorOperationsUpstream) SubmitRenderSubtitlesJob(_ context.Context, _ *dictator.RenderSubtitlesRequest) (*dictator.SubmitRenderSubtitlesJobResponse, error) {
	upstream.submissions.Add(1)
	return &dictator.SubmitRenderSubtitlesJobResponse{JobId: "private-job", State: dictator.SubtitleJobState(1)}, nil
}
func (upstream *hostedDictatorOperationsUpstream) GetRenderSubtitlesJob(_ context.Context, request *dictator.GetRenderSubtitlesJobRequest) (*dictator.GetRenderSubtitlesJobResponse, error) {

	return &dictator.GetRenderSubtitlesJobResponse{JobId: request.JobId, State: dictator.SubtitleJobState(upstream.state()), LanguageCode: "en", SrtArtifactId: "private-srt"}, nil
}

func (upstream *hostedDictatorOperationsUpstream) SubmitExtractReferenceSampleJob(_ context.Context, _ *dictator.ExtractReferenceSampleRequest) (*dictator.SubmitExtractReferenceSampleJobResponse, error) {
	upstream.submissions.Add(1)
	return &dictator.SubmitExtractReferenceSampleJobResponse{JobId: "private-job", State: dictator.ExtractReferenceSampleJobState(1)}, nil
}
func (upstream *hostedDictatorOperationsUpstream) GetExtractReferenceSampleJob(_ context.Context, request *dictator.GetExtractReferenceSampleJobRequest) (*dictator.GetExtractReferenceSampleJobResponse, error) {
	artifact := &dictator.ArtifactRef{ArtifactId: "private-sample", AudioMetadata: &dictator.AudioMetadata{DurationSeconds: 1.125}}
	if upstream.mode == "absent" {
		artifact.AudioMetadata = nil
	}
	if upstream.mode == "invalid_result" {
		artifact.ArtifactId = ""
	}
	return &dictator.GetExtractReferenceSampleJobResponse{JobId: request.JobId, State: dictator.ExtractReferenceSampleJobState(upstream.state()), SampleArtifact: artifact}, nil
}

func TestHostedDictatorInputOperationUsage(t *testing.T) {
	scenarios := []struct{ capability, operation, input, controls string }{
		{"audio.transcribe", ModelOperationAudioTranscription, "", `{"language":"en"}`},
		{"audio.diarize", ModelOperationAudioDiarization, "", `{"language":"en","model_size":"base","utterance_gap_seconds":0.5}`},
		{"audio.align", ModelOperationAudioAlignment, `,"transcript":"Private transcript"`, `{"language":"en","remove_punctuation":true}`},
		{"subtitles.create", ModelOperationSubtitleCreation, `,"transcript":"Private transcript"`, `{"language":"en","granularity":"sentence","group_size":2}`},
		{"audio.voice.extract", ModelOperationVoiceExtraction, `,"transcript":"Private transcript","display_name":"Sample","language":"en"`, `{"model_size":"base","duration_seconds":0.5}`},
	}
	for _, offering := range internalCanonicalProviderCatalog().ModelCatalog().Offerings {
		if offering.Provider != ProviderNameDictator {
			continue
		}
		model := offering.Model
		for _, scenario := range scenarios {
			if !slices.Contains(offering.Operations, scenario.operation) {
				continue
			}
			for _, mode := range []string{"success", "running", "failed", "cancelled", "observation_failure", "delivery_failure", "artifact_loss", "invalid_result", "absent"} {
				if model != ModelNameDictatorWhisperBase && mode != "success" {
					continue
				}
				if mode == "artifact_loss" && scenario.capability != "audio.align" && scenario.capability != "subtitles.create" {
					continue
				}
				if mode == "invalid_result" && scenario.capability != "audio.diarize" && scenario.capability != "audio.voice.extract" {
					continue
				}
				if mode == "absent" && scenario.capability != "audio.voice.extract" {
					continue
				}
				t.Run(model+"/"+scenario.capability+"/"+mode, func(t *testing.T) {
					database, _, read := newJournalTransactionFixture(t)
					upstream := &hostedDictatorOperationsUpstream{mode: mode}
					listener, err := net.Listen("tcp", "127.0.0.1:0")
					if err != nil {
						t.Fatal(err)
					}
					native := grpc.NewServer()
					dictator.RegisterTranscriptionServiceServer(native, upstream)
					dictator.RegisterAlignmentServiceServer(native, upstream)
					dictator.RegisterSubtitleServiceServer(native, upstream)
					dictator.RegisterVoiceServiceServer(native, upstream)
					dictator.RegisterArtifactServiceServer(native, upstream)
					go func() {
						if err := native.Serve(listener); err != nil {
							t.Error(err)
						}
					}()
					t.Cleanup(native.Stop)
					server, service := newHostedVoiceHTTPFixture(t, database, ProviderNameDictator, model, map[string]string{dictatorAddressField: listener.Addr().String(), dictatorTokenField: "hosted-input-secret", dictatorTLSField: "false"})
					offerings, _ := json.Marshal([]hostedGrantOffering{{Model: model, Operations: []string{scenario.operation}}})
					if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-voices").Update("offerings", offerings).Error; err != nil {
						t.Fatal(err)
					}
					if err := database.database.Create(&managedHostedGrantRevisionRecord{GrantID: "grant-voices", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Input usage acceptance", CreatedAt: service.store.now()}).Error; err != nil {
						t.Fatal(err)
					}
					imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
					definition := service.providers.definitions[ProviderNameDictator]
					offering, err := service.catalog.ResolveOffering(ProviderNameDictator, model)
					if err != nil {
						t.Fatal(err)
					}
					service.adapters[mediaOperationAdapterKey(scenario.capability, ProviderNameDictator, model)] = &accountDictatorAdapter{provider: ProviderNameDictator, model: model, transport: definition.transports[offering.Transport], tenants: imageAdapter.tenants, store: service.store, assets: service.assets}
					asset, err := service.assets.upload(tenant{identifier: tenantID("managed-first")}, "audio/wav", strings.NewReader("controlled audio"))
					if err != nil {
						t.Fatal(err)
					}
					modelSize, _ := dictatorWhisperSizeForModel(model)
					controls := strings.Replace(scenario.controls, `"model_size":"base"`, `"model_size":"`+modelSize+`"`, 1)
					intent := fmt.Sprintf(`{"capability":%q,"provider":"dictator","model":%q,"input":{"audio_asset_id":%q%s},"controls":%s}`, scenario.capability, model, asset.AssetID, scenario.input, controls)
					id := hostedSpeechHTTP(t, server, "input-usage", intent, http.StatusAccepted)["operation_id"].(string)
					table := ""
					switch mode {
					case "observation_failure":
						table = "managed_journal_observation_records"
					case "delivery_failure":
						table = "managed_journal_delivery_records"
					}
					if table != "" {
						if err := database.database.Exec("CREATE TRIGGER reject_input_usage BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT, 'controlled input usage failure'); END").Error; err != nil {
							t.Fatal(err)
						}
					}
					service.runOperation("input-worker", id)
					result := hostedMediaWorkerStatus(t, server, id)
					wantState := MediaOperationStateSucceeded
					switch mode {
					case "failed":
						wantState = MediaOperationStateFailed
					case "cancelled":
						wantState = MediaOperationStateCancelled
					case "artifact_loss", "invalid_result", "observation_failure", "delivery_failure":
						wantState = MediaOperationStateUncertain
					}
					if result["state"] != wantState {
						t.Fatalf("input operation=%v", result)
					}
					if table != "" {
						if upstream.downloads.Load() != 0 || result["error"].(map[string]any)["code"] != llmproxycontract.ErrorCodeUsageJournalUnavailable {
							t.Fatalf("usage failure=%v downloads=%d", result, upstream.downloads.Load())
						}
						if err := database.database.Exec("DROP TRIGGER reject_input_usage").Error; err != nil {
							t.Fatal(err)
						}
					}
					hostedSpeechHTTP(t, server, "input-usage", intent, http.StatusOK)
					service.runOperation("duplicate-worker", id)
					if upstream.submissions.Load() != 1 {
						t.Fatal("input replay repeated provider work")
					}
					pending, err := database.pendingJournalDeliveries(t.Context(), 100)
					if err != nil {
						t.Fatal(err)
					}
					if table != "" {
						if len(pending) != 0 {
							t.Fatal("partial input usage write")
						}
						return
					}
					if len(pending) != 1 {
						t.Fatalf("input observations=%d", len(pending))
					}
					var quantities []journalQuantity
					if err := json.Unmarshal(pending[0].Quantities, &quantities); err != nil {
						t.Fatal(err)
					}
					if len(quantities) == 0 || quantities[0].Dimension != "input_audio_seconds" || quantities[0].Unit != "second" || quantities[0].Value != "" || quantities[0].UnknownReason != journalQuantityUnsupported {
						t.Fatalf("input quantities=%s", pending[0].Quantities)
					}
					if scenario.capability == "audio.voice.extract" {
						value, reason := "1.125", journalUnknownReason("")
						if mode == "absent" {
							value, reason = "", journalQuantityNotReported
						}
						if len(quantities) != 2 || quantities[1].Dimension != "output_audio_seconds" || quantities[1].Value != value || quantities[1].UnknownReason != reason {
							t.Fatalf("sample quantities=%s", pending[0].Quantities)
						}
					} else if len(quantities) != 1 {
						t.Fatalf("invented input quantities=%s", pending[0].Quantities)
					}
					if scenario.capability == "audio.voice.extract" && mode != "absent" && string(pending[0].SourceFields) != `[{"path":"sample_artifact.audio_metadata.duration_seconds","value":"1.125"}]` {
						t.Fatalf("sample source=%s", pending[0].SourceFields)
					}
					entry := read("")["requests"].([]any)[0].(map[string]any)
					if entry["usage_state"] != string(journalUsageUnknown) {
						t.Fatalf("input journal=%v", entry)
					}
					if strings.Contains(string(pending[0].SourceFields), "123") || strings.Contains(string(pending[0].SourceFields), "789") || strings.Contains(string(pending[0].SourceFields), "Private") {
						t.Fatalf("unmeasured or private evidence=%s", pending[0].SourceFields)
					}
				})
			}
		}
	}
}
