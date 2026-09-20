package proxy_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func TestImageGenerationEditsOrderedAssetsAndMask(t *testing.T) {
	fixtures := []imageEditingFixture{
		{catalog: testfixtures.ProviderCatalog(t), provider: "openai", model: "gpt-image-2", path: "/images/edits", header: "Authorization", credential: "Bearer image-provider-secret"},
		imageEditingSecondProvider(t),
	}
	for _, fixture := range fixtures {
		t.Run(fixture.provider, func(t *testing.T) {
			testImageEditingOrderedAssetsAndMask(t, fixture)
		})
	}
}

func TestImageGenerationEditingRejectsInvalidAndForeignAssetsWithoutDispatch(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusBadRequest)
	}))
	defer upstream.Close()
	endpoints := proxy.NewEndpoints()
	endpoints.SetProviderBaseURL("openai", upstream.URL)
	configuration, err := testfixtures.ProvisionManagedRouter(t, proxy.Configuration{
		ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: endpoints,
	}, zap.NewNop().Sugar(), testfixtures.ManagedTenant{
		Secret: "editing-owner", Defaults: proxy.TenantDefaults{Provider: "openai", Model: proxy.ModelNameGPT41},
		ProviderKeys: map[string]string{"openai": "image-provider-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	newClient := func(secret string) llmproxyclient.Client {
		config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
		if err != nil {
			t.Fatal(err)
		}
		client, err := llmproxyclient.NewClient(config, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	owner := newClient("editing-owner")
	cookie := imageGenerationOwnerCookie(t, configuration.Management, "foreign-editing-owner")
	account := requestManagementAccount(t, router, cookie)
	foreign := newClient(generateManagementTenantSecret(t, router, cookie, account.Tenants[0].ID))
	upload := func(client llmproxyclient.Client, mimeType string, data []byte) string {
		asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: mimeType, Data: data})
		if err != nil {
			t.Fatal(err)
		}
		return asset.AssetID
	}
	encodePNG := func(canvas image.Image) []byte {
		var data bytes.Buffer
		if err := png.Encode(&data, canvas); err != nil {
			t.Fatal(err)
		}
		return data.Bytes()
	}
	validBytes := encodePNG(image.NewNRGBA(image.Rect(0, 0, 64, 64)))
	validID := upload(owner, "image/png", validBytes)
	foreignID := upload(foreign, "image/png", validBytes)
	smallMaskID := upload(owner, "image/png", encodePNG(image.NewNRGBA(image.Rect(0, 0, 32, 32))))
	opaqueMaskID := upload(owner, "image/png", encodePNG(image.NewGray(image.Rect(0, 0, 64, 64))))
	opaqueColorMask := image.NewRGBA(image.Rect(0, 0, 64, 64))
	draw.Draw(opaqueColorMask, opaqueColorMask.Bounds(), image.NewUniform(color.RGBA{R: 200, A: 255}), image.Point{}, draw.Src)
	opaqueColorMaskID := upload(owner, "image/png", encodePNG(opaqueColorMask))
	corruptedID := upload(owner, "image/png", validBytes[:len(validBytes)-8])
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, image.NewRGBA(image.Rect(0, 0, 64, 64)), nil); err != nil {
		t.Fatal(err)
	}
	jpegID := upload(owner, "image/jpeg", jpegBytes.Bytes())
	tooMany := make([]string, 17)
	for index := range tooMany {
		tooMany[index] = validID
	}
	for index, scenario := range []struct {
		name   string
		images []string
		mask   string
	}{
		{"empty", nil, ""},
		{"too many", tooMany, ""},
		{"foreign image", []string{foreignID}, ""},
		{"foreign mask", []string{validID}, foreignID},
		{"mask dimensions", []string{validID}, smallMaskID},
		{"opaque mask", []string{validID}, opaqueMaskID},
		{"opaque color mask", []string{validID}, opaqueColorMaskID},
		{"JPEG mask", []string{validID}, jpegID},
		{"corrupted image", []string{corruptedID}, ""},
		{"native identifier", []string{"file_provider_native"}, ""},
		{"external URL", []string{"https://example.com/image.png"}, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, err := owner.CreateImageEditing(t.Context(), "rejected-"+strconv.Itoa(index), llmproxyclient.ImageEditingInput{
				ImageGenerationInput: imageGenerationTestIntent(), ImageAssetIDs: scenario.images, MaskAssetID: scenario.mask,
			})
			if httpFailureStatus(err) != http.StatusBadRequest || calls.Load() != 0 {
				t.Fatalf("rejected edit: %v calls=%d", err, calls.Load())
			}
		})
	}
}

