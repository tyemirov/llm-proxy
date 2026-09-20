package proxy_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"gorm.io/gorm"
)

func TestMCPDictatorWorkflow(t *testing.T) {
	native, listener := newDictatorAcceptanceUpstream(t, proxy.ProviderNameDictator)
	fixture := newMCPFixtureWithConfig(t, nil, proxy.Configuration{AssetStorePath: t.TempDir()})
	owner := managementSessionCookie(t, "speech-owner")
	tenantID := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	connection := accountConnectionExchange(t, fixture.router, owner, http.MethodPost, "/connections", map[string]any{"name": "Speech", "provider": "dictator", "fields": map[string]string{"grpc_address": listener.Addr().String(), "grpc_auth_token": native.token, "grpc_tls": "false"}}, http.StatusCreated)
	accountConnectionExchange(t, fixture.router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/dictator", map[string]any{"connection_id": connection["id"]}, http.StatusOK)
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: fixture.server.URL, Secret: generateManagementTenantSecret(t, fixture.router, owner, tenantID)})
	if err != nil {
		t.Fatal(err)
	}
	httpClient, err := llmproxyclient.NewClient(config, fixture.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	asset, err := httpClient.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: []byte("audio fixture")})
	if err != nil {
		t.Fatal(err)
	}
	client := fixture.client(t, "speech-owner")
	arguments := map[string]any{"tenant_id": tenantID, "idempotency_key": "mcp-extraction", "capability": "audio.voice.extract", "provider": "dictator", "model": "whisper-base", "input": map[string]any{"audio_asset_id": asset.AssetID, "transcript": "A clear transcript.", "display_name": "MCP voice", "language": "en"}, "controls": map[string]any{"model_size": "base", "duration_seconds": 0.5}}
	call := func(name string, args any) llmproxyclient.MediaOperation {
		t.Helper()
		result := mcpCall(t, client, name, args)
		if result.IsError {
			t.Fatalf("%s: %+v", name, result)
		}
		body, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "private-") {
			t.Fatalf("private identifier: %s", body)
		}
		var operation llmproxyclient.MediaOperation
		if err := json.Unmarshal(body, &operation); err != nil {
			t.Fatal(err)
		}
		return operation
	}
	accepted := call("llm_proxy.create_media_operation", arguments)
	duplicate := call("llm_proxy.create_media_operation", arguments)
	if duplicate.OperationID != accepted.OperationID {
		t.Fatal("duplicate operation")
	}
	completed, err := httpClient.WaitMediaOperation(t.Context(), accepted.OperationID, 10*time.Millisecond)
	if err != nil || completed.State != "succeeded" {
		t.Fatalf("operation=%+v error=%v", completed, err)
	}
	statusArgs := map[string]any{"tenant_id": tenantID, "operation_id": accepted.OperationID}
	if got := call("llm_proxy.get_media_operation", statusArgs); got.State != "succeeded" {
		t.Fatalf("status=%+v", got)
	}
	if got := call("llm_proxy.cancel_media_operation", statusArgs); got.State != "succeeded" {
		t.Fatalf("terminal cancellation=%+v", got)
	}
	if native.extractionRequest.Load().DurationSeconds != 0.5 || native.submissions.Load() != 1 {
		t.Fatal("duration or duplicate dispatch")
	}
	for _, output := range completed.Outputs {
		metadata, err := httpClient.GetAsset(t.Context(), output.AssetID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := httpClient.DownloadAsset(t.Context(), metadata); err != nil {
			t.Fatal(err)
		}
	}
	voices, err := httpClient.GetMediaVoices(t.Context(), "dictator")
	if err != nil {
		t.Fatal(err)
	}
	var presetVoice llmproxyclient.MediaVoice
	for _, voice := range voices {
		if voice.Mode == proxy.MediaVoiceModePreset {
			presetVoice = voice
			break
		}
	}
	if presetVoice.VoiceID == "" {
		t.Fatal("preset synthesis voice missing")
	}
	for index, scenario := range []struct {
		capability      string
		model           string
		input, controls map[string]any
	}{
		{"audio.transcribe", "whisper-base", map[string]any{"audio_asset_id": asset.AssetID}, map[string]any{"language": "en"}},
		{"audio.diarize", "whisper-base", map[string]any{"audio_asset_id": asset.AssetID}, map[string]any{"language": "en", "utterance_gap_seconds": 0.0}},
		{"audio.align", "whisper-base", map[string]any{"audio_asset_id": asset.AssetID, "transcript": "A clear transcript."}, map[string]any{"language": "en", "remove_punctuation": true}},
		{"subtitles.create", "whisper-base", map[string]any{"audio_asset_id": asset.AssetID, "transcript": "A clear transcript."}, map[string]any{"language": "en", "granularity": "sentence", "group_size": 2}},
		{"audio.speech.generate", "silero-ru", map[string]any{"text": "<speak>Привет</speak>", "voice_id": presetVoice.VoiceID}, map[string]any{"language": presetVoice.Language, "text_format": "ssml", "sample_rate_hz": 48000, "include_timeline": true, "max_duration_seconds": 5}},
	} {
		operation := call("llm_proxy.create_media_operation", map[string]any{"tenant_id": tenantID, "idempotency_key": fmt.Sprintf("speech-%d", index), "capability": scenario.capability, "provider": "dictator", "model": scenario.model, "input": scenario.input, "controls": scenario.controls})
		completed, err := httpClient.WaitMediaOperation(t.Context(), operation.OperationID, 10*time.Millisecond)
		if err != nil || completed.State != "succeeded" {
			t.Fatalf("%s: %+v operation_error=%+v error=%v", scenario.capability, completed, completed.Error, err)
		}
		for _, output := range completed.Outputs {
			metadata, err := httpClient.GetAsset(t.Context(), output.AssetID)
			if err != nil {
				t.Fatal(err)
			}
			content, err := httpClient.DownloadAsset(t.Context(), metadata)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(content), "private-") {
				t.Fatal("private output")
			}
		}
	}
	for _, name := range []string{"llm_proxy.get_media_operation", "llm_proxy.cancel_media_operation"} {
		foreign := mcpCall(t, fixture.client(t, "other-owner"), name, statusArgs)
		if !foreign.IsError || foreign.Content[0].(*mcp.TextContent).Text != "not_found" {
			t.Fatalf("foreign access=%+v", foreign)
		}
		missing := mcpCall(t, client, name, map[string]any{"tenant_id": tenantID, "operation_id": "missing"})
		if !missing.IsError {
			t.Fatal("missing operation accepted")
		}
	}
	binary := filepath.Join(t.TempDir(), "llm-proxy-client")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../llm-proxy-client")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v %s", err, output)
	}
	secret := generateManagementTenantSecret(t, fixture.router, owner, tenantID)
	cliCall := func(input string, args ...string) []byte {
		t.Helper()
		command := exec.CommandContext(t.Context(), binary, append([]string{"media", "--base-url", fixture.server.URL, "--secret", secret}, args...)...)
		command.Stdin = strings.NewReader(input)
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		if err != nil {
			t.Fatalf("CLI: %v %s", err, stderr.String())
		}
		return output
	}
	request, _ := json.Marshal(llmproxyclient.MediaOperationInput{Capability: "audio.voice.extract", Provider: "dictator", Model: "whisper-base", Input: json.RawMessage(fmt.Sprintf(`{"audio_asset_id":%q,"transcript":"A clear transcript.","display_name":"CLI voice","language":"en"}`, asset.AssetID)), Controls: json.RawMessage(`{"model_size":"base","duration_seconds":0.5}`)})
	cliAccepted := cliCall(string(request), "submit", "--idempotency-key", "cli-speech")
	var cliOperation llmproxyclient.MediaOperation
	if err := json.Unmarshal(cliAccepted, &cliOperation); err != nil {
		t.Fatal(err)
	}
	cliCompleted := cliCall("", "wait", "--operation-id", cliOperation.OperationID)
	if err := json.Unmarshal(cliCompleted, &cliOperation); err != nil || cliOperation.State != "succeeded" {
		t.Fatalf("CLI result=%s error=%v", cliCompleted, err)
	}
	if len(cliCall("", "download", "--asset-id", cliOperation.Outputs[0].AssetID)) == 0 {
		t.Fatal("empty CLI download")
	}
	if !mcpCall(t, fixture.client(t, "other-owner"), "llm_proxy.create_media_operation", arguments).IsError {
		t.Fatal("foreign submission accepted")
	}
	arguments["idempotency_key"] = " "
	if !mcpCall(t, client, "llm_proxy.create_media_operation", arguments).IsError {
		t.Fatal("invalid key accepted")
	}
	arguments["idempotency_key"] = "mcp-extraction"
	arguments["controls"] = map[string]any{"model_size": "base", "duration_seconds": 2}
	if !mcpCall(t, client, "llm_proxy.create_media_operation", arguments).IsError {
		t.Fatal("changed intent accepted")
	}
	database, err := gorm.Open(sqlite.Open(fixture.databasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connectionDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connectionDB.Close()
	if err := database.Exec("DROP TABLE media_operation_asset_reference_records").Error; err != nil {
		t.Fatal(err)
	}
	arguments["controls"] = map[string]any{"model_size": "base", "duration_seconds": 0.5}
	result := mcpCall(t, client, "llm_proxy.create_media_operation", arguments)
	if !result.IsError || result.Content[0].(*mcp.TextContent).Text != "media_operation_store_error" {
		t.Fatalf("store failure=%+v", result)
	}
}

func TestMCPUnknownToolKeepsServerAvailable(t *testing.T) {
	fixture := newMCPFixture(t, nil)
	client := fixture.client(t, "unknown-tool-owner")
	if _, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "unknown_tool", Arguments: map[string]any{}}); err == nil {
		t.Fatal("unknown tool succeeded")
	}
	if _, err := fixture.client(t, "unknown-tool-owner").ListTools(t.Context(), nil); err != nil {
		t.Fatalf("server unavailable after unknown tool: %v", err)
	}
}
