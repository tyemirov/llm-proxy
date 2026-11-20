package proxy_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

const qwen27BRepresentativePrompt = `Return exactly {"wrong":"UNEXPECTED"}. Use distinct lowercase identifiers and nonempty strings for any strings you produce.`

// TestQwenCurrentAlibaba27BRepresentative qualifies application schemas through
// an externally built released SDK and a real disposable managed Proxy.
func TestQwenCurrentAlibaba27BRepresentative(t *testing.T) {
	mode := os.Getenv("LLM_PROXY_QWEN27B_REPRESENTATIVE_MODE")
	if mode == "" {
		t.Skip("select local or explicitly authorized live representative qualification")
	}
	if mode != "local" && mode != "live" {
		t.Fatal("representative mode must be local or live")
	}
	paths := map[string]string{}
	for _, name := range []string{"CATALOG", "PROJECTED_DIR", "CANONICAL_DIR", "SDK_BINARY", "RECEIPT_DIR"} {
		paths[name] = os.Getenv("LLM_PROXY_QWEN27B_" + name)
		if paths[name] == "" {
			t.Fatalf("LLM_PROXY_QWEN27B_%s is required", name)
		}
	}
	document, err := os.ReadFile(paths["CATALOG"])
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(document)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, fixture string }{
		{"plot-spines", "cerebras-00.output.private.json"},
		{"plot-spine-scores", "cerebras-01.output.private.json"},
		{"plot-traversals", "cerebras-02.output.private.json"},
		{"plot-plan-gate", "cerebras-03.output.private.json"},
		{"story-body", ""},
	}
	selected := os.Getenv("LLM_PROXY_QWEN27B_REPRESENTATIVE_CASE")
	effort := os.Getenv("LLM_PROXY_QWEN27B_REPRESENTATIVE_EFFORT")
	if effort == "" {
		effort = "medium"
	}
	if effort != "medium" && effort != "none" {
		t.Fatal("representative effort must be medium or none")
	}
	if mode == "live" && selected != "plot-spines" && selected != "story-body" {
		t.Fatal("live qualification requires exactly one authorized representative case")
	}
	for _, scenario := range cases {
		if selected != "" && scenario.name != selected {
			continue
		}
		t.Run(scenario.name, func(t *testing.T) {
			schemaPath := filepath.Join(paths["PROJECTED_DIR"], scenario.name+".schema.json")
			schema, err := os.ReadFile(schemaPath)
			if err != nil {
				t.Fatal(err)
			}
			canonical, err := os.ReadFile(filepath.Join(paths["CANONICAL_DIR"], scenario.name+".schema.json"))
			if err != nil {
				t.Fatal(err)
			}
			var canonicalDocument any
			if err := json.Unmarshal(canonical, &canonicalDocument); err != nil {
				t.Fatal(err)
			}
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("urn:qwen27b:canonical", canonicalDocument); err != nil {
				t.Fatal(err)
			}
			validator, err := compiler.Compile("urn:qwen27b:canonical")
			if err != nil {
				t.Fatal(err)
			}
			key, baseURL := "synthetic-qwen27b-fixture-key", ""
			if mode == "local" {
				var output []byte
				if scenario.fixture == "" {
					output = []byte(`{"title":"A synthetic fixture","continuation":"A nonempty continuation."}`)
				} else {
					output, err = os.ReadFile(filepath.Join(os.Getenv("LLM_PROXY_QWEN27B_FIXTURE_DIR"), scenario.fixture))
					if err != nil {
						t.Fatal(err)
					}
				}
				var value any
				if json.Unmarshal(output, &value) != nil || validator.Validate(value) != nil {
					t.Fatal("local fixture does not satisfy canonical schema")
				}
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": string(output)}}}}, "usage": map[string]int{"input_tokens": 30, "output_tokens": 8, "total_tokens": 38}})
				}))
				t.Cleanup(upstream.Close)
				baseURL = upstream.URL
			} else {
				key, baseURL = os.Getenv("DASHSCOPE_API_KEY"), os.Getenv("DASHSCOPE_BASE_URL")
				if key == "" || baseURL == "" {
					t.Fatal("declared Proxy DashScope inputs are required")
				}
			}
			var schemaDocument any
			if err := json.Unmarshal(schema, &schemaDocument); err != nil {
				t.Fatal(err)
			}
			expectedWire, err := json.Marshal(map[string]any{"input": []any{map[string]string{"role": "user", "content": qwen27BRepresentativePrompt}}, "model": currentQwenSharedModel, "store": false, "max_output_tokens": 8192, "reasoning": map[string]string{"effort": effort}, "text": map[string]any{"format": map[string]any{"name": "llm_proxy_structured_output", "schema": schemaDocument, "strict": true, "type": "json_schema"}}})
			if err != nil {
				t.Fatal(err)
			}
			original := proxy.HTTPClient
			capturePath := filepath.Join(paths["RECEIPT_DIR"], mode+"-"+effort+"-"+scenario.name+".native.private.json")
			observer := &qwen27BProbeHTTPDoer{next: &http.Client{Timeout: 240 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, t: t, key: key, maxTokens: 8192, effort: effort, bodyLimit: 32768, expectedWire: expectedWire, privateCapturePath: capturePath}
			proxy.HTTPClient = observer
			t.Cleanup(func() { proxy.HTTPClient = original })
			tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
			tenant.ProviderKeys = map[string]string{proxy.ProviderNameDashScope: key}
			tenant.ProviderTextModels = map[string]string{proxy.ProviderNameDashScope: currentQwenSharedModel}
			tenant.Defaults = proxy.TenantDefaults{Provider: proxy.ProviderNameDashScope, Model: currentQwenSharedModel}
			router, err := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600, RequestTimeoutSeconds: 240, MaxRequestTimeoutSeconds: 240, Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameDashScope: baseURL}, nil)}, zap.NewNop().Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			receiptPath := filepath.Join(paths["RECEIPT_DIR"], mode+"-"+effort+"-"+scenario.name+".sdk.private.json")
			ctx, cancel := context.WithTimeout(context.Background(), 310*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, paths["SDK_BINARY"], server.URL, schemaPath, receiptPath)
			command.Stdin = strings.NewReader(TestSecret)
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.Getenv("TMPDIR")}
			output, runErr := command.CombinedOutput()
			t.Logf("released_sdk_status=%s projected_sha256=%x canonical_sha256=%x", strings.TrimSpace(string(output)), sha256.Sum256(schema), sha256.Sum256(canonical))
			if observer.calls.Load() != 1 {
				t.Fatalf("expected exactly one provider request, observed %d", observer.calls.Load())
			}
			nativeBytes, captureErr := os.ReadFile(capturePath)
			if captureErr != nil {
				t.Fatal("native private observation was not saved")
			}
			var native struct {
				VisibleText string `json:"visible_text"`
			}
			if json.Unmarshal(nativeBytes, &native) != nil {
				t.Fatal("native private observation is invalid")
			}
			diagnostics := map[string]any{}
			var nativeValue any
			if decodeErr := json.Unmarshal([]byte(native.VisibleText), &nativeValue); decodeErr != nil {
				diagnostics["json_error"] = decodeErr.Error()
			} else {
				if validationErr := validator.Validate(nativeValue); validationErr != nil {
					diagnostics["canonical_error"] = validationErr.Error()
				} else {
					diagnostics["canonical_valid"] = true
				}
				projectedCompiler := jsonschema.NewCompiler()
				if err := projectedCompiler.AddResource("urn:qwen27b:projected", schemaDocument); err != nil {
					t.Fatal(err)
				}
				projectedValidator, err := projectedCompiler.Compile("urn:qwen27b:projected")
				if err != nil {
					t.Fatal(err)
				}
				if validationErr := projectedValidator.Validate(nativeValue); validationErr != nil {
					diagnostics["projected_error"] = validationErr.Error()
				} else {
					diagnostics["projected_valid"] = true
				}
			}
			qwen27BWritePrivateJSON(t, filepath.Join(paths["RECEIPT_DIR"], mode+"-"+effort+"-"+scenario.name+".validation.private.json"), diagnostics)
			if mode == "local" && (diagnostics["canonical_valid"] != true || diagnostics["projected_valid"] != true) {
				t.Fatal("private native observation failed local fixture validation")
			}
			if runErr != nil {
				t.Fatalf("released SDK representative failed: %v", runErr)
			}
			receiptBytes, err := os.ReadFile(receiptPath)
			if err != nil {
				t.Fatal(err)
			}
			var receipt struct {
				VisibleText string `json:"visible_text"`
				Success     bool   `json:"success"`
			}
			if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
				t.Fatal(err)
			}
			var value any
			if !receipt.Success || json.Unmarshal([]byte(receipt.VisibleText), &value) != nil || validator.Validate(value) != nil {
				t.Fatal("released SDK output failed canonical application schema validation")
			}
			if strings.TrimSpace(receipt.VisibleText) == "" {
				t.Fatal("released SDK returned empty output")
			}
			t.Log("canonical_application_schema=passed")
		})
	}
}

