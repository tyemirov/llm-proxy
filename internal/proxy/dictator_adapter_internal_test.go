package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

type controlledDictatorProtocol struct {
	submitRequest dictatorProtocolRequest
	submit        dictatorProtocolObservation
	submitError   error
	observations  []dictatorProtocolObservation
	observeError  error
	cancelled     bool
	cancelError   error
	voices        []MediaVoiceProviderRecord
	voicesError   error
}

func (protocol *controlledDictatorProtocol) Submit(_ context.Context, request dictatorProtocolRequest) (dictatorProtocolObservation, error) {
	protocol.submitRequest = request
	return protocol.submit, protocol.submitError
}

func (protocol *controlledDictatorProtocol) Observe(context.Context, string, dictatorProviderHandle) (dictatorProtocolObservation, error) {
	if protocol.observeError != nil {
		return dictatorProtocolObservation{}, protocol.observeError
	}
	observation := protocol.observations[0]
	protocol.observations = protocol.observations[1:]
	return observation, nil
}

func (protocol *controlledDictatorProtocol) Cancel(context.Context, string, dictatorProviderHandle) (bool, error) {
	return protocol.cancelled, protocol.cancelError
}

func (protocol *controlledDictatorProtocol) DiscoverVoices(context.Context) ([]MediaVoiceProviderRecord, error) {
	return protocol.voices, protocol.voicesError
}

