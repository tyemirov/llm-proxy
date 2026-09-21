package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	CatalogProtocolElevenLabsAlignment = "elevenlabs_alignment"
	alignmentInputLimit                = "input_audio_bytes"
)

type providerAlignmentInput struct {
	AudioAssetID string `json:"audio_asset_id"`
	Transcript   string `json:"transcript"`
}
type providerAlignmentSegment struct {
	Text  *string  `json:"text"`
	Start *float64 `json:"start"`
	End   *float64 `json:"end"`
	Loss  *float64 `json:"loss,omitempty"`
}
type providerAlignmentOutput struct {
	Characters []providerAlignmentSegment `json:"characters"`
	Words      []providerAlignmentSegment `json:"words"`
	Loss       *float64                   `json:"loss,omitempty"`
}

type providerAlignmentAdapter struct {
	route             ProviderCatalogService
	provider          providerDefinition
	tenants           *managedTenantStore
	store             *mediaOperationStore
	assets            *tenantAssetStore
	binding           string
	maximumInputBytes int64
}

func newProviderAlignmentAdapter(route ProviderCatalogService, provider providerDefinition, tenants *managedTenantStore, store *mediaOperationStore, assets *tenantAssetStore) *providerAlignmentAdapter {
	return &providerAlignmentAdapter{route: route, provider: provider, tenants: tenants, store: store, assets: assets, binding: providerServiceBinding(route, provider, CatalogProtocolElevenLabsAlignment), maximumInputBytes: int64(*route.Limits[0].Value)}
}

func providerServiceBinding(route ProviderCatalogService, provider providerDefinition, codec string) string {
	transport := provider.transports[route.Transport]
	document, _ := json.Marshal(struct {
		Route            ProviderCatalogService
		Transport        string
		Endpoint         ProviderCatalogEndpoint
		Authentication   ProviderCatalogAuthentication
		Headers          []ProviderCatalogHeader
		RequestCodec     string
		ResponseCodec    string
		EndpointOverride string
	}{route, route.Transport, transport.endpoint, transport.authentication, transport.headers, transport.requestCodec, transport.responseCodec, transport.endpointURLOverride})
	digest := sha256.Sum256(document)
	return codec + ":" + hex.EncodeToString(digest[:])
}

func (adapter *providerAlignmentAdapter) Validate(_ context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	var input providerAlignmentInput
	var controls struct{}
	if !decodeImageRequestObject(request.Input, &input) || !decodeImageRequestObject(request.Controls, &controls) || strings.TrimSpace(input.Transcript) == "" || !utf8.ValidString(input.Transcript) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	metadata, err := adapter.assets.metadata(tenant{identifier: tenantID(request.TenantID)}, input.AudioAssetID)
	if err != nil || !strings.HasPrefix(metadata.MIMEType, "audio/") || metadata.SizeBytes > adapter.maximumInputBytes {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	input.Transcript = strings.TrimSpace(input.Transcript)
	body, _ := json.Marshal(input)
	return MediaOperationValidatedRequest{Input: body, Controls: json.RawMessage(`{}`), InputAssetIDs: []string{input.AudioAssetID}, ExecutionBinding: adapter.binding}, nil
}

func (adapter *providerAlignmentAdapter) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, err := resolveMediaProvider(ctx, request, adapter.provider.identifier.string(), adapter.route.Transport, adapter.provider, adapter.tenants, adapter.store)
	if err != nil || request.ExecutionBinding != adapter.binding {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	var input providerAlignmentInput
	_ = json.Unmarshal(request.Input, &input)
	owner := tenant{identifier: tenantID(request.TenantID)}
	metadata, err := adapter.assets.metadata(owner, input.AudioAssetID)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationInvalid.Error()}
	}
	asset, err := adapter.assets.resolve(owner, input.AudioAssetID, metadata.MIMEType)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationInvalid.Error()}
	}
	defer asset.Close()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("text", input.Transcript)
	headers := textproto.MIMEHeader{}
	headers.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": input.AudioAssetID}))
	headers.Set("Content-Type", metadata.MIMEType)
	_, _ = form.CreatePart(headers)
	headerBytes := body.Len()
	_ = form.Close()
	// Multipart headers and trailer are bounded; the owned input asset is streamed.
	reader := io.MultiReader(bytes.NewReader(body.Bytes()[:headerBytes]), asset.file, bytes.NewReader(body.Bytes()[headerBytes:]))
	native, _ := http.NewRequestWithContext(ctx, http.MethodPost, provider.textEndpointURL, reader)
	native.ContentLength = int64(body.Len()) + metadata.SizeBytes
	native.Header.Set("Content-Type", form.FormDataContentType())
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
	data, err := io.ReadAll(io.LimitReader(response.Body, adapter.assets.maxAssetBytes+1))
	if err != nil || int64(len(data)) > adapter.assets.maxAssetBytes {
		return imageGenerationUncertain()
	}
	output, valid := decodeProviderAlignment(data)
	if !valid {
		return imageGenerationUncertain()
	}
	encoded, _ := json.Marshal(output)
	return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{{MIMEType: "application/json", Data: encoded}}}
}

func (*providerAlignmentAdapter) Recover(context.Context, MediaOperationExecutionRequest) MediaOperationExecutionResult {
	return imageGenerationUncertain()
}
func (*providerAlignmentAdapter) Cancel(context.Context, MediaOperationExecutionRequest) MediaOperationCancellationResult {
	return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
}

func decodeProviderAlignment(body []byte) (providerAlignmentOutput, bool) {
	var output providerAlignmentOutput
	if json.Unmarshal(body, &output) != nil || output.Characters == nil || output.Words == nil || output.Loss != nil && *output.Loss < 0 {
		return providerAlignmentOutput{}, false
	}
	for _, segments := range [][]providerAlignmentSegment{output.Characters, output.Words} {
		previous := 0.0
		for _, segment := range segments {
			if segment.Text == nil || segment.Start == nil || segment.End == nil || *segment.Start < previous || *segment.End < *segment.Start || segment.Loss != nil && *segment.Loss < 0 {
				return providerAlignmentOutput{}, false
			}
			previous = *segment.Start
		}
	}
	spoken := make([]providerAlignmentSegment, 0, len(output.Words))
	for _, word := range output.Words {
		if strings.IndexFunc(*word.Text, func(value rune) bool { return unicode.IsLetter(value) || unicode.IsNumber(value) }) >= 0 {
			spoken = append(spoken, word)
		}
	}
	if len(spoken) == 0 {
		return providerAlignmentOutput{}, false
	}
	output.Words = spoken
	return output, true
}
