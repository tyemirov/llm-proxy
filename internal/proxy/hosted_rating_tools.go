package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Every generation, including a continuation, uses the immutable tool ceiling.
// The provider payload cannot raise the authorized bound.
func (execution *hostedTextExecution) bindAcceptedToolLimit(request *http.Request) error {
	// Admission accepts search only on the OpenAI Responses route. Execution
	// retains that route and reads its immutable bound before provider dispatch.
	var record managedPriceSnapshotRecord
	if err := execution.database.database.WithContext(request.Context()).Where("request_id = ? AND billing_account_id = ?", execution.request.ID, execution.request.BillingAccountID).First(&record).Error; err != nil {
		return fmt.Errorf("%w: read accepted tool ceiling: %w", errHostedAuthorityDenied, err)
	}
	_, document, err := restoreHostedPriceSnapshot(record)
	if err != nil {
		return fmt.Errorf("%w: restore accepted tool ceiling: %w", errHostedAuthorityDenied, err)
	}
	maximum := ""
	for _, bound := range document.Bounds {
		if bound.Dimension == "web_search_calls" && bound.Unit == "call" {
			maximum = bound.Maximum
		}
	}
	if maximum == "" || maximum == "0" || !ratingRational(maximum).IsInt() {
		return fmt.Errorf("%w: accepted search ceiling missing", errHostedAuthorityDenied)
	}
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return fmt.Errorf("read hosted tool payload: %w", err)
	}
	if err := request.Body.Close(); err != nil {
		return fmt.Errorf("close hosted tool payload: %w", err)
	}
	payload["max_tool_calls"] = json.RawMessage(maximum)
	// Payload values were decoded from JSON; the ceiling is a validated integer.
	encoded, _ := json.Marshal(payload)
	request.Body = io.NopCloser(bytes.NewReader(encoded))
	request.ContentLength = int64(len(encoded))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(encoded)), nil }
	return nil
}
