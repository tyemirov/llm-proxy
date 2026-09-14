package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const (
	dictatorProtocolStateQueued    = "queued"
	dictatorProtocolStateRunning   = "running"
	dictatorProtocolStateSucceeded = "succeeded"
	dictatorProtocolStateFailed    = "failed"
	dictatorProtocolStateCancelled = "cancelled"
	dictatorProviderHandleVersion  = 1
)

type dictatorProtocol interface {
	Submit(context.Context, dictatorProtocolRequest) (dictatorProtocolObservation, error)
	Observe(context.Context, string, dictatorProviderHandle) (dictatorProtocolObservation, error)
	Cancel(context.Context, string, dictatorProviderHandle) (bool, error)
	DiscoverVoices(context.Context) ([]MediaVoiceProviderRecord, error)
}

type dictatorProtocolRequest struct {
	Capability    string
	DispatchToken string
	Input         json.RawMessage
	Controls      json.RawMessage
	Assets        []dictatorProtocolAsset
	Voice         *mediaVoiceRecord
}

type dictatorProtocolAsset struct {
	AssetID  string
	MIMEType string
	Data     []byte
}

type dictatorProviderHandle struct {
	Version           int      `json:"version"`
	JobID             string   `json:"job_id"`
	SourceArtifactIDs []string `json:"source_artifact_ids,omitempty"`
	ResultArtifactIDs []string `json:"result_artifact_ids,omitempty"`
}

type dictatorProtocolObservation struct {
	State   string
	Handle  dictatorProviderHandle
	Outputs []MediaOperationOutput
	Voice   *MediaVoiceProviderRecord
}

type dictatorMediaOperationAdapter struct {
	protocol            dictatorProtocol
	assets              *tenantAssetStore
	store               *mediaOperationStore
	credentialReference string
	pollInterval        time.Duration
}

