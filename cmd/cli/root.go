package main

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/temirov/llm-proxy/internal/apperrors"
	"github.com/temirov/llm-proxy/internal/constants"
	"github.com/temirov/llm-proxy/internal/proxy"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

const (
	envPrefix = "llm_proxy"

	keyOpenAIAPIKey                 = "openai_api_key"
	keyDeepSeekAPIKey               = "deepseek_api_key"
	keyDashScopeAPIKey              = "dashscope_api_key"
	keyMoonshotAPIKey               = "moonshot_api_key"
	keySiliconFlowAPIKey            = "siliconflow_api_key"
	keyZhipuAPIKey                  = "zhipu_api_key"
	keyServiceSecret                = "service_secret"
	keyDefaultProvider              = "default_provider"
	keyDefaultModel                 = "default_model"
	keyDefaultDictationProvider     = "default_dictation_provider"
	keyDeepSeekBaseURL              = "deepseek_base_url"
	keyDashScopeBaseURL             = "dashscope_base_url"
	keyMoonshotBaseURL              = "moonshot_base_url"
	keySiliconFlowBaseURL           = "siliconflow_base_url"
	keySiliconFlowTranscriptionsURL = "siliconflow_transcriptions_url"
	keyZhipuBaseURL                 = "zhipu_base_url"
	keyLogLevel                     = "log_level"
	keySystemPrompt                 = "system_prompt"
	keyWorkers                      = "workers"
	keyQueueSize                    = "queue_size"
	keyPort                         = "port"
	keyRequestTimeoutSeconds        = "request_timeout_seconds"
	keyUpstreamPollTimeoutSeconds   = "upstream_poll_timeout_seconds"
	keyMaxOutputTokens              = "max_output_tokens"
	keyMaxPromptBytes               = "max_prompt_bytes"
	keyDictationModel               = "dictation_model"
	keyMaxInputAudioBytes           = "max_input_audio_bytes"

	flagOpenAIAPIKey                 = keyOpenAIAPIKey
	flagDeepSeekAPIKey               = keyDeepSeekAPIKey
	flagDashScopeAPIKey              = keyDashScopeAPIKey
	flagMoonshotAPIKey               = keyMoonshotAPIKey
	flagSiliconFlowAPIKey            = keySiliconFlowAPIKey
	flagZhipuAPIKey                  = keyZhipuAPIKey
	flagServiceSecret                = keyServiceSecret
	flagDefaultProvider              = keyDefaultProvider
	flagDefaultModel                 = keyDefaultModel
	flagDefaultDictationProvider     = keyDefaultDictationProvider
	flagDeepSeekBaseURL              = keyDeepSeekBaseURL
	flagDashScopeBaseURL             = keyDashScopeBaseURL
	flagMoonshotBaseURL              = keyMoonshotBaseURL
	flagSiliconFlowBaseURL           = keySiliconFlowBaseURL
	flagSiliconFlowTranscriptionsURL = keySiliconFlowTranscriptionsURL
	flagZhipuBaseURL                 = keyZhipuBaseURL
	flagLogLevel                     = keyLogLevel
	flagSystemPrompt                 = keySystemPrompt
	flagWorkers                      = keyWorkers
	flagQueueSize                    = keyQueueSize
	flagPort                         = keyPort
	flagRequestTimeout               = "request_timeout"
	flagUpstreamPollTimeout          = "upstream_poll_timeout"
	flagMaxOutputTokens              = keyMaxOutputTokens
	flagMaxPromptBytes               = keyMaxPromptBytes
	flagDictationModel               = keyDictationModel
	flagMaxInputAudioBytes           = keyMaxInputAudioBytes

	envOpenAIAPIKey                 = "OPENAI_API_KEY"
	envDeepSeekAPIKey               = "DEEPSEEK_API_KEY"
	envDashScopeAPIKey              = "DASHSCOPE_API_KEY"
	envMoonshotAPIKey               = "MOONSHOT_API_KEY"
	envSiliconFlowAPIKey            = "SILICONFLOW_API_KEY"
	envZhipuAPIKey                  = "ZHIPU_API_KEY"
	envServiceSecret                = "SERVICE_SECRET"
	envDefaultProvider              = "LLM_PROXY_DEFAULT_PROVIDER"
	envDefaultModel                 = "LLM_PROXY_DEFAULT_MODEL"
	envDefaultDictationProvider     = "LLM_PROXY_DEFAULT_DICTATION_PROVIDER"
	envDeepSeekBaseURL              = "DEEPSEEK_BASE_URL"
	envDashScopeBaseURL             = "DASHSCOPE_BASE_URL"
	envMoonshotBaseURL              = "MOONSHOT_BASE_URL"
	envSiliconFlowBaseURL           = "SILICONFLOW_BASE_URL"
	envSiliconFlowTranscriptionsURL = "SILICONFLOW_TRANSCRIPTIONS_URL"
	envZhipuBaseURL                 = "ZHIPU_BASE_URL"
	envLogLevel                     = "LOG_LEVEL"
	envSystemPrompt                 = "SYSTEM_PROMPT"
	envWorkers                      = "LLM_PROXY_WORKERS"
	envQueueSize                    = "LLM_PROXY_QUEUE_SIZE"
	envPort                         = "HTTP_PORT"
	envRequestTimeoutSeconds        = "LLM_PROXY_REQUEST_TIMEOUT_SECONDS"
	envUpstreamPollTimeoutSeconds   = "LLM_PROXY_UPSTREAM_POLL_TIMEOUT_SECONDS"
	envMaxOutputTokens              = "LLM_PROXY_MAX_OUTPUT_TOKENS"
	envMaxPromptBytes               = "LLM_PROXY_MAX_PROMPT_BYTES"
	envDictationModel               = "LLM_PROXY_DICTATION_MODEL"
	envMaxInputAudioBytes           = "LLM_PROXY_MAX_INPUT_AUDIO_BYTES"

	quoteCharacters = "\"'"
)

