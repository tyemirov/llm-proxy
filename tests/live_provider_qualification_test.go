package tests_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOperationalLiveQualification(t *testing.T) {
	for _, scenario := range []struct {
		name, headers, answer string
		media, accepted       bool
	}{
		{name: "missing"},
		{name: "zero", headers: "X-LLM-Proxy-Request-Tokens: 0\nX-LLM-Proxy-Response-Tokens: 2\nX-LLM-Proxy-Total-Tokens: 2"},
		{name: "inconsistent", headers: "X-LLM-Proxy-Request-Tokens: 3\nX-LLM-Proxy-Response-Tokens: 2\nX-LLM-Proxy-Total-Tokens: 9"},
		{name: "duplicate", headers: "X-LLM-Proxy-Request-Tokens: 3\nX-LLM-Proxy-Request-Tokens: 3\nX-LLM-Proxy-Response-Tokens: 2\nX-LLM-Proxy-Total-Tokens: 5"},
		{name: "invalid", headers: "X-LLM-Proxy-Request-Tokens: unknown\nX-LLM-Proxy-Response-Tokens: 2\nX-LLM-Proxy-Total-Tokens: 5"},
		{name: "missing_request_id", headers: qualificationUsageHeaders},
		{name: "duplicate_request_id", headers: qualificationUsageHeaders + "\nX-LLM-Proxy-Request-ID: ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"},
		{name: "text", headers: qualificationUsageHeaders, accepted: true},
		{name: "images", headers: qualificationUsageHeaders, answer: "RED BLUE", media: true, accepted: true},
		{name: "wrong_image_order", headers: qualificationUsageHeaders, answer: "BLUE RED", media: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newOperationalLiveHarnessFixture(t)
			curlPath := filepath.Join(fixture.toolDirectory, "curl")
			source, err := os.ReadFile(curlPath)
			if err != nil {
				t.Fatal(err)
			}
			script := strings.ReplaceAll(string(source), "'X-LLM-Proxy-Request-ID: ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'", "'X-LLM-Proxy-Request-ID: ABCDEFGHIJKLMNOPQRSTUVWXYZ234567' '"+scenario.headers+"'")
			if scenario.name == "missing_request_id" {
				script = strings.ReplaceAll(script, "'X-LLM-Proxy-Request-ID: ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'", "''")
			}
			script = strings.ReplaceAll(script, "builtin printf '%s' OK >", "write_response_headers\n      builtin printf '%s' OK >")
			if scenario.media {
				script = strings.ReplaceAll(script, "builtin printf '%s' RED >", "builtin printf '%s' '"+scenario.answer+"' >")
			}
			curlFile, err := os.Create(curlPath)
			if err != nil {
				t.Fatal(err)
			}
			_, writeError := curlFile.WriteString(script)
			closeError := curlFile.Close()
			if writeError != nil {
				t.Fatal(writeError)
			}
			if closeError != nil {
				t.Fatal(closeError)
			}
			capture := filepath.Join(t.TempDir(), "operations")
			command := exec.Command(filepath.Join(operationalRepositoryRoot(t), "scripts/test_live_providers.sh"))
			if scenario.media {
				command.Args = append(command.Args, "--media")
			}
			command.Env = []string{"PATH=" + fixture.toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"), "GO=" + filepath.Join(fixture.toolDirectory, "go"), "PROXY_PID_CAPTURE=" + fixture.proxyPIDPath, "LLM_PROXY_LIVE_PORT=" + strconv.Itoa(operationalLoopbackPort(t)), "OPENAI_API_KEY=fixture-key", "LLM_PROXY_LIVE_PROVIDERS=openai", "LLM_PROXY_LIVE_QUALIFICATION=true", "LIVE_OPERATION_CAPTURE=" + capture}
			output, err := command.CombinedOutput()
			if (err == nil) != scenario.accepted {
				t.Fatalf("accepted=%v err=%v output=%s", scenario.accepted, err, output)
			}
			if !scenario.accepted {
				return
			}
			var evidence map[string]any
			for _, line := range strings.Split(string(output), "\n") {
				if strings.HasPrefix(line, "{\"event\":\"live_qualification\"") {
					if err := json.Unmarshal([]byte(line), &evidence); err != nil {
						t.Fatal(err)
					}
				}
			}
			if evidence["request_tokens"] != float64(3) || evidence["response_tokens"] != float64(2) || evidence["total_tokens"] != float64(5) || evidence["request_id"] != "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567" {
				t.Fatalf("missing safe evidence: %s", output)
			}
			elapsed, ok := evidence["elapsed_ms"].(float64)
			if !ok || elapsed < 0 {
				t.Fatalf("invalid duration: %v", evidence)
			}
			operations, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			prefix := "smoke-payload "
			if scenario.media {
				prefix = "image-payload "
			}
			observedPayloads := 0
			for _, line := range strings.Split(string(operations), "\n") {
				if !strings.HasPrefix(line, prefix) {
					continue
				}
				observedPayloads++
				var payload struct {
					MaxTokens int `json:"max_tokens"`
					Messages  []struct {
						Attachments []struct {
							MIME string `json:"mime_type"`
							Data string `json:"data"`
						} `json:"attachments"`
					} `json:"messages"`
				}
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.MaxTokens != 4096 {
					t.Fatalf("max_tokens=%d", payload.MaxTokens)
				}
				if !scenario.media {
					continue
				}
				attachments := payload.Messages[0].Attachments
				if len(attachments) != 2 {
					t.Fatalf("attachments=%d", len(attachments))
				}
				for index, attachment := range attachments {
					expected := []string{"png", "jpeg"}[index]
					if attachment.MIME != "image/"+expected {
						t.Fatalf("attachment %d MIME=%s", index, attachment.MIME)
					}
					data, err := base64.StdEncoding.DecodeString(attachment.Data)
					if err != nil {
						t.Fatal(err)
					}
					picture, format, err := image.Decode(bytes.NewReader(data))
					if err != nil || format != expected {
						t.Fatalf("decode image %d format=%s err=%v", index, format, err)
					}
					red, green, blue, _ := picture.At(128, 128).RGBA()
					if green > 1000 || (index == 0 && (red < 60000 || blue > 1000)) || (index == 1 && (blue < 60000 || red > 1000)) {
						t.Fatalf("image %d color=%d,%d,%d", index, red, green, blue)
					}
				}
			}
			if observedPayloads != 1 {
				t.Fatalf("expected one %s operation, got %d", prefix, observedPayloads)
			}
		})
	}
}

const qualificationUsageHeaders = "X-LLM-Proxy-Request-Tokens: 3\nX-LLM-Proxy-Response-Tokens: 2\nX-LLM-Proxy-Total-Tokens: 5"

func TestOperationalLiveQualificationRejectsInvalidBoolean(t *testing.T) {
	for _, value := range []string{"", "TRUE", "1"} {
		t.Run("value_"+value, func(t *testing.T) {
			command := exec.Command(filepath.Join(operationalRepositoryRoot(t), "scripts/test_live_providers.sh"), "--write-config", filepath.Join(t.TempDir(), "config.yml"))
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "LLM_PROXY_LIVE_QUALIFICATION=" + value}
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "LLM_PROXY_LIVE_QUALIFICATION must be true or false") {
				t.Fatalf("invalid boolean accepted: %v %s", err, output)
			}
		})
	}
}
