// Package main provides the installable llm-proxy-client command.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

const (
	commandUse   = "llm-proxy-client"
	commandShort = "Send a JSON POST prompt request through llm-proxy"

	flagBaseURL      = "base-url"
	flagSecret       = "secret"
	flagProvider     = "provider"
	flagModel        = "model"
	flagPrompt       = "prompt"
	flagPromptFile   = "prompt-file"
	flagWebSearch    = "web-search"
	flagSystemPrompt = "system-prompt"
	flagMaxTokens    = "max-tokens"
	flagTimeout      = "timeout"

	envNameBaseURL = "LLM_PROXY_BASE_URL"
	envNameSecret  = "LLM_PROXY_SECRET"

	defaultTimeout = 120 * time.Second
)

type commandOptions struct {
	baseURL      string
	secret       string
	provider     string
	model        string
	prompt       string
	promptFile   string
	webSearch    bool
	systemPrompt string
	maxTokens    int
	timeout      time.Duration
}

type httpClientFactory func(timeout time.Duration) llmproxyclient.HTTPDoer

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, defaultHTTPClientFactory))
}

func run(
	arguments []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	httpClientFactoryValue httpClientFactory,
) int {
	executeError := execute(arguments, stdin, stdout, httpClientFactoryValue)
	if executeError != nil {
		_, _ = io.WriteString(stderr, executeError.Error()+"\n")
		return 1
	}
	return 0
}

func execute(
	arguments []string,
	stdin io.Reader,
	stdout io.Writer,
	httpClientFactoryValue httpClientFactory,
) error {
	rootCommand := newRootCommand(stdin, stdout, httpClientFactoryValue)
	rootCommand.SetArgs(arguments)
	return rootCommand.Execute()
}

func newRootCommand(
	stdin io.Reader,
	stdout io.Writer,
	httpClientFactoryValue httpClientFactory,
) *cobra.Command {
	options := commandOptions{}
	rootCommand := &cobra.Command{
		Use:           commandUse,
		Short:         commandShort,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, arguments []string) error {
			prompt, promptError := readPrompt(stdin, options.prompt, options.promptFile)
			if promptError != nil {
				return promptError
			}
			config, configError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
				BaseURL:  configuredString(command, flagBaseURL, envNameBaseURL, options.baseURL),
				Secret:   configuredString(command, flagSecret, envNameSecret, options.secret),
				Provider: options.provider,
				Timeout:  options.timeout,
			})
			if configError != nil {
				return fmt.Errorf("llm_proxy_client_config_failed: %w", configError)
			}
			requestInput := llmproxyclient.RequestInput{
				Prompt:       prompt,
				Model:        options.model,
				WebSearch:    options.webSearch,
				SystemPrompt: options.systemPrompt,
			}
			if command.Flags().Changed(flagMaxTokens) {
				requestInput.MaxTokens = &options.maxTokens
			}
			request, requestError := llmproxyclient.NewRequest(requestInput)
			if requestError != nil {
				return fmt.Errorf("llm_proxy_client_request_failed: %w", requestError)
			}
			client, clientError := llmproxyclient.NewClient(config, httpClientFactoryValue(config.Timeout()))
			if clientError != nil {
				return fmt.Errorf("llm_proxy_client_create_failed: %w", clientError)
			}
			responseText, postError := client.Post(context.Background(), request)
			if postError != nil {
				return fmt.Errorf("llm_proxy_client_post_failed: %w", postError)
			}
			_, writeError := io.WriteString(stdout, responseText)
			if writeError != nil {
				return fmt.Errorf("llm_proxy_client_stdout_write_failed: %w", writeError)
			}
			return nil
		},
	}

	flagSet := rootCommand.Flags()
	flagSet.StringVar(&options.baseURL, flagBaseURL, "", "llm-proxy base URL")
	flagSet.StringVar(&options.secret, flagSecret, "", "llm-proxy shared secret")
	flagSet.StringVar(&options.provider, flagProvider, "", "provider query override")
	flagSet.StringVar(&options.model, flagModel, "", "model body field")
	flagSet.StringVar(&options.prompt, flagPrompt, "", "prompt text")
	flagSet.StringVar(&options.promptFile, flagPromptFile, "", "path to prompt text file")
	flagSet.BoolVar(&options.webSearch, flagWebSearch, false, "enable OpenAI web search")
	flagSet.StringVar(&options.systemPrompt, flagSystemPrompt, "", "system prompt body field")
	flagSet.IntVar(&options.maxTokens, flagMaxTokens, 0, "positive output token cap")
	flagSet.DurationVar(&options.timeout, flagTimeout, defaultTimeout, "request timeout")

	return rootCommand
}

func configuredString(command *cobra.Command, flagName string, envName string, flagValue string) string {
	if command.Flags().Changed(flagName) {
		return flagValue
	}
	return os.Getenv(envName)
}

func readPrompt(stdin io.Reader, promptValue string, promptFile string) (string, error) {
	if promptValue != "" && promptFile != "" {
		return "", fmt.Errorf("llm_proxy_client_prompt_source_conflict: choose --prompt or --prompt-file")
	}
	if promptValue != "" {
		return promptValue, nil
	}
	if promptFile != "" {
		promptBytes, readError := os.ReadFile(promptFile)
		if readError != nil {
			return "", fmt.Errorf("llm_proxy_client_prompt_file_read_failed: %w", readError)
		}
		return string(promptBytes), nil
	}
	promptBytes, readError := io.ReadAll(stdin)
	if readError != nil {
		return "", fmt.Errorf("llm_proxy_client_stdin_read_failed: %w", readError)
	}
	return string(promptBytes), nil
}

func defaultHTTPClientFactory(timeout time.Duration) llmproxyclient.HTTPDoer {
	return &http.Client{Timeout: timeout}
}
