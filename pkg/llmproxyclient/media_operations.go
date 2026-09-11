package llmproxyclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const mediaResourceMaximumBytes = 8 * 1024 * 1024

// MediaOperationInput is one complete media-generation intent.
type MediaOperationInput struct {
	Capability string          `json:"capability"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Input      json.RawMessage `json:"input"`
	Controls   json.RawMessage `json:"controls"`
}

// MediaOperation describes one durable tenant-owned generation.
type MediaOperation struct {
	OperationID       string                 `json:"operation_id"`
	Capability        string                 `json:"capability"`
	Provider          string                 `json:"provider"`
	Model             string                 `json:"model"`
	CatalogRevision   string                 `json:"catalog_revision"`
	State             string                 `json:"state"`
	CancellationState string                 `json:"cancellation_state"`
	Outputs           []MediaOperationOutput `json:"outputs"`
	Error             *MediaOperationError   `json:"error,omitempty"`
	Cost              MediaOperationCost     `json:"cost"`
	AcceptedAt        time.Time              `json:"accepted_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	DeadlineAt        time.Time              `json:"deadline_at"`
}

// MediaOperationOutput identifies one ordered result asset.
type MediaOperationOutput struct {
	AssetID   string `json:"asset_id"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	Ordinal   int    `json:"ordinal"`
}

// MediaOperationError is caller-safe terminal error evidence.
type MediaOperationError struct {
	Code string `json:"code"`
}

// MediaOperationCost reports exact evidence or an explicit unavailable reason.
type MediaOperationCost struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// MediaCapabilities lists tenant-available durable routes.
type MediaCapabilities struct {
	CatalogRevision string                 `json:"catalog_revision"`
	Routes          []MediaCapabilityRoute `json:"routes"`
}

// MediaCapabilityRoute is one tenant-available capability, provider, and model route.
type MediaCapabilityRoute struct {
	Capability string            `json:"capability"`
	Provider   string            `json:"provider"`
	Model      string            `json:"model"`
	Controls   []json.RawMessage `json:"controls"`
	Limits     []json.RawMessage `json:"limits"`
}

// CreateMediaOperation validates and accepts one durable media operation.
func (client Client) CreateMediaOperation(contextValue context.Context, idempotencyKey string, input MediaOperationInput) (MediaOperation, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || strings.TrimSpace(input.Capability) == "" || strings.TrimSpace(input.Provider) == "" || strings.TrimSpace(input.Model) == "" || !validJSONObject(input.Input) || !validJSONObject(input.Controls) {
		return MediaOperation{}, fmt.Errorf("%w: invalid media operation", ErrInvalidClientRequest)
	}
	body, _ := json.Marshal(input)
	requestURL := client.config.mediaResourceURL(llmproxycontract.MediaOperationsPath)
	request := (&http.Request{Method: http.MethodPost, URL: &requestURL, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body))}).WithContext(contextValue)
	request.Header.Set(headerContentType, "application/json")
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, idempotencyKey)
	return client.doMediaOperation(request, http.StatusAccepted, http.StatusOK)
}

// GetMediaOperation reads one tenant-owned durable operation.
func (client Client) GetMediaOperation(contextValue context.Context, operationID string) (MediaOperation, error) {
	if !strings.HasPrefix(operationID, "mop_") {
		return MediaOperation{}, fmt.Errorf("%w: invalid media operation identifier", ErrInvalidClientRequest)
	}
	requestURL := client.config.mediaResourceURL(llmproxycontract.MediaOperationsPath + "/" + operationID)
	request := (&http.Request{Method: http.MethodGet, URL: &requestURL, Header: http.Header{}}).WithContext(contextValue)
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	return client.doMediaOperation(request, http.StatusOK)
}

// CancelMediaOperation explicitly requests cancellation and returns the observed outcome.
func (client Client) CancelMediaOperation(contextValue context.Context, operationID string) (MediaOperation, error) {
	if !strings.HasPrefix(operationID, "mop_") {
		return MediaOperation{}, fmt.Errorf("%w: invalid media operation identifier", ErrInvalidClientRequest)
	}
	requestURL := client.config.mediaResourceURL(llmproxycontract.MediaOperationsPath + "/" + operationID + "/cancellation")
	request := (&http.Request{Method: http.MethodPut, URL: &requestURL, Header: http.Header{}}).WithContext(contextValue)
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	return client.doMediaOperation(request, http.StatusOK)
}

// WaitMediaOperation polls status without changing cancellation authority.
func (client Client) WaitMediaOperation(contextValue context.Context, operationID string, interval time.Duration) (MediaOperation, error) {
	if interval <= 0 {
		return MediaOperation{}, fmt.Errorf("%w: invalid media operation poll interval", ErrInvalidClientRequest)
	}
	var current MediaOperation
	for {
		operation, operationError := client.GetMediaOperation(contextValue, operationID)
		if operationError != nil {
			if contextValue.Err() != nil {
				return current, contextValue.Err()
			}
			return MediaOperation{}, operationError
		}
		current = operation
		if mediaOperationTerminal(operation.State) {
			return operation, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-contextValue.Done():
			timer.Stop()
			return current, contextValue.Err()
		case <-timer.C:
		}
	}
}

// GetMediaCapabilities reads routes available to the authenticated tenant.
func (client Client) GetMediaCapabilities(contextValue context.Context) (MediaCapabilities, error) {
	requestURL := client.config.mediaResourceURL(llmproxycontract.MediaCapabilitiesPath)
	request := (&http.Request{Method: http.MethodGet, URL: &requestURL, Header: http.Header{}}).WithContext(contextValue)
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	responseBody, responseError := client.doMediaResource(request, http.StatusOK)
	if responseError != nil {
		return MediaCapabilities{}, responseError
	}
	var capabilities MediaCapabilities
	if decodeError := decodeExactJSON(responseBody, &capabilities); decodeError != nil || capabilities.CatalogRevision == "" || capabilities.Routes == nil || !validMediaCapabilityRoutes(capabilities.Routes) {
		return MediaCapabilities{}, fmt.Errorf("%w: invalid media capabilities response", ErrClientHTTPFailure)
	}
	return capabilities, nil
}

func validMediaCapabilityRoutes(routes []MediaCapabilityRoute) bool {
	for _, route := range routes {
		if route.Capability == "" || route.Provider == "" || route.Model == "" || route.Controls == nil || route.Limits == nil {
			return false
		}
	}
	return true
}

// GetAsset reads tenant-owned asset metadata.
func (client Client) GetAsset(contextValue context.Context, assetID string) (Asset, error) {
	requestURL, identifierError := client.config.assetResourceURL(assetID, "")
	if identifierError != nil {
		return Asset{}, identifierError
	}
	request := (&http.Request{Method: http.MethodGet, URL: &requestURL, Header: http.Header{}}).WithContext(contextValue)
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	responseBody, responseError := client.doMediaResource(request, http.StatusOK)
	if responseError != nil {
		return Asset{}, responseError
	}
	var asset Asset
	if decodeError := decodeExactJSON(responseBody, &asset); decodeError != nil || !validAsset(asset) {
		return Asset{}, fmt.Errorf("%w: invalid asset response", ErrClientHTTPFailure)
	}
	return asset, nil
}

// DownloadAsset reads exact authenticated bytes and checks the advertised byte count.
func (client Client) DownloadAsset(contextValue context.Context, asset Asset) ([]byte, error) {
	if !validAsset(asset) {
		return nil, fmt.Errorf("%w: invalid asset", ErrInvalidClientRequest)
	}
	requestURL, _ := client.config.assetResourceURL(asset.AssetID, "/content")
	request := (&http.Request{Method: http.MethodGet, URL: &requestURL, Header: http.Header{}}).WithContext(contextValue)
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	response, requestError := client.httpClient.Do(request)
	if requestError != nil || response == nil || response.Body == nil {
		return nil, fmt.Errorf("%w: download asset", ErrClientHTTPFailure)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
		return nil, newHTTPFailure(response.StatusCode, body)
	}
	data, readError := io.ReadAll(io.LimitReader(response.Body, asset.SizeBytes+1))
	if readError != nil || int64(len(data)) != asset.SizeBytes || response.Header.Get("Content-Type") != asset.MIMEType {
		return nil, fmt.Errorf("%w: invalid asset content", ErrClientHTTPFailure)
	}
	return data, nil
}

// DeleteAsset deletes one unreferenced tenant asset.
func (client Client) DeleteAsset(contextValue context.Context, assetID string) error {
	requestURL, identifierError := client.config.assetResourceURL(assetID, "")
	if identifierError != nil {
		return identifierError
	}
	request := (&http.Request{Method: http.MethodDelete, URL: &requestURL, Header: http.Header{}}).WithContext(contextValue)
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	_, responseError := client.doMediaResource(request, http.StatusNoContent)
	return responseError
}

func (client Client) doMediaOperation(request *http.Request, statuses ...int) (MediaOperation, error) {
	responseBody, responseError := client.doMediaResource(request, statuses...)
	if responseError != nil {
		return MediaOperation{}, responseError
	}
	var operation MediaOperation
	if decodeError := decodeExactJSON(responseBody, &operation); decodeError != nil || !validMediaOperation(operation) {
		return MediaOperation{}, fmt.Errorf("%w: invalid media operation response", ErrClientHTTPFailure)
	}
	return operation, nil
}

func (client Client) doMediaResource(request *http.Request, statuses ...int) ([]byte, error) {
	response, requestError := client.httpClient.Do(request)
	if requestError != nil || response == nil || response.Body == nil {
		return nil, fmt.Errorf("%w: media resource request", ErrClientHTTPFailure)
	}
	defer response.Body.Close()
	body, readError := io.ReadAll(io.LimitReader(response.Body, mediaResourceMaximumBytes+1))
	if readError != nil || len(body) > mediaResourceMaximumBytes {
		return nil, fmt.Errorf("%w: read media resource response", ErrClientHTTPFailure)
	}
	for _, status := range statuses {
		if response.StatusCode == status {
			return body, nil
		}
	}
	return nil, newHTTPFailure(response.StatusCode, body)
}

func (config Config) mediaResourceURL(resourcePath string) url.URL {
	requestURL := *config.baseURL
	trimmedPath := strings.TrimRight(strings.TrimSpace(requestURL.Path), "/")
	trimmedPath = strings.TrimSuffix(trimmedPath, "/v2")
	requestURL.Path = trimmedPath + resourcePath
	requestURL.RawQuery = ""
	requestURL.Fragment = ""
	return requestURL
}

func (config Config) assetResourceURL(assetID string, suffix string) (url.URL, error) {
	if !assetIdentifierPattern.MatchString(assetID) {
		return url.URL{}, fmt.Errorf("%w: invalid asset identifier", ErrInvalidClientRequest)
	}
	return config.mediaResourceURL(llmproxycontract.AssetPath + "/" + assetID + suffix), nil
}

func validJSONObject(value json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(value))
	var object map[string]any
	return decoder.Decode(&object) == nil && object != nil && decoder.Decode(&struct{}{}) == io.EOF
}

func decodeExactJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(destination); decodeError != nil {
		return decodeError
	}
	if trailingError := decoder.Decode(&struct{}{}); trailingError != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func validMediaOperation(operation MediaOperation) bool {
	if !strings.HasPrefix(operation.OperationID, "mop_") || operation.Capability == "" || operation.Provider == "" || operation.Model == "" || operation.CatalogRevision == "" || operation.AcceptedAt.IsZero() || operation.UpdatedAt.Before(operation.AcceptedAt) || !operation.DeadlineAt.After(operation.AcceptedAt) || operation.Outputs == nil {
		return false
	}
	switch operation.State {
	case llmproxycontract.MediaOperationStateQueued, llmproxycontract.MediaOperationStateRunning, llmproxycontract.MediaOperationStateSucceeded, llmproxycontract.MediaOperationStateFailed, llmproxycontract.MediaOperationStateCancelled, llmproxycontract.MediaOperationStateUncertain:
		return true
	default:
		return false
	}
}

func mediaOperationTerminal(state string) bool {
	switch state {
	case llmproxycontract.MediaOperationStateSucceeded, llmproxycontract.MediaOperationStateFailed, llmproxycontract.MediaOperationStateCancelled, llmproxycontract.MediaOperationStateUncertain:
		return true
	default:
		return false
	}
}

func validAsset(asset Asset) bool {
	return assetIdentifierPattern.MatchString(asset.AssetID) && supportedClientMediaMIME(asset.MIMEType) && asset.SizeBytes > 0 && asset.State == "available" && !asset.CreatedAt.IsZero() && asset.ExpiresAt.After(asset.CreatedAt)
}
