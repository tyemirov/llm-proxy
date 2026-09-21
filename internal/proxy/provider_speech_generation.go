package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// CatalogProtocolElevenLabsSpeech selects native speech generation.
const CatalogProtocolElevenLabsSpeech = "elevenlabs_speech"

const (
	speechRateNative                   = "native_speed"
	speechRateTextPacing               = "text_pacing"
	speechControlSpeed                 = "speed"
	speechControlTimestamps            = "timestamps"
	speechControlNormalization         = "text_normalization"
	speechControlLanguageNormalization = "language_text_normalization"
	speechControlLanguage              = "language_code"
	speechTextLimit                    = "input_text_characters"
	speechDictionaryLimit              = "pronunciation_dictionaries"
	speechContextLimit                 = "continuity_operations"
)

type speechDictionaryReference struct {
	DictionaryID string `json:"dictionary_id"`
	VersionID    string `json:"version_id"`
}

type providerGenerationInput struct {
	Text               string                      `json:"text"`
	VoiceID            string                      `json:"voice_id"`
	PreviousText       string                      `json:"previous_text,omitempty"`
	NextText           string                      `json:"next_text,omitempty"`
	PreviousOperations []string                    `json:"previous_operation_ids,omitempty"`
	NextOperations     []string                    `json:"next_operation_ids,omitempty"`
	Dictionaries       []speechDictionaryReference `json:"dictionaries,omitempty"`
}

type providerGenerationSettings struct {
	providerVoiceSettings
	Speed *float64 `json:"speed,omitempty"`
}

type providerGenerationControls struct {
	providerGenerationSettings
	OutputFormat          string  `json:"output_format"`
	Timestamps            *bool   `json:"timestamps"`
	Seed                  *uint64 `json:"seed,omitempty"`
	LanguageCode          string  `json:"language_code,omitempty"`
	Normalization         string  `json:"text_normalization,omitempty"`
	LanguageNormalization *bool   `json:"language_text_normalization,omitempty"`
}

type nativeSpeechDictionary struct {
	DictionaryID string `json:"pronunciation_dictionary_id"`
	VersionID    string `json:"version_id"`
}

type nativeGenerationInput struct {
	Text                  string                      `json:"text"`
	Model                 string                      `json:"model_id"`
	LanguageCode          string                      `json:"language_code,omitempty"`
	Settings              *providerGenerationSettings `json:"voice_settings,omitempty"`
	Seed                  *uint64                     `json:"seed,omitempty"`
	PreviousText          string                      `json:"previous_text,omitempty"`
	NextText              string                      `json:"next_text,omitempty"`
	PreviousRequests      []string                    `json:"previous_request_ids,omitempty"`
	NextRequests          []string                    `json:"next_request_ids,omitempty"`
	Dictionaries          []nativeSpeechDictionary    `json:"pronunciation_dictionary_locators,omitempty"`
	Normalization         string                      `json:"apply_text_normalization,omitempty"`
	LanguageNormalization *bool                       `json:"apply_language_text_normalization,omitempty"`
}

type providerGenerationAdapter struct {
	*providerSpeechAdapter
	limits            map[string]int
	rateMode          string
	dictionaryBinding string
}

func newProviderGenerationAdapter(offering ProviderOffering, provider providerDefinition, tenants *managedTenantStore, store *mediaOperationStore, assets *tenantAssetStore, revision string) *providerGenerationAdapter {
	adapter := &providerGenerationAdapter{providerSpeechAdapter: newProviderSpeechAdapter(offering, provider, tenants, store, assets, revision, ModelOperationSpeechGeneration, CatalogProtocolElevenLabsSpeech), limits: map[string]int{}, rateMode: provider.transports[offering.Transport].protocolParameters.SpeechRate}
	for _, limit := range offering.Limits {
		adapter.limits[limit.ID] = *limit.Value
	}
	for _, service := range provider.services {
		if service.Operation == ModelOperationPronunciationDictionaryCreation {
			adapter.dictionaryBinding = providerServiceBinding(service, provider, CatalogProtocolElevenLabsDictionary)
		}
	}
	return adapter
}

