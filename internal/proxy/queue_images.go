package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const queueImageResponseBytes = 1 << 20
const queueImagePollInterval = time.Second

var queueRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

type queueImageInput struct {
	Prompt string `json:"prompt"`
}
type queueImageControls struct {
	AspectRatio  string `json:"aspect_ratio"`
	OutputFormat string `json:"output_format"`
	OutputCount  int    `json:"output_count"`
}
type queueImageHandle struct {
	RequestID   string `json:"request_id"`
	StatusURL   string `json:"status_url"`
	ResponseURL string `json:"response_url"`
	CancelURL   string `json:"cancel_url"`
}
type queueImageAdapter struct {
	offering           ProviderOffering
	provider           providerDefinition
	tenants            *managedTenantStore
	store              *mediaOperationStore
	controls           map[string]CatalogControl
	limits             map[string]int
	maximumOutputBytes int64
	binding            string
}

func newQueueImageAdapter(offering ProviderOffering, provider providerDefinition, tenants *managedTenantStore, store *mediaOperationStore, assets *tenantAssetStore) *queueImageAdapter {
	adapter := &queueImageAdapter{offering: offering, provider: provider, tenants: tenants, store: store, controls: map[string]CatalogControl{}, limits: map[string]int{}}
	for _, control := range offering.Controls {
		adapter.controls[control.ID] = control
	}
	for _, limit := range offering.Limits {
		adapter.limits[limit.ID] = *limit.Value
	}
	adapter.maximumOutputBytes = min(int64(adapter.limits[imageLimitOutput]), assets.maxAssetBytes)
	document, _ := json.Marshal(offering)
	digest := sha256.Sum256(document)
	adapter.binding = CatalogProtocolFALQueueImages + ":" + hex.EncodeToString(digest[:])
	return adapter
}

func (adapter *queueImageAdapter) Validate(_ context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	var input queueImageInput
	var controls queueImageControls
	if !decodeImageRequestObject(request.Input, &input) || !decodeImageRequestObject(request.Controls, &controls) || strings.TrimSpace(input.Prompt) == "" || !utf8.ValidString(input.Prompt) || utf8.RuneCountInString(input.Prompt) > adapter.limits[imageLimitPrompt] || !slices.Contains(adapter.controls[imageControlAspectRatio].Values, controls.AspectRatio) || !slices.Contains(adapter.controls[imageControlFormat].Values, controls.OutputFormat) || controls.OutputCount < int(*adapter.controls[imageControlCount].Minimum) || controls.OutputCount > int(*adapter.controls[imageControlCount].Maximum) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	inputJSON, _ := json.Marshal(input)
	controlsJSON, _ := json.Marshal(controls)
	return MediaOperationValidatedRequest{Input: inputJSON, Controls: controlsJSON, ExecutionBinding: adapter.binding}, nil
}

func (adapter *queueImageAdapter) resolve(ctx context.Context, request MediaOperationExecutionRequest) (providerDefinition, error) {
	return resolveMediaProvider(ctx, request, adapter.offering.Provider, adapter.offering.Transport, adapter.provider, adapter.tenants, adapter.store)
}

func (adapter *queueImageAdapter) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, err := adapter.resolve(ctx, request)
	if err != nil || request.ExecutionBinding != adapter.binding {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	var input queueImageInput
	var controls queueImageControls
	_ = json.Unmarshal(request.Input, &input)
	_ = json.Unmarshal(request.Controls, &controls)
	body, _ := json.Marshal(struct {
		Prompt       string `json:"prompt"`
		AspectRatio  string `json:"aspect_ratio"`
		OutputFormat string `json:"output_format"`
		Count        int    `json:"num_images"`
	}{input.Prompt, controls.AspectRatio, controls.OutputFormat, controls.OutputCount})
	endpoint := strings.ReplaceAll(provider.textEndpointURL, "{model}", adapter.offering.ProviderModel)
	// The catalog validates the endpoint; the model identifier is a validated URL path.
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	authorizeQueueRequest(req, provider)
	response, err := request.HTTP.Submission.Do(req)
	if err != nil {
		return imageSubmissionFailure(err)
	}
	var handle queueImageHandle
	status := response.StatusCode
	err = readQueueJSON(response, &handle)
	if status < 200 || status >= 300 {
		if err := request.recordQueueUsage(response.Header); err != nil {
			return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: llmproxycontract.ErrorCodeUsageJournalUnavailable}
		}
	}
	if status >= 400 && status < 500 && status != http.StatusRequestTimeout {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	if err != nil || status < 200 || status >= 300 || !validQueueHandle(handle, endpoint) {
		return imageGenerationUncertain()
	}
	encoded, _ := json.Marshal(handle)
	request.ProviderHandle = string(encoded)
	if err := request.PersistProviderReceipt(MediaOperationProviderReceipt{Handle: request.ProviderHandle, RequestID: handle.RequestID}); err != nil {
		return imageGenerationUncertain()
	}
	return adapter.poll(ctx, request, provider, handle, controls)
}

func authorizeQueueRequest(request *http.Request, provider providerDefinition) {
	auth := provider.activeTransport.authentication
	request.Header.Set(auth.Header, auth.Prefix+provider.credentialFor(endpointKindText))
}

func readQueueJSON(response *http.Response, result any) error {
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, queueImageResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > queueImageResponseBytes {
		return errMediaOperationInvalid
	}
	return json.Unmarshal(data, result)
}

