package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

const (
	messageMediaTypeAudio messageMediaType = "audio"
	messageMediaTypeImage messageMediaType = "image"
)

const (
	messageAudioMIMEM4A  = "audio/m4a"
	messageAudioMIMEMPEG = "audio/mpeg"
	messageAudioMIMEWAV  = "audio/wav"
	messageImageMIMEJPEG = "image/jpeg"
	messageImageMIMEPNG  = "image/png"
	messageImageMIMEWebP = "image/webp"
)

type messageMediaType string

type chatMessageAttachmentPayload struct {
	Type     string  `json:"type"`
	MIMEType string  `json:"mime_type"`
	Data     *string `json:"data,omitempty"`
	AssetID  *string `json:"asset_id,omitempty"`
	SHA256   string  `json:"sha256"`
}

type messageMedia struct {
	mediaType messageMediaType
	mimeType  string
	sha256    string
	sizeBytes int64
	data      []byte
	asset     *tenantAssetReader
}

var providerMessageMediaMIMEs = map[string]map[messageMediaType]map[string]struct{}{
	ProviderNameOpenAI: {
		messageMediaTypeImage: {
			messageImageMIMEJPEG: {},
			messageImageMIMEPNG:  {},
			messageImageMIMEWebP: {},
		},
	},
	ProviderNameAnthropic: {
		messageMediaTypeImage: {
			messageImageMIMEJPEG: {},
			messageImageMIMEPNG:  {},
			messageImageMIMEWebP: {},
		},
	},
	ProviderNameGemini: {
		messageMediaTypeAudio: {
			messageAudioMIMEM4A:  {},
			messageAudioMIMEMPEG: {},
			messageAudioMIMEWAV:  {},
		},
		messageMediaTypeImage: {
			messageImageMIMEJPEG: {},
			messageImageMIMEPNG:  {},
			messageImageMIMEWebP: {},
		},
	},
	ProviderNameXAI: {
		messageMediaTypeImage: {
			messageImageMIMEJPEG: {},
			messageImageMIMEPNG:  {},
		},
	},
}

func newMessageMedia(payload chatMessageAttachmentPayload, requestTenant tenant, assetStore *tenantAssetStore) (messageMedia, error) {
	mediaType := messageMediaType(payload.Type)
	if !supportedMessageMediaTypeMIME(mediaType, payload.MIMEType) {
		if mediaType != messageMediaTypeImage && mediaType != messageMediaTypeAudio {
			return messageMedia{}, fmt.Errorf("%w: unsupported attachment type=%q", ErrInvalidChatMessages, payload.Type)
		}
		return messageMedia{}, fmt.Errorf("%w: unsupported %s MIME type=%q", ErrInvalidChatMessages, mediaType, payload.MIMEType)
	}
	if (payload.Data == nil) == (payload.AssetID == nil) {
		return messageMedia{}, fmt.Errorf("%w: attachment requires exactly one of data or asset_id", ErrInvalidChatMessages)
	}
	if payload.Data != nil {
		data, dataError := decodeHashBoundMessageMedia(*payload.Data, payload.SHA256)
		if dataError != nil {
			return messageMedia{}, dataError
		}
		return messageMedia{mediaType: mediaType, mimeType: payload.MIMEType, sha256: payload.SHA256, sizeBytes: int64(len(data)), data: data}, nil
	}
	assetID := strings.TrimSpace(*payload.AssetID)
	if assetID != *payload.AssetID {
		return messageMedia{}, fmt.Errorf("%w: asset_id is not canonical", ErrInvalidChatMessages)
	}
	asset, assetError := assetStore.resolve(requestTenant, assetID, payload.MIMEType, payload.SHA256)
	if assetError != nil {
		return messageMedia{}, assetError
	}
	return messageMedia{mediaType: mediaType, mimeType: payload.MIMEType, sha256: payload.SHA256, sizeBytes: asset.metadata.SizeBytes, asset: asset}, nil
}

func supportedMessageMediaTypeMIME(mediaType messageMediaType, mimeType string) bool {
	switch mediaType {
	case messageMediaTypeImage:
		return mimeType == messageImageMIMEJPEG || mimeType == messageImageMIMEPNG || mimeType == messageImageMIMEWebP
	case messageMediaTypeAudio:
		return mimeType == messageAudioMIMEM4A || mimeType == messageAudioMIMEMPEG || mimeType == messageAudioMIMEWAV
	default:
		return false
	}
}

func supportedMessageMediaMIME(mimeType string) bool {
	return supportedMessageMediaTypeMIME(messageMediaTypeImage, mimeType) || supportedMessageMediaTypeMIME(messageMediaTypeAudio, mimeType)
}

func (media *messageMedia) reader() (io.Reader, error) {
	if media.asset == nil {
		return bytes.NewReader(media.data), nil
	}
	if _, seekError := media.asset.file.Seek(0, io.SeekStart); seekError != nil {
		return nil, fmt.Errorf("%w: open attachment", errAssetStore)
	}
	return media.asset.file, nil
}

