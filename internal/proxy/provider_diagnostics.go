package proxy

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

type providerDiagnosticsResponse struct {
	Provider        string `json:"provider"`
	Scope           string `json:"scope"`
	OperationsTotal int64  `json:"operations_total"`
	Queued          int64  `json:"queued"`
	Running         int64  `json:"running"`
	Succeeded       int64  `json:"succeeded"`
	Failed          int64  `json:"failed"`
	Cancelled       int64  `json:"cancelled"`
	Uncertain       int64  `json:"uncertain"`
}

func (service *mediaOperationService) diagnosticsHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		provider := ctx.Param("provider")
		if _, exists := service.providers.definitions[providerID(provider)]; !exists {
			ctx.JSON(http.StatusNotFound, mediaOperationErrorEnvelope{Error: mediaOperationErrorResponse{Code: llmproxycontract.ErrorCodeProviderDiagnosticsNotFound}})
			return
		}
		tenantID := authenticatedTenantFromContext(ctx).identifier.string()
		_, err := service.store.credentialReference(ctx.Request.Context(), tenantID, providerID(provider))
		if errors.Is(err, errMediaOperationUnavailable) {
			ctx.JSON(http.StatusNotFound, mediaOperationErrorEnvelope{Error: mediaOperationErrorResponse{Code: llmproxycontract.ErrorCodeProviderDiagnosticsNotFound}})
			return
		}
		if err != nil {
			writeMediaOperationError(ctx, err)
			return
		}
		var result providerDiagnosticsResponse
		err = service.store.database.WithContext(ctx.Request.Context()).Model(&mediaOperationRecord{}).
			Select(`COUNT(*) AS operations_total,
                COALESCE(SUM(public_state = ?), 0) AS queued,
                COALESCE(SUM(public_state = ?), 0) AS running,
                COALESCE(SUM(public_state = ?), 0) AS succeeded,
                COALESCE(SUM(public_state = ?), 0) AS failed,
                COALESCE(SUM(public_state = ?), 0) AS cancelled,
                COALESCE(SUM(public_state = ?), 0) AS uncertain`,
				MediaOperationStateQueued, MediaOperationStateRunning, MediaOperationStateSucceeded,
				MediaOperationStateFailed, MediaOperationStateCancelled, MediaOperationStateUncertain).
			Where("tenant_id = ? AND provider = ?", tenantID, provider).Scan(&result).Error
		if err != nil {
			writeMediaOperationError(ctx, errMediaOperationStore)
			return
		}
		result.Provider = provider
		result.Scope = llmproxycontract.ProviderDiagnosticsScopeTenant
		ctx.JSON(http.StatusOK, result)
	}
}
