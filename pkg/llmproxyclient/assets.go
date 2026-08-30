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

// AssetUploadInput is the exact media content supplied to UploadAsset.
type AssetUploadInput struct {
	MIMEType string
	Data     []byte
}

// Asset is a tenant asset returned by llm-proxy.
type Asset struct {
	AssetID   string    `json:"asset_id"`
	MIMEType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AssetUploadURL builds the authenticated tenant asset upload URL for this config.
func (config Config) AssetUploadURL() string {
	requestURL := config.assetUploadURL()
	return requestURL.String()
}

func (config Config) assetUploadURL() url.URL {
	requestURL := *config.baseURL
	requestURL.Path = assetEndpointPath(requestURL.Path)
	queryValues := requestURL.Query()
	for queryKeyName := range postBodyQueryKeys {
		queryValues.Del(queryKeyName)
	}
	queryValues.Del(queryFormat)
	queryValues.Del(queryProvider)
	queryValues.Set(queryKey, config.secret)
	requestURL.RawQuery = queryValues.Encode()
	return requestURL
}

func assetEndpointPath(basePath string) string {
	trimmedPath := strings.TrimRight(strings.TrimSpace(basePath), "/")
	trimmedPath = strings.TrimSuffix(trimmedPath, "/v2")
	if trimmedPath == "" {
		return llmproxycontract.AssetPath
	}
	return trimmedPath + llmproxycontract.AssetPath
}

// UploadAsset stores exact media bytes for later attachment references.
func (client Client) UploadAsset(contextValue context.Context, input AssetUploadInput) (Asset, error) {
	mimeType := strings.ToLower(strings.TrimSpace(input.MIMEType))
	if !supportedClientMediaMIME(mimeType) {
		return Asset{}, fmt.Errorf("%w: unsupported asset MIME type=%q", ErrInvalidClientRequest, input.MIMEType)
	}
	if len(input.Data) == 0 {
		return Asset{}, fmt.Errorf("%w: asset data is empty", ErrInvalidClientRequest)
	}
	requestURL := client.config.assetUploadURL()
	request := (&http.Request{
		Method:        http.MethodPost,
		URL:           &requestURL,
		Header:        http.Header{},
		Body:          io.NopCloser(bytes.NewReader(input.Data)),
		ContentLength: int64(len(input.Data)),
	}).WithContext(contextValue)
	request.Header.Set(headerContentType, mimeType)
	response, requestError := client.httpClient.Do(request)
	if requestError != nil {
		return Asset{}, fmt.Errorf("%w: upload asset", ErrClientHTTPFailure)
	}
	defer response.Body.Close()
	responseBody, readError := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
	if readError != nil || len(responseBody) > 64*1024 {
		return Asset{}, fmt.Errorf("%w: read asset response", ErrClientHTTPFailure)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Asset{}, newHTTPFailure(response.StatusCode, responseBody)
	}
	var asset Asset
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(&asset); decodeError != nil {
		return Asset{}, fmt.Errorf("%w: decode asset response", ErrClientHTTPFailure)
	}
	if trailingError := decoder.Decode(&struct{}{}); trailingError != io.EOF {
		return Asset{}, fmt.Errorf("%w: decode asset response", ErrClientHTTPFailure)
	}
	if !assetIdentifierPattern.MatchString(asset.AssetID) || asset.MIMEType != mimeType || asset.SizeBytes != int64(len(input.Data)) || asset.State != "available" || asset.CreatedAt.IsZero() || !asset.ExpiresAt.After(asset.CreatedAt) {
		return Asset{}, fmt.Errorf("%w: invalid asset response", ErrClientHTTPFailure)
	}
	return asset, nil
}

func supportedClientMediaMIME(mimeType string) bool {
	switch mimeType {
	case audioMIMEM4A, audioMIMEMPEG, audioMIMEWAV, imageMIMEJPEG, imageMIMEPNG, imageMIMEWebP:
		return true
	default:
		return false
	}
}
