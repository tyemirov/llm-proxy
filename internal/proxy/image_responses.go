package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

var imageResponseHandlePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

type imageResponsesSnapshot struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output []struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Status string `json:"status"`
		Result string `json:"result"`
	} `json:"output"`
}

func (adapter *imageGenerationAdapter) validateResponsesInput(ctx context.Context, request MediaOperationAdapterRequest, input imageGenerationInput, controls imageGenerationControls) error {
	if !slices.Contains(adapter.controls[imageControlResponsesModel].Values, controls.ResponsesModel) || controls.OutputCount != adapter.limits[imageLimitResponsesOutputs] || input.MaskAssetID != "" {
		return errMediaOperationInvalid
	}
	if input.PreviousOperationID != "" {
		_, err := adapter.parentResponseHandle(ctx, request.TenantID, request.CredentialReference, input.PreviousOperationID, controls.ResponsesModel)
		return err
	}
	return nil
}

func (adapter *imageGenerationAdapter) parentResponseHandle(ctx context.Context, tenantID, credential, operationID, responsesModel string) (string, error) {
	if !mediaOperationIdentifierPattern.MatchString(operationID) {
		return "", errMediaOperationInvalid
	}
	var parent mediaOperationRecord
	err := adapter.store.database.WithContext(ctx).First(&parent, "operation_id = ? AND tenant_id = ? AND provider = ? AND model = ? AND credential_reference = ? AND public_state = ?", operationID, tenantID, adapter.offering.Provider, adapter.offering.Model, credential, MediaOperationStateSucceeded).Error
	if err != nil || !imageResponseHandlePattern.MatchString(parent.ProviderHandle) || parent.ExecutionBinding != adapter.responsesExecutionBinding(responsesModel) {
		return "", errMediaOperationInvalid
	}
	var controls imageGenerationControls
	if json.Unmarshal(parent.NormalizedControls, &controls) != nil || controls.Surface != "responses" || controls.ResponsesModel != responsesModel {
		return "", errMediaOperationInvalid
	}
	return parent.ProviderHandle, nil
}

