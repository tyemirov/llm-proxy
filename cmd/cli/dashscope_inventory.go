package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	dashScopeInventoryCommand      = "inventory-dashscope"
	dashScopeInventoryAPIKey       = "DASHSCOPE_API_KEY"
	dashScopeInventoryBaseURL      = "DASHSCOPE_BASE_URL"
	dashScopeInventoryPath         = "/api/v1/models"
	dashScopeInventoryPageSize     = 20
	dashScopeInventoryMaxModels    = 20000
	dashScopeInventoryMaxPageBytes = 2 << 20
	dashScopeInventoryMaxBytes     = 32 << 20
)

var dashScopeInventoryHost = regexp.MustCompile(`^([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)\.(ap-southeast-1|us-east-1)\.maas\.aliyuncs\.com$`)
var dashScopeInventoryNow = time.Now

type dashScopeInventoryConnection struct {
	endpoint url.URL
	region   string
	apiKey   string
}

type dashScopeInventoryExport struct {
	RetrievedAt           string                         `json:"retrieved_at"`
	SourceHost            string                         `json:"source_host"`
	AccessRegion          string                         `json:"access_region"`
	WorkspaceServiceScope string                         `json:"workspace_service_scope"`
	Collections           []dashScopeInventoryCollection `json:"collections"`
}

type dashScopeInventoryCollection struct {
	Supports string                         `json:"supports"`
	Query    map[string]string              `json:"query"`
	Total    int                            `json:"total"`
	Pages    []dashScopeInventoryPageRecord `json:"pages"`
	Models   []json.RawMessage              `json:"models"`
}

type dashScopeInventoryPageRecord struct {
	PageNo     int    `json:"page_no"`
	RequestID  string `json:"request_id"`
	ModelCount int    `json:"model_count"`
}

type dashScopeInventoryEnvelope struct {
	Success   *bool           `json:"success"`
	Code      json.RawMessage `json:"code"`
	RequestID string          `json:"request_id"`
	Output    *struct {
		Total    *int               `json:"total"`
		PageNo   *int               `json:"page_no"`
		PageSize *int               `json:"page_size"`
		Models   *[]json.RawMessage `json:"models"`
	} `json:"output"`
}

func newDashScopeInventoryConnection(apiKey, baseURL string) (dashScopeInventoryConnection, error) {
	if apiKey == "" {
		return dashScopeInventoryConnection{}, fmt.Errorf("%s requires %s", dashScopeInventoryCommand, dashScopeInventoryAPIKey)
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return dashScopeInventoryConnection{}, fmt.Errorf("%s requires a canonical workspace %s", dashScopeInventoryCommand, dashScopeInventoryBaseURL)
	}
	host := dashScopeInventoryHost.FindStringSubmatch(endpoint.Host)
	if endpoint.Scheme != "https" || len(host) == 0 || host[1] == "trial" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.Path != "/compatible-mode/v1" || endpoint.RawPath != "" || endpoint.Opaque != "" || endpoint.String() != baseURL {
		return dashScopeInventoryConnection{}, fmt.Errorf("%s requires a canonical Singapore or US East workspace %s", dashScopeInventoryCommand, dashScopeInventoryBaseURL)
	}
	endpoint.Path = dashScopeInventoryPath
	return dashScopeInventoryConnection{endpoint: *endpoint, region: host[2], apiKey: apiKey}, nil
}

func newDashScopeInventoryCommand() *cobra.Command {
	return &cobra.Command{
		Use:          dashScopeInventoryCommand,
		Short:        "Export complete Alibaba workspace model inventories for review",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, arguments []string) error {
			connection, err := newDashScopeInventoryConnection(os.Getenv(dashScopeInventoryAPIKey), os.Getenv(dashScopeInventoryBaseURL))
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(command.Context(), 2*time.Minute)
			defer cancel()
			client := &http.Client{Transport: http.DefaultTransport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			report := dashScopeInventoryExport{SourceHost: connection.endpoint.Host, AccessRegion: connection.region, WorkspaceServiceScope: "unknown", Collections: []dashScopeInventoryCollection{}}
			remainingBytes := dashScopeInventoryMaxBytes
			for _, supports := range []string{"inference", "deploy"} {
				collection, err := connection.collect(ctx, client, supports, &remainingBytes)
				if err != nil {
					return err
				}
				report.Collections = append(report.Collections, collection)
			}
			report.RetrievedAt = dashScopeInventoryNow().UTC().Format(time.RFC3339Nano)
			if err := json.NewEncoder(command.OutOrStdout()).Encode(report); err != nil {
				return fmt.Errorf("write DashScope inventory export: %w", err)
			}
			return nil
		},
	}
}

