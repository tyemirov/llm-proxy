package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// accountDictatorAdapter binds every provider interaction to the tenant's
// current account connection. The protocol-neutral adapter owns job handling.
type accountDictatorAdapter struct {
	provider  string
	model     string
	transport providerTransportDefinition
	tenants   *managedTenantStore
	store     *mediaOperationStore
	assets    *tenantAssetStore
}

func (adapter *accountDictatorAdapter) bind(ctx context.Context, tenantID, reference string) (*dictatorMediaOperationAdapter, func(), error) {
	protocol, closeConnection, err := adapter.bindProtocol(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}
	bound := &dictatorMediaOperationAdapter{provider: adapter.provider, model: adapter.model, protocol: protocol, voiceAuthority: protocol.binding, assets: adapter.assets, store: adapter.store, pollInterval: 250 * time.Millisecond}
	return bound, closeConnection, nil
}

func (adapter *accountDictatorAdapter) bindProtocol(ctx context.Context, tenantID, reference string) (*dictatorGRPCProtocol, func(), error) {
	var assignment managedTenantConnectionRecord
	err := adapter.store.database.WithContext(ctx).Preload("Connection.Fields").Where("tenant_id = ? AND provider_id = ?", tenantID, adapter.provider).First(&assignment).Error
	if err != nil {
		return nil, nil, fmt.Errorf("resolve Dictator assignment: %w", err)
	}
	current := assignment.ConnectionID + ":v" + strconv.FormatUint(assignment.Connection.Version, 10)
	if reference != "" && current != reference {
		return nil, nil, errMediaOperationUnavailable
	}
	settings, err := adapter.tenants.accountConnectionSettings(assignment.Connection)
	if err != nil {
		return nil, nil, err
	}
	connection, _, err := openDictatorConnection(ctx, settings.connectionValues, adapter.transport)
	if err != nil {
		return nil, nil, err
	}
	binding := assignment.ConnectionID + ":" + mediaSHA256Hex([]byte(settings.connectionValues[adapter.transport.endpoint.SettingField]+"\x00"+settings.connectionValues[adapter.transport.authentication.Field]+"\x00"+settings.connectionValues[dictatorTLSField]))
	protocol := &dictatorGRPCProtocol{provider: adapter.provider, model: adapter.model, connection: connection, token: settings.connectionValues[adapter.transport.authentication.Field], binding: binding, maxAssetBytes: adapter.assets.maxAssetBytes}
	return protocol, func() { _ = connection.Close() }, nil
}

func (adapter *accountDictatorAdapter) Validate(ctx context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	validated, err := (&dictatorMediaOperationAdapter{provider: adapter.provider, model: adapter.model}).Validate(ctx, request)
	if err != nil || request.Capability != llmproxycontract.MediaCapabilityAudioSpeechGenerate {
		return validated, err
	}
	protocol, closeConnection, err := adapter.bindProtocol(ctx, request.TenantID, request.CredentialReference)
	if err != nil {
		return MediaOperationValidatedRequest{}, err
	}
	defer closeConnection()
	var input dictatorCanonicalInput
	var controls dictatorCanonicalControls
	_ = json.Unmarshal(validated.Input, &input)
	_ = json.Unmarshal(validated.Controls, &controls)
	voice, err := adapter.store.providerMediaVoice(ctx, request.TenantID, input.VoiceID)
	if err != nil || voice.Provider != request.Provider || voice.Model != request.Model || voice.Authority != protocol.binding {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if _, err := protocol.synthesisRequest(input, controls, &voice); err != nil {
		return MediaOperationValidatedRequest{}, err
	}
	return validated, nil
}

func (adapter *accountDictatorAdapter) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	bound, closeConnection, err := adapter.bind(ctx, request.TenantID, request.CredentialReference)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	defer closeConnection()
	return bound.Execute(ctx, request)
}

func (adapter *accountDictatorAdapter) Recover(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	bound, closeConnection, err := adapter.bind(ctx, request.TenantID, request.CredentialReference)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ProviderHandle: request.ProviderHandle, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	defer closeConnection()
	return bound.Recover(ctx, request)
}

func (adapter *accountDictatorAdapter) Cancel(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationCancellationResult {
	bound, closeConnection, err := adapter.bind(ctx, request.TenantID, request.CredentialReference)
	if err != nil {
		return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
	}
	defer closeConnection()
	return bound.Cancel(ctx, request)
}

func (adapter *accountDictatorAdapter) DiscoverMediaVoices(ctx context.Context, tenantID string) (MediaVoiceDiscovery, error) {
	bound, closeConnection, err := adapter.bind(ctx, tenantID, "")
	if err != nil {
		return MediaVoiceDiscovery{}, err
	}
	defer closeConnection()
	return bound.DiscoverMediaVoices(ctx, tenantID)
}
