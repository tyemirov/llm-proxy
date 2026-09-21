package proxy_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestTenantAssetSupportedTypesMatchPublicContract(t *testing.T) {
	router := mediaAssetRouter(t, "http://127.0.0.1:1", t.TempDir(), testfixtures.ModelCatalog(t), 60)
	server := httptest.NewServer(router)
	defer server.Close()
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "secret-a"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, assetContractClient{t, contract, server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, mimeType := range []string{"image/jpeg", "image/png", "image/webp", "audio/m4a", "audio/mpeg", "audio/wav", "audio/flac", "audio/ogg", "video/mp4", "video/webm", "application/json", "application/x-subrip", "application/octet-stream"} {
		t.Run(mimeType, func(t *testing.T) {
			data := []byte(`{"exact":"asset bytes"}`)
			asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: mimeType, Data: data})
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := client.GetAsset(t.Context(), asset.AssetID)
			if err != nil || metadata.MIMEType != mimeType {
				t.Fatalf("metadata=%+v error=%v", metadata, err)
			}
			download, err := client.DownloadAsset(t.Context(), metadata)
			if err != nil || !bytes.Equal(download, data) {
				t.Fatalf("asset bytes changed: error=%v", err)
			}
		})
	}
}

type assetContractClient struct {
	t        *testing.T
	contract *openapitest.Contract
	client   *http.Client
}

func (client assetContractClient) Do(request *http.Request) (*http.Response, error) {
	path := "/model/v1/assets"
	if request.Method == http.MethodGet {
		path += "/{asset_id}"
		if strings.HasSuffix(request.URL.Path, "/content") {
			path += "/content"
		}
	}
	var body []byte
	if request.Body != nil {
		var err error
		body, err = io.ReadAll(request.Body)
		if err != nil {
			client.t.Fatal(err)
		}
		_ = request.Body.Close()
		request.Body = io.NopCloser(bytes.NewReader(body))
	}
	if err := client.contract.ValidateRequest(path, request.Method, request, body); err != nil {
		client.t.Error(err)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return nil, err
	}
	body, err = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		client.t.Fatal(err)
	}
	if err := client.contract.ValidateResponse(path, request.Method, response.StatusCode, response.Header, body); err != nil {
		client.t.Error(err)
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}
