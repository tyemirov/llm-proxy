package proxy

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"io"
	"strings"
)

type imageResponsesStreamEvent struct {
	Type         string                 `json:"type"`
	Sequence     *int                   `json:"sequence_number"`
	Response     imageResponsesSnapshot `json:"response"`
	ItemID       string                 `json:"item_id"`
	OutputIndex  *int                   `json:"output_index"`
	PartialIndex *int                   `json:"partial_image_index"`
	PartialImage string                 `json:"partial_image_b64"`
	OutputFormat string                 `json:"output_format"`
	Item         struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"item"`
}

func (adapter *imageGenerationAdapter) decodeResponsesStream(body io.Reader, controls imageGenerationControls, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	maximumEventBytes := (adapter.maximumOutputBytes+2)/3*4 + 65536
	reader := &io.LimitedReader{R: body, N: maximumEventBytes * int64(controls.PartialImages+8)}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), int(maximumEventBytes))
	var data strings.Builder
	var eventName, handle, imageItemID string
	var lastDigest [sha256.Size]byte
	var partialDigests [][sha256.Size]byte
	lastSequence, imageItemIndex := -1, -1
	invalid := imageResponsesReadFailure(errMediaOperationInvalid)
	bindImageItem := func(id string, index *int) bool {
		if !imageResponseHandlePattern.MatchString(id) || index == nil || *index < 0 {
			return false
		}
		if imageItemID == "" {
			imageItemID, imageItemIndex = id, *index
		}
		return imageItemID == id && imageItemIndex == *index
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				eventName = value
			case "data":
				if int64(data.Len()+len(value)+1) > maximumEventBytes {
					return invalid
				}
				data.WriteString(value)
				data.WriteByte('\n')
			}
			continue
		}
		if data.Len() == 0 {
			eventName = ""
			continue
		}
		encoded := []byte(data.String())
		var event imageResponsesStreamEvent
		if json.Unmarshal(encoded, &event) != nil || event.Type == "" || (eventName != "" && eventName != event.Type) {
			return invalid
		}
		data.Reset()
		eventName = ""
		if event.Type == "error" {
			return MediaOperationExecutionResult{State: MediaOperationStateFailed, ProviderHandle: handle, ErrorCode: "provider_error"}
		}
		if event.Sequence == nil || *event.Sequence < 0 || *event.Sequence < lastSequence {
			return invalid
		}
		digest := sha256.Sum256(encoded)
		if *event.Sequence == lastSequence {
			if digest != lastDigest {
				return invalid
			}
			continue
		}
		lastSequence, lastDigest = *event.Sequence, digest
		if event.Type == "response.created" {
			if handle != "" || !imageResponseHandlePattern.MatchString(event.Response.ID) || !imageResponsePending(event.Response) {
				return invalid
			}
			if err := request.PersistProviderReceipt(MediaOperationProviderReceipt{Handle: event.Response.ID, RequestID: event.Response.ID}); err != nil {
				return imageGenerationUncertain()
			}
			handle = event.Response.ID
			continue
		}
		if handle == "" {
			return invalid
		}
		switch event.Type {
		case "response.queued", "response.in_progress":
			if event.Response.ID != handle || !imageResponsePending(event.Response) {
				return invalid
			}
		case "response.output_item.added", "response.output_item.done":
			if event.Item.Type == "image_generation_call" && !bindImageItem(event.Item.ID, event.OutputIndex) {
				return invalid
			}
		case "response.image_generation_call.in_progress", "response.image_generation_call.generating", "response.image_generation_call.completed":
			if !bindImageItem(event.ItemID, event.OutputIndex) {
				return invalid
			}
		case "response.image_generation_call.partial_image":
			if !bindImageItem(event.ItemID, event.OutputIndex) || event.PartialIndex == nil || *event.PartialIndex < 0 || *event.PartialIndex >= controls.PartialImages || *event.PartialIndex > len(partialDigests) || (event.OutputFormat != "" && event.OutputFormat != controls.OutputFormat) {
				return invalid
			}
			output, err := adapter.decodeImage(event.PartialImage, controls)
			if err != nil {
				return invalid
			}
			index := *event.PartialIndex
			imageDigest := sha256.Sum256(output.Data)
			if index < len(partialDigests) && partialDigests[index] != imageDigest {
				return invalid
			}
			if err := request.PublishPartial(MediaOperationPartialOutput{MediaOperationOutput: output, OutputOrdinal: 0, PartialOrdinal: index}); err != nil {
				return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ProviderHandle: handle, ErrorCode: "asset_publication_failed"}
			}
			if index == len(partialDigests) {
				partialDigests = append(partialDigests, imageDigest)
			}
		case "response.completed", "response.failed", "response.incomplete":
			if event.Response.ID != handle || event.Type != "response."+event.Response.Status {
				return invalid
			}
			if event.Response.Status == "completed" && imageItemID != "" {
				if imageItemIndex >= len(event.Response.Output) || event.Response.Output[imageItemIndex].ID != imageItemID || event.Response.Output[imageItemIndex].Type != "image_generation_call" {
					return invalid
				}
			}
			return adapter.imageResponseResult(event.Response, controls, request)
		case "response.content_part.added", "response.content_part.done", "response.output_text.delta", "response.output_text.done", "response.output_text.annotation.added", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done", "response.reasoning_text.delta", "response.reasoning_text.done":
			// Text and reasoning items do not create image output positions.
		default:
			return invalid
		}
	}
	if reader.N == 0 || scanner.Err() == bufio.ErrTooLong {
		return invalid
	}
	result := imageGenerationUncertain()
	result.ProviderHandle = handle
	return result
}
