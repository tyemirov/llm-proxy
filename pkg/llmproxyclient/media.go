package llmproxyclient

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
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
	sha256         string
}

func (attachment messageAttachment) clientMessageAttachment() messageAttachment {
	return messageAttachment{
		attachmentType: attachment.attachmentType,
		mimeType:       attachment.mimeType,
		data:           append([]byte(nil), attachment.data...),
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
		payload = append(payload, map[string]string{
			"type":      attachment.attachmentType,
			"mime_type": attachment.mimeType,
			"data":      base64.StdEncoding.EncodeToString(attachment.data),
			"sha256":    attachment.sha256,
		})
	}
	return payload
}