// This allowance exists only in one disposable test process. Nano-USD keeps
// the selected offering's declared per-million token prices exact.
type qwen27BWorkflowAllowance struct {
	mu                       sync.Mutex
	AllocationNanoUSD        int64                    `json:"allocation_nano_usd"`
	SettledNanoUSD           int64                    `json:"settled_nano_usd"`
	ReservedNanoUSD          int64                    `json:"reserved_nano_usd"`
	Uncertain                bool                     `json:"uncertain"`
	StopReason               string                   `json:"stop_reason,omitempty"`
	Receipts                 []qwen27BWorkflowReceipt `json:"receipts"`
	AllocationCeilingNanoUSD int64                    `json:"allocation_ceiling_nano_usd"`
	InputNanoUSD             int64                    `json:"input_nano_usd_per_token"`
	OutputNanoUSD            int64                    `json:"output_nano_usd_per_token"`
	ContextTokenCeiling      int64                    `json:"context_token_ceiling"`
	AllowedOutputs           []int                    `json:"allowed_output_caps"`
	NativeWire               string                   `json:"native_wire"`
	Effort                   string                   `json:"effort"`
	path                     string
}

type qwen27BWorkflowReceipt struct {
	Attempt         int    `json:"attempt"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	RequestSHA256   string `json:"request_sha256"`
	ReservedNanoUSD int64  `json:"reserved_nano_usd"`
	ActualNanoUSD   *int64 `json:"actual_nano_usd,omitempty"`
	InputTokens     *int64 `json:"input_tokens,omitempty"`
	OutputTokens    *int64 `json:"output_tokens,omitempty"`
	State           string `json:"state"`
	NativeCapture   string `json:"native_capture,omitempty"`
}

func qwen27BConfiguredAllowance(t *testing.T, catalog *proxy.ProviderCatalog, provider, effort string, budgets map[string]int, allocation, ceiling int64, path string) *qwen27BWorkflowAllowance {
	t.Helper()
	var selected *proxy.ProviderCatalogOffering
	for _, definition := range catalog.Schema().Providers {
		if definition.ID == provider {
			for index := range definition.Offerings {
				if definition.Offerings[index].Model == currentQwenSharedModel {
					selected = &definition.Offerings[index]
				}
			}
		}
	}
	if selected == nil || selected.ReasoningEffort == nil || !slices.Contains(selected.ReasoningEffort.Efforts, effort) {
		t.Fatal("selected frozen provider/model/effort is not declared")
	}
	a := &qwen27BWorkflowAllowance{AllocationNanoUSD: allocation, AllocationCeilingNanoUSD: ceiling, Effort: effort, path: path}
	if provider == "cerebras" {
		a.NativeWire = "chat_completions"
	} else if provider == "dashscope" {
		a.NativeWire = "responses"
	} else {
		t.Fatal("unsupported matched Qwen provider")
	}
	for _, limit := range selected.Limits {
		if limit.ID == "context_tokens" && limit.Value != nil {
			a.ContextTokenCeiling = int64(*limit.Value)
		}
	}
	rates := map[string]int64{}
	for _, price := range selected.Prices {
		if price.Operation == "text" && price.Available {
			for _, rate := range price.Rates {
				if rate.Component != "input_tokens" && rate.Component != "output_tokens" {
					continue
				}
				if rate.Currency != "USD" || rate.Unit != "USD/1M_tokens" || rates[rate.Component] != 0 {
					t.Fatal("selected provider prices are not one exact USD per-million rate")
				}
				value, ok := new(big.Rat).SetString(string(rate.Rate))
				if !ok {
					t.Fatal("invalid selected provider rate")
				}
				value.Mul(value, big.NewRat(1000, 1))
				if !value.IsInt() || !value.Num().IsInt64() {
					t.Fatal("selected provider rate cannot use exact nanoUSD accounting")
				}
				rates[rate.Component] = value.Num().Int64()
			}
		}
	}
	a.InputNanoUSD, a.OutputNanoUSD = rates["input_tokens"], rates["output_tokens"]
	if a.ContextTokenCeiling <= 0 || a.InputNanoUSD <= 0 || a.OutputNanoUSD <= 0 {
		t.Fatal("selected provider context and prices are required")
	}
	for _, maximum := range budgets {
		if maximum <= 0 || maximum > selected.OutputTokenLimit {
			t.Fatal("frozen stage budget exceeds selected offering output limit")
		}
		if !slices.Contains(a.AllowedOutputs, maximum) {
			a.AllowedOutputs = append(a.AllowedOutputs, maximum)
		}
	}
	slices.Sort(a.AllowedOutputs)
	return a
}

func qwen27BSavePrivateJSON(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writeErr := json.NewEncoder(file).Encode(value)
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func (allowance *qwen27BWorkflowAllowance) persist() error {
	if allowance.path == "" {
		return nil
	}
	return qwen27BSavePrivateJSON(allowance.path, allowance)
}

func (allowance *qwen27BWorkflowAllowance) reserve(maximum int, body []byte) (int, error) {
	allowance.mu.Lock()
	defer allowance.mu.Unlock()
	if allowance.AllocationNanoUSD <= 0 || allowance.AllocationNanoUSD > allowance.AllocationCeilingNanoUSD || allowance.InputNanoUSD <= 0 || allowance.OutputNanoUSD <= 0 || allowance.ContextTokenCeiling <= 0 {
		return 0, errors.New("workflow allowance is outside the approved operation allocation")
	}
	if allowance.Uncertain || allowance.ReservedNanoUSD != 0 {
		return 0, errors.New("workflow stopped before dispatch: prior attempt remains uncertain or unsettled")
	}
	if len(allowance.Receipts) >= 16 {
		return 0, errors.New("workflow stopped before dispatch: sixteen native creates admitted")
	}
	ceiling := allowance.ContextTokenCeiling*allowance.InputNanoUSD + int64(maximum)*allowance.OutputNanoUSD
	if allowance.AllocationNanoUSD-allowance.SettledNanoUSD < ceiling {
		allowance.StopReason = "remaining_allocation_below_next_create_ceiling"
		if err := allowance.persist(); err != nil {
			return 0, err
		}
		return 0, errors.New("workflow stopped before dispatch: remaining allocation below published next-call ceiling")
	}
	allowance.ReservedNanoUSD = ceiling
	allowance.Receipts = append(allowance.Receipts, qwen27BWorkflowReceipt{Attempt: len(allowance.Receipts) + 1, MaxOutputTokens: maximum, RequestSHA256: fmt.Sprintf("%x", sha256.Sum256(body)), ReservedNanoUSD: ceiling, State: "reserved"})
	if err := allowance.persist(); err != nil {
		allowance.Uncertain = true
		allowance.StopReason = "reservation_receipt_write_failed"
		return 0, err
	}
	return len(allowance.Receipts), nil
}

func (allowance *qwen27BWorkflowAllowance) unknown(reason string) error {
	allowance.mu.Lock()
	defer allowance.mu.Unlock()
	allowance.Uncertain = true
	allowance.StopReason = reason
	if len(allowance.Receipts) > 0 {
		allowance.Receipts[len(allowance.Receipts)-1].State = "unknown_reservation_retained"
	}
	return allowance.persist()
}

func (allowance *qwen27BWorkflowAllowance) observe(status int, body []byte, capture string) error {
	allowance.mu.Lock()
	defer allowance.mu.Unlock()
	row := &allowance.Receipts[len(allowance.Receipts)-1]
	row.NativeCapture = capture
	type counts struct{ Input, Output, Total, Reasoning *int64 }
	u := counts{}
	completed := false
	var decodeErr error
	if allowance.NativeWire == "chat_completions" {
		var result struct {
			Choices []struct {
				Finish string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				Input   *int64 `json:"prompt_tokens"`
				Output  *int64 `json:"completion_tokens"`
				Total   *int64 `json:"total_tokens"`
				Details *struct {
					Reasoning *int64 `json:"reasoning_tokens"`
				} `json:"completion_tokens_details"`
			} `json:"usage"`
		}
		decodeErr = json.Unmarshal(body, &result)
		completed = len(result.Choices) == 1 && result.Choices[0].Finish == "stop"
		if result.Usage != nil {
			u.Input, u.Output, u.Total = result.Usage.Input, result.Usage.Output, result.Usage.Total
			if result.Usage.Details != nil {
				u.Reasoning = result.Usage.Details.Reasoning
			}
		}
	} else if allowance.NativeWire == "responses" {
		var result struct {
			Status string `json:"status"`
			Usage  *struct {
				Input   *int64 `json:"input_tokens"`
				Output  *int64 `json:"output_tokens"`
				Total   *int64 `json:"total_tokens"`
				Details *struct {
					Reasoning *int64 `json:"reasoning_tokens"`
				} `json:"output_tokens_details"`
			} `json:"usage"`
		}
		decodeErr = json.Unmarshal(body, &result)
		completed = result.Status == "completed"
		if result.Usage != nil {
			u.Input, u.Output, u.Total = result.Usage.Input, result.Usage.Output, result.Usage.Total
			if result.Usage.Details != nil {
				u.Reasoning = result.Usage.Details.Reasoning
			}
		}
	}
	known := decodeErr == nil && status >= 200 && status < 300 && completed && u.Input != nil && u.Output != nil && u.Total != nil
	if known {
		known = *u.Input >= 0 && *u.Input <= allowance.ContextTokenCeiling && *u.Output >= 0 && *u.Output <= int64(row.MaxOutputTokens) && *u.Total == *u.Input+*u.Output
		if u.Reasoning != nil {
			known = known && *u.Reasoning >= 0 && *u.Reasoning <= *u.Output
		}
	}
	if !known {
		allowance.Uncertain = true
		allowance.StopReason = "native_usage_parse_or_completion_uncertain"
		row.State = "unknown_reservation_retained"
		return allowance.persist()
	}
	cost := *u.Input*allowance.InputNanoUSD + *u.Output*allowance.OutputNanoUSD
	row.InputTokens = u.Input
	row.OutputTokens = u.Output
	row.ActualNanoUSD = &cost
	row.State = "settled"
	allowance.SettledNanoUSD += cost
	allowance.ReservedNanoUSD = 0
	if err := allowance.persist(); err != nil {
		allowance.Uncertain = true
		allowance.StopReason = "settlement_receipt_write_failed"
		return err
	}
	return nil
}

func qwen27BWritePrivateJSON(t *testing.T, path string, value any) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Errorf("create private qualification observation: %v", err)
		return
	}
	writeErr := json.NewEncoder(file).Encode(value)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Errorf("save private qualification observation: write=%v close=%v", writeErr, closeErr)
	}
}

// TestQwenCurrentAlibaba27BFullStoryLocalPreparation never loads provider
// credentials. It proves the frozen Story executable reaches the real Proxy
// boundary and preserves its canonical report when the local provider fails.
func TestQwenCurrentAlibaba27BFullStoryLocalPreparation(t *testing.T) {
	qwen27BFullStoryGuarded(t, false)
}

func TestQwenCurrentAlibaba27BFullStoryGuardedAttempt(t *testing.T) {
	if os.Getenv("LLM_PROXY_QWEN27B_FULL_STORY_MODE") != "live" {
		t.Skip("guarded live workflow remains disabled")
	}
	if os.Getenv("LLM_PROXY_QWEN27B_ISOLATED_SCOPE_APPROVED") != "true" {
		t.Fatal("primary live test dispatch authorization required before credential loading")
	}
	qwen27BFullStoryGuarded(t, true)
}

func qwen27BFullStoryGuarded(t *testing.T, live bool) {
	t.Helper()
	argumentsPath := os.Getenv("LLM_PROXY_QWEN27B_FULL_STORY_ARGUMENTS")
	if argumentsPath == "" {
		t.Skip("select credential-free frozen Story local preparation")
	}
	privateDirectory := os.Getenv("LLM_PROXY_QWEN27B_FULL_STORY_DIRECTORY")
	catalogPath := os.Getenv("LLM_PROXY_QWEN27B_FULL_STORY_CATALOG")
	argumentBytes, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	var arguments []string
	if json.Unmarshal(argumentBytes, &arguments) != nil {
		t.Fatal("invalid frozen Story arguments")
	}
	manifestBytes, err := os.ReadFile(filepath.Join(filepath.Dir(argumentsPath), "full-story-preparation.private.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		BinarySHA       string            `json:"binary_sha256"`
		PlotSHA         string            `json:"plot_sha256"`
		BudgetsSHA      string            `json:"output_budgets_sha256"`
		CatalogSHA      string            `json:"isolated_proxy_candidate_catalog_sha256"`
		StoryCatalogSHA string            `json:"story_catalog_sha256"`
		Contracts       map[string]string `json:"canonical_contract_sha256"`
		Arguments       []string          `json:"isolated_arguments"`
	}
	if json.Unmarshal(manifestBytes, &manifest) != nil {
		t.Fatal("invalid frozen preparation manifest")
	}
	preparedArgs, _ := json.Marshal(manifest.Arguments)
	if !jsonBodiesEqual(argumentBytes, preparedArgs) {
		t.Fatal("workflow arguments differ from frozen preparation")
	}
	option := func(name string) string {
		for index, value := range arguments {
			if value == name && index+1 < len(arguments) {
				return arguments[index+1]
			}
		}
		return ""
	}
	verifyHash := func(path, expected string) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != expected {
			t.Fatal("frozen workflow artifact hash changed")
		}
	}
	verifyHash(arguments[0], manifest.BinarySHA)
	verifyHash(option("--snapshot"), manifest.PlotSHA)
	verifyHash(option("--output-budgets-json"), manifest.BudgetsSHA)
	verifyHash(option("--provider-catalog"), manifest.StoryCatalogSHA)
	for name, expected := range manifest.Contracts {
		verifyHash(filepath.Join(option("--contracts"), name+".schema.json"), expected)
	}
	provider, effort := option("--provider"), option("--reasoning-effort")
	if (provider != "dashscope" && provider != "cerebras") || option("--model") != currentQwenSharedModel || (effort != "medium" && (provider != "cerebras" || effort != "high")) || option("--timeout-seconds") != "1800" {
		t.Fatal("workflow route differs from frozen medium contract")
	}
	catalogBytes, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(catalogBytes)
	if err != nil {
		t.Fatal(err)
	}
	verifyHash(catalogPath, manifest.CatalogSHA)
	budgetBytes, err := os.ReadFile(option("--output-budgets-json"))
	if err != nil {
		t.Fatal(err)
	}
	var budgets map[string]int
	if json.Unmarshal(budgetBytes, &budgets) != nil || len(budgets) != 9 {
		t.Fatal("nine frozen stage budgets required")
	}
	key, baseURL := "synthetic-full-story-connection", ""
	var next proxy.HTTPDoer
	allocation := int64(1253271780)
	tenantSecret := TestSecret
	if live {
		allocation, err = strconv.ParseInt(os.Getenv("LLM_PROXY_QWEN27B_WORKFLOW_ALLOCATION_NANO_USD"), 10, 64)
		if err != nil || allocation <= 0 || allocation > 1253271780 {
			t.Fatal("explicit bounded workflow allocation required")
		}
		if provider == "cerebras" {
			key = os.Getenv("CEREBRAS_API_KEY")
		} else {
			key, baseURL = os.Getenv("DASHSCOPE_API_KEY"), os.Getenv("DASHSCOPE_BASE_URL")
		}
		if key == "" || provider == "dashscope" && baseURL == "" {
			t.Fatal("declared Proxy-owned selected provider inputs required")
		}
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			t.Fatal("create disposable tenant secret")
		}
		tenantSecret = fmt.Sprintf("%x", secretBytes)
		client := &http.Client{Timeout: 1800 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		next = client
		t.Cleanup(client.CloseIdleConnections)
	} else {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if provider == "cerebras" {
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"unexpected":"controlled-local-failure"}`}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 30, "completion_tokens": 8, "total_tokens": 38}})
			} else {
				json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": `{"unexpected":"controlled-local-failure"}`}}}}, "usage": map[string]int{"input_tokens": 30, "output_tokens": 8, "total_tokens": 38}})
			}
		}))
		t.Cleanup(upstream.Close)
		next = upstream.Client()
		baseURL = upstream.URL
	}
	original := proxy.HTTPClient
	allowance := qwen27BConfiguredAllowance(t, catalog, provider, effort, budgets, allocation, 1253271780, filepath.Join(privateDirectory, "workflow-allowance.private.json"))
	var observer proxy.HTTPDoer
	var nativeCalls func() int64
	if provider == "cerebras" {
		value := &cerebrasPaidNativeOutputObserver{next: next, key: key, path: filepath.Join(privateDirectory, "native"), workflow: allowance}
		observer = value
		nativeCalls = value.calls.Load
	} else {
		value := &qwen27BProbeHTTPDoer{next: next, t: t, key: key, effort: effort, bodyLimit: 4194304, privateCapturePath: filepath.Join(privateDirectory, "native"), workflow: allowance}
		observer = value
		nativeCalls = value.calls.Load
	}
	proxy.HTTPClient = observer
	t.Cleanup(func() { proxy.HTTPClient = original })
	tenant := proxy.StandardManagedTenantTestConfiguration(tenantSecret)
	tenant.ProviderKeys = map[string]string{provider: key}
	tenant.ProviderTextModels = map[string]string{provider: currentQwenSharedModel}
	tenant.Defaults = proxy.TenantDefaults{Provider: provider, Model: currentQwenSharedModel}
	configuration := proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600, RequestTimeoutSeconds: 1800, MaxRequestTimeoutSeconds: 1800}
	if !live || provider == "dashscope" {
		configuration.Endpoints = providerEndpointOverrides(map[string]string{provider: baseURL}, nil)
	}
	router, err := buildRouterWithManagedTenant(t, configuration, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	outputDirectory := filepath.Join(privateDirectory, "story-run")
	for index, value := range arguments {
		if value == "--proxy-url" {
			arguments[index+1] = server.URL
		}
		if value == "--output-directory" {
			arguments[index+1] = outputDirectory
		}
	}
	lifetime := 30 * time.Second
	if live {
		lifetime = 16*1800*time.Second + time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), lifetime)
	defer cancel()
	command := exec.CommandContext(ctx, arguments[0], arguments[1:]...)
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.Getenv("TMPDIR"), "TELLTALE_EVALUATION_PROXY_SECRET=" + tenantSecret}
	output, runErr := command.CombinedOutput()
	file, err := os.Create(filepath.Join(privateDirectory, "story-execution.private.log"))
	if err != nil {
		t.Fatal(err)
	}
	file.Write(output)
	file.Close()
	if !live && (runErr == nil || nativeCalls() != 1) {
		t.Fatalf("controlled first-call failure was not preserved: exit_error=%v native_calls=%d", runErr, nativeCalls())
	}
	reportBytes, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Status string            `json:"status"`
		Calls  []json.RawMessage `json:"client_calls"`
	}
	if json.Unmarshal(reportBytes, &report) != nil || report.Status == "" || !live && (report.Status != "failed" || len(report.Calls) != 1) {
		t.Fatal("frozen Story canonical failure report missing or inconsistent")
	}
	if !live && (allowance.SettledNanoUSD != 30*allowance.InputNanoUSD+8*allowance.OutputNanoUSD || allowance.ReservedNanoUSD != 0 || allowance.Uncertain) {
		t.Fatal("exact first Story fixture did not settle known native usage")
	}
	t.Logf("frozen_story_guarded_attempt_finished live=%t provider=%s effort=%s native_calls=%d canonical_report_status=%s application_calls=%d settled_nano_usd=%d retained_reservation_nano_usd=%d uncertain=%t child_environment=only_local_proxy_tenant_secret", live, provider, effort, nativeCalls(), report.Status, len(report.Calls), allowance.SettledNanoUSD, allowance.ReservedNanoUSD, allowance.Uncertain)
}

