package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: os.Getenv("LLM_PROXY_BASE_URL"), Secret: os.Getenv("LLM_PROXY_TENANT_KEY")})
	if err != nil {
		return err
	}
	client, err := llmproxyclient.NewClient(configuration, &http.Client{})
	if err != nil {
		return err
	}
	var input llmproxyclient.ImageGenerationInput
	decoder := json.NewDecoder(strings.NewReader(os.Getenv("IMAGE_GENERATION_INPUT_JSON")))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return fmt.Errorf("read image intent: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("read image intent: trailing JSON")
	}
	outputDirectory := os.Getenv("IMAGE_OUTPUT_DIRECTORY")
	if outputDirectory == "" {
		return fmt.Errorf("IMAGE_OUTPUT_DIRECTORY is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	operation, err := client.CreateImageGeneration(ctx, os.Getenv("MEDIA_OPERATION_IDEMPOTENCY_KEY"), input)
	if err != nil {
		return err
	}
	fmt.Printf("operation=%s\n", operation.OperationID)
	operation, err = client.WaitMediaOperation(ctx, operation.OperationID, time.Second)
	if err != nil {
		return fmt.Errorf("wait for %s: %w", operation.OperationID, err)
	}
	if operation.State != llmproxycontract.MediaOperationStateSucceeded {
		return fmt.Errorf("operation %s ended in state %s", operation.OperationID, operation.State)
	}
	for _, output := range operation.Outputs {
		asset, err := client.GetAsset(ctx, output.AssetID)
		if err != nil {
			return err
		}
		data, err := client.DownloadAsset(ctx, asset)
		if err != nil {
			return err
		}
		filename := filepath.Join(outputDirectory, output.AssetID+"."+input.OutputFormat)
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("create image %s: %w", filename, err)
		}
		_, writeError := file.Write(data)
		closeError := file.Close()
		if writeError != nil {
			return fmt.Errorf("write image %s: %w", filename, writeError)
		}
		if closeError != nil {
			return fmt.Errorf("close image %s: %w", filename, closeError)
		}
		fmt.Printf("ordinal=%d asset=%s mime=%s bytes=%d path=%s\n", output.Ordinal, asset.AssetID, asset.MIMEType, len(data), filename)
	}
	return nil
}
