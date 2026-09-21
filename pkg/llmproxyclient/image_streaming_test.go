package llmproxyclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestImageGenerationClientValidatesProgressiveAssetsOverHTTP(t *testing.T) {
	valid := MediaOperationPartialOutput{AssetID: "ast_0123456789abcdef0123456789abcdef", MIMEType: "image/png", SizeBytes: 100, OutputOrdinal: 0, PartialOrdinal: 0}
	for _, scenario := range []struct {
		name      string
		change    func(*MediaOperationPartialOutput)
		parent    string
		duplicate bool
		valid     bool
	}{
		{name: "valid", valid: true},
		{name: "valid parent", valid: true, parent: "mop_0123456789abcdef0123456789abcdef"},
		{name: "native parent", parent: "resp_native"},
		{name: "invalid asset", change: func(output *MediaOperationPartialOutput) { output.AssetID = "file_native" }},
		{name: "invalid MIME", change: func(output *MediaOperationPartialOutput) { output.MIMEType = "invalid" }},
		{name: "invalid bytes", change: func(output *MediaOperationPartialOutput) { output.SizeBytes = 0 }},
		{name: "invalid output order", change: func(output *MediaOperationPartialOutput) { output.OutputOrdinal = -1 }},
		{name: "invalid partial order", change: func(output *MediaOperationPartialOutput) { output.PartialOrdinal = -1 }},
		{name: "duplicate position", duplicate: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			partial := valid
			if scenario.change != nil {
				scenario.change(&partial)
			}
			partials := []MediaOperationPartialOutput{partial}
			if scenario.duplicate {
				partials = append(partials, partial)
			}
			var operation map[string]any
			if err := json.Unmarshal([]byte(validMediaOperationJSON("running")), &operation); err != nil {
				t.Fatal(err)
			}
			operation["partial_outputs"] = partials
			if scenario.parent != "" {
				operation["previous_operation_id"] = scenario.parent
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get("Authorization") != "Bearer preview-client" {
					t.Error("missing tenant authentication")
				}
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(operation)
			}))
			defer server.Close()
			config, err := NewConfig(ConfigInput{BaseURL: server.URL, Secret: "preview-client"})
			if err != nil {
				t.Fatal(err)
			}
			client, err := NewClient(config, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.GetMediaOperation(t.Context(), "mop_0123456789abcdef0123456789abcdef")
			if (err == nil) != scenario.valid {
				t.Fatalf("valid=%v result=%+v error=%v", scenario.valid, result, err)
			}
			if scenario.valid && (len(result.PartialOutputs) != 1 || result.PartialOutputs[0] != valid) {
				t.Fatalf("preview=%+v", result)
			}
		})
	}
}
