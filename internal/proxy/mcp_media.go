package proxy

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

const (
	mcpCreateMediaOperationTool = "llm_proxy.create_media_operation"
	mcpGetMediaOperationTool    = "llm_proxy.get_media_operation"
	mcpCancelMediaOperationTool = "llm_proxy.cancel_media_operation"
)

type mcpCreateMediaOperationInput struct {
	TenantID       string         `json:"tenant_id"`
	IdempotencyKey string         `json:"idempotency_key"`
	Capability     string         `json:"capability"`
	Provider       string         `json:"provider"`
	Model          string         `json:"model"`
	Input          map[string]any `json:"input"`
	Controls       map[string]any `json:"controls"`
}

type mcpMediaOperationInput struct {
	TenantID    string `json:"tenant_id"`
	OperationID string `json:"operation_id"`
}

func registerMCPMedia(server *mcp.Server, management *managementService, service *mediaOperationService) {
	schema, _ := jsonschema.For[mediaOperationResponse](nil)
	mcp.AddTool[mcpCreateMediaOperationInput, any](server, &mcp.Tool{
		Name: mcpCreateMediaOperationTool, Description: "Submit a durable media operation for an owned tenant. The same idempotency key and request identify one operation. Provider charges can apply.", OutputSchema: schema,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input mcpCreateMediaOperationInput) (*mcp.CallToolResult, any, error) {
		requestTenant, err := management.store.mcpTenant(ctx.Value(mcpIdentityKey{}).(mcpIdentity).subject, input.TenantID)
		if err != nil {
			return mcpMediaFailure(err), nil, nil
		}
		if !mediaIdempotencyKeyPattern.MatchString(input.IdempotencyKey) {
			return mcpMediaFailure(errMediaOperationInvalid), nil, nil
		}
		body, _ := json.Marshal(input.Input)
		controls, _ := json.Marshal(input.Controls)
		operation, created, err := service.create(ctx, requestTenant, input.IdempotencyKey, mediaOperationCreatePayload{Capability: input.Capability, Provider: input.Provider, Model: input.Model, Input: body, Controls: controls})
		if err != nil {
			return mcpMediaFailure(err), nil, nil
		}
		if created {
			service.enqueue(operation.OperationID)
		}
		response, err := service.store.publicResponse(ctx, input.TenantID, operation.OperationID)
		if err != nil {
			return mcpMediaFailure(err), nil, nil
		}
		return nil, response, nil
	})
	for _, tool := range []struct {
		name        string
		description string
		readOnly    bool
		execute     func(context.Context, tenant, string) (mediaOperationResponse, error)
	}{
		{mcpGetMediaOperationTool, "Read a durable media operation. Poll this tool until the operation reaches a terminal state.", true, func(ctx context.Context, requestTenant tenant, operationID string) (mediaOperationResponse, error) {
			return service.store.publicResponse(ctx, requestTenant.identifier.string(), operationID)
		}},
		{mcpCancelMediaOperationTool, "Request cancellation of a durable media operation. Read the returned cancellation state to determine its outcome.", false, service.cancel},
	} {
		mcp.AddTool[mcpMediaOperationInput, any](server, &mcp.Tool{Name: tool.name, Description: tool.description, OutputSchema: schema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tool.readOnly, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}}, func(ctx context.Context, _ *mcp.CallToolRequest, input mcpMediaOperationInput) (*mcp.CallToolResult, any, error) {
			requestTenant, err := management.store.mcpTenant(ctx.Value(mcpIdentityKey{}).(mcpIdentity).subject, input.TenantID)
			if err != nil {
				return mcpMediaFailure(err), nil, nil
			}
			response, err := tool.execute(ctx, requestTenant, input.OperationID)
			if err != nil {
				return mcpMediaFailure(err), nil, nil
			}
			return nil, response, nil
		})
	}
}

func mcpMediaFailure(err error) *mcp.CallToolResult {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return mcpToolFailure(mcpNotFound)
	}
	for _, publicError := range []error{errMediaOperationInvalid, errMediaOperationNotFound, errMediaOperationUnavailable, errMediaOperationIntentConflict, errMediaOperationCapacity, errMediaOperationExpired} {
		if errors.Is(err, publicError) {
			return mcpToolFailure(publicError.Error())
		}
	}
	return mcpToolFailure(errMediaOperationStore.Error())
}
