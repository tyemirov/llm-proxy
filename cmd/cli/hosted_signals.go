package main

import (
	"encoding/json"
	"github.com/spf13/cobra"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func newHostedFinancialSignalsCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{
		Use:   "hosted-financial-signals",
		Short: "Read financial work queues, comparison differences, and exact exposure totals",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, arguments []string) error {
			configuration, err := loadRuntimeConfiguration(configPath)
			if err != nil {
				return err
			}
			report, err := proxy.ReadHostedFinancialSignals(command.Context(), configuration.Management.DatabasePath)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(report)
		},
	}
	command.Flags().StringVar(&configPath, flagConfig, defaultConfigPath, "path to authoritative config.yml")
	return command
}

func init() { rootCmd.AddCommand(newHostedFinancialSignalsCommand()) }