type imageEditingFixture struct {
	catalog                                   *proxy.ProviderCatalog
	provider, model, path, header, credential string
}

func imageEditingSecondProvider(t *testing.T) imageEditingFixture {
	t.Helper()
	const providerID = "editing-fixture"
	schema := testfixtures.ProviderCatalog(t).Schema()
	for _, provider := range schema.Providers {
		if provider.ID != "openai" {
			continue
		}
		provider.ID = providerID
		provider.Fields = append([]proxy.ProviderCatalogField(nil), provider.Fields...)
		provider.Fields[0].ID = "edit_token"
		provider.Fields[0].Environment = ""
		provider.Transports = append([]proxy.ProviderCatalogTransport(nil), provider.Transports...)
		for index := range provider.Transports {
			provider.Transports[index].Components.Authentication = proxy.ProviderCatalogAuthentication{Kind: proxy.CatalogAuthenticationHeader, Field: "edit_token", Header: "X-Edit-Token"}
			if provider.Transports[index].ID == "text" {
				provider.Transports[index].ID = "fixture-responses"
				provider.Transports[index].Endpoint.Path = "/compose"
			}
			if provider.Transports[index].ID == "image-edit" {
				provider.Transports[index].ID = "fixture-edit"
				provider.Transports[index].Endpoint.Path = "/paint"
			}
		}
		provider.Offerings = append([]proxy.ProviderCatalogOffering(nil), provider.Offerings...)
		for index := range provider.Offerings {
			if provider.Offerings[index].Transport == "text" {
				provider.Offerings[index].Transport = "fixture-responses"
			}
			if provider.Offerings[index].Model == "gpt-5" {
				provider.Offerings[index].UpstreamModel = "fixture-text-model"
			}
			if provider.Offerings[index].Model == "gpt-image-2" {
				provider.Offerings[index].UpstreamModel = "fixture-image-model"
				provider.Offerings[index].ImageRoutes.Editing = "fixture-edit"
				provider.Offerings[index].ImageRoutes.Responses = "fixture-responses"
			}
		}
		schema.Providers = append(schema.Providers, provider)
		break
	}
	document, err := yaml.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(document)
	if err != nil {
		t.Fatal(err)
	}
	return imageEditingFixture{catalog: catalog, provider: providerID, model: "fixture-image-model", path: "/paint", header: "X-Edit-Token", credential: "image-provider-secret"}
}