func TestDictatorAdapterValidatesEveryRetainedCapability(t *testing.T) {
	fixture := newMediaOperationInternalFixture(t)
	adapter, adapterError := newDictatorMediaOperationAdapter(&controlledDictatorProtocol{}, fixture.service.assets, fixture.service.store, "deployment:dictator:test", time.Millisecond)
	if adapterError != nil {
		t.Fatal(adapterError)
	}
	assetID := "ast_0123456789abcdef0123456789abcdef"
	voiceID := "voi_0123456789abcdef0123456789abcdef"
	testCases := []struct {
		capability string
		input      string
		controls   string
	}{
		{llmproxycontract.MediaCapabilityAudioTranscribe, `{"audio_asset_id":"` + assetID + `"}`, `{"detect_language":true}`},
		{llmproxycontract.MediaCapabilityAudioDiarize, `{"audio_asset_id":"` + assetID + `"}`, `{"language":"en","model_size":"medium","utterance_gap_seconds":0.5}`},
		{llmproxycontract.MediaCapabilityAudioAlign, `{"audio_asset_id":"` + assetID + `","transcript":"Hello world."}`, `{"language":"en","remove_punctuation":true}`},
		{llmproxycontract.MediaCapabilitySubtitlesCreate, `{"audio_asset_id":"` + assetID + `"}`, `{"detect_language":true,"granularity":"word","group_size":2}`},
		{llmproxycontract.MediaCapabilityAudioSpeechGenerate, `{"text":"Hello world.","voice_id":"` + voiceID + `"}`, `{"language":"en","text_format":"plain","sample_rate_hz":24000,"include_timeline":true}`},
		{llmproxycontract.MediaCapabilityAudioVoiceExtract, `{"audio_asset_id":"` + assetID + `","transcript":"Hello world.","display_name":"Narrator","language":"en"}`, `{"model_size":"medium"}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.capability, func(t *testing.T) {
			validated, validationError := adapter.Validate(MediaOperationAdapterRequest{Capability: testCase.capability, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(testCase.input), Controls: json.RawMessage(testCase.controls)})
			if validationError != nil || validated.Input == nil || validated.Controls == nil {
				t.Fatalf("validated=%+v error=%v", validated, validationError)
			}
			expectedAssets := []string{}
			if testCase.capability != llmproxycontract.MediaCapabilityAudioSpeechGenerate {
				expectedAssets = []string{assetID}
			}
			if !reflect.DeepEqual(validated.InputAssetIDs, expectedAssets) {
				t.Fatalf("input assets=%v want=%v", validated.InputAssetIDs, expectedAssets)
			}
		})
	}

	invalidRequests := []MediaOperationAdapterRequest{
		{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + assetID + `"}`), Controls: json.RawMessage(`{`)},
		{Capability: "future", Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`)},
		{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, Provider: ProviderNameOpenAI, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + assetID + `"}`), Controls: json.RawMessage(`{"detect_language":true}`)},
		{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + assetID + `","future":true}`), Controls: json.RawMessage(`{"detect_language":true}`)},
		{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + assetID + `"}`), Controls: json.RawMessage(`{"detect_language":true,"text_format":"plain"}`)},
		{Capability: llmproxycontract.MediaCapabilityAudioSpeechGenerate, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"text":"hello","voice_id":"` + voiceID + `"}`), Controls: json.RawMessage(`{"language":"en","text_format":"ssml","sample_rate_hz":16000}`)},
	}
	for invalidIndex, request := range invalidRequests {
		if _, validationError := adapter.Validate(request); !errors.Is(validationError, errMediaOperationInvalid) {
			t.Fatalf("invalid request %d error=%v", invalidIndex, validationError)
		}
	}
}

func TestDictatorAdapterPersistsPrivateHandleAndRecovers(t *testing.T) {
	fixture := newMediaOperationInternalFixture(t)
	asset, uploadError := fixture.service.assets.upload(fixture.tenant, "audio/wav", bytesReader("audio"))
	if uploadError != nil {
		t.Fatal(uploadError)
	}
	handle := dictatorProviderHandle{Version: dictatorProviderHandleVersion, JobID: "native-job", SourceArtifactIDs: []string{"native-source"}}
	completedHandle := dictatorProviderHandle{Version: dictatorProviderHandleVersion, JobID: "native-job", SourceArtifactIDs: []string{"native-source"}, ResultArtifactIDs: []string{"native-result"}}
	protocol := &controlledDictatorProtocol{
		submit:       dictatorProtocolObservation{State: dictatorProtocolStateRunning, Handle: handle},
		observations: []dictatorProtocolObservation{{State: dictatorProtocolStateSucceeded, Handle: completedHandle, Outputs: []MediaOperationOutput{{MIMEType: "application/json", Data: []byte(`{"transcript":"hello"}`)}}}},
	}
	adapter, adapterError := newDictatorMediaOperationAdapter(protocol, fixture.service.assets, fixture.service.store, "deployment:dictator:test", time.Millisecond)
	if adapterError != nil {
		t.Fatal(adapterError)
	}
	validated, validationError := adapter.Validate(MediaOperationAdapterRequest{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + asset.AssetID + `"}`), Controls: json.RawMessage(`{"detect_language":true}`)})
	if validationError != nil {
		t.Fatal(validationError)
	}
	var persistedHandle string
	result := adapter.Execute(context.Background(), MediaOperationExecutionRequest{
		TenantID: fixture.tenant.identifier.string(), DispatchToken: "dispatch", Capability: llmproxycontract.MediaCapabilityAudioTranscribe,
		Input: validated.Input, Controls: validated.Controls, PersistProviderHandle: func(value string) error { persistedHandle = value; return nil },
	})
	if result.State != MediaOperationStateSucceeded || persistedHandle == "" || !reflect.DeepEqual(protocol.submitRequest.Assets[0].Data, []byte("audio")) || protocol.submitRequest.DispatchToken != "dispatch" {
		t.Fatalf("result=%+v persisted=%q request=%+v", result, persistedHandle, protocol.submitRequest)
	}
	if privateHandle, decodeError := decodeDictatorProviderHandle(result.ProviderHandle); decodeError != nil || privateHandle.JobID != "native-job" || !reflect.DeepEqual(privateHandle.ResultArtifactIDs, []string{"native-result"}) {
		t.Fatalf("private handle=%+v error=%v", privateHandle, decodeError)
	}

	protocol.observations = []dictatorProtocolObservation{{State: dictatorProtocolStateFailed, Handle: completedHandle}}
	recovered := adapter.Recover(context.Background(), MediaOperationExecutionRequest{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, ProviderHandle: persistedHandle})
	if recovered.State != MediaOperationStateFailed || recovered.ErrorCode != "provider_error" {
		t.Fatalf("recovered=%+v", recovered)
	}
	protocol.cancelled = true
	if cancellation := adapter.Cancel(context.Background(), MediaOperationExecutionRequest{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, ProviderHandle: persistedHandle}); cancellation.State != MediaCancellationConfirmed {
		t.Fatalf("cancellation=%+v", cancellation)
	}
}

func TestDictatorVoiceExtractionPublishesOnlyGatewayVoiceIdentity(t *testing.T) {
	fixture := newMediaOperationInternalFixture(t)
	fixture.service.assets.maxAssetBytes = 1024
	record := fixture.record(MediaOperationStateRunning, MediaProviderExecutionDispatched)
	record.Capability = llmproxycontract.MediaCapabilityAudioVoiceExtract
	record.CatalogOperation = ModelOperationVoiceExtraction
	record.Provider = ProviderNameDictator
	record.Model = ModelNameDictatorSpeechV1
	if saveError := fixture.database.Save(&record).Error; saveError != nil {
		t.Fatal(saveError)
	}
	if claimError := fixture.database.Create(&mediaOperationClaimRecord{OperationID: record.OperationID, WorkerID: "worker", Generation: 1, ExpiresAt: fixture.now.Add(time.Minute)}).Error; claimError != nil {
		t.Fatal(claimError)
	}
	fixture.service.finish(record.OperationID, 1, MediaOperationExecutionResult{
		State: MediaOperationStateSucceeded,
		Voice: &MediaVoiceProviderRecord{
			Provider: ProviderNameDictator, Model: "qwen3", Mode: MediaVoiceModeExtracted, Language: "en", DisplayName: "Narrator",
			SampleRates: []int{48000, 24000}, DefaultSampleRate: 24000, ProviderVoiceReference: "native-speaker-artifact",
		},
	})
	response, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), record.OperationID)
	if responseError != nil || response.State != MediaOperationStateSucceeded || len(response.Outputs) != 1 || response.Outputs[0].MIMEType != "application/json" {
		t.Fatalf("response=%+v error=%v", response, responseError)
	}
	reader, resolveError := fixture.service.assets.resolve(fixture.tenant, response.Outputs[0].AssetID, "application/json")
	if resolveError != nil {
		t.Fatal(resolveError)
	}
	publicBytes, readError := io.ReadAll(reader.file)
	_ = reader.Close()
	if readError != nil || strings.Contains(string(publicBytes), "native-speaker-artifact") || !strings.Contains(string(publicBytes), `"voice_id":"voi_`) {
		t.Fatalf("public voice=%s error=%v", publicBytes, readError)
	}
	var privateVoice mediaVoiceRecord
	if queryError := fixture.database.Where("tenant_id = ? AND provider = ?", fixture.tenant.identifier.string(), ProviderNameDictator).First(&privateVoice).Error; queryError != nil || privateVoice.ProviderVoiceReference != "native-speaker-artifact" {
		t.Fatalf("private voice=%+v error=%v", privateVoice, queryError)
	}
}

