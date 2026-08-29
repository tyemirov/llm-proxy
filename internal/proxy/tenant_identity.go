package proxy

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type tenantIdentityResponse struct {
	TenantID string `json:"tenant_id"`
}

func tenantIdentityHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant := authenticatedTenantFromContext(ginContext)
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		ginContext.JSON(http.StatusOK, tenantIdentityResponse{TenantID: requestTenant.identifier.string()})
	}
}
