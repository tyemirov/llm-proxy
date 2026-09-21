package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

const mediaPollInterval = 250 * time.Millisecond

type mediaCommandOptions struct {
	voiceQuery                                                            llmproxyclient.MediaVoiceQuery
	voiceIncludeTotal                                                     bool
	baseURL, secret, operationID, assetID, file, mimeType, idempotencyKey string
	timeout                                                               time.Duration
}

func newMediaCommand(stdin io.Reader, stdout io.Writer, factory httpClientFactory) *cobra.Command {
	options := mediaCommandOptions{}
	parent := &cobra.Command{Use: "media", Short: "Use tenant-owned speech operations and assets"}
	parent.PersistentFlags().StringVar(&options.baseURL, flagBaseURL, "", "LLM Proxy API URL")
	parent.PersistentFlags().StringVar(&options.secret, flagSecret, "", "Tenant bearer key")
	parent.PersistentFlags().DurationVar(&options.timeout, "timeout", 5*time.Minute, "Client request or wait limit; expiry does not cancel the operation")
	commands := []struct {
		name, description string
		configure         func(*cobra.Command)
		execute           func(context.Context, llmproxyclient.Client) (any, error)
	}{
		{"submit", "Accept a media request from stdin and return its durable operation ID", func(command *cobra.Command) {
			command.Flags().StringVar(&options.idempotencyKey, "idempotency-key", "", "Stable key for this complete request")
		}, func(ctx context.Context, client llmproxyclient.Client) (any, error) {
			var wire struct {
				llmproxyclient.MediaOperationInput
				Model json.RawMessage `json:"model"`
			}
			decoder := json.NewDecoder(stdin)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&wire); err != nil {
				return nil, fmt.Errorf("read media request: %w", err)
			}
			if err := decoder.Decode(&struct{}{}); err != io.EOF {
				return nil, fmt.Errorf("media request must contain one JSON object")
			}
			input := wire.MediaOperationInput
			if len(wire.Model) > 0 && (json.Unmarshal(wire.Model, &input.Model) != nil || strings.TrimSpace(input.Model) == "") {
				return nil, fmt.Errorf("media model must be a nonempty string when present")
			}
			return client.CreateMediaOperation(ctx, options.idempotencyKey, input)
		}},
		{"status", "Read an operation", func(command *cobra.Command) {
			command.Flags().StringVar(&options.operationID, "operation-id", "", "Operation ID")
		}, func(ctx context.Context, client llmproxyclient.Client) (any, error) {
			return client.GetMediaOperation(ctx, options.operationID)
		}},
		{"wait", "Wait for an existing operation without another submission", func(command *cobra.Command) {
			command.Flags().StringVar(&options.operationID, "operation-id", "", "Operation ID")
		}, func(ctx context.Context, client llmproxyclient.Client) (any, error) {
			return client.WaitMediaOperation(ctx, options.operationID, mediaPollInterval)
		}},
		{"cancel", "Request cancellation and report its observed outcome", func(command *cobra.Command) {
			command.Flags().StringVar(&options.operationID, "operation-id", "", "Operation ID")
		}, func(ctx context.Context, client llmproxyclient.Client) (any, error) {
			return client.CancelMediaOperation(ctx, options.operationID)
		}},
		{"voices", "Discover tenant voices", func(command *cobra.Command) {
			command.Flags().StringVar(&options.voiceQuery.Provider, flagProvider, "", "Provider ID")
			command.Flags().StringVar(&options.voiceQuery.Cursor, "cursor", "", "Opaque cursor from the preceding page")
			command.Flags().StringVar(&options.voiceQuery.Search, "search", "", "Voice name search")
			command.Flags().IntVar(&options.voiceQuery.PageSize, "page-size", 0, "Maximum requested page size (1 to 100)")
			command.Flags().StringVar(&options.voiceQuery.Sort, "sort", "", "Voice order: name or created_at_unix")
			command.Flags().StringVar(&options.voiceQuery.SortDirection, "sort-direction", "", "Sort direction: asc or desc")
			command.Flags().StringVar(&options.voiceQuery.VoiceType, "voice-type", "", "Provider voice type")
			command.Flags().StringVar(&options.voiceQuery.Category, "category", "", "Provider voice category")
			command.Flags().BoolVar(&options.voiceIncludeTotal, "include-total-count", false, "Request the total voice count")
			command.PreRun = func(command *cobra.Command, _ []string) {
				if command.Flags().Changed("include-total-count") {
					options.voiceQuery.IncludeTotalCount = &options.voiceIncludeTotal
				}
			}
		}, func(ctx context.Context, client llmproxyclient.Client) (any, error) {
			return client.GetMediaVoices(ctx, options.voiceQuery)
		}},
		{"capabilities", "List available media routes", func(*cobra.Command) {}, func(ctx context.Context, client llmproxyclient.Client) (any, error) {
			return client.GetMediaCapabilities(ctx)
		}},
		{"upload", "Upload a file and return its tenant asset ID", func(command *cobra.Command) {
			command.Flags().StringVar(&options.file, "file", "", "Input file")
			command.Flags().StringVar(&options.mimeType, "mime-type", "", "File MIME type")
		}, func(ctx context.Context, client llmproxyclient.Client) (any, error) {
			data, err := os.ReadFile(options.file)
			if err != nil {
				return nil, fmt.Errorf("read media file: %w", err)
			}
			return client.UploadAsset(ctx, llmproxyclient.AssetUploadInput{MIMEType: options.mimeType, Data: data})
		}},
		{"download", "Write exact asset bytes to stdout", func(command *cobra.Command) { command.Flags().StringVar(&options.assetID, "asset-id", "", "Asset ID") }, func(ctx context.Context, client llmproxyclient.Client) (any, error) {
			asset, err := client.GetAsset(ctx, options.assetID)
			if err != nil {
				return nil, err
			}
			return client.DownloadAsset(ctx, asset)
		}},
	}
	for _, definition := range commands {
		command := &cobra.Command{Use: definition.name, Short: definition.description, Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
			if options.timeout <= 0 {
				return fmt.Errorf("media timeout must be positive")
			}
			config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: configuredString(command, flagBaseURL, envNameBaseURL, options.baseURL), Secret: configuredString(command, flagSecret, envNameDefaultTenantKey, options.secret)})
			if err != nil {
				return fmt.Errorf("configure media client: %w", err)
			}
			client, err := llmproxyclient.NewClient(config, factory())
			if err != nil {
				return fmt.Errorf("create media client: %w", err)
			}
			ctx, cancel := context.WithTimeout(command.Context(), options.timeout)
			defer cancel()
			result, err := definition.execute(ctx, client)
			if err != nil {
				return fmt.Errorf("media %s: %w", definition.name, err)
			}
			if data, ok := result.([]byte); ok {
				_, err = stdout.Write(data)
			} else {
				err = json.NewEncoder(stdout).Encode(result)
			}
			if err != nil {
				return fmt.Errorf("write media result: %w", err)
			}
			return nil
		}}
		definition.configure(command)
		parent.AddCommand(command)
	}
	return parent
}
