package main

import (
	"strings"
	"testing"
)

func TestRootCommandRejectsInvalidMediaOperationConfiguration(testingInstance *testing.T) {
	for _, testCase := range []struct {
		name          string
		serverYAML    string
		expectedError string
	}{
		{name: "zero workers", serverYAML: "  media_operation_workers: 0\n", expectedError: "server.media_operation_workers must be positive"},
		{name: "zero capacity", serverYAML: "  media_operation_capacity: 0\n", expectedError: "server.media_operation_capacity must be positive"},
		{name: "zero tenant capacity", serverYAML: "  tenant_media_operation_capacity: 0\n", expectedError: "server.tenant_media_operation_capacity must be positive"},
		{name: "zero lifetime", serverYAML: "  media_operation_lifetime_seconds: 0\n", expectedError: "server.media_operation_lifetime_seconds must be positive"},
		{name: "zero claim", serverYAML: "  media_operation_claim_seconds: 0\n", expectedError: "server.media_operation_claim_seconds must be positive"},
		{name: "zero claim renewal", serverYAML: "  media_operation_claim_renewal_seconds: 0\n", expectedError: "server.media_operation_claim_renewal_seconds must be positive"},
	} {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			configPath := writeTestConfig(testingInstance, testingInstance.TempDir(), "\nserver:\n"+testCase.serverYAML+completeLiteralRuntimeYAML())
			withServeProxy(testingInstance, failingServeProxy(testingInstance))
			executionError := executeRootCommand(testingInstance, "--config", configPath)
			if executionError == nil || !strings.Contains(executionError.Error(), testCase.expectedError) {
				testingInstance.Fatalf("error=%v want contains %q", executionError, testCase.expectedError)
			}
		})
	}
}
