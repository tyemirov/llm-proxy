package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

const systemOneMaximumBytes = 32 << 20
const systemOneMaximumDepth = 64
const usageEndpointSystemOne = "system_one"

var newSystemOneUpstreamRequest = http.NewRequestWithContext

//go:embed system_one_schema.json
var systemOneSchemaDocument []byte

var addSystemOneSchemaResource = func(compiler *jsonschema.Compiler, resource string, document any) error {
	return compiler.AddResource(resource, document)
}

var systemOneSchemas = sync.OnceValues(compileSystemOneSchemas)

func compileSystemOneSchemas() (map[string]*jsonschema.Schema, error) {
	var document any
	if err := json.Unmarshal(systemOneSchemaDocument, &document); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	const resource = "urn:llm-proxy:typesafe"
	if err := addSystemOneSchemaResource(compiler, resource, document); err != nil {
		return nil, err
	}
	schemas := map[string]*jsonschema.Schema{}
	for _, name := range []string{"SystemOneRequest", "SystemOneResponse", "HTTPBadRequestError", "HTTPValidationError"} {
		schema, err := compiler.Compile(resource + "#/components/schemas/" + name)
		if err != nil {
			return nil, err
		}
		schemas[name] = schema
	}
	return schemas, nil
}

type systemOneRequest struct {
	wire      []byte
	model     string
	questions map[string]any
}

func writeSystemOneError(c *gin.Context, status int, _ string, message string) {
	c.JSON(status, gin.H{"detail": message})
}
func writeSystemOneValidationError(c *gin.Context, message string) {
	c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": []gin.H{{"loc": []string{"body"}, "msg": message, "type": "value_error"}}})
}

func systemOneClientAdapter(configuration Configuration, auth tenantAuthenticator, providers *providerRegistry, client HTTPDoer, store *managedTenantStore, logger *zap.SugaredLogger) (ClientProtocolAdapter, error) {
	schemas, err := systemOneSchemas()
	if err != nil {
		return ClientProtocolAdapter{}, fmt.Errorf("load System One contract: %w", err)
	}
	handler := systemOneHandler(providers, client, store, logger, schemas)
	authenticated := func(c *gin.Context) {
		c.Set(contextKeyClientErrorEncoder, clientErrorEncoder(writeSystemOneError))
		c.Header("Cache-Control", "no-store")
		c.Header("Vary", "Authorization")
		c.Header("X-Request-ID", requestIDFromContext(c))
		values := c.Request.Header.Values("Authorization")
		if len(values) != 1 || len(values[0]) < 7 || !strings.EqualFold(values[0][:7], "Bearer ") || strings.TrimSpace(values[0][7:]) != values[0][7:] {
			writeSystemOneError(c, 401, "", "A tenant bearer key is required.")
			return
		}
		requestTenant, ok := auth.authenticate(c.Request.Context(), values[0][7:])
		if !ok {
			writeSystemOneError(c, 401, "", "The tenant bearer key is invalid.")
			return
		}
		c.Set(contextKeyTenant, requestTenant)
		budget, valid := configuration.requestTimeoutPolicy.resolve(c.Request.Header.Values(llmproxycontract.HeaderRequestTimeoutSeconds))
		if !valid {
			writeSystemOneError(c, 400, "", "Invalid request timeout.")
			return
		}
		c.Set(contextKeyRequestTimeoutState, &requestTimeoutState{budget: budget, outcome: requestOutcomeValidation, managedUsageOutcome: managedUsageOutcomeInvalidRequest})
		c.Header(llmproxycontract.HeaderRequestTimeoutSeconds, strconv.Itoa(budget.seconds))
		ctx, cancel := context.WithTimeoutCause(c.Request.Context(), budget.duration, errRequestTimeoutBudgetExpired)
		defer cancel()
		body := c.Request.Body
		stop := context.AfterFunc(ctx, func() { _ = body.Close() })
		defer stop()
		c.Request = c.Request.WithContext(requestContextWithTelemetry(ctx, newRequestTelemetry(requestIDFromContext(c), llmproxycontract.SystemOnePath)))
		handler(c)
	}
	return ClientProtocolAdapter{Name: "typesafe-system-one", Routes: []ClientProtocolRoute{{http.MethodPost, llmproxycontract.SystemOnePath, authenticated}}}, nil
}