func (adapter *imageGenerationAdapter) executeResponses(ctx context.Context, request MediaOperationExecutionRequest, controls imageGenerationControls) MediaOperationExecutionResult {
	if request.ExecutionBinding != adapter.responsesExecutionBinding(controls.ResponsesModel) {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	provider, err := adapter.resolveImageProvider(ctx, request, adapter.offering.ImageRoutes.Responses)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	var input imageGenerationInput
	_ = json.Unmarshal(request.Input, &input)
	previous := ""
	if input.PreviousOperationID != "" {
		previous, err = adapter.parentResponseHandle(ctx, request.TenantID, request.CredentialReference, input.PreviousOperationID, controls.ResponsesModel)
		if err != nil {
			return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
		}
	}
	reader, writer := io.Pipe()
	go func() {
		_ = writer.CloseWithError(adapter.writeImageResponsesBody(writer, request, input, controls, previous))
	}()
	defer reader.Close()
	requestURL, _ := url.Parse(provider.textEndpointURL)
	providerRequest := (&http.Request{Method: provider.activeTransport.endpoint.Method, URL: requestURL, Header: http.Header{"Content-Type": {"application/json"}}, Body: reader, ContentLength: -1}).WithContext(ctx)
	response, err := newProviderTransportHTTPDoer(request.HTTP.Submission, provider, provider.textAPIKey).Do(providerRequest)
	if err != nil {
		return imageSubmissionFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_rate_limited"}
	}
	if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusRequestTimeout {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	if response.StatusCode != http.StatusOK {
		return imageGenerationUncertain()
	}
	if controls.Stream {
		result := adapter.decodeResponsesStream(response.Body, controls, request)
		_ = response.Body.Close()
		if result.State == MediaOperationStateUncertain && result.ErrorCode == "provider_outcome_unknown" && result.ProviderHandle != "" {
			return adapter.pollImageResponse(ctx, request, provider, result.ProviderHandle, controls)
		}
		return result
	}
	snapshot, err := adapter.readImageResponse(response.Body)
	if err != nil {
		return imageResponsesReadFailure(err)
	}
	if err := request.PersistProviderHandle(snapshot.ID); err != nil {
		return imageGenerationUncertain()
	}
	// Release submission admission before polling the accepted resource.
	_ = response.Body.Close()
	if imageResponsePending(snapshot) {
		return adapter.pollImageResponse(ctx, request, provider, snapshot.ID, controls)
	}
	return adapter.imageResponseResult(snapshot, controls)
}

func (adapter *imageGenerationAdapter) writeImageResponsesBody(writer io.Writer, request MediaOperationExecutionRequest, input imageGenerationInput, controls imageGenerationControls, previous string) error {
	action := "generate"
	if request.Capability == llmproxycontract.MediaCapabilityImageEdit {
		action = "edit"
	}
	var partialImages *int
	if controls.Stream {
		partialImages = &controls.PartialImages
	}
	header, _ := json.Marshal(struct {
		Model              string                   `json:"model"`
		PreviousResponseID string                   `json:"previous_response_id,omitempty"`
		Background         bool                     `json:"background"`
		Stream             bool                     `json:"stream,omitempty"`
		Store              bool                     `json:"store"`
		MaximumToolCalls   int                      `json:"max_tool_calls"`
		Tools              []imageResponsesTool     `json:"tools"`
		ToolChoice         imageResponsesToolChoice `json:"tool_choice"`
	}{Model: adapter.responsesModels[controls.ResponsesModel], PreviousResponseID: previous, Background: true, Stream: controls.Stream, Store: true, MaximumToolCalls: controls.OutputCount,
		Tools: []imageResponsesTool{{Type: "image_generation", Model: adapter.offering.ProviderModel, Action: action, Quality: controls.Quality, Size: controls.Size, Background: controls.Background, OutputFormat: controls.OutputFormat, OutputCompression: controls.OutputCompression, PartialImages: partialImages}}, ToolChoice: imageResponsesToolChoice{Type: "image_generation"}})
	if _, err := writer.Write(header[:len(header)-1]); err != nil {
		return err
	}
	if _, err := io.WriteString(writer, `,"input":[{"role":"user","content":[`); err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	if err := encoder.Encode(imageResponsesContent{Type: "input_text", Text: input.Prompt}); err != nil {
		return err
	}
	for _, assetID := range input.ImageAssetIDs {
		data, metadata, err := adapter.readEditingAsset(request.TenantID, assetID)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(writer, ","); err != nil {
			return err
		}
		if err := encoder.Encode(imageResponsesContent{Type: "input_image", ImageURL: "data:" + metadata.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(data)}); err != nil {
			return err
		}
	}
	_, err := io.WriteString(writer, `]}]}`)
	return err
}

type imageResponsesTool struct {
	PartialImages     *int   `json:"partial_images,omitempty"`
	Type              string `json:"type"`
	Model             string `json:"model"`
	Action            string `json:"action"`
	Quality           string `json:"quality"`
	Size              string `json:"size"`
	Background        string `json:"background"`
	OutputFormat      string `json:"output_format"`
	OutputCompression *int   `json:"output_compression,omitempty"`
}

type imageResponsesToolChoice struct {
	Type string `json:"type"`
}
type imageResponsesContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

func (adapter *imageGenerationAdapter) readImageResponse(reader io.Reader) (imageResponsesSnapshot, error) {
	maximum := (adapter.maximumOutputBytes+2)/3*4 + 65536
	encoded, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return imageResponsesSnapshot{}, err
	}
	var snapshot imageResponsesSnapshot
	if int64(len(encoded)) > maximum || json.Unmarshal(encoded, &snapshot) != nil || !imageResponseHandlePattern.MatchString(snapshot.ID) || !slices.Contains([]string{"queued", "in_progress", "completed", "failed", "cancelled", "incomplete"}, snapshot.Status) {
		return imageResponsesSnapshot{}, errMediaOperationInvalid
	}
	return snapshot, nil
}

func imageResponsePending(snapshot imageResponsesSnapshot) bool {
	return snapshot.Status == "queued" || snapshot.Status == "in_progress"
}

func (adapter *imageGenerationAdapter) imageResponseResult(snapshot imageResponsesSnapshot, controls imageGenerationControls) MediaOperationExecutionResult {
	if snapshot.Status == "cancelled" {
		return MediaOperationExecutionResult{State: MediaOperationStateCancelled, ProviderHandle: snapshot.ID}
	}
	if snapshot.Status == "failed" || snapshot.Status == "incomplete" {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ProviderHandle: snapshot.ID, ErrorCode: "provider_error"}
	}
	// The response boundary validates status; pending responses stay in the poller.
	var outputs []MediaOperationOutput
	for _, item := range snapshot.Output {
		if item.Type != "image_generation_call" {
			continue
		}
		output, err := adapter.decodeImage(item.Result, controls)
		if err != nil || item.Status != "completed" {
			return imageResponsesReadFailure(errMediaOperationInvalid)
		}
		outputs = append(outputs, output)
	}
	if len(outputs) != controls.OutputCount {
		return imageResponsesReadFailure(errMediaOperationInvalid)
	}
	return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, ProviderHandle: snapshot.ID, Outputs: outputs}
}

