package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
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
	Type     string `json:"type"`
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"`
	SHA256   string `json:"sha256"`
}

type messageMedia struct {
	mediaType messageMediaType
	mimeType  string
	data      []byte
}

var providerMessageMediaInputs = map[string]map[messageMediaType]struct{}{
	ProviderNameGemini: {
		messageMediaTypeAudio: {},
		messageMediaTypeImage: {},
	},
}

func newMessageMedia(payload chatMessageAttachmentPayload) (messageMedia, error) {
	mediaType := messageMediaType(payload.Type)
	switch mediaType {
	case messageMediaTypeImage:
		switch payload.MIMEType {
		case messageImageMIMEJPEG, messageImageMIMEPNG, messageImageMIMEWebP:
		default:
			return messageMedia{}, fmt.Errorf("%w: unsupported image MIME type=%q", ErrInvalidChatMessages, payload.MIMEType)
		}
	case messageMediaTypeAudio:
		switch payload.MIMEType {
		case messageAudioMIMEM4A, messageAudioMIMEMPEG, messageAudioMIMEWAV:
		default:
			return messageMedia{}, fmt.Errorf("%w: unsupported audio MIME type=%q", ErrInvalidChatMessages, payload.MIMEType)
		}
	default:
		return messageMedia{}, fmt.Errorf("%w: unsupported attachment type=%q", ErrInvalidChatMessages, payload.Type)
	}
	data, dataError := decodeHashBoundMessageMedia(payload.Data, payload.SHA256)
	if dataError != nil {
		return messageMedia{}, dataError
	}
	return messageMedia{mediaType: mediaType, mimeType: payload.MIMEType, data: data}, nil
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
	_, supported := providerMessageMediaInputs[providerName][mediaInput]
	return supported
}

func validateMessageMediaForResolvedTextRoute(provider providerDefinition, model textModelDefinition, messages chatMessages) error {
	for _, message := range messages {
		for _, attachment := range message.attachments {
			if !model.supportsMediaInput(attachment.mediaType) {
				return fmt.Errorf(
					"%w: provider=%s model=%s capability=media_input type=%s",
					ErrUnsupportedCapability,
					provider.identifier.string(),
					model.string(),
					attachment.mediaType,
				)
			}
		}
	}
	return nil
}
