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

func requestLogPath(requestURL *url.URL) string {
	return requestURL.EscapedPath()
}

// requestResponseLogger emits structured request and response metadata for traceability.
func requestResponseLogger(structuredLogger *zap.SugaredLogger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestStart := time.Now()
		requestMethod := ginContext.Request.Method
		requestPath := requestLogPath(ginContext.Request.URL)
		requestClientIP := ginContext.ClientIP()

		structuredLogger.Infow(
			logEventRequestReceived,
			logFieldMethod, requestMethod,
			constants.LogFieldPath, requestPath,
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

func tenantAuthenticatedHandler(authenticator tenantAuthenticator, structuredLogger *zap.SugaredLogger, handler gin.HandlerFunc) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		if !authenticateTenantRequest(ginContext, authenticator, structuredLogger) {
			return
		}
		handler(ginContext)
	}
}

func authenticateTenantRequest(ginContext *gin.Context, authenticator tenantAuthenticator, structuredLogger *zap.SugaredLogger) bool {
	requestTenant, authenticated := authenticator.authenticate(ginContext.Query(queryParameterKey))
	if !authenticated {
		structuredLogger.Warnw(
			logEventForbiddenRequest,
		)
		ginContext.String(http.StatusForbidden, errorMissingClientKey)
		ginContext.Abort()
		return false
	}
	ginContext.Set(contextKeyTenant, requestTenant)
	return true
}

type tenantAuthenticator struct {
	staticTenants  tenantRegistry
	managedTenants *managedTenantStore
}

func newTenantAuthenticator(staticTenants tenantRegistry, managedTenants *managedTenantStore) tenantAuthenticator {
	return tenantAuthenticator{
		staticTenants:  staticTenants,
		managedTenants: managedTenants,
	}
}

func (authenticator tenantAuthenticator) authenticate(rawSecret string) (tenant, bool) {
	if requestTenant, authenticated := authenticator.staticTenants.authenticate(rawSecret); authenticated {
		return requestTenant, true
	}
	if authenticator.managedTenants == nil {
		return tenant{}, false
	}
	return authenticator.managedTenants.authenticate(rawSecret)
}

func (authenticator tenantAuthenticator) containsStaticSecretDigest(secretDigest [sha256.Size]byte) bool {
	return authenticator.staticTenants.containsSecretDigest(secretDigest)
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