func imageResponsesReadFailure(err error) MediaOperationExecutionResult {
	if errors.Is(err, errMediaOperationInvalid) {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_result_invalid"}
	}
	return imageGenerationUncertain()
}

func (adapter *imageGenerationAdapter) pollImageResponse(ctx context.Context, request MediaOperationExecutionRequest, provider providerDefinition, handle string, controls imageGenerationControls) MediaOperationExecutionResult {
	lifecycle := pollableResourceLifecycle[imageResponsesSnapshot]{
		observe: func(ctx context.Context) (imageResponsesSnapshot, error) {
			return adapter.fetchImageResponse(ctx, request.HTTP.Status, provider, http.MethodGet, handle, "")
		},
		isPending: imageResponsePending, visibilityPolicy: provider.activeTransport.resourceVisibility,
		recordObservation: func(imageResponsesSnapshot, error, pollableResourceRetryDecision) {},
	}
	snapshot, err := lifecycle.observeUntilTerminal(ctx)
	if err != nil {
		return imageResponsesReadFailure(err)
	}
	return adapter.imageResponseResult(snapshot, controls)
}

func (adapter *imageGenerationAdapter) fetchImageResponse(ctx context.Context, client HTTPDoer, provider providerDefinition, method, handle, suffix string) (imageResponsesSnapshot, error) {
	requestURL, _ := url.Parse(provider.textEndpointURL)
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/" + handle + suffix
	request := (&http.Request{Method: method, URL: requestURL, Header: http.Header{}}).WithContext(ctx)
	response, err := newProviderTransportHTTPDoer(client, provider, provider.textAPIKey).Do(request)
	if err != nil {
		return imageResponsesSnapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return imageResponsesSnapshot{}, newProviderHTTPError(response.StatusCode, response.Header)
	}
	snapshot, err := adapter.readImageResponse(response.Body)
	if err != nil {
		return imageResponsesSnapshot{}, err
	}
	if snapshot.ID != handle {
		return imageResponsesSnapshot{}, errMediaOperationInvalid
	}
	return snapshot, nil
}

// responsesExecutionBinding pins private handles to the accepted route and upstream model identities.
func (adapter *imageGenerationAdapter) responsesExecutionBinding(model string) string {
	if model == "" {
		return ""
	}
	provider, _ := adapter.provider.resolvedTransport(adapter.offering.ImageRoutes.Responses)
	transport := provider.activeTransport
	encoded, _ := json.Marshal(struct {
		Endpoint       string
		Transport      string
		RequestCodec   string
		ResponseCodec  string
		Authentication ProviderCatalogAuthentication
		Headers        []ProviderCatalogHeader
		ImageModel     string
		ResponsesModel string
	}{provider.textEndpointURL, transport.identifier, transport.requestCodec, transport.responseCodec, transport.authentication, transport.headers, adapter.offering.ProviderModel, adapter.responsesModels[model]})
	return mediaSHA256Hex(encoded)
}
