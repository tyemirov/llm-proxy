package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	defaultConfigPath             = "config.yml"
	flagConfig                    = "config"
	flagPublicCapabilitiesOnly    = "public-capabilities-only"
	logEventStarting              = "starting proxy"
	logEventStartingPublicCatalog = "starting public capability API"
)

var runtimeConfiguration proxy.Configuration
var publicCapabilityConfiguration publicCapabilityAPIConfiguration
var serveProxy = proxy.Serve
var servePublicCapabilityAPI = proxy.ServePublicCapabilities
var loadConfiguration = loadRuntimeConfiguration

const (
	// rootCmdShort provides a brief description of the root command.
	rootCmdShort = "Tiny HTTP proxy for LLM providers"

	// rootCmdLong provides a detailed description of the root command.
	rootCmdLong = "Accepts GET / and JSON POST / for prompts and POST /dictate for audio transcription; forwards to configured LLM providers."

	// rootCmdExample demonstrates how to use the root command.
	rootCmdExample = `llm-proxy --config config.yml`
)

// Execute runs the command-line interface.
func Execute() {
	rootCmd.SilenceUsage = false
	rootCmd.SilenceErrors = false
	if executeError := rootCmd.Execute(); executeError != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:     "llm-proxy",
	Short:   rootCmdShort,
	Long:    rootCmdLong,
	Example: rootCmdExample,
	PreRunE: func(command *cobra.Command, arguments []string) error {
		configPath, _ := command.Flags().GetString(flagConfig)
		publicCapabilitiesOnly, _ := command.Flags().GetBool(flagPublicCapabilitiesOnly)
		if publicCapabilitiesOnly {
			configuration, loadError := loadPublicCapabilityAPIConfiguration(configPath)
			if loadError != nil {
				return loadError
			}
			publicCapabilityConfiguration = configuration
			return nil
		}
		configuration, loadError := loadConfiguration(configPath)
		if loadError != nil {
			return loadError
		}
		runtimeConfiguration = configuration
		return nil
	},
	RunE: func(command *cobra.Command, arguments []string) error {
		publicCapabilitiesOnly, _ := command.Flags().GetBool(flagPublicCapabilitiesOnly)
		logLevel := runtimeConfiguration.LogLevel
		if publicCapabilitiesOnly {
			logLevel = publicCapabilityConfiguration.LogLevel
		}
		logger := loggerForLevel(logLevel)
		defer func() { _ = logger.Sync() }()
		structuredLogger := logger.Sugar()
		if publicCapabilitiesOnly {
			structuredLogger.Infow(logEventStartingPublicCatalog,
				"port", publicCapabilityConfiguration.Port,
				"provider_count", len(publicCapabilityConfiguration.Catalog.Providers),
			)
			return servePublicCapabilityAPI(
				publicCapabilityConfiguration.Catalog,
				publicCapabilityConfiguration.Port,
				publicCapabilityConfiguration.LogLevel,
			)
		}

		structuredLogger.Infow(logEventStarting,
			"port", runtimeConfiguration.Port,
			"log_level", strings.ToLower(runtimeConfiguration.LogLevel),
		)
		return serveProxy(runtimeConfiguration, structuredLogger)
	},
}

func loggerForLevel(logLevel string) *zap.Logger {
	switch strings.ToLower(logLevel) {
	case proxy.LogLevelDebug:
		logger, _ := zap.NewDevelopment()
		return logger
	default:
		logger, _ := zap.NewProduction()
		return logger
	}
}

func init() {
	rootCmd.Flags().String(flagConfig, defaultConfigPath, "path to authoritative config.yml")
	rootCmd.Flags().Bool(flagPublicCapabilitiesOnly, false, "serve only the public capability REST resource")
}
