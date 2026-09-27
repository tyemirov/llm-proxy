package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostedDictationFileUploadAuthorityAndCleanup(t *testing.T) {
	audio := bytes.Repeat([]byte("a"), 15_000_000)
	digest := sha256.Sum256(audio)
	for _, scenario := range []struct {
		name                                  string
		status, uploads, generations, deletes int
	}{
		{"complete", http.StatusOK, 1, 1, 1},
		{"revoked after start", http.StatusForbidden, 0, 0, 0},
		{"claim expired after start", http.StatusConflict, 0, 0, 0},
		{"revoked after upload", http.StatusForbidden, 1, 0, 1},
		{"cleanup failure", http.StatusBadGateway, 1, 1, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			var starts, uploads, generations, deletes atomic.Int64
			revoke := func() {
				if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-dictation").Update("state", hostedGrantRevoked).Error; err != nil {
					t.Error(err)
				}
			}
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get("x-goog-api-key") != "sk-platform-dictation" {
					t.Error("missing pinned upload credential")
				}
				switch request.URL.Path {
				case "/upload/v1beta/files":
					starts.Add(1)
					if request.Method != http.MethodPost || request.Header.Get("X-Goog-Upload-Header-Content-Length") != strconv.Itoa(len(audio)) {
						t.Error("incorrect upload start")
					}
					if scenario.name == "revoked after start" {
						revoke()
					}
					if scenario.name == "claim expired after start" && starts.Load() == 1 {
						if err := database.database.Model(&managedJournalRequestRecord{}).Where("tenant_id = ?", "managed-first").Update("claim_expires_at", time.Now().Add(-time.Second)).Error; err != nil {
							t.Error(err)
						}
					}
					writer.Header().Set("X-Goog-Upload-URL", upstream.URL+"/upload-session")
				case "/upload-session":
					uploads.Add(1)
					data, err := io.ReadAll(request.Body)
					if err != nil || !bytes.Equal(data, audio) {
						t.Error("audio changed during upload")
					}
					if scenario.name == "revoked after upload" {
						revoke()
					}
					writer.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(writer).Encode(map[string]any{"file": map[string]any{"name": "files/private-audio", "uri": upstream.URL + "/files/private-audio", "mimeType": "audio/wav", "sizeBytes": strconv.Itoa(len(audio)), "sha256Hash": base64.StdEncoding.EncodeToString(digest[:]), "state": "ACTIVE"}}); err != nil {
						t.Error(err)
					}
				case "/interactions":
					generations.Add(1)
					var payload struct {
						Input []struct {
							URI  string `json:"uri"`
							Data string `json:"data"`
						} `json:"input"`
					}
					if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || len(payload.Input) != 1 || payload.Input[0].URI != upstream.URL+"/files/private-audio" || payload.Input[0].Data != "" {
						t.Error("transcription does not use the accepted upload")
					}
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{"id":"private-upload-result","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"uploaded transcript"}]}],"usage":{"total_input_tokens":10,"total_output_tokens":3,"total_tokens":13,"total_cached_tokens":0,"total_thought_tokens":0,"total_tool_use_tokens":0,"input_tokens_by_modality":[{"modality":"audio","tokens":10}],"output_tokens_by_modality":[{"modality":"text","tokens":3}]}}`)
				case "/files/private-audio":
					if request.Method != http.MethodDelete {
						t.Error("unexpected file observation")
						writer.WriteHeader(http.StatusBadRequest)
						return
					}
					deletes.Add(1)
					if scenario.name == "cleanup failure" {
						writer.WriteHeader(http.StatusBadGateway)
						return
					}
					writer.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected file request %s", request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(upstream.Close)
			server := newHostedDictationProviderServer(t, database, upstream.URL, t.TempDir(), "gemini", "gemini-3.5-transcribe")
			body := hostedDictationProviderHTTP(t, server, dictatePath, "uploaded-audio", string(audio), "gemini", "gemini-3.5-transcribe", scenario.status)
			if starts.Load() != 1 || uploads.Load() != int64(scenario.uploads) || generations.Load() != int64(scenario.generations) || deletes.Load() != int64(scenario.deletes) {
				t.Fatalf("file effects: starts=%d uploads=%d generations=%d deletes=%d", starts.Load(), uploads.Load(), generations.Load(), deletes.Load())
			}
			if scenario.status != http.StatusOK && strings.Contains(body, "uploaded transcript") {
				t.Fatal("failed cleanup published transcript")
			}
			request := read("")["requests"].([]any)[0].(map[string]any)
			var observations int64
			if err := database.database.Model(&managedJournalObservationRecord{}).Count(&observations).Error; err != nil {
				t.Fatal(err)
			}
			if observations != int64(scenario.generations) {
				t.Fatalf("file operations created usage observations: %d", observations)
			}
			if scenario.generations == 1 && request["usage_state"] != "complete" {
				t.Fatalf("cleanup discarded usage: %v", request)
			}
			status := http.StatusBadGateway
			if scenario.status == http.StatusOK {
				status = http.StatusOK
			}
			if scenario.name == "claim expired after start" {
				status = http.StatusOK
			}
			hostedDictationProviderHTTP(t, server, transcriptionsPath, "uploaded-audio", string(audio), "gemini", "gemini-3.5-transcribe", status)
			wantStarts, wantGenerations := int64(1), int64(scenario.generations)
			if scenario.name == "claim expired after start" {
				wantStarts, wantGenerations = 2, 1
			}
			if starts.Load() != wantStarts || generations.Load() != wantGenerations {
				t.Fatal("replay repeated upload or generation")
			}
			if current := read("/" + request["id"].(string)); current["execution_id"] != request["execution_id"] {
				t.Fatal("replay replaced the accepted execution")
			}
		})
	}
}
