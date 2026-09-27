package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

const (
	alignmentMaximumInputSeconds = 10 * 60 * 60
	alignmentDurationDimension   = "input_audio_seconds"
	alignmentUsagePath           = "/v1/workspace/analytics/query/usage-by-product-over-time"
)

var (
	errAlignmentUsageInvalid    = errors.New("invalid_alignment_usage")
	alignmentUsageNumberPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]{1,3})?$`)
)

type alignmentUsageQuery struct {
	StartTime       int64                  `json:"start_time"`
	EndTime         int64                  `json:"end_time"`
	IntervalSeconds int                    `json:"interval_seconds"`
	Filters         []alignmentUsageFilter `json:"filters"`
}

type alignmentUsageFilter struct {
	Column    string   `json:"column"`
	Operation string   `json:"operation"`
	Values    []string `json:"values"`
}

func (adapter *providerAlignmentAdapter) alignmentResult(ctx context.Context, request MediaOperationExecutionRequest, receipt providerAlignmentReceipt) MediaOperationExecutionResult {
	if request.recordUsage != nil {
		if receipt.TraceID == "" {
			if err := request.recordUsage(journalUsageEvidenceInput{
				AdapterRevision: alignmentAdapterRevision, Outcome: journalOutcomeContinue,
				Quantities: []journalQuantity{{Dimension: alignmentDurationDimension, Unit: "second", UnknownReason: journalQuantityNotReported}},
			}); err != nil {
				return imageGenerationUncertain()
			}
			return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{{MIMEType: "application/json", Data: receipt.Output}}}
		}
		provider, err := resolveMediaProvider(ctx, request, adapter.provider.identifier.string(), adapter.route.Transport, adapter.provider, adapter.tenants, adapter.store)
		if err != nil {
			return imageGenerationUncertain()
		}
		query := alignmentUsageQuery{
			StartTime: receipt.StartedAt.Add(-time.Minute).UnixMilli(), EndTime: receipt.CompletedAt.Add(time.Minute).UnixMilli(), IntervalSeconds: 60,
			Filters: []alignmentUsageFilter{{Column: "trace_id", Operation: "eq", Values: []string{receipt.TraceID}}},
		}
		body, _ := json.Marshal(query)
		target, _ := url.Parse(provider.textEndpointURL)
		target.Path, target.RawPath, target.RawQuery, target.Fragment = alignmentUsagePath, "", "", ""
		lifecycle := pollableResourceLifecycle[*journalUsageEvidenceInput]{
			observe: func(ctx context.Context) (*journalUsageEvidenceInput, error) {
				native, _ := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
				native.Header.Set("Content-Type", "application/json")
				authorizeQueueRequest(native, provider)
				response, err := request.HTTP.Status.Do(native)
				if err != nil {
					return nil, fmt.Errorf("read alignment usage: %w", err)
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return nil, fmt.Errorf("read alignment usage: HTTP %d: %w", response.StatusCode, errMediaOperationUnavailable)
				}
				data, err := io.ReadAll(io.LimitReader(response.Body, providerMetadataMaximumBytes+1))
				if err != nil {
					return nil, fmt.Errorf("read alignment usage body: %w", err)
				}
				if len(data) > providerMetadataMaximumBytes {
					return nil, errAlignmentUsageInvalid
				}
				return decodeAlignmentUsage(data, receipt.TraceID)
			},
			isPending:         func(input *journalUsageEvidenceInput) bool { return input == nil },
			recordObservation: func(*journalUsageEvidenceInput, error, pollableResourceRetryDecision) {},
		}
		usage, err := lifecycle.observeUntilTerminal(ctx)
		if err != nil || request.recordUsage(*usage) != nil {
			return imageGenerationUncertain()
		}
	}
	return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{{MIMEType: "application/json", Data: receipt.Output}}}
}

// A trace filter selects one request. Empty buckets are not measurements.
// Decode native decimal tokens before converting minutes to exact seconds.
func decodeAlignmentUsage(data []byte, traceID string) (*journalUsageEvidenceInput, error) {
	var table struct {
		Columns     []string            `json:"columns"`
		ColumnTypes []string            `json:"column_types"`
		ColumnUnits []*string           `json:"column_units"`
		Rows        [][]json.RawMessage `json:"rows"`
	}
	if json.Unmarshal(data, &table) != nil || table.Rows == nil || len(table.Columns) != len(table.ColumnTypes) || len(table.Columns) != len(table.ColumnUnits) {
		return nil, errAlignmentUsageInvalid
	}
	columns := make(map[string]int, len(table.Columns))
	for index, column := range table.Columns {
		if _, exists := columns[column]; exists {
			return nil, errAlignmentUsageInvalid
		}
		columns[column] = index
	}
	for _, field := range []struct{ name, kind, unit string }{{"total_minutes", "Float", "min"}, {"total_cost", "Float", "usd"}, {"usage_count", "Int", ""}} {
		index, present := columns[field.name]
		if !present || table.ColumnTypes[index] != field.kind {
			return nil, errAlignmentUsageInvalid
		}
		unit := table.ColumnUnits[index]
		if field.unit == "" && unit != nil || field.unit != "" && (unit == nil || *unit != field.unit) {
			return nil, errAlignmentUsageInvalid
		}
	}
	var measured *journalUsageEvidenceInput
	for _, row := range table.Rows {
		if len(row) != len(table.Columns) {
			return nil, errAlignmentUsageInvalid
		}
		minutes, validMinutes := alignmentUsageDecimal(row[columns["total_minutes"]])
		cost, validCost := alignmentUsageDecimal(row[columns["total_cost"]])
		count := string(row[columns["usage_count"]])
		if !validMinutes || !validCost || count != "0" && count != "1" {
			return nil, errAlignmentUsageInvalid
		}
		if count == "0" {
			if minutes.Sign() != 0 || cost.Sign() != 0 {
				return nil, errAlignmentUsageInvalid
			}
			continue
		}
		if measured != nil {
			return nil, errAlignmentUsageInvalid
		}
		seconds := new(big.Rat).Mul(minutes, big.NewRat(60, 1))
		if seconds.Cmp(big.NewRat(alignmentMaximumInputSeconds, 1)) > 0 {
			return nil, errAlignmentUsageInvalid
		}
		measured = &journalUsageEvidenceInput{
			AdapterRevision: alignmentAdapterRevision, ProviderRequestID: traceID, Outcome: journalOutcomeContinue,
			Quantities: []journalQuantity{{Dimension: alignmentDurationDimension, Unit: "second", Value: ratingDecimal(seconds)}},
			SourceFields: []journalSourceField{
				{Path: "usage.total_minutes", Value: ratingDecimal(minutes)},
				{Path: "usage.total_cost", Value: ratingDecimal(cost)},
				{Path: "usage.usage_count", Value: count},
			},
		}
	}
	return measured, nil
}

func alignmentUsageDecimal(raw json.RawMessage) (*big.Rat, bool) {
	if len(raw) > 1024 || !alignmentUsageNumberPattern.Match(raw) {
		return nil, false
	}
	value, valid := new(big.Rat).SetString(string(raw))
	return value, valid
}
