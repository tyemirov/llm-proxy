package proxy_test

import (
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestApplyTunablesSetsDefaultDictationConfiguration(testingInstance *testing.T) {
	configuration := proxy.Configuration{}
	configuration.ApplyTunables()
	if configuration.MaxInputAudioBytes != proxy.DefaultMaxInputAudioBytes {
		testingInstance.Fatalf("maxInputAudioBytes=%d want=%d", configuration.MaxInputAudioBytes, proxy.DefaultMaxInputAudioBytes)
	}
}

func TestApplyTunablesSetsDefaultPromptPayloadLimit(testingInstance *testing.T) {
	configuration := proxy.Configuration{}
	configuration.ApplyTunables()
	if configuration.MaxPromptBytes != proxy.DefaultMaxPromptBytes {
		testingInstance.Fatalf("maxPromptBytes=%d want=%d", configuration.MaxPromptBytes, proxy.DefaultMaxPromptBytes)
	}
}
