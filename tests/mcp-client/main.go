// Command mcp-client qualifies a local OAuth grant with the official MCP client.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type input struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
	TenantID string `json:"tenant_id"`
}
type bearerTransport struct{ token string }

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(clone)
}
func run() error {
	var configuration input
	if err := json.NewDecoder(os.Stdin).Decode(&configuration); err != nil {
		return fmt.Errorf("invalid acceptance input")
	}
	if configuration.Endpoint == "" || configuration.Token == "" || configuration.TenantID == "" {
		return fmt.Errorf("missing acceptance input")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "llm-proxy-local-acceptance", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: configuration.Endpoint, HTTPClient: &http.Client{Transport: bearerTransport{configuration.Token}}}, nil)
	if err != nil {
		return fmt.Errorf("MCP connection failed")
	}
	defer session.Close()
	list, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "llm_proxy.list_tenants", Arguments: struct{}{}})
	if err != nil || list.IsError {
		return fmt.Errorf("MCP discovery failed")
	}
	data, err := json.Marshal(list.StructuredContent)
	if err != nil {
		return err
	}
	var tenants struct {
		Tenants []struct {
			ID string `json:"id"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal(data, &tenants); err != nil {
		return err
	}
	found := false
	for _, tenant := range tenants.Tenants {
		if tenant.ID == configuration.TenantID {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("owned tenant absent")
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "llm_proxy.generate_text", Arguments: map[string]any{"tenant_id": configuration.TenantID, "messages": []map[string]string{{"role": "user", "content": "Local MCP acceptance"}}}})
	if err != nil || result.IsError {
		return fmt.Errorf("MCP generation failed")
	}
	data, err = json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	var output struct {
		Text      string `json:"text"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		return err
	}
	if output.Text != "Local MCP answer" || output.RequestID == "" {
		return fmt.Errorf("MCP generation output mismatch")
	}
	_, err = fmt.Fprintln(os.Stdout, "MCP OAuth discovery and generation passed")
	return err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
