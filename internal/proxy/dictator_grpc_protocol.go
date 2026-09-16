package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const dictatorArtifactChunkBytes = 64 * 1024

type dictatorGRPCProtocol struct {
	provider      string
	model         string
	connection    *grpc.ClientConn
	token         string
	binding       string
	maxAssetBytes int64
}

type dictatorVoiceReference struct {
	Binding    string                   `json:"binding"`
	Engine     dictator.SynthesisEngine `json:"engine"`
	Preset     string                   `json:"preset,omitempty"`
	Artifact   string                   `json:"artifact,omitempty"`
	Transcript string                   `json:"transcript,omitempty"`
}

type dictatorExtractionContext struct {
	Transcript  string `json:"transcript"`
	DisplayName string `json:"display_name"`
	Language    string `json:"language"`
}

func (protocol *dictatorGRPCProtocol) context(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+protocol.token)
}

func (protocol *dictatorGRPCProtocol) DiscoverVoices(ctx context.Context) ([]MediaVoiceProviderRecord, error) {
	response, err := dictator.NewVoiceServiceClient(protocol.connection).ListSynthesisVoices(protocol.context(ctx), &dictator.ListSynthesisVoicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("discover Dictator voices: %w", err)
	}
	result := make([]MediaVoiceProviderRecord, 0, len(response.Voices))
	for _, voice := range response.Voices {
		if voice.RequiresReferenceAudio {
			continue
		}
		if voice.VoiceId == "" || (voice.SynthesisEngine != dictator.SynthesisEngine_SYNTHESIS_ENGINE_SILERO_RU && voice.SynthesisEngine != dictator.SynthesisEngine_SYNTHESIS_ENGINE_QWEN3) {
			return nil, errors.New("invalid Dictator voice")
		}
		reference, _ := json.Marshal(dictatorVoiceReference{Binding: protocol.binding, Engine: voice.SynthesisEngine, Preset: voice.VoiceId})
		rates := make([]int, len(voice.NativeSampleRateHz))
		for index, rate := range voice.NativeSampleRateHz {
			rates[index] = int(rate)
		}
		result = append(result, MediaVoiceProviderRecord{Authority: protocol.binding, Provider: protocol.provider, Model: protocol.model, Mode: MediaVoiceModePreset, Language: voice.LanguageCode, DisplayName: voice.DisplayName, Default: voice.IsDefault, SampleRates: rates, DefaultSampleRate: int(voice.DefaultSampleRateHz), ProviderVoiceReference: string(reference)})
	}
	return result, nil
}

func (protocol *dictatorGRPCProtocol) upload(ctx context.Context, asset dictatorProtocolAsset) (string, error) {
	stream, err := dictator.NewArtifactServiceClient(protocol.connection).UploadArtifact(ctx)
	if err != nil {
		return "", fmt.Errorf("upload Dictator input: %w", err)
	}
	if err := stream.Send(&dictator.UploadArtifactChunk{Payload: &dictator.UploadArtifactChunk_Metadata{Metadata: &dictator.UploadArtifactMetadata{Filename: "input.wav", MediaType: asset.MIMEType}}}); err != nil {
		return "", err
	}
	for offset := 0; offset < len(asset.Data); offset += dictatorArtifactChunkBytes {
		end := min(offset+dictatorArtifactChunkBytes, len(asset.Data))
		if err := stream.Send(&dictator.UploadArtifactChunk{Payload: &dictator.UploadArtifactChunk_Content{Content: asset.Data[offset:end]}}); err != nil {
			return "", err
		}
	}
	response, err := stream.CloseAndRecv()
	if err != nil {
		return "", fmt.Errorf("finish Dictator upload: %w", err)
	}
	artifact := response.GetArtifact()
	if artifact.GetArtifactId() == "" || artifact.GetSizeBytes() != int64(len(asset.Data)) || artifact.GetSha256() != mediaSHA256Hex(asset.Data) {
		return "", errors.New("invalid Dictator uploaded artifact")
	}
	return artifact.ArtifactId, nil
}

