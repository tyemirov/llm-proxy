package proxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

// sanitizeRequestURI replaces sensitive query parameter values with a placeholder.
func sanitizeRequestURI(requestURL *url.URL) string {
	queryParameters := requestURL.Query()
	if queryParameters.Has(queryParameterKey) {
		queryParameters.Set(queryParameterKey, redactedPlaceholder)
	}
	sanitizedURL := *requestURL
	sanitizedURL.RawQuery = queryParameters.Encode()
	return sanitizedURL.RequestURI()
}

// requestResponseLogger emits structured request and response metadata for traceability.
func requestResponseLogger(structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestMethod := ginContext.Request.Method
		requestPath := sanitizeRequestURI(ginContext.Request.URL)
		requestClientIP := ginContext.ClientIP()

		structuredLogger.Infow(
			logEventRequestReceived,
			logFieldMethod, requestMethod,
			logFieldPath, requestPath,
			logFieldClientIP, requestClientIP,
		)

		ginContext.Next()

		responseStatus := ginContext.Writer.Status()
		responseLatencyMillis := time.Since(requestStart).Milliseconds()
		responseFields := []any{
			logFieldStatus, responseStatus,
			constants.LogFieldLatencyMilliseconds, responseLatencyMillis,
		}
		if requestTenant, authenticated := tenantIfPresentFromContext(ginContext); authenticated {
			responseFields = append(responseFields, logFieldTenantID, requestTenant.identifier.string())
		}
		structuredLogger.Infow(logEventResponseSent, responseFields...)
	}
}

// tenantMiddleware authenticates the `key` query parameter and attaches the matched tenant to the request context.
func tenantMiddleware(tenants tenantRegistry, structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant, authenticated := tenants.authenticate(ginContext.Query(queryParameterKey))
		if !authenticated {
			structuredLogger.Warnw(
				logEventForbiddenRequest,
			)
			ginContext.String(http.StatusForbidden, errorMissingClientKey)
			ginContext.Abort()
			return
		}
		ginContext.Set(contextKeyTenant, requestTenant)
		ginContext.Next()
	}
}

func tenantIfPresentFromContext(ginContext *gin.Context) (tenant, bool) {
	contextValue, exists := ginContext.Get(contextKeyTenant)
	if !exists {
		return tenant{}, false
	}
	requestTenant, ok := contextValue.(tenant)
	return requestTenant, ok
}

func authenticatedTenantFromContext(ginContext *gin.Context) tenant {
	return ginContext.MustGet(contextKeyTenant).(tenant)
}

func constantTimeDigestEquals(firstDigest [sha256.Size]byte, secondDigest [sha256.Size]byte) bool {
	return hmac.Equal(firstDigest[:], secondDigest[:])
}