func TestQwenCurrentAlibaba27BWorkflowAllowance(t *testing.T) {
	for _, scenario := range []struct {
		name, body      string
		allocation      int64
		admit           int
		spent, reserved int64
		uncertain       bool
		maximum         int
	}{
		{"two-settled-then-financial-refusal", `{"status":"completed","usage":{"input_tokens":100,"output_tokens":100,"total_tokens":200,"output_tokens_details":{"reasoning_tokens":80}}}`, 598654000, 2, 700000, 0, false, 32768},
		{"lower-stage-cap-reservation", `{"status":"completed","usage":{"input_tokens":100,"output_tokens":100,"total_tokens":200}}`, 524926000, 2, 700000, 0, false, 8192},
		{"missing-usage-stops-next", `{"status":"completed"}`, 1606168040, 1, 0, 598304000, true, 32768},
		{"parse-failure-stops-next", `not JSON`, 1606168040, 1, 0, 598304000, true, 32768},
		{"incomplete-stops-continuation", `{"status":"incomplete","usage":{"input_tokens":100,"output_tokens":100,"total_tokens":200}}`, 1606168040, 1, 0, 598304000, true, 32768},
		{"sixteen-native-create-cap", `{"status":"completed","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`, 1606168040, 16, 0, 0, false, 32768},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var forwarded atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				forwarded.Add(1)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, scenario.body)
			}))
			defer upstream.Close()
			directory := t.TempDir()
			allowance := &qwen27BWorkflowAllowance{AllocationNanoUSD: scenario.allocation, AllocationCeilingNanoUSD: 1606168040, InputNanoUSD: 500, OutputNanoUSD: 3000, ContextTokenCeiling: 1000000, AllowedOutputs: []int{scenario.maximum}, NativeWire: "responses", Effort: "medium", path: filepath.Join(directory, "allowance.json")}
			observer := &qwen27BProbeHTTPDoer{next: upstream.Client(), t: t, key: "synthetic-allowance-key", effort: "medium", bodyLimit: 4194304, workflow: allowance, privateCapturePath: filepath.Join(directory, "native")}
			for index := 0; index < scenario.admit+1; index++ {
				request, _ := http.NewRequest(http.MethodPost, upstream.URL+"/responses", strings.NewReader(fmt.Sprintf(`{"model":"qwen3.8-27b","max_output_tokens":%d,"reasoning":{"effort":"medium"},"store":false}`, scenario.maximum)))
				response, err := observer.Do(request)
				if index < scenario.admit {
					if err != nil {
						t.Fatal(err)
					}
					body, _ := io.ReadAll(response.Body)
					response.Body.Close()
					if string(body) != scenario.body {
						t.Fatal("allowance observer changed provider response")
					}
				} else if err == nil {
					response.Body.Close()
					t.Fatal("guard did not refuse the next native dispatch")
				}
			}
			if forwarded.Load() != int64(scenario.admit) || observer.calls.Load() != int64(scenario.admit) || allowance.SettledNanoUSD != scenario.spent || allowance.ReservedNanoUSD != scenario.reserved || allowance.Uncertain != scenario.uncertain {
				t.Fatalf("guard result calls=%d settled=%d reserved=%d uncertain=%t", forwarded.Load(), allowance.SettledNanoUSD, allowance.ReservedNanoUSD, allowance.Uncertain)
			}
		})
	}
}

