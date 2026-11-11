package llmproxyclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// SystemOneRequestOptions carries TypeSafe wire metadata. Body bytes can contain
// JSON, multipart data, or an encoded representation selected by ContentEncoding.
type SystemOneRequestOptions struct {
	ContentType     string
	ContentEncoding string
	Headers         http.Header
}

// SystemOneResponse retains the exact System One response, including provider errors.
type SystemOneResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// PostSystemOne sends the TypeSafe wire body without interpreting its questions.
// For non-2xx responses it returns both the response and an HTTPFailure.
func (client Client) PostSystemOne(ctx context.Context, body []byte, options SystemOneRequestOptions) (SystemOneResponse, error) {
	requestURL := client.config.mediaResourceURL(llmproxycontract.SystemOnePath)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(body))
	if err != nil {
		return SystemOneResponse{}, fmt.Errorf("%w: create System One request: %w", ErrInvalidClientRequest, err)
	}
	for name, values := range options.Headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	contentType := options.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	request.Header.Set(headerContentType, contentType)
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	if options.ContentEncoding != "" {
		request.Header.Set("Content-Encoding", options.ContentEncoding)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return SystemOneResponse{}, fmt.Errorf("%w: post System One: %w", ErrClientHTTPFailure, err)
	}
	defer response.Body.Close()
	result := SystemOneResponse{StatusCode: response.StatusCode, Headers: response.Header.Clone()}
	result.Body, err = io.ReadAll(response.Body)
	if err != nil {
		return result, fmt.Errorf("%w: read System One response: %w", ErrClientHTTPFailure, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, newHTTPFailure(response.StatusCode, result.Body)
	}
	return result, nil
}