// Queue URLs are private capabilities, confined to the configured origin and one request root.
func validQueueHandle(handle queueImageHandle, endpoint string) bool {
	if !queueRequestID.MatchString(handle.RequestID) {
		return false
	}
	base, _ := url.Parse(endpoint)
	status, err := url.Parse(handle.StatusURL)
	if err != nil || status.User != nil || status.RawQuery != "" || status.Fragment != "" || status.Scheme != base.Scheme || status.Host != base.Host || status.RawPath != "" || path.Clean(status.Path) != status.Path || !strings.HasSuffix(status.Path, "/requests/"+handle.RequestID+"/status") {
		return false
	}
	prefix := strings.TrimSuffix(status.Path, "/requests/"+handle.RequestID+"/status")
	if prefix == "" || !(base.Path == prefix || strings.HasPrefix(base.Path, prefix+"/")) {
		return false
	}
	root := strings.TrimSuffix(handle.StatusURL, "/status")
	return handle.CancelURL == root+"/cancel" && (handle.ResponseURL == root || handle.ResponseURL == root+"/response")
}

func (adapter *queueImageAdapter) retained(ctx context.Context, request MediaOperationExecutionRequest) (providerDefinition, queueImageHandle, bool) {
	var handle queueImageHandle
	provider, err := adapter.resolve(ctx, request)
	if err != nil || request.ExecutionBinding != adapter.binding || !decodeImageRequestObject([]byte(request.ProviderHandle), &handle) {
		return provider, handle, false
	}
	endpoint := strings.ReplaceAll(provider.textEndpointURL, "{model}", adapter.offering.ProviderModel)
	return provider, handle, validQueueHandle(handle, endpoint)
}

func (adapter *queueImageAdapter) Recover(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, handle, valid := adapter.retained(ctx, request)
	if !valid {
		return imageGenerationUncertain()
	}
	var controls queueImageControls
	_ = json.Unmarshal(request.Controls, &controls)
	return adapter.poll(ctx, request, provider, handle, controls)
}

func (adapter *queueImageAdapter) Cancel(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationCancellationResult {
	provider, handle, valid := adapter.retained(ctx, request)
	if !valid {
		return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, handle.CancelURL, nil)
	authorizeQueueRequest(req, provider)
	response, err := request.HTTP.Status.Do(req)
	if err != nil {
		return MediaOperationCancellationResult{State: MediaCancellationRequested}
	}
	var observation struct {
		Status string `json:"status"`
	}
	err = readQueueJSON(response, &observation)
	if err == nil && response.StatusCode == http.StatusBadRequest && observation.Status == "ALREADY_COMPLETED" {
		return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
	}
	// Acceptance, malformed responses, and transport uncertainty never prove terminal cancellation.
	return MediaOperationCancellationResult{State: MediaCancellationRequested}
}

func (adapter *queueImageAdapter) poll(ctx context.Context, request MediaOperationExecutionRequest, provider providerDefinition, handle queueImageHandle, controls queueImageControls) MediaOperationExecutionResult {
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, handle.StatusURL, nil)
		authorizeQueueRequest(req, provider)
		response, err := request.HTTP.Status.Do(req)
		if err != nil {
			return imageGenerationUncertain()
		}
		var status struct {
			Status    string `json:"status"`
			RequestID string `json:"request_id"`
			Error     string `json:"error"`
			ErrorType string `json:"error_type"`
		}
		if readQueueJSON(response, &status) != nil || response.StatusCode != http.StatusOK || status.RequestID != handle.RequestID {
			return imageGenerationUncertain()
		}
		switch status.Status {
		case "COMPLETED":
			if status.Error != "" || status.ErrorType != "" {
				if err := request.recordQueueUsage(response.Header); err != nil {
					return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: llmproxycontract.ErrorCodeUsageJournalUnavailable}
				}
				return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
			}
			return adapter.result(ctx, request, provider, handle.ResponseURL, controls)
		case "IN_QUEUE", "IN_PROGRESS":
			timer := time.NewTimer(queueImagePollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return imageGenerationUncertain()
			case <-timer.C:
			}
		default:
			return imageGenerationUncertain()
		}
	}
}

func (adapter *queueImageAdapter) result(ctx context.Context, request MediaOperationExecutionRequest, provider providerDefinition, endpoint string, controls queueImageControls) MediaOperationExecutionResult {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	authorizeQueueRequest(req, provider)
	response, err := request.HTTP.Status.Do(req)
	if err != nil {
		return imageGenerationUncertain()
	}
	if err := request.recordQueueUsage(response.Header); err != nil {
		response.Body.Close()
		return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: llmproxycontract.ErrorCodeUsageJournalUnavailable}
	}
	var result struct {
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	}
	if readQueueJSON(response, &result) != nil || response.StatusCode != http.StatusOK || len(result.Images) != controls.OutputCount {
		return imageGenerationUncertain()
	}
	outputs := make([]MediaOperationOutput, 0, len(result.Images))
	for _, artifact := range result.Images {
		output, err := adapter.download(ctx, request.HTTP.Transfer, provider.activeTransport.artifactOrigins, artifact.URL, controls.OutputFormat)
		if err != nil {
			return imageGenerationUncertain()
		}
		outputs = append(outputs, output)
	}
	return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: outputs}
}

func (adapter *queueImageAdapter) download(ctx context.Context, client HTTPDoer, origins []string, endpoint, expectedFormat string) (MediaOperationOutput, error) {
	target, err := url.Parse(endpoint)
	if err != nil || target.User != nil || target.Fragment != "" || !slices.Contains(origins, upstreamRequestOrigin(target)) {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	response, err := client.Do(request)
	if err != nil {
		return MediaOperationOutput{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, adapter.maximumOutputBytes+1))
	if err != nil {
		return MediaOperationOutput{}, err
	}
	if int64(len(data)) > adapter.maximumOutputBytes {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != expectedFormat || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > int64(adapter.limits[imageLimitOutputPixels]) {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return MediaOperationOutput{}, err
	}
	return MediaOperationOutput{MIMEType: "image/" + format, Data: data}, nil
}
