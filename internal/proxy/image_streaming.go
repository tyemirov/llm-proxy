package proxy

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"io"
	"strings"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

type imageStreamEvent struct {
	Type         string `json:"type"`
	Base64       string `json:"b64_json"`
	PartialIndex *int   `json:"partial_image_index"`
	OutputFormat string `json:"output_format"`
}

func (adapter *imageGenerationAdapter) decodeStream(body io.Reader, controls imageGenerationControls, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	maximumEventBytes := (adapter.maximumOutputBytes+2)/3*4 + 65536
	// Bound complete transport input, including duplicate events and comments.
	reader := &io.LimitedReader{R: body, N: maximumEventBytes * int64(controls.PartialImages+4)}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), int(maximumEventBytes))
	var data strings.Builder
	var eventName string
	var partialDigests [][sha256.Size]byte
	prefix := "image_generation"
	if request.Capability == llmproxycontract.MediaCapabilityImageEdit {
		prefix = "image_edit"
	}
	invalid := MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_result_invalid"}
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
		var event imageStreamEvent
		if json.Unmarshal([]byte(data.String()), &event) != nil || (eventName != "" && eventName != event.Type) {
			return invalid
		}
		data.Reset()
		eventName = ""
		if event.Type == "error" {
			return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
		}
		if event.OutputFormat != controls.OutputFormat {
			return invalid
		}
		output, err := adapter.decodeImage(event.Base64, controls)
		if err != nil {
			return invalid
		}
		switch event.Type {
		case prefix + ".partial_image":
			if event.PartialIndex == nil || *event.PartialIndex < 0 || *event.PartialIndex >= controls.PartialImages || *event.PartialIndex > len(partialDigests) {
				return invalid
			}
			index := *event.PartialIndex
			digest := sha256.Sum256(output.Data)
			if index < len(partialDigests) && partialDigests[index] != digest {
				return invalid
			}
			if err := request.PublishPartial(MediaOperationPartialOutput{MediaOperationOutput: output, OutputOrdinal: 0, PartialOrdinal: index}); err != nil {
				return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: "asset_publication_failed"}
			}
			if index == len(partialDigests) {
				partialDigests = append(partialDigests, digest)
			}
		case prefix + ".completed":
			return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{output}}
		default:
			return invalid
		}
	}
	if reader.N == 0 || scanner.Err() == bufio.ErrTooLong {
		return invalid
	}
	return imageGenerationUncertain()
}