func TestDictatorControlledProtocolThroughPublicOperationHandlers(t *testing.T) {
	fixture := newMediaOperationInternalFixture(t)
	fixture.service.claimRenewal = time.Hour
	fixture.service.assets.maxAssetBytes = 1024
	asset, uploadError := fixture.service.assets.upload(fixture.tenant, "audio/wav", bytesReader("audio"))
	if uploadError != nil {
		t.Fatal(uploadError)
	}
	handle := dictatorProviderHandle{Version: dictatorProviderHandleVersion, JobID: "native-job", SourceArtifactIDs: []string{"native-source"}, ResultArtifactIDs: []string{"native-result"}}
	protocol := &controlledDictatorProtocol{submit: dictatorProtocolObservation{
		State: dictatorProtocolStateSucceeded, Handle: handle,
		Outputs: []MediaOperationOutput{{MIMEType: "application/json", Data: []byte(`{"transcript":"hello","language":"en","words":[{"text":"hello","start":0,"end":1}]}`)}},
	}}
	providerCatalog := internalCanonicalProviderCatalog()
	connectionValues := map[string]map[string]string{ProviderNameDictator: {"grpc_address": "dictator.internal:50051", "grpc_auth_token": "private-token", "grpc_tls": "true"}}
	fixture.service.catalog, _ = NewCatalogService(providerCatalog.ModelCatalog())
	fixture.service.providers = newProviderRegistry(Configuration{ProviderCatalog: providerCatalog, ProviderConnectionValues: connectionValues, Endpoints: NewEndpoints()})
	if registrationError := fixture.service.registerDictatorAdapter(Configuration{ProviderConnectionValues: connectionValues, dictatorProtocol: protocol, dictatorPollInterval: time.Millisecond}); registrationError != nil {
		t.Fatal(registrationError)
	}

	capabilitiesResponse := httptest.NewRecorder()
	capabilitiesContext, _ := gin.CreateTestContext(capabilitiesResponse)
	capabilitiesContext.Request = httptest.NewRequest(http.MethodGet, llmproxycontract.MediaCapabilitiesPath, nil)
	capabilitiesContext.Set(contextKeyTenant, fixture.tenant)
	fixture.service.capabilitiesHandler()(capabilitiesContext)
	if capabilitiesResponse.Code != http.StatusOK || !strings.Contains(capabilitiesResponse.Body.String(), `"capability":"audio.transcribe"`) || !strings.Contains(capabilitiesResponse.Body.String(), `"provider":"dictator"`) {
		t.Fatalf("capabilities status=%d body=%s", capabilitiesResponse.Code, capabilitiesResponse.Body.String())
	}

	createBody := `{"capability":"audio.transcribe","provider":"dictator","model":"dictator-speech-v1","input":{"audio_asset_id":"` + asset.AssetID + `"},"controls":{"detect_language":true}}`
	createResponse := httptest.NewRecorder()
	createContext, _ := gin.CreateTestContext(createResponse)
	createContext.Request = httptest.NewRequest(http.MethodPost, llmproxycontract.MediaOperationsPath, strings.NewReader(createBody))
	createContext.Request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "dictator-controlled")
	createContext.Set(contextKeyTenant, fixture.tenant)
	fixture.service.createHandler()(createContext)
	if createResponse.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	var accepted mediaOperationResponse
	if decodeError := json.Unmarshal(createResponse.Body.Bytes(), &accepted); decodeError != nil {
		t.Fatal(decodeError)
	}
	if deadlineUpdateError := fixture.database.Model(&mediaOperationRecord{}).Where("operation_id = ?", accepted.OperationID).Update("deadline_at", time.Now().UTC().Add(time.Hour)).Error; deadlineUpdateError != nil {
		t.Fatal(deadlineUpdateError)
	}
	fixture.service.runOperation("dictator-worker", accepted.OperationID)
	completed, completedError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), accepted.OperationID)
	if completedError != nil || completed.State != MediaOperationStateSucceeded || len(completed.Outputs) != 1 {
		t.Fatalf("completed=%+v error=%v", completed, completedError)
	}
	var privateRecord mediaOperationRecord
	if queryError := fixture.database.First(&privateRecord, "operation_id = ?", accepted.OperationID).Error; queryError != nil || !strings.Contains(privateRecord.ProviderHandle, "native-job") || strings.Contains(createResponse.Body.String(), "native-job") {
		t.Fatalf("private handle=%q query error=%v create=%s", privateRecord.ProviderHandle, queryError, createResponse.Body.String())
	}
}