func (protocol *dictatorGRPCProtocol) download(ctx context.Context, identifier string) (MediaOperationOutput, error) {
	stream, err := dictator.NewArtifactServiceClient(protocol.connection).DownloadArtifact(ctx, &dictator.DownloadArtifactRequest{ArtifactId: identifier, ChunkSize: dictatorArtifactChunkBytes})
	if err != nil {
		return MediaOperationOutput{}, err
	}
	var content []byte
	var artifact *dictator.ArtifactRef
	finished := false
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return MediaOperationOutput{}, fmt.Errorf("download Dictator artifact: %w", err)
		}
		if finished || chunk.Offset != int64(len(content)) || int64(len(content))+int64(len(chunk.Content)) > protocol.maxAssetBytes {
			return MediaOperationOutput{}, errors.New("invalid Dictator artifact stream")
		}
		if chunk.Artifact != nil {
			if artifact != nil && (artifact.ArtifactId != chunk.Artifact.ArtifactId || artifact.SizeBytes != chunk.Artifact.SizeBytes || artifact.Sha256 != chunk.Artifact.Sha256 || artifact.MediaType != chunk.Artifact.MediaType) {
				return MediaOperationOutput{}, errors.New("dictator artifact metadata changed")
			}
			artifact = chunk.Artifact
		}
		content = append(content, chunk.Content...)
		finished = chunk.Eof
	}
	if !finished || artifact.GetArtifactId() != identifier || artifact.GetSizeBytes() != int64(len(content)) || artifact.GetSha256() != mediaSHA256Hex(content) || artifact.GetMediaType() == "" {
		return MediaOperationOutput{}, errors.New("invalid Dictator result artifact")
	}
	return MediaOperationOutput{MIMEType: artifact.MediaType, Data: content}, nil
}

