package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func newProviderReconciliationCommand() *cobra.Command {
	var configPath, runID, evidencePath, sourcePath string
	command := &cobra.Command{
		Use:   "reconcile-provider-costs",
		Short: "Compare retained usage costs with an invoice or provider export",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, arguments []string) error {
			configuration, err := loadRuntimeConfiguration(configPath)
			if err != nil {
				return err
			}
			evidence, err := os.Open(evidencePath)
			if err != nil {
				return fmt.Errorf("open normalized provider evidence: %w", err)
			}
			defer evidence.Close()
			source, err := os.Open(sourcePath)
			if err != nil {
				return fmt.Errorf("open original provider source: %w", err)
			}
			defer source.Close()
			report, err := proxy.ReconcileProviderCosts(command.Context(), configuration.Management, runID, evidence, source)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(report)
		},
	}
	command.Flags().StringVar(&configPath, flagConfig, defaultConfigPath, "path to authoritative config.yml")
	command.Flags().StringVar(&runID, "run-id", "", "durable identifier for this provider comparison")
	command.Flags().StringVar(&evidencePath, "evidence", "", "normalized invoice or export evidence JSON")
	command.Flags().StringVar(&sourcePath, "source", "", "original provider source whose digest the evidence declares")
	return command
}

func init() { rootCmd.AddCommand(newProviderReconciliationCommand()) }