func testImageEditingOrderedAssetsAndMask(t *testing.T, fixture imageEditingFixture) {
	t.Helper()
	images := make([][]byte, 3)
	for index := range images {
		canvas := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
		canvas.SetRGBA(index, index, color.RGBA{R: uint8(60 + index), A: 255})
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, canvas); err != nil {
			t.Fatal(err)
		}
		images[index] = encoded.Bytes()
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != fixture.path || request.Header.Get(fixture.header) != fixture.credential || (fixture.header != "Authorization" && request.Header.Get("Authorization") != "") {
			t.Errorf("unexpected edit request method=%s path=%s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := request.ParseMultipartForm(4 << 20); err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		defer request.MultipartForm.RemoveAll()
		expected := map[string][]string{"model": {fixture.model}, "prompt": {"Place the first subject beside the second"}, "quality": {"low"}, "size": {"1024x1024"}, "background": {"transparent"}, "output_format": {"png"}, "n": {"2"}}
		if !reflect.DeepEqual(request.MultipartForm.Value, expected) || len(request.MultipartForm.File["image[]"]) != 2 || len(request.MultipartForm.File["mask"]) != 1 {
			t.Errorf("edit multipart fields=%v files=%v", request.MultipartForm.Value, request.MultipartForm.File)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		parts := append(request.MultipartForm.File["image[]"], request.MultipartForm.File["mask"][0])
		for index, part := range parts {
			file, err := part.Open()
			if err != nil {
				t.Error(err)
				return
			}
			data, readError := io.ReadAll(file)
			closeError := file.Close()
			if readError != nil || closeError != nil || part.Header.Get("Content-Type") != "image/png" || !bytes.Equal(data, images[index]) {
				t.Errorf("edit input %d differs: read=%v close=%v", index, readError, closeError)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]string{
			{"b64_json": base64.StdEncoding.EncodeToString(images[1])},
			{"b64_json": base64.StdEncoding.EncodeToString(images[0])},
		}})
	}))
	defer upstream.Close()
	client := imageGenerationTestClient(t, upstream, fixture.catalog, fixture.provider)
	assets := make([]llmproxyclient.Asset, len(images))
	for index, data := range images {
		asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: data})
		if err != nil {
			t.Fatal(err)
		}
		assets[index] = asset
	}
	inputJSON, _ := json.Marshal(map[string]any{"prompt": "Place the first subject beside the second", "image_asset_ids": []string{assets[0].AssetID, assets[1].AssetID}, "mask_asset_id": assets[2].AssetID})
	input := llmproxyclient.MediaOperationInput{
		Capability: "image.edit", Provider: fixture.provider, Model: "gpt-image-2", Input: inputJSON,
		Controls: json.RawMessage(`{"surface":"images","quality":"low","size":"1024x1024","background":"transparent","output_format":"png","output_count":2}`),
	}
	typedInput := llmproxyclient.ImageEditingInput{
		ImageGenerationInput: imageGenerationTestIntent(),
		ImageAssetIDs:        []string{assets[0].AssetID, assets[1].AssetID}, MaskAssetID: assets[2].AssetID,
	}
	typedInput.Prompt = "Place the first subject beside the second"
	typedInput.Provider = fixture.provider
	typedInput.Background = "transparent"
	typedInput.OutputCount = 2
	accepted, err := client.CreateImageEditing(t.Context(), "ordered-image-edit", typedInput)
	if err != nil {
		t.Fatalf("accept ordered image edit through the official client: %v", err)
	}
	result := waitForImageGeneration(t, client, accepted)
	if result.State != proxy.MediaOperationStateSucceeded || len(result.Outputs) != 2 {
		t.Fatalf("edit result=%+v", result)
	}
	for index, output := range result.Outputs {
		asset, err := client.GetAsset(t.Context(), output.AssetID)
		if err != nil {
			t.Fatal(err)
		}
		data, err := client.DownloadAsset(t.Context(), asset)
		if err != nil || output.Ordinal != index || !bytes.Equal(data, images[1-index]) {
			t.Fatalf("edit output %d differs: %v", index, err)
		}
	}
	duplicate, err := client.CreateMediaOperation(t.Context(), "ordered-image-edit", input)
	if err != nil || duplicate.OperationID != accepted.OperationID || calls.Load() != 1 {
		t.Fatalf("duplicate=%+v error=%v calls=%d", duplicate, err, calls.Load())
	}
	input.Input, _ = json.Marshal(map[string]any{"prompt": "Place the first subject beside the second", "image_asset_ids": []string{assets[1].AssetID, assets[0].AssetID}, "mask_asset_id": assets[2].AssetID})
	if _, err := client.CreateMediaOperation(t.Context(), "ordered-image-edit", input); httpFailureStatus(err) != http.StatusConflict || calls.Load() != 1 {
		t.Fatalf("changed ordered intent: %v calls=%d", err, calls.Load())
	}
}
