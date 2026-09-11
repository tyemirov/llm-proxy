package llmproxyclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type mediaOperationDoer func(*http.Request) (*http.Response, error)

func (doer mediaOperationDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type mediaOperationErrorReader struct{}

func (mediaOperationErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (mediaOperationErrorReader) Close() error             { return nil }

func mediaOperationTestClient(doer HTTPDoer) Client {
	return Client{config: Config{baseURL: &url.URL{Scheme: "https", Host: "proxy.example", Path: "/v2"}, secret: "secret"}, httpClient: doer}
}

func mediaOperationTestResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func validMediaOperationJSON(state string) string {
	return `{"operation_id":"mop_0123456789abcdef0123456789abcdef","capability":"video.generate","provider":"xai","model":"grok-imagine-video-1.5","catalog_revision":"revision","state":"` + state + `","cancellation_state":"not_requested","outputs":[],"cost":{"available":false,"reason":"exact_price_unavailable"},"accepted_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:01Z","deadline_at":"2026-01-01T00:01:00Z"}`
}

func TestMediaOperationClientRejectsInvalidInputs(testingInstance *testing.T) {
	client := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("must not dispatch")
	}))
	validInput := MediaOperationInput{Capability: "video.generate", Provider: "xai", Model: "model", Input: []byte(`{}`), Controls: []byte(`{}`)}
	invalidInputs := []MediaOperationInput{
		{},
		{Capability: "video.generate", Provider: "xai", Model: "model", Input: []byte(`[]`), Controls: []byte(`{}`)},
		{Capability: "video.generate", Provider: "xai", Model: "model", Input: []byte(`{}`), Controls: []byte(`null`)},
	}
	for _, input := range invalidInputs {
		if _, operationError := client.CreateMediaOperation(context.Background(), "key", input); !errors.Is(operationError, ErrInvalidClientRequest) {
			testingInstance.Fatalf("input=%s error=%v", input.Input, operationError)
		}
	}
	if _, operationError := client.CreateMediaOperation(context.Background(), "", validInput); !errors.Is(operationError, ErrInvalidClientRequest) {
		testingInstance.Fatal(operationError)
	}
	if _, operationError := client.GetMediaOperation(context.Background(), "invalid"); !errors.Is(operationError, ErrInvalidClientRequest) {
		testingInstance.Fatal(operationError)
	}
	if _, operationError := client.CancelMediaOperation(context.Background(), "invalid"); !errors.Is(operationError, ErrInvalidClientRequest) {
		testingInstance.Fatal(operationError)
	}
	if _, operationError := client.WaitMediaOperation(context.Background(), "mop_0123456789abcdef0123456789abcdef", 0); !errors.Is(operationError, ErrInvalidClientRequest) {
		testingInstance.Fatal(operationError)
	}
	if _, assetError := client.GetAsset(context.Background(), "invalid"); !errors.Is(assetError, ErrInvalidClientRequest) {
		testingInstance.Fatal(assetError)
	}
	if download, assetError := client.DownloadAsset(context.Background(), Asset{}); download != nil || !errors.Is(assetError, ErrInvalidClientRequest) {
		testingInstance.Fatal(assetError)
	}
	if assetError := client.DeleteAsset(context.Background(), "invalid"); !errors.Is(assetError, ErrInvalidClientRequest) {
		testingInstance.Fatal(assetError)
	}
	if _, assetError := client.config.assetResourceURL("invalid", ""); !errors.Is(assetError, ErrInvalidClientRequest) {
		testingInstance.Fatal(assetError)
	}
}

func TestMediaOperationClientHandlesPollingAndResourceFailures(testingInstance *testing.T) {
	operationID := "mop_0123456789abcdef0123456789abcdef"
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	requestFailureClient := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	}))
	if _, operationError := requestFailureClient.WaitMediaOperation(cancelledContext, operationID, time.Millisecond); !errors.Is(operationError, context.Canceled) {
		testingInstance.Fatalf("cancelled wait error=%v", operationError)
	}
	if _, operationError := requestFailureClient.WaitMediaOperation(context.Background(), operationID, time.Millisecond); !errors.Is(operationError, ErrClientHTTPFailure) {
		testingInstance.Fatalf("failed wait error=%v", operationError)
	}

	queuedClient := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		return mediaOperationTestResponse(http.StatusOK, validMediaOperationJSON("queued")), nil
	}))
	waitContext, cancelWait := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelWait()
	if current, operationError := queuedClient.WaitMediaOperation(waitContext, operationID, time.Millisecond); !errors.Is(operationError, context.DeadlineExceeded) || current.State != "queued" {
		testingInstance.Fatalf("current=%+v error=%v", current, operationError)
	}

	nilResponseClient := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) { return nil, nil }))
	if _, operationError := nilResponseClient.GetMediaOperation(context.Background(), operationID); !errors.Is(operationError, ErrClientHTTPFailure) {
		testingInstance.Fatal(operationError)
	}
	readFailureClient := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: mediaOperationErrorReader{}}, nil
	}))
	if _, operationError := readFailureClient.GetMediaOperation(context.Background(), operationID); !errors.Is(operationError, ErrClientHTTPFailure) {
		testingInstance.Fatal(operationError)
	}
	largeResponseClient := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		return mediaOperationTestResponse(http.StatusOK, strings.Repeat("x", mediaResourceMaximumBytes+1)), nil
	}))
	if _, operationError := largeResponseClient.GetMediaOperation(context.Background(), operationID); !errors.Is(operationError, ErrClientHTTPFailure) {
		testingInstance.Fatal(operationError)
	}
	invalidOperationClient := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		return mediaOperationTestResponse(http.StatusOK, `{}`), nil
	}))
	if _, operationError := invalidOperationClient.GetMediaOperation(context.Background(), operationID); !errors.Is(operationError, ErrClientHTTPFailure) {
		testingInstance.Fatal(operationError)
	}
}

