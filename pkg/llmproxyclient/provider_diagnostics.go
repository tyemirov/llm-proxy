package llmproxyclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

var diagnosticProviderPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// ProviderDiagnostics counts retained operations for the authenticated tenant and provider.
type ProviderDiagnostics struct {
	Provider        string `json:"provider"`
	Scope           string `json:"scope"`
	OperationsTotal int64  `json:"operations_total"`
	Queued          int64  `json:"queued"`
	Running         int64  `json:"running"`
	Succeeded       int64  `json:"succeeded"`
	Failed          int64  `json:"failed"`
	Cancelled       int64  `json:"cancelled"`
	Uncertain       int64  `json:"uncertain"`
}

// GetProviderDiagnostics reads retained tenant operation counts without contacting the provider.
func (client Client) GetProviderDiagnostics(ctx context.Context, provider string) (ProviderDiagnostics, error) {
	if !diagnosticProviderPattern.MatchString(provider) {
		return ProviderDiagnostics{}, fmt.Errorf("%w: invalid diagnostic provider", ErrInvalidClientRequest)
	}
	requestURL := client.config.mediaResourceURL(llmproxycontract.ProviderDiagnosticsPath + "/" + provider)
	request := (&http.Request{Method: http.MethodGet, URL: &requestURL, Header: http.Header{}}).WithContext(ctx)
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	body, err := client.doMediaResource(request, http.StatusOK)
	if err != nil {
		return ProviderDiagnostics{}, err
	}
	failure := fmt.Errorf("%w: invalid provider diagnostics response", ErrClientHTTPFailure)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || len(fields) != 9 {
		return ProviderDiagnostics{}, failure
	}
	for _, value := range fields {
		if string(value) == "null" {
			return ProviderDiagnostics{}, failure
		}
	}
	var result ProviderDiagnostics
	if err := decodeExactJSON(body, &result); err != nil || result.Provider != provider || result.Scope != llmproxycontract.ProviderDiagnosticsScopeTenant || result.OperationsTotal < 0 || result.Queued < 0 || result.Running < 0 || result.Succeeded < 0 || result.Failed < 0 || result.Cancelled < 0 || result.Uncertain < 0 {
		return ProviderDiagnostics{}, failure
	}
	remaining := result.OperationsTotal
	for _, count := range []int64{result.Queued, result.Running, result.Succeeded, result.Failed, result.Cancelled, result.Uncertain} {
		if count > remaining {
			return ProviderDiagnostics{}, failure
		}
		remaining -= count
	}
	if remaining != 0 {
		return ProviderDiagnostics{}, failure
	}

	return result, nil
}