func TestDictatorAdapterFailureAndPrivacyEdges(t *testing.T) {
	fixture := newMediaOperationInternalFixture(t)
	protocol := &controlledDictatorProtocol{voices: []MediaVoiceProviderRecord{{Provider: ProviderNameDictator}}}
	adapter, adapterError := newDictatorMediaOperationAdapter(protocol, fixture.service.assets, fixture.service.store, "deployment:dictator:test", time.Millisecond)
	if adapterError != nil {
		t.Fatal(adapterError)
	}
	if _, invalidError := newDictatorMediaOperationAdapter(nil, fixture.service.assets, fixture.service.store, "deployment:dictator:test", time.Millisecond); invalidError == nil {
		t.Fatal("nil protocol accepted")
	}
	if voices, voicesError := adapter.DiscoverMediaVoices(context.Background()); voicesError != nil || len(voices) != 1 {
		t.Fatalf("voices=%v error=%v", voices, voicesError)
	}

	missingAssetRequest := MediaOperationExecutionRequest{TenantID: fixture.tenant.identifier.string(), Capability: llmproxycontract.MediaCapabilityAudioTranscribe, Input: json.RawMessage(`{"audio_asset_id":"ast_0123456789abcdef0123456789abcdef"}`), Controls: json.RawMessage(`{"detect_language":true}`)}
	if result := adapter.Execute(context.Background(), missingAssetRequest); result.State != MediaOperationStateFailed || result.ErrorCode != "provider_result_invalid" {
		t.Fatalf("missing asset result=%+v", result)
	}
	if _, requestError := adapter.protocolRequest(context.Background(), MediaOperationExecutionRequest{Input: json.RawMessage(`{`)}); requestError == nil {
		t.Fatal("malformed canonical input accepted")
	}

	asset, uploadError := fixture.service.assets.upload(fixture.tenant, "audio/wav", bytesReader("audio"))
	if uploadError != nil {
		t.Fatal(uploadError)
	}
	validRequest := missingAssetRequest
	validRequest.Input = json.RawMessage(`{"audio_asset_id":"` + asset.AssetID + `"}`)
	originalStat := assetStat
	t.Cleanup(func() { assetStat = originalStat })
	assetStat = func(*os.File) (os.FileInfo, error) { return nil, io.ErrUnexpectedEOF }
	if _, requestError := adapter.protocolRequest(context.Background(), validRequest); requestError == nil {
		t.Fatal("asset resolve failure accepted")
	}
	assetStat = originalStat
	originalSeek := assetSeek
	t.Cleanup(func() { assetSeek = originalSeek })
	assetSeek = func(file *os.File, offset int64, whence int) (int64, error) {
		position, seekError := file.Seek(offset, whence)
		if seekError != nil {
			return 0, seekError
		}
		if closeError := file.Close(); closeError != nil {
			return 0, closeError
		}
		return position, nil
	}
	if _, requestError := adapter.protocolRequest(context.Background(), validRequest); requestError == nil {
		t.Fatal("asset read failure accepted")
	}
	assetSeek = originalSeek

	protocol.submitError = io.ErrUnexpectedEOF
	if result := adapter.Execute(context.Background(), validRequest); result.State != MediaOperationStateUncertain || result.ErrorCode != "provider_outcome_unknown" {
		t.Fatalf("submit failure result=%+v", result)
	}
	protocol.submitError = nil
	protocol.submit = dictatorProtocolObservation{State: dictatorProtocolStateRunning, Handle: dictatorProviderHandle{Version: 2, JobID: "native"}}
	if result := adapter.Execute(context.Background(), validRequest); result.State != MediaOperationStateUncertain {
		t.Fatalf("invalid handle result=%+v", result)
	}
	protocol.submit = dictatorProtocolObservation{State: dictatorProtocolStateSucceeded, Handle: dictatorProviderHandle{Version: 1, JobID: "native"}}
	if result := adapter.Execute(context.Background(), validRequest); result.State != MediaOperationStateUncertain || result.ProviderHandle == "" {
		t.Fatalf("missing persistence result=%+v", result)
	}
	validRequest.PersistProviderHandle = func(string) error { return io.ErrUnexpectedEOF }
	if result := adapter.Execute(context.Background(), validRequest); result.State != MediaOperationStateUncertain || result.ProviderHandle == "" {
		t.Fatalf("persistence failure result=%+v", result)
	}

	if result := adapter.Recover(context.Background(), MediaOperationExecutionRequest{ProviderHandle: `{`}); result.State != MediaOperationStateUncertain {
		t.Fatalf("malformed recovery result=%+v", result)
	}
	protocol.observeError = io.ErrUnexpectedEOF
	encoded := encodedDictatorProviderHandle(dictatorProviderHandle{Version: 1, JobID: "native"})
	if result := adapter.Recover(context.Background(), MediaOperationExecutionRequest{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, ProviderHandle: encoded}); result.State != MediaOperationStateUncertain || result.ProviderHandle != encoded {
		t.Fatalf("recovery failure result=%+v", result)
	}
	if cancellation := adapter.Cancel(context.Background(), MediaOperationExecutionRequest{ProviderHandle: `{`}); cancellation.State != MediaCancellationUnsupported {
		t.Fatalf("malformed cancellation=%+v", cancellation)
	}
	protocol.cancelError = io.ErrUnexpectedEOF
	if cancellation := adapter.Cancel(context.Background(), MediaOperationExecutionRequest{ProviderHandle: encoded}); cancellation.State != MediaCancellationUnsupported {
		t.Fatalf("provider cancellation failure=%+v", cancellation)
	}
	protocol.cancelError = nil
	protocol.cancelled = false
	if cancellation := adapter.Cancel(context.Background(), MediaOperationExecutionRequest{ProviderHandle: encoded}); cancellation.State != MediaCancellationUnsupported {
		t.Fatalf("unconfirmed cancellation=%+v", cancellation)
	}

	protocol.observeError = nil
	if result := adapter.awaitObservation(context.Background(), llmproxycontract.MediaCapabilityAudioTranscribe, dictatorProtocolObservation{State: "future", Handle: dictatorProviderHandle{Version: 1, JobID: "native"}}); result.State != MediaOperationStateUncertain {
		t.Fatalf("future observation result=%+v", result)
	}
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	if result := adapter.awaitObservation(cancelledContext, llmproxycontract.MediaCapabilityAudioTranscribe, dictatorProtocolObservation{State: dictatorProtocolStateRunning, Handle: dictatorProviderHandle{Version: 1, JobID: "native"}}); result.ErrorCode != "operation_deadline_exceeded" {
		t.Fatalf("deadline result=%+v", result)
	}
	protocol.observeError = io.ErrUnexpectedEOF
	if result := adapter.awaitObservation(context.Background(), llmproxycontract.MediaCapabilityAudioTranscribe, dictatorProtocolObservation{State: dictatorProtocolStateRunning, Handle: dictatorProviderHandle{Version: 1, JobID: "native"}}); result.ErrorCode != "provider_outcome_unknown" {
		t.Fatalf("observe failure result=%+v", result)
	}
	if result, terminal := dictatorExecutionResult(dictatorProtocolObservation{State: dictatorProtocolStateSucceeded, Handle: dictatorProviderHandle{Version: 2, JobID: "native"}}); !terminal || result.State != MediaOperationStateUncertain {
		t.Fatalf("invalid terminal observation result=%+v terminal=%t", result, terminal)
	}
	if result, terminal := dictatorExecutionResult(dictatorProtocolObservation{State: dictatorProtocolStateCancelled, Handle: dictatorProviderHandle{Version: 1, JobID: "native"}}); !terminal || result.State != MediaOperationStateCancelled {
		t.Fatalf("cancelled observation result=%+v terminal=%t", result, terminal)
	}
}