const (
	// messageServiceSecretEmpty is logged when SERVICE_SECRET is missing.
	messageServiceSecretEmpty = "SERVICE_SECRET is empty; refusing to start"
	// messageOpenAIAPIKeyEmpty is logged when OPENAI_API_KEY is missing.
	messageOpenAIAPIKeyEmpty = "OPENAI_API_KEY is empty; refusing to start"
	// logEventStartingProxy indicates the proxy is starting.
	logEventStartingProxy = "starting proxy"
)

const (
	// bindingErrorSeparator is used to join multiple binding errors.
	bindingErrorSeparator = "; "
)

var config proxy.Configuration

const (
	// rootCmdShort provides a brief description of the root command.
	// Additional commands should define their short description using a constant following this pattern.
	rootCmdShort = "Tiny HTTP proxy for LLM providers"

	// rootCmdLong provides a detailed description of the root command.
	// Additional commands should define their long description using a constant following this pattern.
	rootCmdLong = "Accepts GET / and JSON POST / for prompts and POST /dictate for audio transcription; forwards to configured LLM providers."

	// rootCmdExample demonstrates how to use the root command.
	// Additional commands should define their usage examples using a constant following this pattern.
	rootCmdExample = `llm-proxy --service_secret=mysecret --openai_api_key=sk-xxxxx --log_level=debug
SERVICE_SECRET=mysecret OPENAI_API_KEY=sk-xxxxx LOG_LEVEL=debug llm-proxy
SERVICE_SECRET=mysecret DEEPSEEK_API_KEY=sk-xxxxx LLM_PROXY_DEFAULT_PROVIDER=deepseek llm-proxy`
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
	RunE: func(command *cobra.Command, arguments []string) error {
		populateStringConfiguration(command, flagServiceSecret, keyServiceSecret, &config.ServiceSecret, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagOpenAIAPIKey, keyOpenAIAPIKey, &config.OpenAIKey, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagDeepSeekAPIKey, keyDeepSeekAPIKey, &config.DeepSeekKey, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagDashScopeAPIKey, keyDashScopeAPIKey, &config.DashScopeKey, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagMoonshotAPIKey, keyMoonshotAPIKey, &config.MoonshotKey, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagSiliconFlowAPIKey, keySiliconFlowAPIKey, &config.SiliconFlowKey, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagZhipuAPIKey, keyZhipuAPIKey, &config.ZhipuKey, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagDefaultProvider, keyDefaultProvider, &config.DefaultProvider, proxy.DefaultProvider, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagDefaultModel, keyDefaultModel, &config.DefaultModel, proxy.DefaultModel, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagDefaultDictationProvider, keyDefaultDictationProvider, &config.DefaultDictationProvider, proxy.DefaultDictationProvider, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagDeepSeekBaseURL, keyDeepSeekBaseURL, &config.DeepSeekBaseURL, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagDashScopeBaseURL, keyDashScopeBaseURL, &config.DashScopeBaseURL, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagMoonshotBaseURL, keyMoonshotBaseURL, &config.MoonshotBaseURL, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagSiliconFlowBaseURL, keySiliconFlowBaseURL, &config.SiliconFlowBaseURL, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagSiliconFlowTranscriptionsURL, keySiliconFlowTranscriptionsURL, &config.SiliconFlowTranscriptionsURL, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagZhipuBaseURL, keyZhipuBaseURL, &config.ZhipuBaseURL, constants.EmptyString, trimSpacesAndQuotes)
		populateIntConfiguration(command, flagPort, keyPort, &config.Port, proxy.DefaultPort)
		populateStringConfiguration(command, flagLogLevel, keyLogLevel, &config.LogLevel, proxy.LogLevelInfo, identityTransformer)
		populateStringConfiguration(command, flagSystemPrompt, keySystemPrompt, &config.SystemPrompt, constants.EmptyString, identityTransformer)
		populateIntConfiguration(command, flagWorkers, keyWorkers, &config.WorkerCount, proxy.DefaultWorkers)
		populateIntConfiguration(command, flagQueueSize, keyQueueSize, &config.QueueSize, proxy.DefaultQueueSize)
		populateIntConfiguration(command, flagRequestTimeout, keyRequestTimeoutSeconds, &config.RequestTimeoutSeconds, proxy.DefaultRequestTimeoutSeconds)
		populateIntConfiguration(command, flagUpstreamPollTimeout, keyUpstreamPollTimeoutSeconds, &config.UpstreamPollTimeoutSeconds, proxy.DefaultUpstreamPollTimeoutSeconds)
		populateIntConfiguration(command, flagMaxOutputTokens, keyMaxOutputTokens, &config.MaxOutputTokens, proxy.DefaultMaxOutputTokens)
		populateInt64Configuration(command, flagMaxPromptBytes, keyMaxPromptBytes, &config.MaxPromptBytes, proxy.DefaultMaxPromptBytes)
		populateStringConfiguration(command, flagDictationModel, keyDictationModel, &config.DictationModel, proxy.DefaultDictationModel, trimSpacesAndQuotes)
		populateInt64Configuration(command, flagMaxInputAudioBytes, keyMaxInputAudioBytes, &config.MaxInputAudioBytes, proxy.DefaultMaxInputAudioBytes)
		config.ApplyTunables()

		var logger *zap.Logger
		var loggerError error
		switch strings.ToLower(config.LogLevel) {
		case proxy.LogLevelDebug:
			logger, loggerError = zap.NewDevelopment()
		default:
			logger, loggerError = zap.NewProduction()
		}
		if loggerError != nil {
			return loggerError
		}
		defer func() { _ = logger.Sync() }()
		sugar := logger.Sugar()

		if strings.TrimSpace(config.ServiceSecret) == constants.EmptyString {
			sugar.Error(messageServiceSecretEmpty)
			return apperrors.ErrMissingServiceSecret
		}
		if strings.EqualFold(config.DefaultProvider, proxy.ProviderNameOpenAI) && strings.TrimSpace(config.OpenAIKey) == constants.EmptyString {
			sugar.Error(messageOpenAIAPIKeyEmpty)
			return apperrors.ErrMissingOpenAIKey
		}

		sugar.Infow(logEventStartingProxy,
			"port", config.Port,
			"log_level", strings.ToLower(config.LogLevel),
			"default_provider", config.DefaultProvider,
			"secret_fingerprint", utils.Fingerprint(config.ServiceSecret),
		)
		return proxy.Serve(config, sugar)
	},
}

