package main

import (
	"github.com/spf13/cobra"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func newQualificationBudgetCommand() *cobra.Command {
	command := &cobra.Command{Use: "qualification-budget [inspect|close|reconcile-max]", Short: "Inspect or close the local operator qualification journal", Args: cobra.ExactArgs(1), PreRunE: func(*cobra.Command, []string) error { return nil }, RunE: func(command *cobra.Command, args []string) error {
		path, _ := command.Flags().GetString("management-database")
		budgetID, _ := command.Flags().GetString("budget-id")
		if args[0] == "reconcile-max" {
			attemptID, _ := command.Flags().GetUint64("attempt-id")
			return proxy.ReconcileQualificationBudgetMaximum(path, budgetID, attemptID, command.OutOrStdout())
		}
		return proxy.OperateQualificationBudget(path, budgetID, args[0], command.OutOrStdout())
	}}
	command.Flags().String("management-database", "", "existing managed database path")
	_ = command.MarkFlagRequired("management-database")
	command.Flags().Uint64("attempt-id", 0, "uncertain inactive attempt to consume at its reserved maximum")
	command.Flags().String("budget-id", "", "initialized budget identity")
	_ = command.MarkFlagRequired("budget-id")
	return command
}
