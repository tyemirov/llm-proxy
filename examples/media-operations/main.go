package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func main() {
	if runError := run(); runError != nil {
		log.Fatal(runError)
	}
}

func run() error {
	configuration, configurationError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: requiredEnvironment("LLM_PROXY_BASE_URL"),
		Secret:  requiredEnvironment("LLM_PROXY_TENANT_KEY"),
	})
	if configurationError != nil {
		return configurationError
	}
	client, clientError := llmproxyclient.NewClient(configuration, &http.Client{})
	if clientError != nil {
		return clientError
	}

	operationContext, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	capabilities, capabilitiesError := client.GetMediaCapabilities(operationContext)
	if capabilitiesError != nil {
		return capabilitiesError
	}
	operation, creationError := client.CreateMediaOperation(operationContext, requiredEnvironment("MEDIA_OPERATION_IDEMPOTENCY_KEY"), llmproxyclient.MediaOperationInput{
		Capability: requiredEnvironment("MEDIA_OPERATION_CAPABILITY"),
		Provider:   requiredEnvironment("MEDIA_OPERATION_PROVIDER"),
		Model:      requiredEnvironment("MEDIA_OPERATION_MODEL"),
		Input:      json.RawMessage(requiredEnvironment("MEDIA_OPERATION_INPUT_JSON")),
		Controls:   json.RawMessage(requiredEnvironment("MEDIA_OPERATION_CONTROLS_JSON")),
	})
	if creationError != nil {
		return creationError
	}
	operation, waitError := client.WaitMediaOperation(operationContext, operation.OperationID, time.Second)
	if waitError != nil {
		return fmt.Errorf("wait for %s: %w", operation.OperationID, waitError)
	}
	if operation.State != llmproxycontract.MediaOperationStateSucceeded {
		return fmt.Errorf("operation %s ended in state %s", operation.OperationID, operation.State)
	}
	for _, output := range operation.Outputs {
		asset, assetError := client.GetAsset(operationContext, output.AssetID)
		if assetError != nil {
			return assetError
		}
		data, downloadError := client.DownloadAsset(operationContext, asset)
		if downloadError != nil {
			return downloadError
		}
		digest := sha256.Sum256(data)
		fmt.Printf("operation=%s catalog=%s asset=%s mime=%s bytes=%d sha256=%s\n", operation.OperationID, capabilities.CatalogRevision, asset.AssetID, asset.MIMEType, len(data), hex.EncodeToString(digest[:]))
	}
	return nil
}

func requiredEnvironment(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("missing %s", name)
	}
	return value
}
