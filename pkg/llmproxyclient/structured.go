package llmproxyclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const clientStructuredSchemaResource = "urn:llm-proxy-client:structured-output-schema"

var clientIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

var (
	marshalClientStructuredSchema = json.Marshal
	addClientStructuredResource   = func(compiler *jsonschema.Compiler, resource string, document any) error {
		return compiler.AddResource(resource, document)
	}
)

// StructuredOutputInput supplies one caller-owned JSON Schema for provider-enforced output.
type StructuredOutputInput struct {
	JSONSchema []byte
}

type structuredOutput struct {
	canonical json.RawMessage
}

func newStructuredOutput(input *StructuredOutputInput) (*structuredOutput, error) {
	if input == nil {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(input.JSONSchema))
	decoder.UseNumber()
	var document map[string]any
	if decodeError := decoder.Decode(&document); decodeError != nil || document == nil {
		return nil, fmt.Errorf("%w: decode structured output schema: %v", ErrInvalidClientRequest, decodeError)
	}
	if decodeError := decoder.Decode(&struct{}{}); decodeError != io.EOF {
		return nil, fmt.Errorf("%w: structured output schema must contain one JSON value", ErrInvalidClientRequest)
	}
	canonical, marshalError := marshalClientStructuredSchema(document)
	if marshalError != nil {
		return nil, fmt.Errorf("%w: encode structured output schema: %v", ErrInvalidClientRequest, marshalError)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if resourceError := addClientStructuredResource(compiler, clientStructuredSchemaResource, document); resourceError != nil {
		return nil, fmt.Errorf("%w: load structured output schema: %v", ErrInvalidClientRequest, resourceError)
	}
	if _, compileError := compiler.Compile(clientStructuredSchemaResource); compileError != nil {
		return nil, fmt.Errorf("%w: compile structured output schema: %v", ErrInvalidClientRequest, compileError)
	}
	return &structuredOutput{canonical: append(json.RawMessage(nil), canonical...)}, nil
}

func validClientIdempotencyKey(value string) bool {
	return clientIdempotencyKeyPattern.MatchString(value)
}

// StructuredRequestResult is one durable reconciliation snapshot.
type StructuredRequestResult struct {
	State          string `json:"state"`
	ProxyRequestID string `json:"proxy_request_id"`
	StartedAt      string `json:"started_at"`
	UpdatedAt      string `json:"updated_at"`
	ElapsedSeconds int64  `json:"elapsed_seconds"`
	FailureCode    string `json:"-"`
	Output         string `json:"-"`
}

// StructuredRequestPendingError reports a durable structured request that has not reached a terminal state.
type StructuredRequestPendingError struct {
	snapshot StructuredRequestResult
}

// Error reports the accepted durable request state.
func (pending *StructuredRequestPendingError) Error() string {
	return fmt.Sprintf("%s: state=%s", ErrStructuredRequestPending, pending.snapshot.State)
}

// Unwrap preserves errors.Is(error, ErrStructuredRequestPending).
func (pending *StructuredRequestPendingError) Unwrap() error {
	return ErrStructuredRequestPending
}

// Snapshot returns the durable request state that the proxy accepted.
func (pending *StructuredRequestPendingError) Snapshot() StructuredRequestResult {
	return pending.snapshot
}

type structuredRequestErrorResponse struct {
	Error struct {
		Code           string `json:"code"`
		State          string `json:"state"`
		Cause          string `json:"cause"`
		ProxyRequestID string `json:"proxy_request_id"`
	} `json:"error"`
}

// GetStructuredRequest reconciles one structured request without submitting provider work.
func (client Client) GetStructuredRequest(contextValue context.Context, idempotencyKey string) (StructuredRequestResult, error) {
	if !validClientIdempotencyKey(idempotencyKey) {
		return StructuredRequestResult{}, fmt.Errorf("%w: invalid idempotency key", ErrInvalidClientRequest)
	}
	requestURL := *client.config.baseURL
	requestURL.Path = structuredRequestEndpointPath(requestURL.Path)
	requestURL = client.config.authenticatedPostURL(requestURL, "")
	queryValues := requestURL.Query()
	queryValues.Del(queryFormat)
	queryValues.Del(queryProvider)
	requestURL.RawQuery = queryValues.Encode()
	httpRequest := (&http.Request{
		Method: http.MethodGet, URL: &requestURL, Header: http.Header{},
	}).WithContext(contextValue)
	httpRequest.Header.Set(headerAccept, jsonContentType)
	httpRequest.Header.Set(llmproxycontract.HeaderIdempotencyKey, idempotencyKey)
	httpResponse, httpError := client.httpClient.Do(httpRequest)
	if httpError != nil {
		return StructuredRequestResult{}, fmt.Errorf("%w: get structured request: %v", ErrClientHTTPFailure, httpError)
	}
	responseBody, readError := io.ReadAll(httpResponse.Body)
	_ = httpResponse.Body.Close()
	if readError != nil {
		return StructuredRequestResult{}, fmt.Errorf("%w: read structured request response: %v", ErrClientHTTPFailure, readError)
	}
	if httpResponse.StatusCode == http.StatusOK {
		if !json.Valid(responseBody) {
			return StructuredRequestResult{}, fmt.Errorf("%w: structured request result is invalid JSON", ErrClientHTTPFailure)
		}
		return StructuredRequestResult{
			State:  strings.TrimSpace(httpResponse.Header.Get(llmproxycontract.HeaderStructuredRequestState)),
			Output: string(responseBody),
		}, nil
	}
	if httpResponse.StatusCode == http.StatusAccepted {
		return decodeStructuredRequestPending(responseBody)
	}
	result := StructuredRequestResult{}
	var errorResponse structuredRequestErrorResponse
	if json.Unmarshal(responseBody, &errorResponse) == nil {
		result.State = strings.TrimSpace(errorResponse.Error.State)
		result.ProxyRequestID = strings.TrimSpace(errorResponse.Error.ProxyRequestID)
		result.FailureCode = strings.TrimSpace(errorResponse.Error.Cause)
	}
	return result, newHTTPFailure(httpResponse.StatusCode, responseBody)
}

func decodeStructuredRequestPending(responseBody []byte) (StructuredRequestResult, error) {
	var result StructuredRequestResult
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(&result); decodeError != nil {
		return StructuredRequestResult{}, fmt.Errorf("%w: decode structured request status: %v", ErrClientHTTPFailure, decodeError)
	}
	if decodeError := decoder.Decode(&struct{}{}); decodeError != io.EOF ||
		(result.State != "not_dispatched" && result.State != "dispatched") ||
		strings.TrimSpace(result.ProxyRequestID) == "" ||
		strings.TrimSpace(result.StartedAt) == "" ||
		strings.TrimSpace(result.UpdatedAt) == "" ||
		result.ElapsedSeconds < 0 {
		return StructuredRequestResult{}, fmt.Errorf("%w: invalid structured request status", ErrClientHTTPFailure)
	}
	return result, nil
}

func structuredRequestEndpointPath(basePath string) string {
	trimmedPath := strings.TrimRight(strings.TrimSpace(basePath), "/")
	if trimmedPath == "" || trimmedPath == "/v2" {
		return llmproxycontract.StructuredRequestPath
	}
	if strings.HasSuffix(trimmedPath, llmproxycontract.StructuredRequestPath) {
		return trimmedPath
	}
	if strings.HasSuffix(trimmedPath, "/v2") {
		return trimmedPath + "/requests"
	}
	return trimmedPath + llmproxycontract.StructuredRequestPath
}