func TestMediaOperationClientValidatesCapabilitiesAndAssets(testingInstance *testing.T) {
	failureClient := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		return mediaOperationTestResponse(http.StatusBadGateway, `{"error":{"code":"provider_error"}}`), nil
	}))
	if _, capabilityError := failureClient.GetMediaCapabilities(context.Background()); capabilityError == nil {
		testingInstance.Fatal("expected capability failure")
	}
	if _, assetError := failureClient.GetAsset(context.Background(), "ast_0123456789abcdef0123456789abcdef"); assetError == nil {
		testingInstance.Fatal("expected asset failure")
	}
	if assetError := failureClient.DeleteAsset(context.Background(), "ast_0123456789abcdef0123456789abcdef"); assetError == nil {
		testingInstance.Fatal("expected delete failure")
	}

	invalidCapabilities := []string{
		`{}`,
		`{"catalog_revision":"revision","routes":[{"capability":"","provider":"xai","model":"model","controls":[],"limits":[]}]}`,
	}
	for _, body := range invalidCapabilities {
		client := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
			return mediaOperationTestResponse(http.StatusOK, body), nil
		}))
		if _, capabilityError := client.GetMediaCapabilities(context.Background()); !errors.Is(capabilityError, ErrClientHTTPFailure) {
			testingInstance.Fatalf("body=%s error=%v", body, capabilityError)
		}
	}

	invalidAssetClient := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		return mediaOperationTestResponse(http.StatusOK, `{}`), nil
	}))
	if _, assetError := invalidAssetClient.GetAsset(context.Background(), "ast_0123456789abcdef0123456789abcdef"); !errors.Is(assetError, ErrClientHTTPFailure) {
		testingInstance.Fatal(assetError)
	}
}

func TestMediaOperationClientValidatesDownloadedBytes(testingInstance *testing.T) {
	asset := Asset{AssetID: "ast_0123456789abcdef0123456789abcdef", MIMEType: "video/mp4", SizeBytes: 4, State: "available", CreatedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Minute)}
	testCases := []struct {
		name string
		doer HTTPDoer
	}{
		{name: "request", doer: mediaOperationDoer(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF })},
		{name: "body", doer: mediaOperationDoer(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: nil}, nil
		})},
		{name: "status", doer: mediaOperationDoer(func(*http.Request) (*http.Response, error) {
			return mediaOperationTestResponse(http.StatusNotFound, `{}`), nil
		})},
		{name: "read", doer: mediaOperationDoer(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: mediaOperationErrorReader{}}, nil
		})},
		{name: "size", doer: mediaOperationDoer(func(*http.Request) (*http.Response, error) {
			response := mediaOperationTestResponse(http.StatusOK, "bad")
			response.Header.Set("Content-Type", "video/mp4")
			return response, nil
		})},
		{name: "mime", doer: mediaOperationDoer(func(*http.Request) (*http.Response, error) {
			return mediaOperationTestResponse(http.StatusOK, "data"), nil
		})},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			if content, downloadError := mediaOperationTestClient(testCase.doer).DownloadAsset(context.Background(), asset); content != nil || !errors.Is(downloadError, ErrClientHTTPFailure) {
				testingInstance.Fatalf("content=%q error=%v", content, downloadError)
			}
		})
	}
}

func TestMediaOperationClientExactJSONAndStateValidation(testingInstance *testing.T) {
	var destination map[string]any
	if decodeError := decodeExactJSON([]byte(`{`), &destination); decodeError == nil {
		testingInstance.Fatal("expected decode failure")
	}
	if decodeError := decodeExactJSON([]byte(`{} {}`), &destination); decodeError == nil {
		testingInstance.Fatal("expected trailing JSON failure")
	}
	if validMediaOperation(MediaOperation{}) {
		testingInstance.Fatal("empty operation accepted")
	}
	invalidState := validMediaOperationJSON("future")
	client := mediaOperationTestClient(mediaOperationDoer(func(*http.Request) (*http.Response, error) {
		return mediaOperationTestResponse(http.StatusOK, invalidState), nil
	}))
	if _, operationError := client.GetMediaOperation(context.Background(), "mop_0123456789abcdef0123456789abcdef"); !errors.Is(operationError, ErrClientHTTPFailure) {
		testingInstance.Fatal(operationError)
	}
}
