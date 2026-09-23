package proxy

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

var interactionModalityNames = map[string]string{"text": "text", "image": "image", "audio": "audio", "video": "video", "document": "document"}
var vertexModalityNames = map[string]string{"": "text", "MODALITY_UNSPECIFIED": "text", "TEXT": "text", "IMAGE": "image", "AUDIO": "audio", "VIDEO": "video", "DOCUMENT": "document"}

type journalModalityField struct {
	prefix     string
	parent     string
	path       string
	countField string
	names      map[string]string
}

func (field journalModalityField) observe(payload map[string]any, evidence *journalUsageEvidenceInput) {
	var parent *big.Int
	for _, quantity := range evidence.Quantities {
		if quantity.Dimension == field.parent && quantity.Value != "" {
			parent, _ = new(big.Int).SetString(quantity.Value, 10)
		}
	}
	missing := journalQuantity{Dimension: field.prefix + "modality_tokens", Unit: "token", IncludedIn: field.parent, UnknownReason: journalQuantityNotReported}
	raw, present := journalMeterPath(payload, field.path)
	if !present || raw == nil {
		// A reported zero total needs no subdivisions. No missing count becomes zero.
		if parent == nil || parent.Sign() != 0 {
			evidence.Quantities = append(evidence.Quantities, missing)
		}
		return
	}
	items, valid := raw.([]any)
	if !valid {
		missing.UnknownReason = journalQuantityInvalid
		evidence.Quantities = append(evidence.Quantities, missing)
		return
	}
	quantities := []journalQuantity{}
	seen := map[string]bool{}
	sum := new(big.Int)
	complete, contradiction := true, false
	for index, rawItem := range items {
		item, valid := rawItem.(map[string]any)
		if !valid {
			missing.UnknownReason = journalQuantityInvalid
			complete = false
			continue
		}
		name, named := item["modality"].(string)
		_, suppliedName := item["modality"]
		modality, supported := field.names[name]
		if suppliedName && !named {
			supported = false
		}
		number, numeric := item[field.countField].(json.Number)
		validCount := numeric && journalDecimalPattern.MatchString(number.String()) && !strings.Contains(number.String(), ".")
		if validCount {
			value, _ := new(big.Int).SetString(number.String(), 10)
			sum.Add(sum, value)
			evidence.SourceFields = append(evidence.SourceFields, journalSourceField{Path: fmt.Sprintf("%s[%d].%s", field.path, index, field.countField), Value: number.String()})
		} else {
			complete = false
		}
		if !supported {
			missing.UnknownReason = journalQuantityUnsupported
			complete = false
			continue
		}
		if seen[modality] {
			contradiction = true
			continue
		}
		seen[modality] = true
		quantity := journalQuantity{Dimension: field.prefix + modality + "_tokens", Unit: "token", IncludedIn: field.parent, UnknownReason: journalQuantityNotReported}
		if validCount {
			quantity.Value, quantity.UnknownReason = number.String(), ""
		} else if _, supplied := item[field.countField]; supplied {
			quantity.UnknownReason = journalQuantityInvalid
		}
		quantities = append(quantities, quantity)
	}
	if parent != nil && sum.Cmp(parent) > 0 {
		contradiction = true
	}
	if contradiction {
		missing.UnknownReason = journalQuantityInvalid
		for index := range quantities {
			quantities[index].Value, quantities[index].UnknownReason = "", journalQuantityInvalid
		}
	}
	if contradiction || len(quantities) != len(items) || (complete && parent != nil && sum.Cmp(parent) < 0) || (len(items) == 0 && parent == nil) {
		quantities = append(quantities, missing)
	}
	evidence.Quantities = append(evidence.Quantities, quantities...)
}

func validateJournalModalityCache(quantities []journalQuantity) {
	values := map[string]*big.Int{}
	for _, quantity := range quantities {
		if quantity.Value != "" {
			values[quantity.Dimension], _ = new(big.Int).SetString(quantity.Value, 10)
		}
	}
	for index := range quantities {
		quantity := &quantities[index]
		if !strings.HasPrefix(quantity.Dimension, "cache_read_") || quantity.Dimension == "cache_read_tokens" || quantity.Value == "" {
			continue
		}
		input := values["input_"+strings.TrimPrefix(quantity.Dimension, "cache_read_")]
		if input != nil && values[quantity.Dimension].Cmp(input) > 0 {
			quantity.Value, quantity.UnknownReason = "", journalQuantityInvalid
		}
	}
}
