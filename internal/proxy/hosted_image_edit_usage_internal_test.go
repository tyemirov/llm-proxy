package proxy

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedImageEditingRetainsUsageOnBothSurfaces(t *testing.T) {
	png := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))
	encoded := base64.StdEncoding.EncodeToString(png)
	for _, surface := range []string{"images", "responses"} {
		t.Run(surface, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
					t.Error("incorrect edit authority")
				}
				writer.Header().Set("Content-Type", "application/json")
				if surface == "images" {
					if err := request.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						return
					}
					defer request.MultipartForm.RemoveAll()
					file, _, err := request.FormFile("image[]")
					if err != nil {
						t.Error(err)
						return
					}
					content, err := io.ReadAll(file)
					file.Close()
					if err != nil || !bytes.Equal(content, png) {
						t.Error("editing source changed")
					}
					fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":{"total_tokens":13,"input_tokens":10,"output_tokens":3,"input_tokens_details":{"text_tokens":4,"image_tokens":6},"output_tokens_details":{"text_tokens":0,"image_tokens":3}}}`, encoded)
				} else {
					content, err := io.ReadAll(request.Body)
					if err != nil || !strings.Contains(string(content), "data:image/png;base64,"+encoded) {
						t.Error("Responses editing source changed")
					}
					fmt.Fprintf(writer, `{"id":"private-edit","status":"completed","output":[{"id":"private-image","type":"image_generation_call","status":"completed","result":%q}],"usage":{"total_tokens":13,"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`, encoded)
				}
			}))
			t.Cleanup(upstream.Close)
			server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
			if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("offerings", []byte(`[{"model":"gpt-image-2","operations":["image_editing"]}]`)).Error; err != nil {
				t.Fatal(err)
			}
			service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageEdit, "openai", "gpt-image-2")] = service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")]
			asset, err := service.assets.upload(tenant{identifier: tenantID("managed-first")}, "image/png", bytes.NewReader(png))
			if err != nil {
				t.Fatal(err)
			}
			extra := ""
			if surface == "responses" {
				extra = `,"responses_model":"gpt-5"`
			}
			intent := fmt.Sprintf(`{"capability":"image.edit","provider":"openai","model":"gpt-image-2","input":{"prompt":"Private edit","image_asset_ids":[%q]},"controls":{"surface":%q%s,"quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}}`, asset.AssetID, surface, extra)
			id := hostedSpeechHTTP(t, server, "edit", intent, http.StatusAccepted)["operation_id"].(string)
			service.runOperation("edit-worker", id)
			if result := hostedMediaWorkerStatus(t, server, id); result["state"] != MediaOperationStateSucceeded {
				t.Fatalf("edit result=%v", result)
			}
			hostedSpeechHTTP(t, server, "edit", intent, http.StatusOK)
			service.runOperation("duplicate-worker", id)
			pending, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil || len(pending) != 1 || calls.Load() != 1 {
				t.Fatalf("edit evidence=%v calls=%d error=%v", pending, calls.Load(), err)
			}
			dimension := "input_tokens"
			if surface == "responses" {
				dimension = "responses_input_tokens"
			}
			if !strings.Contains(string(pending[0].Quantities), `"dimension":"`+dimension+`","unit":"token","value":"10"`) {
				t.Fatalf("edit quantities=%s", pending[0].Quantities)
			}
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if entry["operation"] != ModelOperationImageEditing || entry["usage_state"] != string(journalUsageUnknown) {
				t.Fatalf("edit attribution=%v", entry)
			}
		})
	}
}