type dictatorCanonicalInput struct {
	AudioAssetID string `json:"audio_asset_id,omitempty"`
	Transcript   string `json:"transcript,omitempty"`
	Text         string `json:"text,omitempty"`
	VoiceID      string `json:"voice_id,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	Language     string `json:"language,omitempty"`
}

type dictatorCanonicalControls struct {
	Language            string  `json:"language,omitempty"`
	DetectLanguage      bool    `json:"detect_language,omitempty"`
	RemovePunctuation   bool    `json:"remove_punctuation,omitempty"`
	Granularity         string  `json:"granularity,omitempty"`
	GroupSize           int     `json:"group_size,omitempty"`
	ModelSize           string  `json:"model_size,omitempty"`
	UtteranceGapSeconds float64 `json:"utterance_gap_seconds,omitempty"`
	TextFormat          string  `json:"text_format,omitempty"`
	SampleRateHz        int     `json:"sample_rate_hz,omitempty"`
	MaxDurationSeconds  float64 `json:"max_duration_seconds,omitempty"`
	IncludeTimeline     bool    `json:"include_timeline,omitempty"`
}

func newDictatorMediaOperationAdapter(protocol dictatorProtocol, assets *tenantAssetStore, store *mediaOperationStore, credentialReference string, pollInterval time.Duration) (*dictatorMediaOperationAdapter, error) {
	credentialReference = strings.TrimSpace(credentialReference)
	if protocol == nil || assets == nil || store == nil || credentialReference == "" || pollInterval <= 0 {
		return nil, errors.New("invalid Dictator adapter configuration")
	}
	return &dictatorMediaOperationAdapter{protocol: protocol, assets: assets, store: store, credentialReference: credentialReference, pollInterval: pollInterval}, nil
}

func (adapter *dictatorMediaOperationAdapter) MediaOperationCredentialReference() string {
	return adapter.credentialReference
}

func (adapter *dictatorMediaOperationAdapter) DiscoverMediaVoices(requestContext context.Context) ([]MediaVoiceProviderRecord, error) {
	return adapter.protocol.DiscoverVoices(requestContext)
}

func (adapter *dictatorMediaOperationAdapter) Validate(request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	if request.Provider != ProviderNameDictator || request.Model != ModelNameDictatorSpeechV1 {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	input, controls, validationError := validateDictatorOperation(request.Capability, request.Input, request.Controls)
	if validationError != nil {
		return MediaOperationValidatedRequest{}, validationError
	}
	canonicalInput, _ := json.Marshal(input)
	canonicalControls, _ := json.Marshal(controls)
	inputAssets := []string{}
	if input.AudioAssetID != "" {
		inputAssets = append(inputAssets, input.AudioAssetID)
	}
	return MediaOperationValidatedRequest{InputAssetIDs: inputAssets, Input: canonicalInput, Controls: canonicalControls}, nil
}

func (adapter *dictatorMediaOperationAdapter) Execute(requestContext context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	protocolRequest, requestError := adapter.protocolRequest(requestContext, request)
	if requestError != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_result_invalid"}
	}
	observation, submitError := adapter.protocol.Submit(requestContext, protocolRequest)
	if submitError != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: "provider_outcome_unknown"}
	}
	handle, handleError := canonicalDictatorProviderHandle(observation.Handle)
	if handleError != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: "provider_outcome_unknown"}
	}
	handleJSON, _ := json.Marshal(handle)
	if request.PersistProviderHandle == nil || request.PersistProviderHandle(string(handleJSON)) != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ProviderHandle: string(handleJSON), ErrorCode: "provider_outcome_unknown"}
	}
	return adapter.awaitObservation(requestContext, request.Capability, observation)
}

func (adapter *dictatorMediaOperationAdapter) Recover(requestContext context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	handle, handleError := decodeDictatorProviderHandle(request.ProviderHandle)
	if handleError != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: "provider_outcome_unknown"}
	}
	observation, observeError := adapter.protocol.Observe(requestContext, request.Capability, handle)
	if observeError != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ProviderHandle: request.ProviderHandle, ErrorCode: "provider_outcome_unknown"}
	}
	return adapter.awaitObservation(requestContext, request.Capability, observation)
}

func (adapter *dictatorMediaOperationAdapter) Cancel(requestContext context.Context, request MediaOperationExecutionRequest) MediaOperationCancellationResult {
	handle, handleError := decodeDictatorProviderHandle(request.ProviderHandle)
	if handleError != nil {
		return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
	}
	confirmed, cancelError := adapter.protocol.Cancel(requestContext, request.Capability, handle)
	if cancelError != nil || !confirmed {
		return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
	}
	return MediaOperationCancellationResult{State: MediaCancellationConfirmed}
}

func (adapter *dictatorMediaOperationAdapter) awaitObservation(requestContext context.Context, capability string, observation dictatorProtocolObservation) MediaOperationExecutionResult {
	for {
		result, terminal := dictatorExecutionResult(observation)
		if terminal {
			return result
		}
		if observation.State != dictatorProtocolStateQueued && observation.State != dictatorProtocolStateRunning {
			return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: "provider_outcome_unknown"}
		}
		timer := time.NewTimer(adapter.pollInterval)
		select {
		case <-requestContext.Done():
			timer.Stop()
			return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ProviderHandle: encodedDictatorProviderHandle(observation.Handle), ErrorCode: "operation_deadline_exceeded"}
		case <-timer.C:
		}
		next, observeError := adapter.protocol.Observe(requestContext, capability, observation.Handle)
		if observeError != nil {
			return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ProviderHandle: encodedDictatorProviderHandle(observation.Handle), ErrorCode: "provider_outcome_unknown"}
		}
		observation = next
	}
}

func dictatorExecutionResult(observation dictatorProtocolObservation) (MediaOperationExecutionResult, bool) {
	handle, handleError := canonicalDictatorProviderHandle(observation.Handle)
	if handleError != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: "provider_outcome_unknown"}, true
	}
	encodedHandle := encodedDictatorProviderHandle(handle)
	switch observation.State {
	case dictatorProtocolStateSucceeded:
		return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, ProviderHandle: encodedHandle, Outputs: observation.Outputs, Voice: observation.Voice}, true
	case dictatorProtocolStateFailed:
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ProviderHandle: encodedHandle, ErrorCode: "provider_error"}, true
	case dictatorProtocolStateCancelled:
		return MediaOperationExecutionResult{State: MediaOperationStateCancelled, ProviderHandle: encodedHandle}, true
	default:
		return MediaOperationExecutionResult{}, false
	}
}

func (adapter *dictatorMediaOperationAdapter) protocolRequest(requestContext context.Context, request MediaOperationExecutionRequest) (dictatorProtocolRequest, error) {
	var input dictatorCanonicalInput
	if decodeError := decodeExactDictatorJSON(request.Input, &input); decodeError != nil {
		return dictatorProtocolRequest{}, decodeError
	}
	protocolRequest := dictatorProtocolRequest{Capability: request.Capability, DispatchToken: request.DispatchToken, Input: append(json.RawMessage(nil), request.Input...), Controls: append(json.RawMessage(nil), request.Controls...)}
	requestTenant := tenant{identifier: tenantID(request.TenantID)}
	if input.AudioAssetID != "" {
		metadata, metadataError := adapter.assets.metadata(requestTenant, input.AudioAssetID)
		if metadataError != nil {
			return dictatorProtocolRequest{}, metadataError
		}
		reader, resolveError := adapter.assets.resolve(requestTenant, input.AudioAssetID, metadata.MIMEType)
		if resolveError != nil {
			return dictatorProtocolRequest{}, resolveError
		}
		data, readError := io.ReadAll(reader.file)
		closeError := reader.Close()
		if readError != nil || closeError != nil {
			return dictatorProtocolRequest{}, errAssetStore
		}
		protocolRequest.Assets = append(protocolRequest.Assets, dictatorProtocolAsset{AssetID: input.AudioAssetID, MIMEType: metadata.MIMEType, Data: data})
	}
	if input.VoiceID != "" {
		voice, voiceError := adapter.store.providerMediaVoice(requestContext, request.TenantID, input.VoiceID)
		if voiceError != nil || voice.Provider != ProviderNameDictator {
			return dictatorProtocolRequest{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound)
		}
		protocolRequest.Voice = &voice
	}
	return protocolRequest, nil
}

func validateDictatorOperation(capability string, rawInput json.RawMessage, rawControls json.RawMessage) (dictatorCanonicalInput, dictatorCanonicalControls, error) {
	var input dictatorCanonicalInput
	var controls dictatorCanonicalControls
	if decodeError := decodeExactDictatorJSON(rawInput, &input); decodeError != nil {
		return input, controls, errMediaOperationInvalid
	}
	if decodeError := decodeExactDictatorJSON(rawControls, &controls); decodeError != nil {
		return input, controls, errMediaOperationInvalid
	}
	input.AudioAssetID = strings.TrimSpace(input.AudioAssetID)
	input.Transcript = strings.TrimSpace(input.Transcript)
	input.Text = strings.TrimSpace(input.Text)
	input.VoiceID = strings.TrimSpace(input.VoiceID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Language = strings.TrimSpace(input.Language)
	controls.Language = strings.TrimSpace(controls.Language)
	controls.Granularity = strings.TrimSpace(controls.Granularity)
	controls.ModelSize = strings.TrimSpace(controls.ModelSize)
	controls.TextFormat = strings.TrimSpace(controls.TextFormat)
	validAudio := assetIdentifierPattern.MatchString(input.AudioAssetID)
	switch capability {
	case llmproxycontract.MediaCapabilityAudioTranscribe:
		if !validAudio || !dictatorInputOnlyAudio(input) || !validDictatorLanguageSelection(controls) || !dictatorControlsOnlyLanguage(controls) {
			return input, controls, errMediaOperationInvalid
		}
	case llmproxycontract.MediaCapabilityAudioDiarize:
		if !validAudio || !dictatorInputOnlyAudio(input) || !validDictatorLanguageSelection(controls) || controls.ModelSize == "" || controls.UtteranceGapSeconds < 0 || controls.RemovePunctuation || controls.Granularity != "" || controls.GroupSize != 0 || controls.TextFormat != "" || controls.SampleRateHz != 0 || controls.MaxDurationSeconds != 0 || controls.IncludeTimeline {
			return input, controls, errMediaOperationInvalid
		}
	case llmproxycontract.MediaCapabilityAudioAlign:
		if !validAudio || input.Transcript == "" || input.Text != "" || input.VoiceID != "" || input.DisplayName != "" || input.Language != "" || controls.DetectLanguage || controls.Language == "" || controls.Granularity != "" || controls.GroupSize != 0 || controls.ModelSize != "" || controls.UtteranceGapSeconds != 0 || controls.TextFormat != "" || controls.SampleRateHz != 0 || controls.MaxDurationSeconds != 0 || controls.IncludeTimeline {
			return input, controls, errMediaOperationInvalid
		}
	case llmproxycontract.MediaCapabilitySubtitlesCreate:
		if !validAudio || input.Text != "" || input.VoiceID != "" || input.DisplayName != "" || input.Language != "" || !validDictatorLanguageSelection(controls) || (controls.Granularity != "word" && controls.Granularity != "sentence") || controls.GroupSize <= 0 || controls.RemovePunctuation || controls.ModelSize != "" || controls.UtteranceGapSeconds != 0 || controls.TextFormat != "" || controls.SampleRateHz != 0 || controls.MaxDurationSeconds != 0 || controls.IncludeTimeline {
			return input, controls, errMediaOperationInvalid
		}
	case llmproxycontract.MediaCapabilityAudioSpeechGenerate:
		if input.AudioAssetID != "" || input.Transcript != "" || input.Text == "" || !mediaVoiceIdentifierPattern.MatchString(input.VoiceID) || input.DisplayName != "" || input.Language != "" || (controls.TextFormat != "plain" && controls.TextFormat != "ssml") || (controls.SampleRateHz != 24000 && controls.SampleRateHz != 48000) || controls.MaxDurationSeconds < 0 || controls.Language == "" || controls.DetectLanguage || controls.RemovePunctuation || controls.Granularity != "" || controls.GroupSize != 0 || controls.ModelSize != "" || controls.UtteranceGapSeconds != 0 {
			return input, controls, errMediaOperationInvalid
		}
	case llmproxycontract.MediaCapabilityAudioVoiceExtract:
		if !validAudio || input.Transcript == "" || input.Text != "" || input.VoiceID != "" || input.DisplayName == "" || input.Language == "" || controls.Language != "" || controls.DetectLanguage || controls.ModelSize == "" || controls.RemovePunctuation || controls.Granularity != "" || controls.GroupSize != 0 || controls.UtteranceGapSeconds != 0 || controls.TextFormat != "" || controls.SampleRateHz != 0 || controls.MaxDurationSeconds != 0 || controls.IncludeTimeline {
			return input, controls, errMediaOperationInvalid
		}
	default:
		return input, controls, errMediaOperationInvalid
	}
	return input, controls, nil
}

func dictatorInputOnlyAudio(input dictatorCanonicalInput) bool {
	return input.Transcript == "" && input.Text == "" && input.VoiceID == "" && input.DisplayName == "" && input.Language == ""
}

func dictatorControlsOnlyLanguage(controls dictatorCanonicalControls) bool {
	return !controls.RemovePunctuation && controls.Granularity == "" && controls.GroupSize == 0 && controls.ModelSize == "" && controls.UtteranceGapSeconds == 0 && controls.TextFormat == "" && controls.SampleRateHz == 0 && controls.MaxDurationSeconds == 0 && !controls.IncludeTimeline
}

func validDictatorLanguageSelection(controls dictatorCanonicalControls) bool {
	return (controls.Language != "") != controls.DetectLanguage
}

func decodeExactDictatorJSON(document []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(destination); decodeError != nil {
		return decodeError
	}
	if decodeError := decoder.Decode(&struct{}{}); !errors.Is(decodeError, io.EOF) {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}

func canonicalDictatorProviderHandle(handle dictatorProviderHandle) (dictatorProviderHandle, error) {
	handle.JobID = strings.TrimSpace(handle.JobID)
	if handle.Version != dictatorProviderHandleVersion || handle.JobID == "" {
		return dictatorProviderHandle{}, errors.New("invalid Dictator provider handle")
	}
	for index := range handle.SourceArtifactIDs {
		handle.SourceArtifactIDs[index] = strings.TrimSpace(handle.SourceArtifactIDs[index])
		if handle.SourceArtifactIDs[index] == "" {
			return dictatorProviderHandle{}, errors.New("invalid Dictator source artifact")
		}
	}
	for index := range handle.ResultArtifactIDs {
		handle.ResultArtifactIDs[index] = strings.TrimSpace(handle.ResultArtifactIDs[index])
		if handle.ResultArtifactIDs[index] == "" {
			return dictatorProviderHandle{}, errors.New("invalid Dictator result artifact")
		}
	}
	return handle, nil
}

func decodeDictatorProviderHandle(encoded string) (dictatorProviderHandle, error) {
	var handle dictatorProviderHandle
	if decodeError := decodeExactDictatorJSON([]byte(encoded), &handle); decodeError != nil {
		return dictatorProviderHandle{}, decodeError
	}
	return canonicalDictatorProviderHandle(handle)
}

func encodedDictatorProviderHandle(handle dictatorProviderHandle) string {
	canonical, canonicalError := canonicalDictatorProviderHandle(handle)
	if canonicalError != nil {
		return ""
	}
	encoded, _ := json.Marshal(canonical)
	return string(encoded)
}