// bindOrDie wraps viper bindings and returns a combined error if any bind fails.
func bindOrDie() error {
	var bindingErrors []string
	for _, binding := range environmentBindings() {
		bindArguments := append([]string{binding.key}, binding.environmentVariables...)
		if bindError := viper.BindEnv(bindArguments...); bindError != nil {
			bindingErrors = append(bindingErrors, binding.key+":"+bindError.Error())
		}
	}
	if len(bindingErrors) > 0 {
		return errors.New(strings.Join(bindingErrors, bindingErrorSeparator))
	}
	return nil
}

type environmentBinding struct {
	key                  string
	environmentVariables []string
}

func environmentBindings() []environmentBinding {
	return []environmentBinding{
		{key: keyOpenAIAPIKey, environmentVariables: []string{envOpenAIAPIKey}},
		{key: keyDeepSeekAPIKey, environmentVariables: []string{envDeepSeekAPIKey}},
		{key: keyDashScopeAPIKey, environmentVariables: []string{envDashScopeAPIKey}},
		{key: keyMoonshotAPIKey, environmentVariables: []string{envMoonshotAPIKey}},
		{key: keySiliconFlowAPIKey, environmentVariables: []string{envSiliconFlowAPIKey}},
		{key: keyZhipuAPIKey, environmentVariables: []string{envZhipuAPIKey}},
		{key: keyServiceSecret, environmentVariables: []string{envServiceSecret}},
		{key: keyDefaultProvider, environmentVariables: []string{envDefaultProvider}},
		{key: keyDefaultModel, environmentVariables: []string{envDefaultModel}},
		{key: keyDefaultDictationProvider, environmentVariables: []string{envDefaultDictationProvider}},
		{key: keyDeepSeekBaseURL, environmentVariables: []string{envDeepSeekBaseURL}},
		{key: keyDashScopeBaseURL, environmentVariables: []string{envDashScopeBaseURL}},
		{key: keyMoonshotBaseURL, environmentVariables: []string{envMoonshotBaseURL}},
		{key: keySiliconFlowBaseURL, environmentVariables: []string{envSiliconFlowBaseURL}},
		{key: keySiliconFlowTranscriptionsURL, environmentVariables: []string{envSiliconFlowTranscriptionsURL}},
		{key: keyZhipuBaseURL, environmentVariables: []string{envZhipuBaseURL}},
		{key: keyLogLevel, environmentVariables: []string{envLogLevel}},
		{key: keySystemPrompt, environmentVariables: []string{envSystemPrompt}},
		{key: keyWorkers, environmentVariables: []string{envWorkers}},
		{key: keyQueueSize, environmentVariables: []string{envQueueSize}},
		{key: keyPort, environmentVariables: []string{envPort}},
		{key: keyRequestTimeoutSeconds, environmentVariables: []string{envRequestTimeoutSeconds}},
		{key: keyUpstreamPollTimeoutSeconds, environmentVariables: []string{envUpstreamPollTimeoutSeconds}},
		{key: keyMaxOutputTokens, environmentVariables: []string{envMaxOutputTokens}},
		{key: keyMaxPromptBytes, environmentVariables: []string{envMaxPromptBytes}},
		{key: keyDictationModel, environmentVariables: []string{envDictationModel}},
		{key: keyMaxInputAudioBytes, environmentVariables: []string{envMaxInputAudioBytes}},
	}
}