func TestDictatorCanonicalRequestAndHandleEdges(t *testing.T) {
	assetID := "ast_0123456789abcdef0123456789abcdef"
	voiceID := "voi_0123456789abcdef0123456789abcdef"
	invalidRequests := []MediaOperationAdapterRequest{
		{Capability: llmproxycontract.MediaCapabilityAudioTranscribe, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + assetID + `","transcript":"wrong"}`), Controls: json.RawMessage(`{"detect_language":true}`)},
		{Capability: llmproxycontract.MediaCapabilityAudioDiarize, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + assetID + `"}`), Controls: json.RawMessage(`{"language":"en"}`)},
		{Capability: llmproxycontract.MediaCapabilityAudioAlign, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + assetID + `"}`), Controls: json.RawMessage(`{"language":"en"}`)},
		{Capability: llmproxycontract.MediaCapabilitySubtitlesCreate, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + assetID + `"}`), Controls: json.RawMessage(`{"language":"en","granularity":"paragraph","group_size":1}`)},
		{Capability: llmproxycontract.MediaCapabilityAudioSpeechGenerate, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"text":"hello","voice_id":"` + voiceID + `"}`), Controls: json.RawMessage(`{"language":"en","text_format":"plain","sample_rate_hz":24000,"detect_language":true}`)},
		{Capability: llmproxycontract.MediaCapabilityAudioVoiceExtract, Provider: ProviderNameDictator, Model: ModelNameDictatorSpeechV1, Input: json.RawMessage(`{"audio_asset_id":"` + assetID + `","transcript":"hello","display_name":"Narrator","language":"en"}`), Controls: json.RawMessage(`{}`)},
	}
	fixture := newMediaOperationInternalFixture(t)
	adapter, adapterError := newDictatorMediaOperationAdapter(&controlledDictatorProtocol{}, fixture.service.assets, fixture.service.store, "deployment:dictator:test", time.Millisecond)
	if adapterError != nil {
		t.Fatal(adapterError)
	}
	for index, request := range invalidRequests {
		if _, validationError := adapter.Validate(request); !errors.Is(validationError, errMediaOperationInvalid) {
			t.Fatalf("invalid request %d error=%v", index, validationError)
		}
	}
	if decodeError := decodeExactDictatorJSON([]byte(`{} {}`), &dictatorCanonicalInput{}); decodeError == nil {
		t.Fatal("trailing JSON accepted")
	}
	for _, handle := range []dictatorProviderHandle{
		{Version: 2, JobID: "native"},
		{Version: 1},
		{Version: 1, JobID: "native", SourceArtifactIDs: []string{" "}},
		{Version: 1, JobID: "native", ResultArtifactIDs: []string{" "}},
	} {
		if _, handleError := canonicalDictatorProviderHandle(handle); handleError == nil {
			t.Fatalf("invalid handle accepted: %+v", handle)
		}
	}
	if _, decodeError := decodeDictatorProviderHandle(`{"version":1,"job_id":"native","future":true}`); decodeError == nil {
		t.Fatal("unknown handle field accepted")
	}
	if encoded := encodedDictatorProviderHandle(dictatorProviderHandle{Version: 2, JobID: "native"}); encoded != "" {
		t.Fatalf("invalid handle encoded=%q", encoded)
	}
}

func TestDictatorSpeechVoiceResolutionIsTenantAndProviderBound(t *testing.T) {
	fixture := newMediaOperationInternalFixture(t)
	adapter, adapterError := newDictatorMediaOperationAdapter(&controlledDictatorProtocol{}, fixture.service.assets, fixture.service.store, "deployment:dictator:test", time.Millisecond)
	if adapterError != nil {
		t.Fatal(adapterError)
	}
	requestFor := func(voiceID string) MediaOperationExecutionRequest {
		return MediaOperationExecutionRequest{TenantID: fixture.tenant.identifier.string(), Capability: llmproxycontract.MediaCapabilityAudioSpeechGenerate, Input: json.RawMessage(`{"text":"hello","voice_id":"` + voiceID + `"}`), Controls: json.RawMessage(`{"language":"en","text_format":"plain","sample_rate_hz":24000}`)}
	}
	if _, requestError := adapter.protocolRequest(context.Background(), requestFor("voi_0123456789abcdef0123456789abcdef")); requestError == nil {
		t.Fatal("missing voice accepted")
	}
	for _, provider := range []string{ProviderNameXAI, ProviderNameDictator} {
		voice := MediaVoiceProviderRecord{Provider: provider, Model: "model", Mode: MediaVoiceModePreset, Language: "en", DisplayName: provider, SampleRates: []int{24000}, DefaultSampleRate: 24000, ProviderVoiceReference: "native-" + provider}
		if persistError := fixture.service.store.persistMediaVoices(context.Background(), fixture.tenant.identifier.string(), provider, []MediaVoiceProviderRecord{voice}); persistError != nil {
			t.Fatal(persistError)
		}
		voices, listError := fixture.service.store.listMediaVoices(context.Background(), fixture.tenant.identifier.string(), provider)
		if listError != nil {
			t.Fatal(listError)
		}
		protocolRequest, requestError := adapter.protocolRequest(context.Background(), requestFor(voices[0].VoiceID))
		if provider == ProviderNameXAI && requestError == nil {
			t.Fatal("voice from another provider accepted")
		}
		if provider == ProviderNameDictator && (requestError != nil || protocolRequest.Voice == nil || protocolRequest.Voice.ProviderVoiceReference != "native-dictator") {
			t.Fatalf("Dictator voice request=%+v error=%v", protocolRequest, requestError)
		}
	}
}

func bytesReader(value string) *bytes.Reader {
	return bytes.NewReader([]byte(value))
}
