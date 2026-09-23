package proxy

import (
	"encoding/json"
	"strconv"
)

// This count describes provider search actions, not submitted query strings.
// An explicit empty output list establishes zero. Missing output does not.
func (input *journalUsageEvidenceInput) recordWebSearchUsage(body []byte, codec string) {
	quantity := journalQuantity{Dimension: "web_search_calls", Unit: "call", UnknownReason: journalQuantityUnsupported}
	if codec == CatalogProtocolOpenAIResponses {
		count, reason := responseSearchCount(body)
		quantity.UnknownReason = reason
		if reason == "" {
			quantity.Value = strconv.Itoa(count)
			input.SourceFields = append(input.SourceFields, journalSourceField{Path: "derived.output.web_search.search_count", Value: quantity.Value})
		}
	}
	input.Quantities = append(input.Quantities, quantity)
}

func responseSearchCount(body []byte) (int, journalUnknownReason) {
	var response struct {
		Output json.RawMessage `json:"output"`
	}
	if json.Unmarshal(body, &response) != nil {
		return 0, journalQuantityInvalid
	}
	if len(response.Output) == 0 || string(response.Output) == "null" {
		return 0, journalQuantityNotReported
	}
	var items []struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Status string `json:"status"`
		Action struct {
			Type string `json:"type"`
		} `json:"action"`
	}
	if json.Unmarshal(response.Output, &items) != nil {
		return 0, journalQuantityInvalid
	}
	seen := make(map[string]bool)
	count := 0
	for _, item := range items {
		if item.Type == "" {
			return 0, journalQuantityInvalid
		}
		if item.Type != responseTypeWebSearchCall {
			continue
		}
		if item.ID == "" || seen[item.ID] {
			return 0, journalQuantityInvalid
		}
		seen[item.ID] = true
		if item.Status != "completed" {
			return 0, journalQuantityNotReported
		}
		switch item.Action.Type {
		case "search":
			count++
		case "open_page", "find_in_page":
		case "":
			return 0, journalQuantityInvalid
		default:
			return 0, journalQuantityUnsupported
		}
	}
	return count, ""
}