func init() {
	viper.SetEnvPrefix(envPrefix)
	viper.AutomaticEnv()

	if bindError := bindOrDie(); bindError != nil {
		panic("viper env binding failed: " + bindError.Error())
	}

	rootCmd.Flags().StringVar(&config.ServiceSecret, flagServiceSecret, "", "shared secret for requests (env: "+envServiceSecret+")")
	rootCmd.Flags().StringVar(&config.OpenAIKey, flagOpenAIAPIKey, "", "OpenAI API key (env: "+envOpenAIAPIKey+")")
	rootCmd.Flags().StringVar(&config.DeepSeekKey, flagDeepSeekAPIKey, "", "DeepSeek API key (env: "+envDeepSeekAPIKey+")")
	rootCmd.Flags().StringVar(&config.DashScopeKey, flagDashScopeAPIKey, "", "DashScope API key (env: "+envDashScopeAPIKey+")")
	rootCmd.Flags().StringVar(&config.MoonshotKey, flagMoonshotAPIKey, "", "Moonshot/Kimi API key (env: "+envMoonshotAPIKey+")")
	rootCmd.Flags().StringVar(&config.SiliconFlowKey, flagSiliconFlowAPIKey, "", "SiliconFlow API key (env: "+envSiliconFlowAPIKey+")")
	rootCmd.Flags().StringVar(&config.ZhipuKey, flagZhipuAPIKey, "", "Zhipu/GLM API key (env: "+envZhipuAPIKey+")")
	rootCmd.Flags().StringVar(&config.DefaultProvider, flagDefaultProvider, "", "default text provider (env: "+envDefaultProvider+")")
	rootCmd.Flags().StringVar(&config.DefaultModel, flagDefaultModel, "", "default text model (env: "+envDefaultModel+")")
	rootCmd.Flags().StringVar(&config.DefaultDictationProvider, flagDefaultDictationProvider, "", "default dictation provider (env: "+envDefaultDictationProvider+")")
	rootCmd.Flags().StringVar(&config.DeepSeekBaseURL, flagDeepSeekBaseURL, "", "DeepSeek base URL (env: "+envDeepSeekBaseURL+")")
	rootCmd.Flags().StringVar(&config.DashScopeBaseURL, flagDashScopeBaseURL, "", "DashScope OpenAI-compatible base URL (env: "+envDashScopeBaseURL+")")
	rootCmd.Flags().StringVar(&config.MoonshotBaseURL, flagMoonshotBaseURL, "", "Moonshot OpenAI-compatible base URL (env: "+envMoonshotBaseURL+")")
	rootCmd.Flags().StringVar(&config.SiliconFlowBaseURL, flagSiliconFlowBaseURL, "", "SiliconFlow OpenAI-compatible base URL (env: "+envSiliconFlowBaseURL+")")
	rootCmd.Flags().StringVar(&config.SiliconFlowTranscriptionsURL, flagSiliconFlowTranscriptionsURL, "", "SiliconFlow transcription URL (env: "+envSiliconFlowTranscriptionsURL+")")
	rootCmd.Flags().StringVar(&config.ZhipuBaseURL, flagZhipuBaseURL, "", "Zhipu OpenAI-compatible base URL (env: "+envZhipuBaseURL+")")
	rootCmd.Flags().IntVar(&config.Port, flagPort, 0, "TCP port to listen on (env: "+envPort+")")
	rootCmd.Flags().StringVar(&config.LogLevel, flagLogLevel, "", "logging level: debug or info (env: "+envLogLevel+")")
	rootCmd.Flags().StringVar(&config.SystemPrompt, flagSystemPrompt, "", "system prompt sent to the model (env: "+envSystemPrompt+")")
	rootCmd.Flags().IntVar(&config.WorkerCount, flagWorkers, 0, "number of worker goroutines (env: "+envWorkers+")")
	rootCmd.Flags().IntVar(&config.QueueSize, flagQueueSize, 0, "request queue size (env: "+envQueueSize+")")
	rootCmd.Flags().IntVar(&config.RequestTimeoutSeconds, flagRequestTimeout, 0, "overall request timeout in seconds (env: "+envRequestTimeoutSeconds+")")
	rootCmd.Flags().IntVar(&config.UpstreamPollTimeoutSeconds, flagUpstreamPollTimeout, 0, "upstream poll timeout in seconds for incomplete responses (env: "+envUpstreamPollTimeoutSeconds+")")
	rootCmd.Flags().IntVar(&config.MaxOutputTokens, flagMaxOutputTokens, 0, "maximum output tokens (env: "+envMaxOutputTokens+")")
	rootCmd.Flags().Int64Var(&config.MaxPromptBytes, flagMaxPromptBytes, 0, "maximum accepted JSON prompt payload size for POST / in bytes (env: "+envMaxPromptBytes+")")
	rootCmd.Flags().StringVar(&config.DictationModel, flagDictationModel, "", "default model for /dictate when query model is not provided (env: "+envDictationModel+")")
	rootCmd.Flags().Int64Var(&config.MaxInputAudioBytes, flagMaxInputAudioBytes, 0, "maximum accepted audio payload size for /dictate in bytes (env: "+envMaxInputAudioBytes+")")

	if flagBindError := viper.BindPFlags(rootCmd.Flags()); flagBindError != nil {
		panic("failed to bind flags: " + flagBindError.Error())
	}
}