func systemOneHandler(providers *providerRegistry, client HTTPDoer, store *managedTenantStore, logger *zap.SugaredLogger, schemas map[string]*jsonschema.Schema) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestTenant := authenticatedTenantFromContext(c)
		var usage *tokenUsage
		defer func() {
			recordManagedUsage(store, logger, c, requestTenant, usageEndpointSystemOne, c.Writer.Status(), usage, start)
		}()
		request, err := readSystemOneRequest(c.Request, schemas["SystemOneRequest"])
		if err != nil {
			if systemOneContextEnded(c) {
				return
			}
			var failure systemOneBoundaryError
			if errors.As(err, &failure) {
				if failure.status == http.StatusRequestEntityTooLarge {
					markManagedUsageOutcome(c, managedUsageOutcomePayloadTooLarge)
				}
				writeSystemOneError(c, failure.status, "", failure.message)
			} else {
				writeSystemOneValidationError(c, err.Error())
			}
			return
		}
		provider, model, err := providers.forTenant(requestTenant).resolveSystemOneRequest(request.model)
		if err != nil {
			if errors.Is(err, ErrProviderNotConfigured) {
				bindRequestTelemetryRoute(c, provider, model.identifier)
				markManagedUsageOutcome(c, managedUsageOutcomeProviderNotConfigured)
			}
			writeSystemOneError(c, 400, "", "The requested model is unavailable for this tenant.")
			return
		}
		bindRequestTelemetryRoute(c, provider, model.identifier)
		upstream, err := newSystemOneUpstreamRequest(c.Request.Context(), http.MethodPost, provider.textEndpointURL, bytes.NewReader(request.wire))
		if err != nil {
			writeSystemOneError(c, 502, "", "The upstream request could not be created.")
			return
		}
		for _, name := range []string{"Content-Type", "Content-Encoding", "TypeSafe-Danger-Zone-Images", "TypeSafe-Danger-Zone-Long-Context"} {
			if value := c.Request.Header.Get(name); value != "" {
				upstream.Header.Set(name, value)
			}
		}
		upstream.Header.Set("Accept", "application/json")
		response, err := newProviderTransportHTTPDoer(client, provider, provider.credentialFor(endpointKindText)).Do(upstream)
		if err != nil {
			if systemOneContextEnded(c) {
				return
			}
			markRequestOutcome(c, requestOutcomeProviderFailure, managedRequestFailureOutcome(err))
			status := http.StatusBadGateway
			if errors.Is(err, errQueueFull) {
				status = http.StatusServiceUnavailable
			}
			writeSystemOneError(c, status, "", "The upstream request failed.")
			return
		}
		defer response.Body.Close()
		body, err := systemOneReadBounded(response.Body)
		if err != nil {
			if systemOneContextEnded(c) {
				return
			}
			markRequestOutcome(c, requestOutcomeProviderFailure, managedUsageOutcomeUpstreamError)
			writeSystemOneError(c, 502, "", "The upstream response is invalid.")
			return
		}
		if systemOneContextEnded(c) {
			return
		}
		for _, name := range []string{"X-TypeSafe-Request-ID", "X-Request-ID", "Request-ID", "Retry-After", "TypeSafe-Danger-Zone-Images", "TypeSafe-Danger-Zone-Long-Context"} {
			if value := response.Header.Get(name); value != "" && !strings.Contains(value, provider.textAPIKey) {
				c.Header(name, value)
			}
		}
		if response.StatusCode != http.StatusOK {
			markRequestOutcome(c, requestOutcomeProviderFailure, managedUsageOutcomeUpstreamError)
			if response.StatusCode == 429 {
				markManagedUsageOutcome(c, managedUsageOutcomeRateLimited)
			}
			errorDocument, decodeErr := jsonschema.UnmarshalJSON(bytes.NewReader(body))
			if decodeErr != nil {
				writeSystemOneError(c, 502, "", "The upstream error response is invalid.")
				return
			}
			errorSchema := schemas["HTTPBadRequestError"]
			if response.StatusCode == http.StatusUnprocessableEntity {
				errorSchema = schemas["HTTPValidationError"]
			}
			if errorSchema.Validate(errorDocument) != nil {
				writeSystemOneError(c, 502, "", "The upstream error response is invalid.")
				return
			}
			errorObject := errorDocument.(map[string]any)
			if _, ok := errorObject["detail"]; !ok {
				writeSystemOneError(c, 502, "", "The upstream error response is invalid.")
				return
			}
			if redactSystemOneValue(errorObject, provider.textAPIKey) {
				c.JSON(response.StatusCode, errorObject)
			} else {
				c.Data(response.StatusCode, "application/json", body)
			}
			return
		}
		document, err := decodeSystemOneJSON(body)
		if err != nil || schemas["SystemOneResponse"].Validate(document) != nil || validateSystemOneAnswers(document, request.questions) != nil {
			markRequestOutcome(c, requestOutcomeProviderFailure, managedUsageOutcomeUpstreamError)
			writeSystemOneError(c, 502, "", "The upstream response is invalid.")
			return
		}
		result := document.(map[string]any)
		counts := result["usage"].(map[string]any)
		var inputCount, outputCount *int
		if input, ok := counts["input_tokens"].(float64); ok && input >= 0 && input < float64(int(^uint(0)>>1)/2) {
			count := int(input)
			inputCount = &count
		}
		if output, ok := counts["output_tokens"].(float64); ok && output >= 0 && output < float64(int(^uint(0)>>1)/2) {
			count := int(output)
			outputCount = &count
		}
		usage, _ = normalizeTokenUsage(inputCount, outputCount, nil)
		markRequestOutcome(c, requestOutcomeSuccess, managedUsageOutcomeSuccess)
		c.Data(http.StatusOK, "application/json", body)
	}
}

