package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// Inject failures at the generated SDK transport boundary. The released SDK
// clients and the production mapping, binding, and artifact checks remain real.
func dictatorProtocolBoundary(t *testing.T, unary func(string, any, any) error, stream func(string) (grpc.ClientStream, error)) *dictatorGRPCProtocol {
	t.Helper()
	connection, err := grpc.NewClient("passthrough:///dictator-fixture", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(func(ctx context.Context, method string, request, response any, _ *grpc.ClientConn, _ grpc.UnaryInvoker, _ ...grpc.CallOption) error {
		if values, _ := metadata.FromOutgoingContext(ctx); strings.Join(values.Get("authorization"), "") != "Bearer token" {
			t.Fatal("missing native authorization")
		}
		return unary(method, request, response)
	}), grpc.WithStreamInterceptor(func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, method string, _ grpc.Streamer, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return stream(method)
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return &dictatorGRPCProtocol{provider: ProviderNameDictator, model: ModelNameDictatorWhisperBase, connection: connection, token: "token", binding: "account-a", maxAssetBytes: 32}
}

type dictatorBoundaryStream struct {
	grpc.ClientStream
	replies      []proto.Message
	receiveError error
	sendErrorAt  int
	sends        int
}

func (stream *dictatorBoundaryStream) SendMsg(any) error {
	stream.sends++
	if stream.sends == stream.sendErrorAt {
		return errors.New("transport send interrupted")
	}
	return nil
}
func (stream *dictatorBoundaryStream) CloseSend() error { return nil }
func (stream *dictatorBoundaryStream) RecvMsg(value any) error {
	if len(stream.replies) > 0 {
		proto.Merge(value.(proto.Message), stream.replies[0])
		stream.replies = stream.replies[1:]
		return nil
	}
	if stream.receiveError != nil {
		return stream.receiveError
	}
	return io.EOF
}

func TestDictatorArtifactIntegrityAtSDKBoundary(t *testing.T) {
	content := []byte("audio")
	metadata := &dictator.ArtifactRef{ArtifactId: "artifact", MediaType: "audio/wav", SizeBytes: int64(len(content)), Sha256: mediaSHA256Hex(content)}
	for _, scenario := range []struct {
		name      string
		stream    *dictatorBoundaryStream
		openError bool
	}{
		{name: "open", openError: true},
		{name: "receive", stream: &dictatorBoundaryStream{receiveError: errors.New("provider stream lost")}},
		{name: "offset", stream: &dictatorBoundaryStream{replies: []proto.Message{&dictator.DownloadArtifactChunk{Offset: 1}}}},
		{name: "size", stream: &dictatorBoundaryStream{replies: []proto.Message{&dictator.DownloadArtifactChunk{Content: make([]byte, 33)}}}},
		{name: "after eof", stream: &dictatorBoundaryStream{replies: []proto.Message{&dictator.DownloadArtifactChunk{Eof: true}, &dictator.DownloadArtifactChunk{}}}},
		{name: "metadata changes", stream: &dictatorBoundaryStream{replies: []proto.Message{&dictator.DownloadArtifactChunk{Artifact: metadata}, &dictator.DownloadArtifactChunk{Artifact: &dictator.ArtifactRef{ArtifactId: "other"}}}}},
		{name: "hash", stream: &dictatorBoundaryStream{replies: []proto.Message{&dictator.DownloadArtifactChunk{Artifact: metadata, Content: []byte("wrong"), Eof: true}}}},
		{name: "missing eof", stream: &dictatorBoundaryStream{replies: []proto.Message{&dictator.DownloadArtifactChunk{Artifact: metadata, Content: content}}}},
	} {
		t.Run("download "+scenario.name, func(t *testing.T) {
			protocol := dictatorProtocolBoundary(t, nil, func(string) (grpc.ClientStream, error) {
				if scenario.openError {
					return nil, errors.New("stream unavailable")
				}
				return scenario.stream, nil
			})
			output, err := protocol.download(context.Background(), "artifact")
			if err == nil || len(output.Data) != 0 {
				t.Fatalf("unverified artifact published: %+v error=%v", output, err)
			}
		})
	}
	for _, scenario := range []struct {
		name      string
		stream    *dictatorBoundaryStream
		openError bool
	}{
		{name: "open", openError: true},
		{name: "metadata send", stream: &dictatorBoundaryStream{sendErrorAt: 1}},
		{name: "data send", stream: &dictatorBoundaryStream{sendErrorAt: 2}},
		{name: "receive", stream: &dictatorBoundaryStream{receiveError: errors.New("upload acknowledgment lost")}},
		{name: "identity", stream: &dictatorBoundaryStream{replies: []proto.Message{&dictator.UploadArtifactResponse{}}}},
	} {
		t.Run("upload "+scenario.name, func(t *testing.T) {
			protocol := dictatorProtocolBoundary(t, nil, func(string) (grpc.ClientStream, error) {
				if scenario.openError {
					return nil, errors.New("stream unavailable")
				}
				return scenario.stream, nil
			})
			observation, err := protocol.Submit(context.Background(), dictatorProtocolRequest{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`), Assets: []dictatorProtocolAsset{{MIMEType: "audio/wav", Data: content}}})
			if err == nil || observation.Handle.JobID != "" {
				t.Fatalf("unverified upload submitted a job: %+v error=%v", observation, err)
			}
		})
	}
}

func TestDictatorJobAndVoiceBindingAtSDKBoundary(t *testing.T) {
	ctx := context.Background()
	handle := dictatorProviderHandle{Version: dictatorProviderHandleVersion, Binding: "account-a", JobID: "job"}
	fault := errors.New("private provider diagnostic")
	protocol := dictatorProtocolBoundary(t, func(string, any, any) error { return fault }, nil)
	request := dictatorProtocolRequest{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`)}
	if _, err := protocol.Submit(ctx, request); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	if _, err := protocol.Observe(ctx, request.Capability, handle); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	if _, err := protocol.Cancel(ctx, request.Capability, handle); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	if _, err := protocol.DiscoverVoices(ctx); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	for _, invalid := range []dictatorProtocolRequest{
		{Input: json.RawMessage(`{`), Controls: json.RawMessage(`{}`)},
		{Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{`)},
		{Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`), Capability: "unsupported"},
		{Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`), Capability: llmproxycontract.MediaCapabilityAudioSpeechGenerate},
	} {
		if _, err := protocol.Submit(ctx, invalid); err == nil {
			t.Fatal("accepted invalid persisted request")
		}
	}
	if _, err := protocol.Observe(ctx, "unsupported", handle); err == nil {
		t.Fatal("accepted unknown operation")
	}
	if _, err := protocol.Cancel(ctx, "unsupported", handle); err == nil {
		t.Fatal("cancelled unknown operation")
	}
	foreign := handle
	foreign.Binding = "account-b"
	if _, err := protocol.Observe(ctx, request.Capability, foreign); err == nil {
		t.Fatal("recovered another account's job")
	}
	if _, err := protocol.Cancel(ctx, request.Capability, foreign); err == nil {
		t.Fatal("cancelled another account's job")
	}
	for _, reference := range []string{`{`, `{"binding":"account-b","engine":1}`, `{"binding":"account-a","engine":0}`} {
		if _, err := protocol.synthesisRequest(dictatorCanonicalInput{}, dictatorCanonicalControls{TextFormat: "plain"}, &mediaVoiceRecord{ProviderVoiceReference: reference}); err == nil {
			t.Fatal("accepted unusable voice")
		}
	}
	reference, _ := json.Marshal(dictatorVoiceReference{Binding: protocol.binding, Engine: dictator.SynthesisEngine_SYNTHESIS_ENGINE_SILERO_RU, Preset: "baya"})
	if _, err := protocol.synthesisRequest(dictatorCanonicalInput{}, dictatorCanonicalControls{}, &mediaVoiceRecord{ProviderVoiceReference: string(reference)}); err == nil {
		t.Fatal("accepted unspecified text format")
	}
	for _, scenario := range []struct {
		name       string
		reply      proto.Message
		capability string
	}{
		{"wrong job", &dictator.GetTranscribeJobResponse{JobId: "another", State: 3}, llmproxycontract.MediaCapabilityAudioTranscribe},
		{"unknown state", &dictator.GetTranscribeJobResponse{JobId: "job", State: 99}, llmproxycontract.MediaCapabilityAudioTranscribe},
		{"missing sample", &dictator.GetExtractReferenceSampleJobResponse{JobId: "job", State: 3}, llmproxycontract.MediaCapabilityAudioVoiceExtract},
		{"missing subtitles", &dictator.GetRenderSubtitlesJobResponse{JobId: "job", State: 3}, llmproxycontract.MediaCapabilitySubtitlesCreate},
		{"invalid word timestamp", &dictator.GetTranscribeJobResponse{JobId: "job", State: 3, Words: []*dictator.WordSegment{{StartSeconds: math.NaN()}}}, llmproxycontract.MediaCapabilityAudioTranscribe},
		{"artifact unavailable", &dictator.GetRenderSubtitlesJobResponse{JobId: "job", State: 3, SrtArtifactId: "missing"}, llmproxycontract.MediaCapabilitySubtitlesCreate},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			protocol := dictatorProtocolBoundary(t, func(_ string, _ any, response any) error {
				proto.Merge(response.(proto.Message), scenario.reply)
				return nil
			}, func(string) (grpc.ClientStream, error) { return nil, fault })
			observed, err := protocol.Observe(ctx, scenario.capability, handle)
			if err == nil || len(observed.Outputs) != 0 {
				t.Fatalf("invalid provider output=%+v error=%v", observed, err)
			}
		})
	}
	protocol = dictatorProtocolBoundary(t, func(_ string, _ any, response any) error {
		proto.Merge(response.(proto.Message), &dictator.CancelTranscribeJobResponse{JobId: "wrong", State: 5})
		return nil
	}, nil)
	if confirmed, err := protocol.Cancel(ctx, request.Capability, handle); err == nil || confirmed {
		t.Fatal("confirmed another job's cancellation")
	}
	protocol = dictatorProtocolBoundary(t, func(_ string, _ any, response any) error {
		proto.Merge(response.(proto.Message), &dictator.SubmitTranscribeJobResponse{JobId: "job", State: 99})
		return nil
	}, nil)
	if _, err := protocol.Submit(ctx, request); err == nil {
		t.Fatal("accepted undefined native state")
	}
	protocol = dictatorProtocolBoundary(t, func(_ string, _ any, response any) error {
		proto.Merge(response.(proto.Message), &dictator.GetTranscribeJobResponse{JobId: "job", State: 4})
		return nil
	}, nil)
	if observed, err := protocol.Observe(ctx, request.Capability, handle); err != nil || observed.State != dictatorProtocolStateFailed {
		t.Fatalf("failed job=%+v error=%v", observed, err)
	}
	protocol = dictatorProtocolBoundary(t, func(_ string, request, response any) error {
		subtitles := request.(*dictator.RenderSubtitlesRequest)
		if subtitles.Granularity != dictator.SubtitleGranularity_SUBTITLE_GRANULARITY_WORDS || subtitles.GetSourceText() != "" {
			t.Fatal("word subtitle controls changed")
		}
		proto.Merge(response.(proto.Message), &dictator.SubmitRenderSubtitlesJobResponse{JobId: "job", State: 1})
		return nil
	}, nil)
	if _, err := protocol.Submit(ctx, dictatorProtocolRequest{Capability: llmproxycontract.MediaCapabilitySubtitlesCreate, Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{"granularity":"word"}`)}); err != nil {
		t.Fatal(err)
	}
	for _, voices := range [][]*dictator.SynthesisVoice{{{RequiresReferenceAudio: true}}, {{VoiceId: "invalid", SynthesisEngine: 99}}} {
		protocol = dictatorProtocolBoundary(t, func(_ string, _ any, response any) error {
			proto.Merge(response.(proto.Message), &dictator.ListSynthesisVoicesResponse{Voices: voices})
			return nil
		}, nil)
		result, err := protocol.DiscoverVoices(ctx)
		if voices[0].RequiresReferenceAudio {
			if err != nil || len(result) != 0 {
				t.Fatal("reference-only engine became a preset")
			}
		} else if err == nil {
			t.Fatal("accepted unknown engine")
		}
	}
}

func TestDictatorConnectionAuthorityAtExecutionBoundary(t *testing.T) {
	fixture := newMediaOperationInternalFixture(t)
	if err := fixture.database.AutoMigrate(&managedConnectionFieldRecord{}); err != nil {
		t.Fatal(err)
	}
	store := &managedTenantStore{routingDefaults: fixture.service.providers, providerKeyCipher: internalManagedProviderKeyCipher()}
	adapter := &accountDictatorAdapter{provider: ProviderNameDictator, model: ModelNameDictatorWhisperBase, transport: fixture.service.providers.definitions[providerID(ProviderNameDictator)].transports["speech"], tenants: store, store: fixture.service.store, assets: fixture.service.assets}
	request := MediaOperationExecutionRequest{TenantID: fixture.tenant.identifier.string(), CredentialReference: "connection-internal:v3", ProviderHandle: "private-handle"}
	if result := adapter.Execute(context.Background(), request); result.State != MediaOperationStateFailed {
		t.Fatalf("dispatched without assignment: %+v", result)
	}
	if result := adapter.Recover(context.Background(), request); result.State != MediaOperationStateUncertain || result.ProviderHandle != request.ProviderHandle {
		t.Fatalf("recovered without authority: %+v", result)
	}
	if result := adapter.Cancel(context.Background(), request); result.State != MediaCancellationUnsupported {
		t.Fatalf("cancelled without authority: %+v", result)
	}
	if _, err := adapter.DiscoverMediaVoices(context.Background(), request.TenantID); err == nil {
		t.Fatal("discovered without assignment")
	}
	if err := fixture.database.Model(&managedTenantConnectionRecord{}).Where("tenant_id = ?", request.TenantID).Update("provider_id", ProviderNameDictator).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := adapter.bind(context.Background(), request.TenantID, "connection-internal:v2"); !errors.Is(err, errMediaOperationUnavailable) {
		t.Fatalf("obsolete credential error=%v", err)
	}
	if err := fixture.database.Model(&managedAccountConnectionRecord{}).Where("id = ?", "connection-internal").Update("provider_id", "unknown").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := adapter.bind(context.Background(), request.TenantID, request.CredentialReference); !errors.Is(err, errManagedConnectionInvalid) {
		t.Fatalf("unknown provider error=%v", err)
	}

	if err := fixture.database.Model(&managedAccountConnectionRecord{}).Where("id = ?", "connection-internal").Update("provider_id", ProviderNameDictator).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := adapter.bind(context.Background(), request.TenantID, request.CredentialReference); !errors.Is(err, errManagedConnectionInvalid) || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("missing TLS setting error=%v", err)
	}
}

func TestDictatorTLSAndTargetConstruction(t *testing.T) {
	ctx := context.Background()
	transport := providerTransportDefinition{endpoint: ProviderCatalogEndpoint{SettingField: dictatorAddressField}, authentication: ProviderCatalogAuthentication{Field: dictatorTokenField}}
	connection, authenticated, err := openDictatorConnection(ctx, map[string]string{dictatorAddressField: "localhost:50051", dictatorTLSField: "true", dictatorTokenField: "token"}, transport)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	values, _ := metadata.FromOutgoingContext(authenticated)
	if strings.Join(values.Get("authorization"), "") != "Bearer token" {
		t.Fatal("TLS connection lost authentication")
	}
	for _, values := range []map[string]string{{dictatorTLSField: "invalid"}, {dictatorTLSField: "false", dictatorAddressField: "%"}} {
		if connection, _, err := openDictatorConnection(ctx, values, transport); err == nil {
			connection.Close()
			t.Fatal("accepted invalid connection settings")
		}
		if err := verifyDictatorConnection(ctx, values, transport); !errors.Is(err, errProviderKeyVerificationUnavailable) {
			t.Fatalf("invalid connection verification=%v", err)
		}
	}
}

func TestDictatorDiarizationRejectsNativeResourceFields(t *testing.T) {
	for _, document := range []map[string]any{nil, {"text": "Speaker one.", "source_artifact_id": "private-artifact"}, {"text": "Speaker one.", "words": []any{map[string]any{"word": "Speaker", "speaker": "speaker_1", "start": math.NaN(), "end": 1.0}}}} {
		var result *structpb.Struct
		if document != nil {
			var err error
			result, err = structpb.NewStruct(document)
			if err != nil {
				t.Fatal(err)
			}
		}
		protocol := dictatorProtocolBoundary(t, func(_ string, _ any, response any) error {
			proto.Merge(response.(proto.Message), &dictator.GetDiarizeAudioJobResponse{JobId: "job", State: 3, Diarization: result})
			return nil
		}, nil)
		observed, err := protocol.Observe(context.Background(), llmproxycontract.MediaCapabilityAudioDiarize, dictatorProviderHandle{Version: dictatorProviderHandleVersion, Binding: "account-a", JobID: "job"})
		if err == nil || len(observed.Outputs) != 0 {
			t.Fatalf("invalid native result escaped: %+v error=%v", observed, err)
		}
	}
}