func (protocol *dictatorGRPCProtocol) Submit(ctx context.Context, request dictatorProtocolRequest) (dictatorProtocolObservation, error) {
	ctx = protocol.context(ctx)
	var input dictatorCanonicalInput
	var controls dictatorCanonicalControls
	if err := decodeExactDictatorJSON(request.Input, &input); err != nil {
		return dictatorProtocolObservation{}, err
	}
	if err := decodeExactDictatorJSON(request.Controls, &controls); err != nil {
		return dictatorProtocolObservation{}, err
	}
	handle := dictatorProviderHandle{Version: dictatorProviderHandleVersion, Binding: protocol.binding}
	audioID := ""
	if len(request.Assets) != 0 {
		var err error
		audioID, err = protocol.upload(ctx, request.Assets[0])
		if err != nil {
			return dictatorProtocolObservation{}, err
		}
		handle.SourceArtifactIDs = []string{audioID}
	}
	var state int32
	var err error
	switch request.Capability {
	case llmproxycontract.MediaCapabilityAudioTranscribe:
		response, callErr := dictator.NewTranscriptionServiceClient(protocol.connection).SubmitTranscribeJob(ctx, &dictator.TranscribeRequest{AudioArtifactId: audioID, LanguageCode: controls.Language, AutodetectLanguage: controls.DetectLanguage, IncludeWordSegments: true})
		err = callErr
		if err == nil {
			handle.JobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilityAudioDiarize:
		native := &dictator.DiarizeAudioRequest{AudioArtifactId: audioID, LanguageCode: controls.Language, AutodetectLanguage: controls.DetectLanguage, ModelSize: controls.ModelSize, IncludeWords: true, IncludeUtterances: true, IncludeSpeakerSegments: true, IncludeSpeakers: true}
		native.UtteranceGapSeconds = controls.UtteranceGapSeconds
		response, callErr := dictator.NewTranscriptionServiceClient(protocol.connection).SubmitDiarizeAudioJob(ctx, native)
		err = callErr
		if err == nil {
			handle.JobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilityAudioAlign:
		response, callErr := dictator.NewAlignmentServiceClient(protocol.connection).SubmitAlignTranscriptJob(ctx, &dictator.AlignTranscriptRequest{AudioArtifactId: audioID, TranscriptSource: &dictator.AlignTranscriptRequest_TranscriptText{TranscriptText: input.Transcript}, LanguageCode: controls.Language, RemovePunctuation: controls.RemovePunctuation})
		err = callErr
		if err == nil {
			handle.JobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilitySubtitlesCreate:
		native := &dictator.RenderSubtitlesRequest{AudioArtifactId: audioID, LanguageCode: controls.Language, AutodetectLanguage: controls.DetectLanguage, OutputFormat: dictator.SubtitleFormat_SUBTITLE_FORMAT_SRT, GroupSize: int32(controls.GroupSize), IncludeSrtText: true}
		if controls.Granularity == "word" {
			native.Granularity = dictator.SubtitleGranularity_SUBTITLE_GRANULARITY_WORDS
		} else {
			native.Granularity = dictator.SubtitleGranularity_SUBTITLE_GRANULARITY_SENTENCES
		}
		if input.Transcript != "" {
			native.SourceTextSource = &dictator.RenderSubtitlesRequest_SourceText{SourceText: input.Transcript}
		}
		response, callErr := dictator.NewSubtitleServiceClient(protocol.connection).SubmitRenderSubtitlesJob(ctx, native)
		err = callErr
		if err == nil {
			handle.JobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilityAudioSpeechGenerate:
		native, requestErr := protocol.synthesisRequest(input, controls, request.Voice)
		if requestErr != nil {
			return dictatorProtocolObservation{}, requestErr
		}
		response, callErr := dictator.NewVoiceServiceClient(protocol.connection).SubmitSynthesizeSpeechJob(ctx, native)
		err = callErr
		if err == nil {
			handle.JobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilityAudioVoiceExtract:
		handle.Extraction = &dictatorExtractionContext{Transcript: input.Transcript, DisplayName: input.DisplayName, Language: input.Language}
		response, callErr := dictator.NewVoiceServiceClient(protocol.connection).SubmitExtractReferenceSampleJob(ctx, &dictator.ExtractReferenceSampleRequest{SourceArtifactId: audioID, ModelSize: controls.ModelSize, LanguageCode: input.Language, DurationSeconds: *controls.DurationSeconds})
		err = callErr
		if err == nil {
			handle.JobID, state = response.JobId, int32(response.State)
		}
	default:
		return dictatorProtocolObservation{}, errMediaOperationInvalid
	}
	if err != nil {
		return dictatorProtocolObservation{}, fmt.Errorf("submit Dictator job: %w", err)
	}
	// Always persist the accepted native handle before obtaining result bytes.
	if _, err := dictatorJobState(state); err != nil {
		return dictatorProtocolObservation{}, err
	}
	return dictatorProtocolObservation{State: dictatorProtocolStateQueued, Handle: handle}, nil
}

func (protocol *dictatorGRPCProtocol) synthesisRequest(input dictatorCanonicalInput, controls dictatorCanonicalControls, voice *mediaVoiceRecord) (*dictator.SynthesizeSpeechRequest, error) {
	if voice == nil {
		return nil, errors.New("dictator voice missing")
	}
	var reference dictatorVoiceReference
	if err := decodeExactDictatorJSON([]byte(voice.ProviderVoiceReference), &reference); err != nil {
		return nil, err
	}
	if reference.Binding != protocol.binding {
		return nil, errors.New("dictator voice belongs to another connection")
	}
	if reference.Engine != dictator.SynthesisEngine_SYNTHESIS_ENGINE_SILERO_RU && reference.Engine != dictator.SynthesisEngine_SYNTHESIS_ENGINE_QWEN3 {
		return nil, errors.New("invalid Dictator synthesis engine")
	}
	native := &dictator.SynthesizeSpeechRequest{TextSource: &dictator.SynthesizeSpeechRequest_Text{Text: input.Text}, LanguageCode: controls.Language, SynthesisEngine: reference.Engine, SpeakerArtifactId: reference.Artifact, PresetSpeaker: reference.Preset, SpeakerTranscriptText: reference.Transcript, MaxDurationSeconds: controls.MaxDurationSeconds, IncludeTimeline: controls.IncludeTimeline, AudioFormat: &dictator.AudioFormat{Container: dictator.AudioContainer_AUDIO_CONTAINER_WAV, Codec: dictator.AudioCodec_AUDIO_CODEC_PCM_S16LE, SampleRateHz: int32(controls.SampleRateHz), ChannelCount: 1, BitDepth: 16}}
	switch controls.TextFormat {
	case "plain":
		native.TextFormat = dictator.SynthesisTextFormat_SYNTHESIS_TEXT_FORMAT_PLAIN_TEXT
	case "ssml":
		native.TextFormat = dictator.SynthesisTextFormat_SYNTHESIS_TEXT_FORMAT_SSML
	default:
		return nil, errors.New("invalid Dictator text format")
	}
	return native, nil
}

func dictatorJobState(state int32) (string, error) {
	switch state {
	case 1:
		return dictatorProtocolStateQueued, nil
	case 2:
		return dictatorProtocolStateRunning, nil
	case 3:
		return dictatorProtocolStateSucceeded, nil
	case 4:
		return dictatorProtocolStateFailed, nil
	case 5:
		return dictatorProtocolStateCancelled, nil
	default:
		return "", errors.New("invalid Dictator job state")
	}
}

func dictatorJSONOutput(value any) (MediaOperationOutput, error) {
	data, err := json.Marshal(value)
	return MediaOperationOutput{MIMEType: "application/json", Data: data}, err
}

// Dictator's diarization document has a closed speech-result shape. Native
// artifact and job references are not part of this tenant-visible document.
type dictatorDiarizationOutput struct {
	Text            string                      `json:"text"`
	Language        string                      `json:"languageCode,omitempty"`
	Words           []dictatorDiarizedWord      `json:"words"`
	Utterances      []dictatorDiarizedUtterance `json:"utterances"`
	Speakers        []dictatorSpeakerSummary    `json:"speakers"`
	SpeakerSegments []dictatorSpeakerSegment    `json:"speakerSegments"`
}
type dictatorSpeakerSegment struct {
	Speaker string  `json:"speaker"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
}
type dictatorDiarizedWord struct {
	dictatorSpeakerSegment
	Word string `json:"word"`
}
type dictatorDiarizedUtterance struct {
	dictatorSpeakerSegment
	Text  string                 `json:"text"`
	Words []dictatorDiarizedWord `json:"words"`
}
type dictatorSpeakerSummary struct {
	Speaker        string  `json:"speaker"`
	WordCount      int     `json:"wordCount"`
	UtteranceCount int     `json:"utteranceCount"`
	TotalDuration  float64 `json:"totalDurationSeconds"`
}

type dictatorTranscriptOutput struct {
	Text     string                  `json:"text"`
	Language string                  `json:"language_code"`
	Words    []*dictator.WordSegment `json:"words"`
}

type dictatorTimelineSegment struct {
	Content *string  `json:"content"`
	Start   *float64 `json:"start"`
	End     *float64 `json:"end"`
}

type dictatorTimelineOutput struct {
	TextSegments []dictatorTimelineSegment `json:"textSegments"`
}

func publicDictatorTimeline(output MediaOperationOutput) (MediaOperationOutput, error) {
	var native struct {
		dictatorTimelineOutput
		ImageCues json.RawMessage `json:"imageCues"`
		Voices    json.RawMessage `json:"voices"`
	}
	if err := decodeExactDictatorJSON(output.Data, &native); err != nil {
		return MediaOperationOutput{}, fmt.Errorf("decode Dictator timeline: %w", err)
	}
	if output.MIMEType != "application/json" || len(native.TextSegments) == 0 {
		return MediaOperationOutput{}, errors.New("invalid Dictator timeline")
	}
	for _, segment := range native.TextSegments {
		if segment.Content == nil || segment.Start == nil || segment.End == nil || *segment.Start < 0 || *segment.End < *segment.Start {
			return MediaOperationOutput{}, errors.New("invalid Dictator timeline segment")
		}
	}
	return dictatorJSONOutput(native.dictatorTimelineOutput)
}

func (protocol *dictatorGRPCProtocol) Observe(ctx context.Context, capability string, handle dictatorProviderHandle) (dictatorProtocolObservation, error) {
	if handle.Binding != protocol.binding {
		return dictatorProtocolObservation{}, errors.New("dictator job belongs to another connection")
	}
	ctx = protocol.context(ctx)
	var state int32
	var jobID string
	var err error
	var jsonResult any
	var artifactIDs []string
	var timelineID string
	observation := dictatorProtocolObservation{Handle: handle}
	switch capability {
	case llmproxycontract.MediaCapabilityAudioTranscribe:
		response, callErr := dictator.NewTranscriptionServiceClient(protocol.connection).GetTranscribeJob(ctx, &dictator.GetTranscribeJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			state, jobID = int32(response.State), response.JobId
			jsonResult = dictatorTranscriptOutput{Text: response.Text, Language: response.LanguageCode, Words: response.Words}
		}
	case llmproxycontract.MediaCapabilityAudioDiarize:
		response, callErr := dictator.NewTranscriptionServiceClient(protocol.connection).GetDiarizeAudioJob(ctx, &dictator.GetDiarizeAudioJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			state, jobID = int32(response.State), response.JobId
			if response.State == dictator.DiarizationJobState_DIARIZATION_JOB_STATE_SUCCEEDED {
				if response.Diarization == nil {
					return dictatorProtocolObservation{}, errors.New("dictator diarization result missing")
				}
				// AsMap returns only JSON-compatible values, including strings for nonfinite numbers.
				document, _ := json.Marshal(response.Diarization.AsMap())
				var result dictatorDiarizationOutput
				if decodeError := decodeExactDictatorJSON(document, &result); decodeError != nil {
					return dictatorProtocolObservation{}, decodeError
				}
				jsonResult = result
			}
		}
	case llmproxycontract.MediaCapabilityAudioAlign:
		response, callErr := dictator.NewAlignmentServiceClient(protocol.connection).GetAlignTranscriptJob(ctx, &dictator.GetAlignTranscriptJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			state, jobID = int32(response.State), response.JobId
			jsonResult = dictatorTranscriptOutput{Language: response.LanguageCode, Words: response.Words}
			artifactIDs = []string{response.SrtArtifactId}
		}
	case llmproxycontract.MediaCapabilitySubtitlesCreate:
		response, callErr := dictator.NewSubtitleServiceClient(protocol.connection).GetRenderSubtitlesJob(ctx, &dictator.GetRenderSubtitlesJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			state, jobID = int32(response.State), response.JobId
			artifactIDs = []string{response.SrtArtifactId}
		}
	case llmproxycontract.MediaCapabilityAudioSpeechGenerate:
		response, callErr := dictator.NewVoiceServiceClient(protocol.connection).GetSynthesizeSpeechJob(ctx, &dictator.GetSynthesizeSpeechJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			state, jobID = int32(response.State), response.JobId
			artifactIDs = []string{response.GetAudioArtifact().GetArtifactId()}
			if response.TimelineArtifactId != "" {
				timelineID = response.TimelineArtifactId
				artifactIDs = append(artifactIDs, response.TimelineArtifactId)
			}
		}
	case llmproxycontract.MediaCapabilityAudioVoiceExtract:
		response, callErr := dictator.NewVoiceServiceClient(protocol.connection).GetExtractReferenceSampleJob(ctx, &dictator.GetExtractReferenceSampleJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			state, jobID = int32(response.State), response.JobId
			if response.State == dictator.ExtractReferenceSampleJobState_EXTRACT_REFERENCE_SAMPLE_JOB_STATE_SUCCEEDED {
				if handle.Extraction == nil || response.GetSampleArtifact().GetArtifactId() == "" {
					return dictatorProtocolObservation{}, errors.New("invalid Dictator extracted voice")
				}
				reference, _ := json.Marshal(dictatorVoiceReference{Binding: protocol.binding, Engine: dictator.SynthesisEngine_SYNTHESIS_ENGINE_QWEN3, Artifact: response.SampleArtifact.ArtifactId, Transcript: handle.Extraction.Transcript})
				observation.Voice = &MediaVoiceProviderRecord{Authority: protocol.binding, Provider: protocol.provider, Model: protocol.model, Mode: MediaVoiceModeExtracted, Language: handle.Extraction.Language, DisplayName: handle.Extraction.DisplayName, SampleRates: []int{24000}, DefaultSampleRate: 24000, ProviderVoiceReference: string(reference)}
				observation.Handle.ResultArtifactIDs = []string{response.SampleArtifact.ArtifactId}
			}
		}
	default:
		return dictatorProtocolObservation{}, errMediaOperationInvalid
	}
	if err != nil {
		return dictatorProtocolObservation{}, fmt.Errorf("observe Dictator job: %w", err)
	}
	if jobID != handle.JobID {
		return dictatorProtocolObservation{}, errors.New("dictator job identity changed")
	}
	observation.State, err = dictatorJobState(state)
	if err != nil {
		return dictatorProtocolObservation{}, err
	}
	if observation.State != dictatorProtocolStateSucceeded {
		return observation, nil
	}
	if jsonResult != nil {
		output, err := dictatorJSONOutput(jsonResult)
		if err != nil {
			return dictatorProtocolObservation{}, err
		}
		observation.Outputs = append(observation.Outputs, output)
	}
	for _, identifier := range artifactIDs {
		if identifier == "" {
			return dictatorProtocolObservation{}, errors.New("dictator result artifact missing")
		}
		output, err := protocol.download(ctx, identifier)
		if err != nil {
			return dictatorProtocolObservation{}, err
		}
		if identifier == timelineID {
			output, err = publicDictatorTimeline(output)
			if err != nil {
				return dictatorProtocolObservation{}, err
			}
		}
		observation.Outputs = append(observation.Outputs, output)
		observation.Handle.ResultArtifactIDs = append(observation.Handle.ResultArtifactIDs, identifier)
	}
	return observation, nil
}

func (protocol *dictatorGRPCProtocol) Cancel(ctx context.Context, capability string, handle dictatorProviderHandle) (bool, error) {
	if handle.Binding != protocol.binding {
		return false, errors.New("dictator job belongs to another connection")
	}
	ctx = protocol.context(ctx)
	var state int32
	var jobID string
	var err error
	switch capability {
	case llmproxycontract.MediaCapabilityAudioTranscribe:
		response, callErr := dictator.NewTranscriptionServiceClient(protocol.connection).CancelTranscribeJob(ctx, &dictator.CancelTranscribeJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			jobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilityAudioDiarize:
		response, callErr := dictator.NewTranscriptionServiceClient(protocol.connection).CancelDiarizeAudioJob(ctx, &dictator.CancelDiarizeAudioJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			jobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilityAudioAlign:
		response, callErr := dictator.NewAlignmentServiceClient(protocol.connection).CancelAlignTranscriptJob(ctx, &dictator.CancelAlignTranscriptJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			jobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilitySubtitlesCreate:
		response, callErr := dictator.NewSubtitleServiceClient(protocol.connection).CancelRenderSubtitlesJob(ctx, &dictator.CancelRenderSubtitlesJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			jobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilityAudioSpeechGenerate:
		response, callErr := dictator.NewVoiceServiceClient(protocol.connection).CancelSynthesizeSpeechJob(ctx, &dictator.CancelSynthesizeSpeechJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			jobID, state = response.JobId, int32(response.State)
		}
	case llmproxycontract.MediaCapabilityAudioVoiceExtract:
		response, callErr := dictator.NewVoiceServiceClient(protocol.connection).CancelExtractReferenceSampleJob(ctx, &dictator.CancelExtractReferenceSampleJobRequest{JobId: handle.JobID})
		err = callErr
		if err == nil {
			jobID, state = response.JobId, int32(response.State)
		}
	default:
		return false, errMediaOperationInvalid
	}
	if err != nil {
		return false, fmt.Errorf("cancel Dictator job: %w", err)
	}
	if jobID != handle.JobID {
		return false, errors.New("dictator cancellation identity changed")
	}
	observed, err := dictatorJobState(state)
	return observed == dictatorProtocolStateCancelled, err
}