func TestQwenCurrentProviderAllowance(t *testing.T) {
	for _, profile := range []struct {
		provider, effort string
		maximum          int
		ceiling          int64
	}{{"dashscope", "medium", 65536, 696608000}, {"cerebras", "medium", 40960, 190791680}, {"cerebras", "high", 40960, 190791680}} {
		t.Run(profile.provider+"-"+profile.effort, func(t *testing.T) {
			for _, mode := range []string{"two-settled-financial-stop", "unknown-usage-stop", "over-allocation-stop", "over-cap-native-stop", "sixteen-native-stop"} {
				t.Run(mode, func(t *testing.T) {
					var forwarded atomic.Int64
					usage := map[string]int64{"input_tokens": 100, "output_tokens": 100, "total_tokens": 200}
					if mode == "sixteen-native-stop" {
						usage = map[string]int64{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
					}
					responseBody := map[string]any{"status": "completed", "usage": usage}
					if profile.provider == "cerebras" {
						responseBody = map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "fixture"}, "finish_reason": "stop"}}, "usage": map[string]int64{"prompt_tokens": usage["input_tokens"], "completion_tokens": usage["output_tokens"], "total_tokens": usage["total_tokens"]}}
					}
					if mode == "unknown-usage-stop" {
						delete(responseBody, "usage")
					}
					encoded, _ := json.Marshal(responseBody)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						forwarded.Add(1)
						w.Header().Set("Content-Type", "application/json")
						w.Write(encoded)
					}))
					defer upstream.Close()
					directory := t.TempDir()
					allowance := qwen27BConfiguredAllowance(t, testfixtures.ProviderCatalog(t), profile.provider, profile.effort, map[string]int{"stage": profile.maximum}, 1253271780, 1253271780, filepath.Join(directory, "allowance.json"))
					if allowance.ContextTokenCeiling*allowance.InputNanoUSD+int64(profile.maximum)*allowance.OutputNanoUSD != profile.ceiling {
						t.Fatal("frozen selected-provider reservation does not match catalog prices/context")
					}
					oneCost := 100 * (allowance.InputNanoUSD + allowance.OutputNanoUSD)
					admit := 2
					expectedSpent := 2 * oneCost
					allowance.AllocationNanoUSD = profile.ceiling + oneCost
					if mode == "unknown-usage-stop" {
						allowance.AllocationNanoUSD = 1253271780
						admit = 1
						expectedSpent = 0
					}
					if mode == "over-allocation-stop" {
						allowance.AllocationNanoUSD = 1253271781
						admit = 0
						expectedSpent = 0
					}
					if mode == "over-cap-native-stop" {
						admit = 0
						expectedSpent = 0
					}
					if mode == "sixteen-native-stop" {
						allowance.AllocationNanoUSD = 1253271780
						admit = 16
						expectedSpent = 0
					}
					var doer proxy.HTTPDoer
					var calls func() int64
					path := "/responses"
					payload := map[string]any{"model": currentQwenSharedModel, "max_output_tokens": profile.maximum, "reasoning": map[string]string{"effort": profile.effort}, "store": false}
					if profile.provider == "cerebras" {
						value := &cerebrasPaidNativeOutputObserver{next: upstream.Client(), key: "synthetic-matched-key", path: filepath.Join(directory, "native"), workflow: allowance}
						doer = value
						calls = value.calls.Load
						path = "/chat/completions"
						payload = map[string]any{"model": "qwen-3.8-27b", "max_completion_tokens": profile.maximum, "reasoning_effort": profile.effort, "messages": []any{map[string]string{"role": "user", "content": "fixture"}}}
					} else {
						value := &qwen27BProbeHTTPDoer{next: upstream.Client(), t: t, key: "synthetic-matched-key", effort: profile.effort, bodyLimit: 4194304, privateCapturePath: filepath.Join(directory, "native"), workflow: allowance}
						doer = value
						calls = value.calls.Load
					}
					if mode == "over-cap-native-stop" {
						if profile.provider == "cerebras" {
							payload["max_completion_tokens"] = 131072
						} else {
							payload["max_output_tokens"] = 131072
						}
					}
					requestBody, _ := json.Marshal(payload)
					for index := 0; index < admit+1; index++ {
						request, _ := http.NewRequest(http.MethodPost, upstream.URL+path, bytes.NewReader(requestBody))
						response, err := doer.Do(request)
						if index < admit {
							if err != nil {
								t.Fatal(err)
							}
							body, _ := io.ReadAll(response.Body)
							response.Body.Close()
							if !bytes.Equal(body, encoded) {
								t.Fatal("matched observer changed native response bytes")
							}
						} else if err == nil {
							response.Body.Close()
							t.Fatal("matched guard did not refuse next native dispatch")
						}
					}
					if forwarded.Load() != int64(admit) || calls() != int64(admit) || allowance.SettledNanoUSD != expectedSpent {
						t.Fatal("matched financial/native-create accounting mismatch")
					}
					if mode == "unknown-usage-stop" && (!allowance.Uncertain || allowance.ReservedNanoUSD != profile.ceiling) {
						t.Fatal("unknown usage did not retain exact selected-provider reservation")
					}
					if mode == "over-cap-native-stop" && (len(allowance.Receipts) != 0 || allowance.ReservedNanoUSD != 0) {
						t.Fatal("over-cap native request reached reservation or provider boundary")
					}
				})
			}
		})
	}
}
