package llmproxyclient

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const (
	messageAttachmentTypeAudio = "audio"
	messageAttachmentTypeImage = "image"
)

const (
	audioMIMEM4A  = "audio/m4a"
	audioMIMEMPEG = "audio/mpeg"
	audioMIMEWAV  = "audio/wav"
	imageMIMEJPEG = "image/jpeg"
	imageMIMEPNG  = "image/png"
	imageMIMEWebP = "image/webp"
)

// MessageAttachment is one validated provider-neutral media attachment.
type MessageAttachment interface {
	clientMessageAttachment() messageAttachment
}

type messageAttachment struct {
	attachmentType string
	mimeType       string
	data           []byte
	assetID        string
	sha256         string
}

var assetIdentifierPattern = regexp.MustCompile(`^ast_[0-9a-f]{32}$`)

func (attachment messageAttachment) clientMessageAttachment() messageAttachment {
	return messageAttachment{
		attachmentType: attachment.attachmentType,
		mimeType:       attachment.mimeType,
		data:           append([]byte(nil), attachment.data...),
		assetID:        attachment.assetID,
		sha256:         attachment.sha256,
	}
}

// ImageAttachmentInput is unvalidated image input supplied to NewImageAttachment.
type ImageAttachmentInput struct {
	MIMEType string
	Data     []byte
}

// AudioAttachmentInput is unvalidated audio input supplied to NewAudioAttachment.
type AudioAttachmentInput struct {
	MIMEType string
	Data     []byte
}

// ImageAssetAttachmentInput is an existing tenant image asset supplied to NewImageAssetAttachment.
type ImageAssetAttachmentInput struct {
	AssetID  string
	MIMEType string
	SHA256   string
}

// AudioAssetAttachmentInput is an existing tenant audio asset supplied to NewAudioAssetAttachment.
type AudioAssetAttachmentInput struct {
	AssetID  string
	MIMEType string
	SHA256   string
}

// NewImageAttachment validates and hash-binds exact image bytes.
func NewImageAttachment(input ImageAttachmentInput) (MessageAttachment, error) {
	mimeType := strings.ToLower(strings.TrimSpace(input.MIMEType))
	switch mimeType {
	case imageMIMEJPEG, imageMIMEPNG, imageMIMEWebP:
	default:
		return nil, fmt.Errorf("%w: unsupported image MIME type=%q", ErrInvalidClientRequest, input.MIMEType)
	}
	return newMessageAttachment(messageAttachmentTypeImage, mimeType, input.Data)
}

// NewAudioAttachment validates and hash-binds exact audio bytes.
func NewAudioAttachment(input AudioAttachmentInput) (MessageAttachment, error) {
	mimeType := strings.ToLower(strings.TrimSpace(input.MIMEType))
	switch mimeType {
	case audioMIMEM4A, audioMIMEMPEG, audioMIMEWAV:
	default:
		return nil, fmt.Errorf("%w: unsupported audio MIME type=%q", ErrInvalidClientRequest, input.MIMEType)
	}
	return newMessageAttachment(messageAttachmentTypeAudio, mimeType, input.Data)
}

// NewImageAssetAttachment validates a hash-bound tenant image asset reference.
func NewImageAssetAttachment(input ImageAssetAttachmentInput) (MessageAttachment, error) {
	mimeType := strings.ToLower(strings.TrimSpace(input.MIMEType))
	switch mimeType {
	case imageMIMEJPEG, imageMIMEPNG, imageMIMEWebP:
	default:
		return nil, fmt.Errorf("%w: unsupported image MIME type=%q", ErrInvalidClientRequest, input.MIMEType)
	}
	return newAssetMessageAttachment(messageAttachmentTypeImage, mimeType, input.AssetID, input.SHA256)
}

// NewAudioAssetAttachment validates a hash-bound tenant audio asset reference.
func NewAudioAssetAttachment(input AudioAssetAttachmentInput) (MessageAttachment, error) {
	mimeType := strings.ToLower(strings.TrimSpace(input.MIMEType))
	switch mimeType {
	case audioMIMEM4A, audioMIMEMPEG, audioMIMEWAV:
	default:
		return nil, fmt.Errorf("%w: unsupported audio MIME type=%q", ErrInvalidClientRequest, input.MIMEType)
	}
	return newAssetMessageAttachment(messageAttachmentTypeAudio, mimeType, input.AssetID, input.SHA256)
}

func newAssetMessageAttachment(attachmentType string, mimeType string, assetID string, digest string) (MessageAttachment, error) {
	if !assetIdentifierPattern.MatchString(assetID) {
		return nil, fmt.Errorf("%w: invalid asset_id", ErrInvalidClientRequest)
	}
	decodedDigest, digestError := hex.DecodeString(digest)
	if digestError != nil || len(decodedDigest) != sha256.Size || strings.ToLower(digest) != digest {
		return nil, fmt.Errorf("%w: invalid asset sha256", ErrInvalidClientRequest)
	}
	return messageAttachment{attachmentType: attachmentType, mimeType: mimeType, assetID: assetID, sha256: digest}, nil
}

func newMessageAttachment(attachmentType string, mimeType string, data []byte) (MessageAttachment, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: %s attachment data is empty", ErrInvalidClientRequest, attachmentType)
	}
	copiedData := append([]byte(nil), data...)
	digest := sha256.Sum256(copiedData)
	return messageAttachment{
		attachmentType: attachmentType,
		mimeType:       mimeType,
		data:           copiedData,
		sha256:         hex.EncodeToString(digest[:]),
	}, nil
}

func messageAttachmentPayload(attachments []messageAttachment) []map[string]string {
	payload := make([]map[string]string, 0, len(attachments))
	for _, attachment := range attachments {
		payloadAttachment := map[string]string{
			"type":      attachment.attachmentType,
			"mime_type": attachment.mimeType,
			"sha256":    attachment.sha256,
		}
		if attachment.assetID == "" {
			payloadAttachment["data"] = base64.StdEncoding.EncodeToString(attachment.data)
		} else {
			payloadAttachment["asset_id"] = attachment.assetID
		}
		payload = append(payload, payloadAttachment)
	}
	return payload
}
