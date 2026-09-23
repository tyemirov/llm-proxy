package main

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

const paymentReconciliationCommandName = "reconcile-payments"

func newPaymentReconciliationCommand() *cobra.Command {
	var configPath, runID string
	command := &cobra.Command{
		Use:   paymentReconciliationCommandName,
		Short: "Record payment, receipt, and Ledger differences without financial changes",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, arguments []string) error {
			configuration, err := loadRuntimeConfiguration(configPath)
			if err != nil {
				return err
			}
			report, err := proxy.ReconcilePayments(command.Context(), configuration.Management, configuration.Payments, runID)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(report)
		},
	}
	command.Flags().StringVar(&configPath, flagConfig, defaultConfigPath, "path to authoritative config.yml")
	command.Flags().StringVar(&runID, "run-id", "", "durable run identifier; reuse to resume or read its report")
	return command
}

func init() { rootCmd.AddCommand(newPaymentReconciliationCommand()) }