func (registry *providerRegistry) resolveSystemOneRequest(name string) (providerDefinition, typedDecisionModelDefinition, error) {
	type route struct {
		provider providerDefinition
		model    typedDecisionModelDefinition
	}
	var candidates []route
	for _, identifier := range registry.order {
		provider := registry.definitions[identifier]
		for _, model := range provider.typedDecisionModels {
			resolved, _ := provider.resolvedTransport(model.transportIdentifier)
			configured := resolved.credentialFor(endpointKindText) != "" && resolved.hostedGrantID == ""
			if name == model.identifier.string() || name == model.providerIdentifier.string() {
				if !configured {
					return provider, model, ErrProviderNotConfigured
				}
				return resolved, model, nil
			}
			if configured {
				candidates = append(candidates, route{resolved, model})
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0].provider, candidates[0].model, nil
	}
	return providerDefinition{}, typedDecisionModelDefinition{}, ErrUnknownModel
}

func systemOneContextEnded(c *gin.Context) bool {
	if c.Request.Context().Err() == nil {
		return false
	}
	status := statusClientClosedRequest
	message := "The caller cancelled the request."
	outcome := requestOutcomeCallerCancelled
	if errors.Is(context.Cause(c.Request.Context()), errRequestTimeoutBudgetExpired) {
		status = http.StatusGatewayTimeout
		message = "The request timed out."
		outcome = requestOutcomeProxyTimeout
	}
	markRequestOutcome(c, outcome, managedUsageOutcomeRequestTimeout)
	writeSystemOneError(c, status, "", message)
	return true
}
func systemOneReadBounded(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, systemOneMaximumBytes+1))
	if err != nil {
		return nil, systemOneBadRequest("invalid request body")
	}
	if len(body) > systemOneMaximumBytes {
		return nil, systemOneBoundaryError{http.StatusRequestEntityTooLarge, "request exceeds 32 MiB"}
	}
	return body, nil
}

type systemOneBoundaryError struct {
	status  int
	message string
}

