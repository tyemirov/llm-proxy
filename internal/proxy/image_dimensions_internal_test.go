package proxy

import (
	"errors"
	"os"
	"testing"
)

func TestZAIImageClosedAsset(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "image")
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	width := int64(6000)
	model := textModelDefinition{mediaLimits: []CatalogMediaLimit{{ID: CatalogMediaLimitIDImageWidthPixels, MediaType: string(messageMediaTypeImage), Status: CatalogMediaLimitStatusBounded, Value: &width}}}
	attachment := messageMedia{mediaType: messageMediaTypeImage, mimeType: messageImageMIMEPNG, asset: &tenantAssetReader{file: file}}
	if err = validateImageDimensions(model, attachment); !errors.Is(err, errAssetStore) {
		t.Fatalf("closed asset error=%v", err)
	}
}
