package proxy_test

import (
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
)

const messageUnexpectedPollTimeout = "upstreamPollTimeoutSeconds=%d want=%d"

// TestApplyTunablesSetsDefaultUpstreamPollTimeout verifies the default poll timeout is applied.
func TestApplyTunablesSetsDefaultUpstreamPollTimeout(testingInstance *testing.T) {
	configuration := proxy.Configuration{}
	configuration.ApplyTunables()
	if configuration.UpstreamPollTimeoutSeconds != proxy.DefaultUpstreamPollTimeoutSeconds {
		testingInstance.Fatalf(messageUnexpectedPollTimeout, configuration.UpstreamPollTimeoutSeconds, proxy.DefaultUpstreamPollTimeoutSeconds)
	}
}

func TestApplyTunablesSetsDefaultDictationConfiguration(testingInstance *testing.T) {
	configuration := proxy.Configuration{}
	configuration.ApplyTunables()
	if configuration.DictationModel != proxy.DefaultDictationModel {
		testingInstance.Fatalf("dictationModel=%q want=%q", configuration.DictationModel, proxy.DefaultDictationModel)
	}
	if configuration.MaxInputAudioBytes != proxy.DefaultMaxInputAudioBytes {
		testingInstance.Fatalf("maxInputAudioBytes=%d want=%d", configuration.MaxInputAudioBytes, proxy.DefaultMaxInputAudioBytes)
	}
}
