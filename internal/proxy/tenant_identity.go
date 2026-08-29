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
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		if !validTenantIdentityRequest(ginContext.Request) {
			ginContext.String(http.StatusBadRequest, errorInvalidIdentityRequest)
			return
		}
		requestTenant := authenticatedTenantFromContext(ginContext)
		ginContext.JSON(http.StatusOK, tenantIdentityResponse{TenantID: requestTenant.identifier.string()})
	}
}

func validTenantIdentityRequest(request *http.Request) bool {
	query := request.URL.Query()
	keyValues, hasKey := query[queryParameterKey]
	requestHasNoBody := request.Body == nil || request.Body == http.NoBody
	return len(query) == 1 && hasKey && len(keyValues) == 1 && requestHasNoBody
}
