package proxy

import (
	"context"
	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"google.golang.org/grpc"
)

// Generated SDK calls use the same authority as the operation's HTTP clients.
// Only the current asynchronous protocol methods have an execution role.
type hostedMediaGRPCConnection struct {
	next        grpc.ClientConnInterface
	authorize   hostedMediaAuthorize
	recordUsage func(journalUsageEvidenceInput) error
}

func dictatorHostedMethodRole(method string) hostedProviderRole {
	switch method {
	case dictator.VoiceService_ListSynthesisVoices_FullMethodName:
		return hostedProviderMetadata
	case dictator.TranscriptionService_SubmitTranscribeJob_FullMethodName,
		dictator.TranscriptionService_SubmitDiarizeAudioJob_FullMethodName,
		dictator.AlignmentService_SubmitAlignTranscriptJob_FullMethodName,
		dictator.SubtitleService_SubmitRenderSubtitlesJob_FullMethodName,
		dictator.VoiceService_SubmitSynthesizeSpeechJob_FullMethodName,
		dictator.VoiceService_SubmitExtractReferenceSampleJob_FullMethodName:
		return hostedProviderGeneration
	case dictator.TranscriptionService_GetTranscribeJob_FullMethodName,
		dictator.TranscriptionService_GetDiarizeAudioJob_FullMethodName,
		dictator.AlignmentService_GetAlignTranscriptJob_FullMethodName,
		dictator.SubtitleService_GetRenderSubtitlesJob_FullMethodName,
		dictator.VoiceService_GetSynthesizeSpeechJob_FullMethodName,
		dictator.VoiceService_GetExtractReferenceSampleJob_FullMethodName:
		return hostedProviderObservation
	case dictator.TranscriptionService_CancelTranscribeJob_FullMethodName,
		dictator.TranscriptionService_CancelDiarizeAudioJob_FullMethodName,
		dictator.AlignmentService_CancelAlignTranscriptJob_FullMethodName,
		dictator.SubtitleService_CancelRenderSubtitlesJob_FullMethodName,
		dictator.VoiceService_CancelSynthesizeSpeechJob_FullMethodName,
		dictator.VoiceService_CancelExtractReferenceSampleJob_FullMethodName:
		return hostedProviderCancellation
	case dictator.ArtifactService_UploadArtifact_FullMethodName:
		return hostedProviderStaging
	case dictator.ArtifactService_DownloadArtifact_FullMethodName:
		return hostedProviderAuxiliary
	default:
		return 0
	}
}

func (connection *hostedMediaGRPCConnection) Invoke(ctx context.Context, method string, args, reply any, options ...grpc.CallOption) error {
	role := dictatorHostedMethodRole(method)
	if connection.authorize == nil || role == 0 {
		return errHostedAuthorityDenied
	}
	if err := connection.authorize(ctx, role); err != nil {
		return err
	}
	if err := connection.next.Invoke(ctx, method, args, reply, options...); err != nil {
		return err
	}
	return connection.recordResponseUsage(reply)
}
func (connection *hostedMediaGRPCConnection) NewStream(ctx context.Context, description *grpc.StreamDesc, method string, options ...grpc.CallOption) (grpc.ClientStream, error) {
	role := dictatorHostedMethodRole(method)
	if connection.authorize == nil || role == 0 {
		return nil, errHostedAuthorityDenied
	}
	if err := connection.authorize(ctx, role); err != nil {
		return nil, err
	}
	stream, err := connection.next.NewStream(ctx, description, method, options...)
	if err != nil {
		return nil, err
	}
	return &hostedMediaGRPCStream{ClientStream: stream, authorize: connection.authorize, role: role}, nil
}

type hostedMediaGRPCStream struct {
	grpc.ClientStream
	authorize hostedMediaAuthorize
	role      hostedProviderRole
}

func (stream *hostedMediaGRPCStream) SendMsg(message any) error {
	if err := stream.authorize(stream.Context(), stream.role); err != nil {
		return err
	}
	return stream.ClientStream.SendMsg(message)
}
func (stream *hostedMediaGRPCStream) RecvMsg(message any) error {
	if err := stream.authorize(stream.Context(), stream.role); err != nil {
		return err
	}
	return stream.ClientStream.RecvMsg(message)
}
