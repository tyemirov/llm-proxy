package proxy

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	chatCompletionsPath          = "/v1/chat/completions"
	responsesPath                = "/v1/responses"
	modelsPath                   = "/v1/models"
	transcriptionsPath           = "/v1/audio/transcriptions"
	contextKeyClientErrorEncoder = "client_error_encoder"
)

// ClientProtocolRoute defines one public method, path, and edge handler.
type ClientProtocolRoute struct {
	Method, Path string
	Handler      gin.HandlerFunc
}

// ClientProtocolAdapter owns a named set of client protocol routes.
type ClientProtocolAdapter struct {
	Name   string
	Routes []ClientProtocolRoute
}

// RegisterClientProtocols validates the full registry before it registers any route.
func RegisterClientProtocols(router *gin.Engine, adapters []ClientProtocolAdapter) error {
	seen := map[string]string{}
	for _, route := range router.Routes() {
		seen[route.Method+" "+route.Path] = "resource"
	}
	for _, adapter := range adapters {
		for _, route := range adapter.Routes {
			if adapter.Name == "" || route.Handler == nil || (route.Method != http.MethodGet && route.Method != http.MethodPost) || !strings.HasPrefix(route.Path, "/") {
				return fmt.Errorf("invalid client protocol route: %s", adapter.Name)
			}
			key := route.Method + " " + route.Path
			if owner, exists := seen[key]; exists {
				return fmt.Errorf("client protocol route %s conflicts: %s and %s", key, owner, adapter.Name)
			}
			seen[key] = adapter.Name
		}
	}
	for _, adapter := range adapters {
		for _, route := range adapter.Routes {
			router.Handle(route.Method, route.Path, route.Handler)
		}
	}
	return nil
}

type clientErrorEncoder func(*gin.Context, int, string, string)

func writeClientError(c *gin.Context, status int, code, message string) {
	if encoder, ok := c.Get(contextKeyClientErrorEncoder); ok {
		encoder.(clientErrorEncoder)(c, status, code, message)
		return
	}
	c.String(status, message)
}
func writeOpenAIError(c *gin.Context, status int, code, message string) {
	kind := "invalid_request_error"
	switch {
	case status == http.StatusUnauthorized:
		kind = "authentication_error"
	case status == http.StatusTooManyRequests:
		kind = "rate_limit_error"
	case status >= 500:
		kind = "server_error"
	}
	c.JSON(status, gin.H{"error": gin.H{"message": message, "type": kind, "param": nil, "code": code, "request_id": requestIDFromContext(c)}})
}
func bearerTenantHandler(authenticator tenantAuthenticator, handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(contextKeyClientErrorEncoder, clientErrorEncoder(writeOpenAIError))
		c.Header("X-Request-ID", requestIDFromContext(c))
		c.Header("Cache-Control", "no-store")
		telemetry := newRequestTelemetry(requestIDFromContext(c), requestLogPath(c.Request.URL))
		c.Request = c.Request.WithContext(requestContextWithTelemetry(c.Request.Context(), telemetry))
		start := time.Now()
		values := c.Request.Header.Values("Authorization")
		if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") || strings.TrimSpace(strings.TrimPrefix(values[0], "Bearer ")) != strings.TrimPrefix(values[0], "Bearer ") {
			writeOpenAIError(c, 401, "invalid_api_key", "A tenant bearer key is required.")
			return
		}
		tenant, ok := authenticator.authenticate(c.Request.Context(), strings.TrimPrefix(values[0], "Bearer "))
		telemetry.addPhase(requestTelemetryPhaseAuthentication, time.Since(start))
		if !ok {
			writeOpenAIError(c, 401, "invalid_api_key", "The tenant bearer key is invalid.")
			return
		}
		c.Set(contextKeyTenant, tenant)
		handler(c)
	}
}

type completionEncoder func(*gin.Context, chatRequestParameters, completionResult)

func nativeCompletionEncoder(c *gin.Context, request chatRequestParameters, result completionResult) {
	if len(result.content.toolCalls()) > 0 {
		c.JSON(http.StatusOK, gin.H{"type": "tool_calls", "tool_calls": result.content.toolCalls(), "text": result.content.text(), "usage": result.usage})
		return
	}
	body, mime := formatResponse(result.content.text(), preferredMime(c), request, result.usage)
	c.Data(http.StatusOK, mime, []byte(body))
}

func openAIClientAdapters(configuration Configuration, auth tenantAuthenticator, providers *providerRegistry, coordinator *providerRouter, store *managedTenantStore, logger *zap.SugaredLogger) []ClientProtocolAdapter {
	wrap := func(endpoint string, handler gin.HandlerFunc) gin.HandlerFunc {
		return bearerTenantHandler(auth, managedRequestTimeoutHandler(configuration.requestTimeoutPolicy, logger, store, endpoint, handler))
	}
	return []ClientProtocolAdapter{
		{Name: "openai-chat", Routes: []ClientProtocolRoute{{http.MethodPost, chatCompletionsPath, wrap(usageEndpointV2, openAITextHandler(clientChatProtocol, configuration.MaxPromptBytes, providers, coordinator, store, logger))}}},
		{Name: "openai-responses", Routes: []ClientProtocolRoute{{http.MethodPost, responsesPath, wrap(usageEndpointV2, openAITextHandler(clientResponsesProtocol, configuration.MaxPromptBytes, providers, coordinator, store, logger))}}},
		{Name: "openai-models", Routes: []ClientProtocolRoute{{http.MethodGet, modelsPath, bearerTenantHandler(auth, openAIModelsHandler(configuration.ProviderCatalog, providers))}}},
		{Name: "openai-transcription", Routes: []ClientProtocolRoute{{http.MethodPost, transcriptionsPath, wrap(usageEndpointDictation, openAITranscriptionHandler(configuration.MaxInputAudioBytes, providers, coordinator, store, logger))}}},
	}
}

func registerClientMethodErrors(router *gin.Engine) {
	methods := map[string]string{chatCompletionsPath: http.MethodPost, responsesPath: http.MethodPost, modelsPath: http.MethodGet, transcriptionsPath: http.MethodPost}
	router.NoRoute(func(c *gin.Context) {
		if method, found := methods[c.Request.URL.Path]; found {
			c.Header("Allow", method)
			writeOpenAIError(c, http.StatusMethodNotAllowed, "method_not_allowed", "Unsupported HTTP method.")
			return
		}
		c.String(http.StatusNotFound, "404 page not found")
	})
}