func (media *messageMedia) close() error {
	if media.asset == nil {
		return nil
	}
	return media.asset.Close()
}

func (messages chatMessages) closeMedia() {
	for messageIndex := range messages {
		for attachmentIndex := range messages[messageIndex].attachments {
			_ = messages[messageIndex].attachments[attachmentIndex].close()
		}
	}
}

func (media *messageMedia) bytes() ([]byte, error) {
	if media.asset == nil {
		return media.data, nil
	}
	reader, readerError := media.reader()
	if readerError != nil {
		return nil, readerError
	}
	data, readError := io.ReadAll(io.LimitReader(reader, media.sizeBytes+1))
	if readError != nil || int64(len(data)) != media.sizeBytes {
		return nil, fmt.Errorf("%w: read attachment", errAssetStore)
	}
	return data, nil
}

func decodeHashBoundMessageMedia(rawData string, rawDigest string) ([]byte, error) {
	if rawData == constants.EmptyString {
		return nil, fmt.Errorf("%w: attachment data is empty", ErrInvalidChatMessages)
	}
	decodedData, decodeError := base64.StdEncoding.DecodeString(rawData)
	if decodeError != nil || len(decodedData) == 0 || base64.StdEncoding.EncodeToString(decodedData) != rawData {
		return nil, fmt.Errorf("%w: attachment data is not canonical base64", ErrInvalidChatMessages)
	}
	digestBytes, digestError := hex.DecodeString(rawDigest)
	if digestError != nil || len(digestBytes) != sha256.Size || strings.ToLower(rawDigest) != rawDigest {
		return nil, fmt.Errorf("%w: attachment sha256 is not canonical lowercase hex", ErrInvalidChatMessages)
	}
	actualDigest := sha256.Sum256(decodedData)
	if !bytes.Equal(actualDigest[:], digestBytes) {
		return nil, fmt.Errorf("%w: attachment sha256 does not match data", ErrInvalidChatMessages)
	}
	return decodedData, nil
}

func providerSupportsMessageMedia(providerName string, mediaInput messageMediaType) bool {
	_, supported := providerMessageMediaMIMEs[providerName][mediaInput]
	return supported
}

func providerSupportsMessageMediaMIME(providerName string, mediaInput messageMediaType, mimeType string) bool {
	_, supported := providerMessageMediaMIMEs[providerName][mediaInput][mimeType]
	return supported
}

func validateMessageMediaForResolvedTextRoute(provider providerDefinition, model textModelDefinition, messages chatMessages) error {
	for _, message := range messages {
		for _, attachment := range message.attachments {
			if !model.supportsMediaInput(attachment.mediaType) || !providerSupportsMessageMediaMIME(provider.identifier.string(), attachment.mediaType, attachment.mimeType) {
				return fmt.Errorf(
					"%w: provider=%s model=%s capability=media_input type=%s mime_type=%s",
					ErrUnsupportedCapability,
					provider.identifier.string(),
					model.string(),
					attachment.mediaType,
					attachment.mimeType,
				)
			}
		}
	}
	return nil
}

func validateInlineMessageMediaLimits(model textModelDefinition, messages chatMessages, payloadBytes []byte) error {
	if messages.mediaCount() == 0 {
		return nil
	}
	if inlineRequestBytes, bounded := boundedCatalogMediaLimit(model.mediaLimits, CatalogMediaLimitIDInlineRequestBytes, messageMediaTypeImage); bounded && int64(len(payloadBytes)) > inlineRequestBytes {
		return ErrProviderMediaLimit
	}
	for _, mediaType := range []messageMediaType{messageMediaTypeImage, messageMediaTypeAudio} {
		countLimitID := CatalogMediaLimitIDImageCount
		inlineLimitID := CatalogMediaLimitIDImageInlineBytes
		if mediaType == messageMediaTypeAudio {
			countLimitID = CatalogMediaLimitIDAudioCount
			inlineLimitID = CatalogMediaLimitIDAudioInlineBytes
		}
		if countLimit, bounded := boundedCatalogMediaLimit(model.mediaLimits, countLimitID, mediaType); bounded && messages.mediaTypeCount(mediaType) > countLimit {
			return ErrProviderMediaLimit
		}
		inlineLimit, bounded := boundedCatalogMediaLimit(model.mediaLimits, inlineLimitID, mediaType)
		if !bounded {
			continue
		}
		for _, message := range messages {
			for _, attachment := range message.attachments {
				attachmentBytes := attachment.sizeBytes
				if configuredLimit, found := catalogMediaLimit(model.mediaLimits, inlineLimitID, mediaType); found && configuredLimit.Scope == CatalogMediaLimitScopeAttachmentEncodedBytes {
					attachmentBytes = ((attachment.sizeBytes + 2) / 3) * 4
				}
				if attachment.mediaType == mediaType && attachmentBytes > inlineLimit {
					return ErrProviderMediaLimit
				}
			}
		}
	}
	return nil
}
