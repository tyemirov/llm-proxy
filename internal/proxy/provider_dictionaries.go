package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const CatalogProtocolElevenLabsDictionary = "elevenlabs_dictionary"

type providerDictionaryRule struct {
	Type            string `json:"type"`
	StringToReplace string `json:"string_to_replace"`
	Alias           string `json:"alias,omitempty"`
	Phoneme         string `json:"phoneme,omitempty"`
	Alphabet        string `json:"alphabet,omitempty"`
}

type providerDictionaryInput struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Rules       []providerDictionaryRule `json:"rules"`
}

type providerDictionaryNative struct {
	ID                   string  `json:"id"`
	VersionID            string  `json:"version_id"`
	Name                 string  `json:"name"`
	CreatedBy            *string `json:"created_by"`
	CreationTimeUnix     *int64  `json:"creation_time_unix"`
	VersionRulesNum      *int    `json:"version_rules_num"`
	PermissionOnResource *string `json:"permission_on_resource"`
	Description          *string `json:"description"`
}

type mediaDictionaryRecord struct {
	DictionaryID        string `gorm:"primaryKey"`
	VersionID           string `gorm:"not null"`
	TenantID            string `gorm:"not null;index"`
	Provider            string `gorm:"not null"`
	CredentialReference string `gorm:"not null"`
	ExecutionBinding    string `gorm:"not null"`
	NativeDictionaryID  string `gorm:"not null"`
	NativeVersionID     string `gorm:"not null"`
	Metadata            []byte `gorm:"not null"`
}

type providerDictionaryAdapter struct {
	route    ProviderCatalogService
	provider providerDefinition
	tenants  *managedTenantStore
	store    *mediaOperationStore
	binding  string
}

func newProviderDictionaryAdapter(route ProviderCatalogService, provider providerDefinition, tenants *managedTenantStore, store *mediaOperationStore) *providerDictionaryAdapter {
	return &providerDictionaryAdapter{route: route, provider: provider, tenants: tenants, store: store, binding: providerServiceBinding(route, provider, CatalogProtocolElevenLabsDictionary)}
}

func (adapter *providerDictionaryAdapter) Validate(_ context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	var input providerDictionaryInput
	var controls struct{}
	if !utf8.Valid(request.Input) || !decodeImageRequestObject(request.Input, &input) || !decodeImageRequestObject(request.Controls, &controls) || strings.TrimSpace(input.Name) == "" || len(input.Rules) == 0 {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	var fields struct {
		Rules []map[string]json.RawMessage `json:"rules"`
	}
	_ = json.Unmarshal(request.Input, &fields)
	for index, rule := range input.Rules {
		if strings.TrimSpace(rule.StringToReplace) == "" {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
		switch rule.Type {
		case "alias":
			if len(fields.Rules[index]) != 3 || strings.TrimSpace(rule.Alias) == "" || rule.Phoneme != "" || rule.Alphabet != "" {
				return MediaOperationValidatedRequest{}, errMediaOperationInvalid
			}
		case "phoneme":
			if len(fields.Rules[index]) != 4 || strings.TrimSpace(rule.Phoneme) == "" || strings.TrimSpace(rule.Alphabet) == "" || rule.Alias != "" {
				return MediaOperationValidatedRequest{}, errMediaOperationInvalid
			}
		default:
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	normalized, _ := json.Marshal(input)
	return MediaOperationValidatedRequest{Input: normalized, Controls: json.RawMessage(`{}`), ExecutionBinding: adapter.binding}, nil
}

func (adapter *providerDictionaryAdapter) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, err := resolveMediaProvider(ctx, request, adapter.provider.identifier.string(), adapter.route.Transport, adapter.provider, adapter.tenants, adapter.store)
	if err != nil || request.ExecutionBinding != adapter.binding {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	native, _ := http.NewRequestWithContext(ctx, http.MethodPost, provider.textEndpointURL, bytes.NewReader(request.Input))
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
	data, err := io.ReadAll(io.LimitReader(response.Body, providerMetadataMaximumBytes+1))
	if err != nil || len(data) > providerMetadataMaximumBytes {
		return imageGenerationUncertain()
	}
	result, valid := decodeProviderDictionary(data)
	if !valid {
		return imageGenerationUncertain()
	}
	handle, _ := json.Marshal(result)
	if err := request.PersistProviderHandle(string(handle)); err != nil {
		return imageGenerationUncertain()
	}
	return dictionaryExecutionResult(request, result)
}

func (adapter *providerDictionaryAdapter) Recover(_ context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	result, valid := decodeProviderDictionary([]byte(request.ProviderHandle))
	if request.ExecutionBinding != adapter.binding || !valid {
		return imageGenerationUncertain()
	}
	return dictionaryExecutionResult(request, result)
}

func (*providerDictionaryAdapter) Cancel(context.Context, MediaOperationExecutionRequest) MediaOperationCancellationResult {
	return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
}

func decodeProviderDictionary(data []byte) (providerDictionaryNative, bool) {
	var result providerDictionaryNative
	if json.Unmarshal(data, &result) != nil || strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.VersionID) == "" || strings.TrimSpace(result.Name) == "" || result.CreatedBy == nil || result.CreationTimeUnix == nil || *result.CreationTimeUnix < 0 || result.VersionRulesNum == nil || *result.VersionRulesNum < 0 {
		return providerDictionaryNative{}, false
	}
	return result, true
}

func dictionaryExecutionResult(request MediaOperationExecutionRequest, result providerDictionaryNative) MediaOperationExecutionResult {
	suffix := strings.TrimPrefix(request.OperationID, "mop_")
	public := llmproxycontract.MediaDictionary{DictionaryID: "dic_" + suffix, VersionID: "div_" + suffix, Provider: request.Provider, Name: result.Name, CreatedBy: *result.CreatedBy, CreationTimeUnix: *result.CreationTimeUnix, VersionRulesNum: *result.VersionRulesNum, PermissionOnResource: result.PermissionOnResource, Description: result.Description}
	metadata, _ := json.Marshal(public)
	record := &mediaDictionaryRecord{DictionaryID: public.DictionaryID, VersionID: public.VersionID, TenantID: request.TenantID, Provider: request.Provider, CredentialReference: request.CredentialReference, ExecutionBinding: request.ExecutionBinding, NativeDictionaryID: result.ID, NativeVersionID: result.VersionID, Metadata: metadata}
	return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{{MIMEType: "application/json", Data: metadata}}, dictionary: record}
}