func (failure systemOneBoundaryError) Error() string { return failure.message }
func systemOneBadRequest(message string) error {
	return systemOneBoundaryError{http.StatusBadRequest, message}
}
func decodeSystemOneJSON(body []byte) (any, error) {
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, systemOneBadRequest("invalid JSON or non-finite number")
	}
	return document, nil
}
func systemOneStructuredDepth(value any, depth int) error {
	if depth > systemOneMaximumDepth {
		return systemOneBadRequest("structured value nesting exceeds 64 levels")
	}
	switch object := value.(type) {
	case map[string]any:
		for _, item := range object {
			if err := systemOneStructuredDepth(item, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range object {
			if err := systemOneStructuredDepth(item, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func readSystemOneRequest(request *http.Request, schema *jsonschema.Schema) (systemOneRequest, error) {
	wire, err := systemOneReadBounded(request.Body)
	if err != nil {
		return systemOneRequest{}, err
	}
	decoded := wire
	switch request.Header.Get("Content-Encoding") {
	case "", "identity":
	case "gzip", "x-gzip":
		reader, err := gzip.NewReader(bytes.NewReader(wire))
		if err != nil {
			return systemOneRequest{}, systemOneBadRequest("invalid gzip body")
		}
		defer reader.Close()
		decoded, err = systemOneReadBounded(reader)
		if err != nil {
			return systemOneRequest{}, err
		}
	case "zstd":
		reader, _ := zstd.NewReader(bytes.NewReader(wire), zstd.WithDecoderMaxMemory(systemOneMaximumBytes), zstd.WithDecoderMaxWindow(8<<20), zstd.WithDecoderConcurrency(1))
		defer reader.Close()
		decoded, err = systemOneReadBounded(reader)
		if err != nil {
			return systemOneRequest{}, err
		}
	default:
		return systemOneRequest{}, systemOneBoundaryError{http.StatusUnsupportedMediaType, "unsupported content encoding"}
	}
	contentType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		return systemOneRequest{}, systemOneBoundaryError{http.StatusUnsupportedMediaType, "invalid content type"}
	}
	var parts map[string]bool
	if contentType == "multipart/form-data" {
		reader := multipart.NewReader(bytes.NewReader(decoded), parameters["boundary"])
		part, err := reader.NextPart()
		if err != nil || part.FormName() != "request" {
			return systemOneRequest{}, systemOneBadRequest("first multipart part must be request")
		}
		decoded, err = systemOneReadBounded(part)
		if err != nil {
			return systemOneRequest{}, err
		}
		parts = map[string]bool{}
		for {
			part, err = reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return systemOneRequest{}, systemOneBadRequest("invalid multipart body")
			}
			name := part.FormName()
			if name == "" || name == "request" || parts[name] {
				return systemOneRequest{}, systemOneBadRequest("image parts require unique names")
			}
			parts[name] = true
			if _, err = io.Copy(io.Discard, part); err != nil {
				return systemOneRequest{}, systemOneBadRequest("invalid image part")
			}
		}
	} else if contentType != "application/json" {
		return systemOneRequest{}, systemOneBoundaryError{http.StatusUnsupportedMediaType, "unsupported content type"}
	}
	document, err := decodeSystemOneJSON(decoded)
	if err != nil {
		return systemOneRequest{}, err
	}
	if err = schema.Validate(document); err != nil {
		return systemOneRequest{}, fmt.Errorf("request does not match the TypeSafe schema")
	}
	object := document.(map[string]any)
	questions := object["questions"].(map[string]any)
	if err := systemOneStructuredDepth(object["state"], 0); err != nil {
		return systemOneRequest{}, err
	}
	for _, question := range questions {
		for _, field := range []string{"instructions", "criteria"} {
			if err := systemOneStructuredDepth(question.(map[string]any)[field], 0); err != nil {
				return systemOneRequest{}, err
			}
		}
	}
	images := false
	for _, question := range questions {
		if question.(map[string]any)["type"] == "box" {
			images = true
		}
	}
	if assets, ok := object["assets"].(map[string]any); ok {
		images = true
		for _, asset := range assets {
			item := asset.(map[string]any)
			if part, ok := item["part"].(string); ok {
				if parts == nil || !parts[part] {
					return systemOneRequest{}, systemOneBadRequest("asset part is missing or reused")
				}
				delete(parts, part)
			}
		}
	}
	if len(parts) > 0 {
		return systemOneRequest{}, systemOneBadRequest("image part is not referenced by an asset")
	}
	if images && request.Header.Get("TypeSafe-Danger-Zone-Images") != "true" {
		return systemOneRequest{}, systemOneBadRequest("images require TypeSafe-Danger-Zone-Images: true")
	}
	for _, header := range []string{"TypeSafe-Danger-Zone-Images", "TypeSafe-Danger-Zone-Long-Context"} {
		if values := request.Header.Values(header); len(values) > 1 || (len(values) == 1 && values[0] != "true") {
			return systemOneRequest{}, systemOneBadRequest("experimental header must be true")
		}
	}
	return systemOneRequest{wire: wire, model: object["model"].(string), questions: questions}, nil
}
func validateSystemOneAnswers(document any, questions map[string]any) error {
	answers := document.(map[string]any)["answers"].(map[string]any)
	if len(answers) != len(questions) {
		return fmt.Errorf("answer identifiers differ")
	}
	for name, question := range questions {
		answer, ok := answers[name].(map[string]any)
		if !ok || answer["type"] != question.(map[string]any)["type"] {
			return fmt.Errorf("answer type differs")
		}
	}
	return nil
}
func redactSystemOneValue(value any, secret string) bool {
	changed := false
	switch object := value.(type) {
	case map[string]any:
		for key, item := range object {
			safeKey := strings.ReplaceAll(key, secret, "[redacted]")
			if safeKey != key {
				delete(object, key)
				object[safeKey] = item
				changed = true
			}
			if text, ok := item.(string); ok {
				safeText := strings.ReplaceAll(text, secret, "[redacted]")
				if safeText != text {
					object[safeKey] = safeText
					changed = true
				}
			} else if redactSystemOneValue(item, secret) {
				changed = true
			}
		}
	case []any:
		for index, item := range object {
			if text, ok := item.(string); ok {
				safeText := strings.ReplaceAll(text, secret, "[redacted]")
				if safeText != text {
					object[index] = safeText
					changed = true
				}
			} else if redactSystemOneValue(item, secret) {
				changed = true
			}
		}
	}
	return changed
}