func (connection dashScopeInventoryConnection) collect(ctx context.Context, client *http.Client, supports string, remainingBytes *int) (dashScopeInventoryCollection, error) {
	collection := dashScopeInventoryCollection{Supports: supports, Query: map[string]string{"supports": supports, "page_size": strconv.Itoa(dashScopeInventoryPageSize)}, Pages: []dashScopeInventoryPageRecord{}, Models: []json.RawMessage{}}
	seen := make(map[string]struct{})
	for pageNo := 1; ; pageNo++ {
		endpoint := connection.endpoint
		endpoint.RawQuery = url.Values{"supports": {supports}, "page_no": {strconv.Itoa(pageNo)}, "page_size": {strconv.Itoa(dashScopeInventoryPageSize)}}.Encode()
		request := (&http.Request{Method: http.MethodGet, URL: &endpoint, Header: http.Header{"Authorization": {"Bearer " + connection.apiKey}, "Accept": {"application/json"}}}).WithContext(ctx)
		response, err := client.Do(request)
		if err != nil {
			return collection, fmt.Errorf("read DashScope %s inventory page %d: transport failed", supports, pageNo)
		}
		document, readError := io.ReadAll(io.LimitReader(response.Body, int64(min(dashScopeInventoryMaxPageBytes, *remainingBytes))+1))
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return collection, fmt.Errorf("read DashScope %s inventory page %d: HTTP %d", supports, pageNo, response.StatusCode)
		}
		if readError != nil || len(document) > dashScopeInventoryMaxPageBytes || len(document) > *remainingBytes {
			return collection, fmt.Errorf("read DashScope %s inventory page %d: response unreadable or too large", supports, pageNo)
		}
		*remainingBytes -= len(document)
		envelope, err := parseDashScopeInventoryPage(document, pageNo)
		if err != nil {
			return collection, fmt.Errorf("read DashScope %s inventory page %d: %w", supports, pageNo, err)
		}
		output := envelope.Output
		if pageNo == 1 {
			collection.Total = *output.Total
		}
		if *output.Total != collection.Total {
			return collection, fmt.Errorf("read DashScope %s inventory page %d: total changed", supports, pageNo)
		}
		expectedCount := min(dashScopeInventoryPageSize, collection.Total-len(collection.Models))
		if len(*output.Models) != expectedCount {
			return collection, fmt.Errorf("read DashScope %s inventory page %d: incomplete model count", supports, pageNo)
		}
		for _, model := range *output.Models {
			var identity struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(model, &identity); err != nil || identity.Model == "" || strings.TrimSpace(identity.Model) != identity.Model {
				return collection, fmt.Errorf("read DashScope %s inventory page %d: invalid model identity", supports, pageNo)
			}
			if _, exists := seen[identity.Model]; exists {
				return collection, fmt.Errorf("read DashScope %s inventory page %d: duplicate model identity", supports, pageNo)
			}
			seen[identity.Model] = struct{}{}
			collection.Models = append(collection.Models, model)
		}
		collection.Pages = append(collection.Pages, dashScopeInventoryPageRecord{PageNo: pageNo, RequestID: envelope.RequestID, ModelCount: len(*output.Models)})
		if len(collection.Models) == collection.Total {
			return collection, nil
		}
	}
}

func parseDashScopeInventoryPage(document []byte, pageNo int) (dashScopeInventoryEnvelope, error) {
	var envelope dashScopeInventoryEnvelope
	decoder := json.NewDecoder(bytes.NewReader(document))
	if err := decoder.Decode(&envelope); err != nil {
		return envelope, fmt.Errorf("invalid JSON response")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return envelope, fmt.Errorf("trailing response data")
	}
	if envelope.Success == nil || !*envelope.Success || (len(envelope.Code) > 0 && string(envelope.Code) != "null" && string(envelope.Code) != `""`) {
		return envelope, fmt.Errorf("provider reported failure")
	}
	output := envelope.Output
	if envelope.RequestID == "" || output == nil || output.Total == nil || output.PageNo == nil || output.PageSize == nil || output.Models == nil {
		return envelope, fmt.Errorf("missing page metadata")
	}
	if *output.Total < 0 || *output.Total > dashScopeInventoryMaxModels || *output.PageNo != pageNo || *output.PageSize != dashScopeInventoryPageSize {
		return envelope, fmt.Errorf("invalid page metadata")
	}
	return envelope, nil
}

func init() { rootCmd.AddCommand(newDashScopeInventoryCommand()) }
