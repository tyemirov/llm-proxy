package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedModalityUsageRetainsProviderBreakdowns(t *testing.T) {
	for _, provider := range []string{"gemini", "vertex"} {
		for _, mode := range []string{"exact", "missing", "partial", "duplicate", "unknown_modality", "invalid_count", "exceeds_total", "cache_exceeds_modality", "zero", "wrong_array", "invalid_item", "missing_count", "default_text"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				database, _, read := newJournalTransactionFixture(t)
				var calls atomic.Int64
				fixture := hostedModalityResponse(t, provider, mode)
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.Method == http.MethodDelete {
						writer.WriteHeader(http.StatusNoContent)
						return
					}
					calls.Add(1)
					writer.Header().Set("Content-Type", "application/json")
					_, _ = writer.Write(fixture)
				}))
				t.Cleanup(upstream.Close)
				server := newHostedTextProviderFixture(t, database, upstream.URL, provider, "gemini-3.5-flash")
				for range 2 {
					request, err := http.NewRequest(http.MethodPost, server.URL+"/?provider="+provider+"&model=gemini-3.5-flash", strings.NewReader(`{"prompt":"private prompt"}`))
					if err != nil {
						t.Fatal(err)
					}
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "modality")
					response, err := server.Client().Do(request)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil || response.StatusCode != http.StatusOK {
						t.Fatalf("modality status=%d body=%s error=%v", response.StatusCode, body, err)
					}
					validateHostedIdentityResponse(t, request, response, body)
				}
				pending, err := database.pendingJournalDeliveries(t.Context(), 100)
				if err != nil || len(pending) != 1 || calls.Load() != 1 {
					t.Fatalf("modality observations=%v calls=%d error=%v", pending, calls.Load(), err)
				}
				var quantities []journalQuantity
				if err := json.Unmarshal(pending[0].Quantities, &quantities); err != nil {
					t.Fatal(err)
				}
				values := map[string]journalQuantity{}
				for _, quantity := range quantities {
					values[quantity.Dimension] = quantity
				}
				dimension, value, reason := "input_audio_tokens", "9007199254740993", journalUnknownReason("")
				switch mode {
				case "missing", "partial":
					dimension, value, reason = "input_modality_tokens", "", journalQuantityNotReported
				case "zero":
					dimension, value = "input_tokens", "0"
				case "default_text":
					if provider == "vertex" {
						dimension, value = "input_text_tokens", "10"
					} else {
						dimension, value, reason = "input_modality_tokens", "", journalQuantityUnsupported
					}
				case "wrong_array", "invalid_item":
					dimension, value, reason = "input_modality_tokens", "", journalQuantityInvalid
				case "missing_count":
					value, reason = "", journalQuantityNotReported
				case "unknown_modality":
					dimension, value, reason = "input_modality_tokens", "", journalQuantityUnsupported
				case "duplicate", "invalid_count", "exceeds_total":
					value, reason = "", journalQuantityInvalid
				case "cache_exceeds_modality":
					dimension, value, reason = "cache_read_text_tokens", "", journalQuantityInvalid
				}
				quantity := values[dimension]
				if quantity.Unit != "token" || quantity.Value != value || quantity.UnknownReason != reason {
					t.Fatalf("modality quantity=%+v want=%q reason=%s all=%s", quantity, value, reason, pending[0].Quantities)
				}
				if mode == "exact" {
					for dimension, parent := range map[string]string{"input_audio_tokens": "input_tokens", "cache_read_audio_tokens": "cache_read_tokens", "output_text_tokens": "output_tokens", "tool_input_text_tokens": "tool_input_tokens"} {
						if values[dimension].IncludedIn != parent {
							t.Fatalf("modality inclusion=%s", pending[0].Quantities)
						}
					}
					if !strings.Contains(string(pending[0].SourceFields), "9007199254740993") {
						t.Fatal("modality precision lost")
					}
				}
				usage := journalUsageUnknown
				if mode == "exact" || mode == "zero" || (mode == "default_text" && provider == "vertex") {
					usage = journalUsageComplete
				}
				entry := read("")["requests"].([]any)[0].(map[string]any)
				if entry["usage_state"] != string(usage) {
					t.Fatalf("modality completeness=%v", entry)
				}
				if strings.Contains(string(pending[0].SourceFields), "private") {
					t.Fatal("private modality content retained")
				}
			})
		}
	}
}

func hostedModalityResponse(t *testing.T, provider, mode string) []byte {
	t.Helper()
	countName := "tokens"
	modality := func(value string) string { return value }
	if provider == "vertex" {
		countName = "tokenCount"
		modality = strings.ToUpper
	}
	entry := func(kind string, value any) map[string]any {
		return map[string]any{"modality": modality(kind), countName: value}
	}
	input := []any{entry("text", 10), entry("audio", json.Number("9007199254740993")), entry("image", 2)}
	cache := []any{entry("text", 4), entry("audio", 6)}
	switch mode {
	case "default_text":
		delete(input[0].(map[string]any), "modality")
	case "invalid_item":
		input[1] = "private malformed item"
	case "missing_count":
		delete(input[1].(map[string]any), countName)
	case "partial":
		input = input[:1]
	case "duplicate":
		input = append(input, entry("audio", 1))
	case "unknown_modality":
		input[1] = entry("private_modality", json.Number("9007199254740993"))
	case "invalid_count":
		input[1] = entry("audio", "private invalid quantity")
	case "exceeds_total":
		input[1] = entry("audio", json.Number("9007199254740999"))
	case "cache_exceeds_modality":
		cache = []any{entry("text", 11)}
	}
	usage := map[string]any{"total_input_tokens": json.Number("9007199254741005"), "total_output_tokens": 2, "total_cached_tokens": 10, "total_thought_tokens": 0, "total_tool_use_tokens": 3, "input_tokens_by_modality": input, "cached_tokens_by_modality": cache, "output_tokens_by_modality": []any{entry("text", 2)}, "tool_use_tokens_by_modality": []any{entry("text", 3)}}
	if mode == "cache_exceeds_modality" {
		usage["total_cached_tokens"] = 11
	}
	if mode == "wrong_array" {
		usage["input_tokens_by_modality"] = map[string]any{"private": "invalid array"}
	}
	if mode == "zero" {
		for _, key := range []string{"total_input_tokens", "total_output_tokens", "total_cached_tokens", "total_thought_tokens", "total_tool_use_tokens"} {
			usage[key] = 0
		}
		for _, key := range []string{"input_tokens_by_modality", "cached_tokens_by_modality", "output_tokens_by_modality", "tool_use_tokens_by_modality"} {
			delete(usage, key)
		}
	}
	if mode == "missing" {
		delete(usage, "input_tokens_by_modality")
	}
	var body any
	if provider == "vertex" {
		fields := map[string]string{"total_input_tokens": "promptTokenCount", "total_output_tokens": "candidatesTokenCount", "total_cached_tokens": "cachedContentTokenCount", "total_thought_tokens": "thoughtsTokenCount", "total_tool_use_tokens": "toolUsePromptTokenCount", "input_tokens_by_modality": "promptTokensDetails", "cached_tokens_by_modality": "cacheTokensDetails", "output_tokens_by_modality": "candidatesTokensDetails", "tool_use_tokens_by_modality": "toolUsePromptTokensDetails"}
		native := map[string]any{}
		for key, value := range usage {
			native[fields[key]] = value
		}
		body = map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "retained answer"}}}, "finishReason": "STOP"}}, "usageMetadata": native}
	} else {
		body = map[string]any{"id": "private-modality", "status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]any{"type": "text", "text": "retained answer"}}}}, "usage": usage}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
