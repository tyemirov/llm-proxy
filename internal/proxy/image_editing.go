package proxy

import (
	"bytes"
	"image"
	"image/color"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"slices"
	"strconv"
)

func (adapter *imageGenerationAdapter) readEditingAsset(owner, assetID string) ([]byte, tenantAssetMetadata, error) {
	requestTenant := tenant{identifier: tenantID(owner)}
	metadata, err := adapter.assets.metadata(requestTenant, assetID)
	if err != nil || metadata.SizeBytes > int64(adapter.limits[imageLimitInputBytes]) || !slices.Contains([]string{"image/png", "image/jpeg", "image/webp"}, metadata.MIMEType) {
		return nil, tenantAssetMetadata{}, errMediaOperationInvalid
	}
	reader, err := adapter.assets.resolve(requestTenant, assetID, metadata.MIMEType)
	if err != nil {
		return nil, tenantAssetMetadata{}, errMediaOperationInvalid
	}
	data, readError := io.ReadAll(reader.file)
	closeError := reader.Close()
	if readError != nil || closeError != nil {
		return nil, tenantAssetMetadata{}, errMediaOperationInvalid
	}
	return data, metadata, nil
}

func (adapter *imageGenerationAdapter) validateEditingInputs(owner string, input imageGenerationInput) ([]string, error) {
	if len(input.ImageAssetIDs) == 0 || len(input.ImageAssetIDs) > adapter.limits[imageLimitInputs] {
		return nil, errMediaOperationInvalid
	}
	identifiers := slices.Clone(input.ImageAssetIDs)
	if input.MaskAssetID != "" {
		identifiers = append(identifiers, input.MaskAssetID)
	}
	var first image.Config
	for index, assetID := range identifiers {
		data, metadata, err := adapter.readEditingAsset(owner, assetID)
		if err != nil {
			return nil, err
		}
		configuration, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || "image/"+format != metadata.MIMEType || configuration.Width <= 0 || configuration.Height <= 0 || int64(configuration.Width)*int64(configuration.Height) > int64(adapter.limits[imageLimitInputPixels]) {
			return nil, errMediaOperationInvalid
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			return nil, errMediaOperationInvalid
		}
		if index == 0 {
			first = configuration
		}
		if index == len(input.ImageAssetIDs) && (format != "png" || configuration.Width != first.Width || configuration.Height != first.Height || !imageMaskHasAlpha(configuration.ColorModel)) {
			return nil, errMediaOperationInvalid
		}
	}
	return identifiers, nil
}

func imageMaskHasAlpha(model color.Model) bool {
	switch model {
	case color.NRGBAModel, color.NRGBA64Model:
		return true
	}
	palette, ok := model.(color.Palette)
	if !ok {
		return false
	}
	for _, entry := range palette {
		_, _, _, alpha := entry.RGBA()
		if alpha != 0xffff {
			return true
		}
	}
	return false
}

func (adapter *imageGenerationAdapter) editingBody(owner string, input imageGenerationInput, controls imageGenerationControls) (io.ReadCloser, string) {
	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	go func() {
		err := adapter.writeEditingForm(form, owner, input, controls)
		if err == nil {
			err = form.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	return reader, form.FormDataContentType()
}

func (adapter *imageGenerationAdapter) writeEditingForm(form *multipart.Writer, owner string, input imageGenerationInput, controls imageGenerationControls) error {
	fields := [][2]string{
		{"model", adapter.offering.ProviderModel}, {"prompt", input.Prompt},
		{"quality", controls.Quality}, {"size", controls.Size}, {"background", controls.Background},
		{"output_format", controls.OutputFormat}, {"n", strconv.Itoa(controls.OutputCount)},
	}
	if controls.OutputCompression != nil {
		fields = append(fields, [2]string{"output_compression", strconv.Itoa(*controls.OutputCompression)})
	}
	if controls.Stream {
		fields = append(fields, [2]string{"stream", "true"}, [2]string{"partial_images", strconv.Itoa(controls.PartialImages)})
	}
	for _, field := range fields {
		if err := form.WriteField(field[0], field[1]); err != nil {
			return err
		}
	}
	for _, assetID := range input.ImageAssetIDs {
		if err := adapter.writeEditingAsset(form, owner, assetID, "image[]"); err != nil {
			return err
		}
	}
	if input.MaskAssetID != "" {
		return adapter.writeEditingAsset(form, owner, input.MaskAssetID, "mask")
	}
	return nil
}

func (adapter *imageGenerationAdapter) writeEditingAsset(form *multipart.Writer, owner, assetID, field string) error {
	data, metadata, err := adapter.readEditingAsset(owner, assetID)
	if err != nil {
		return err
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": assetID}))
	header.Set("Content-Type", metadata.MIMEType)
	part, err := form.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
}