func (adapter *providerGenerationAdapter) Validate(ctx context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	var input providerGenerationInput
	var controls providerGenerationControls
	if !utf8.Valid(request.Input) || !decodeImageRequestObject(request.Input, &input) || !decodeImageRequestObject(request.Controls, &controls) || controls.Timestamps == nil || !slices.Contains(adapter.controls[speechControlOutputFormat].Values, controls.OutputFormat) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	input.Text = strings.TrimSpace(input.Text)
	input.PreviousText = strings.TrimSpace(input.PreviousText)
	input.NextText = strings.TrimSpace(input.NextText)
	if input.Text == "" || len(input.Dictionaries) > adapter.limits[speechDictionaryLimit] || len(input.PreviousOperations) > adapter.limits[speechContextLimit] || len(input.NextOperations) > adapter.limits[speechContextLimit] {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	for id, value := range map[string]*float64{speechControlStability: controls.Stability, speechControlSimilarity: controls.SimilarityBoost, speechControlStyle: controls.Style, speechControlSpeed: controls.Speed} {
		control, supported := adapter.controls[id]
		if value != nil && (!supported || *value < *control.Minimum || *value > *control.Maximum) {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
	}
	if _, supported := adapter.controls[speechControlSpeakerBoost]; controls.UseSpeakerBoost != nil && !supported {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if controls.Seed != nil && float64(*controls.Seed) > *adapter.controls[speechControlSeed].Maximum {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	for id, value := range map[string]string{speechControlLanguage: controls.LanguageCode, speechControlNormalization: controls.Normalization} {
		if value != "" && !slices.Contains(adapter.controls[id].Values, value) {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
	}
	_, prefix := adapter.pacedText(input.Text, controls.Speed)
	if utf8.RuneCountInString(input.Text)+utf8.RuneCountInString(prefix) > adapter.limits[speechTextLimit] || utf8.RuneCountInString(input.PreviousText) > adapter.limits[speechTextLimit] || utf8.RuneCountInString(input.NextText) > adapter.limits[speechTextLimit] {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if _, _, err := adapter.nativeInput(ctx, request.TenantID, request.CredentialReference, input, controls); err != nil {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	inputJSON, _ := json.Marshal(input)
	controlsJSON, _ := json.Marshal(controls)
	return MediaOperationValidatedRequest{Input: inputJSON, Controls: controlsJSON, ExecutionBinding: adapter.binding}, nil
}

func (adapter *providerGenerationAdapter) nativeInput(ctx context.Context, tenantID, credential string, input providerGenerationInput, controls providerGenerationControls) (string, nativeGenerationInput, error) {
	voice, err := adapter.nativeVoice(ctx, tenantID, credential, input.VoiceID)
	if err != nil {
		return "", nativeGenerationInput{}, err
	}
	payload := nativeGenerationInput{Text: input.Text, Model: adapter.offering.ProviderModel, LanguageCode: controls.LanguageCode, Seed: controls.Seed, PreviousText: input.PreviousText, NextText: input.NextText, Normalization: controls.Normalization, LanguageNormalization: controls.LanguageNormalization}
	payload.Text, _ = adapter.pacedText(input.Text, controls.Speed)
	settings := controls.providerGenerationSettings
	if adapter.rateMode == speechRateTextPacing {
		settings.Speed = nil
	}
	encoded, _ := json.Marshal(settings)
	if string(encoded) != "{}" {
		payload.Settings = &settings
	}
	for _, reference := range input.Dictionaries {
		var record mediaDictionaryRecord
		if adapter.dictionaryBinding == "" || adapter.store.database.WithContext(ctx).Where("tenant_id = ? AND dictionary_id = ? AND version_id = ? AND provider = ? AND credential_reference = ? AND execution_binding = ?", tenantID, reference.DictionaryID, reference.VersionID, adapter.offering.Provider, credential, adapter.dictionaryBinding).First(&record).Error != nil {
			return "", nativeGenerationInput{}, errMediaOperationInvalid
		}
		payload.Dictionaries = append(payload.Dictionaries, nativeSpeechDictionary{DictionaryID: record.NativeDictionaryID, VersionID: record.NativeVersionID})
	}
	resolve := func(operations []string) ([]string, error) {
		requests := make([]string, 0, len(operations))
		for _, operationID := range operations {
			var record mediaOperationRecord
			if adapter.store.database.WithContext(ctx).Where("tenant_id = ? AND operation_id = ? AND provider = ? AND credential_reference = ? AND catalog_revision = ? AND capability = ? AND public_state = ?", tenantID, operationID, adapter.offering.Provider, credential, adapter.revision, llmproxycontract.MediaCapabilityAudioSpeechGenerate, MediaOperationStateSucceeded).First(&record).Error != nil {
				return nil, errMediaOperationInvalid
			}
			var receipt providerSpeechReceipt
			if json.Unmarshal([]byte(record.ProviderHandle), &receipt) != nil || receipt.RequestID == "" {
				return nil, errMediaOperationInvalid
			}
			requests = append(requests, receipt.RequestID)
		}
		return requests, nil
	}
	payload.PreviousRequests, err = resolve(input.PreviousOperations)
	if err != nil {
		return "", nativeGenerationInput{}, err
	}
	payload.NextRequests, err = resolve(input.NextOperations)
	if err != nil {
		return "", nativeGenerationInput{}, err
	}
	return voice, payload, nil
}

var speechPacingRules = []struct {
	maximum float64
	tag     string
}{
	{0.76, "[drawn out and extremely slowly]"}, {0.82, "[extremely slowly]"}, {0.86, "[very slowly]"}, {0.90, "[deliberately slowly]"}, {0.94, "[slowly]"}, {1.00, "[slightly slower]"}, {1.04, "[slightly faster]"}, {1.08, "[quickly]"}, {1.12, "[very quickly]"}, {1.16, "[extremely quickly]"}, {1.20, "[rapid-fire]"},
}

func (adapter *providerGenerationAdapter) pacedText(text string, speed *float64) (string, string) {
	if adapter.rateMode == speechRateNative || speed == nil || *speed == 1 {
		return text, ""
	}
	index := 0
	for *speed > speechPacingRules[index].maximum {
		index++
	}
	prefix := speechPacingRules[index].tag + " "
	return prefix + text, prefix
}

func (adapter *providerGenerationAdapter) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, err := resolveMediaProvider(ctx, request, adapter.offering.Provider, adapter.offering.Transport, adapter.provider, adapter.tenants, adapter.store)
	if err != nil || request.ExecutionBinding != adapter.binding {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	var input providerGenerationInput
	var controls providerGenerationControls
	_ = json.Unmarshal(request.Input, &input)
	_ = json.Unmarshal(request.Controls, &controls)
	voice, payload, err := adapter.nativeInput(ctx, request.TenantID, request.CredentialReference, input, controls)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationInvalid.Error()}
	}
	target, _ := url.Parse(provider.textEndpointURL)
	target.RawPath = strings.TrimRight(target.EscapedPath(), "/") + "/" + url.PathEscape(voice)
	if *controls.Timestamps {
		target.RawPath += "/with-timestamps"
	}
	target.Path, _ = url.PathUnescape(target.RawPath)
	query := target.Query()
	query.Set("output_format", controls.OutputFormat)
	target.RawQuery = query.Encode()
	body, _ := json.Marshal(payload)
	native, _ := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	native.Header.Set("Content-Type", "application/json")
	authorizeQueueRequest(native, provider)
	response, err := request.HTTP.Submission.Do(native)
	if err != nil {
		return imageSubmissionFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusRequestTimeout {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	if response.StatusCode != http.StatusOK {
		return imageGenerationUncertain()
	}
	receipt, _ := json.Marshal(providerSpeechReceipt{RequestID: response.Header.Get("request-id"), HistoryItemID: response.Header.Get("history-item-id")})
	if err := request.PersistProviderHandle(string(receipt)); err != nil {
		return imageGenerationUncertain()
	}
	maximum := adapter.assets.maxAssetBytes
	if *controls.Timestamps {
		maximum = 2*maximum + int64(adapter.limits[speechTextLimit])*128
	}
	contentType, _, contentError := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if contentError != nil || (*controls.Timestamps && contentType != "application/json") || (!*controls.Timestamps && !strings.HasPrefix(contentType, "audio/") && contentType != "application/octet-stream") {
		return imageGenerationUncertain()
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil || len(data) == 0 || int64(len(data)) > maximum {
		return imageGenerationUncertain()
	}
	var timingJSON []byte
	if *controls.Timestamps {
		var timed struct {
			Audio string `json:"audio_base64"`
			llmproxycontract.MediaSpeechTiming
		}
		if json.Unmarshal(data, &timed) != nil {
			return imageGenerationUncertain()
		}
		data, err = base64.StdEncoding.DecodeString(timed.Audio)
		if err != nil || len(data) == 0 || int64(len(data)) > adapter.assets.maxAssetBytes {
			return imageGenerationUncertain()
		}
		_, prefix := adapter.pacedText(input.Text, controls.Speed)
		if !validateSpeechAlignment(timed.Alignment, prefix) || !validateSpeechAlignment(timed.NormalizedAlignment, prefix) {
			return imageGenerationUncertain()
		}
		timingJSON, _ = json.Marshal(timed.MediaSpeechTiming)
	}
	description := providerSpeechFormats[controls.OutputFormat]
	if description.RawAudio != nil && description.RawAudio.Encoding == "s16le" && len(data)%2 != 0 {
		return imageGenerationUncertain()
	}
	description.OutputFormat = controls.OutputFormat
	encoded, _ := json.Marshal(description)
	outputs := []MediaOperationOutput{{MIMEType: description.MIMEType, Data: data}, {MIMEType: "application/json", Data: encoded}}
	if *controls.Timestamps {
		outputs = append(outputs, MediaOperationOutput{MIMEType: "application/json", Data: timingJSON})
	}
	return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: outputs}
}

func validateSpeechAlignment(alignment *llmproxycontract.SpeechCharacterAlignment, prefix string) bool {
	if alignment == nil {
		return true
	}
	if alignment.Characters == nil || alignment.Starts == nil || alignment.Ends == nil || len(alignment.Characters) != len(alignment.Starts) || len(alignment.Characters) != len(alignment.Ends) {
		return false
	}
	for index, character := range alignment.Characters {
		if character == "" || alignment.Starts[index] < 0 || alignment.Ends[index] < alignment.Starts[index] || (index > 0 && alignment.Starts[index] < alignment.Starts[index-1]) {
			return false
		}
	}
	count := utf8.RuneCountInString(prefix)
	if len(alignment.Characters) < count || strings.Join(alignment.Characters[:count], "") != prefix {
		return false
	}
	alignment.Characters = alignment.Characters[count:]
	alignment.Starts = alignment.Starts[count:]
	alignment.Ends = alignment.Ends[count:]
	return true
}

func (*providerGenerationAdapter) Recover(context.Context, MediaOperationExecutionRequest) MediaOperationExecutionResult {
	return imageGenerationUncertain()
}
func (*providerGenerationAdapter) Cancel(context.Context, MediaOperationExecutionRequest) MediaOperationCancellationResult {
	return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
}
