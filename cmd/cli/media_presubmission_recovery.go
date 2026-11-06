//go:build pilot_presubmission_recovery

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func newMediaPresubmissionRecoveryCommand() *cobra.Command {
	command := &cobra.Command{Use: "recover-media-presubmission", Short: "Operate the exact verified pilot recovery"}
	for _, phase := range []string{"prepare", "preview", "apply", "finalize"} {
		var configPath, inputPath string
		child := &cobra.Command{Use: phase, Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
			configuration, err := loadRuntimeConfiguration(configPath)
			if err != nil {
				return err
			}
			input, err := os.Open(inputPath)
			if err != nil {
				return fmt.Errorf("open recovery input: %w", err)
			}
			result, runError := proxy.RunMediaPresubmissionRecovery(command.Context(), configuration, command.Name(), input)
			if err := input.Close(); err != nil {
				return err
			}
			if runError != nil {
				return runError
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		}}
		child.Flags().StringVar(&configPath, flagConfig, defaultConfigPath, "path to authoritative config.yml")
		child.Flags().StringVar(&inputPath, "input", "", "closed evidence JSON for prepare, or the saved plan for later phases")
		command.AddCommand(child)
	}
	return command
}
func init() { rootCmd.AddCommand(newMediaPresubmissionRecoveryCommand()) }
