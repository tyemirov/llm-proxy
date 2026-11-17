package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCerebrasChatUsageReasoning(t *testing.T) {
	result, err := parseChatCompletionResponse([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14,"completion_tokens_details":{"reasoning_tokens":3}}}`), chatCompletionResponsePolicyDefault)
	if err != nil || result.usage == nil || result.usage.ReasoningTokens == nil || *result.usage.ReasoningTokens != 3 || result.usage.ResponseTokens != 5 {
		t.Fatalf("reasoning usage=%+v err=%v", result.usage, err)
	}
}

func cerebrasTestPolicy() *StructuredOutputPolicy {
	return &StructuredOutputPolicy{Keywords: []string{"$defs", "$ref", "additionalProperties", "anyOf", "const", "description", "enum", "exclusiveMaximum", "exclusiveMinimum", "items", "maximum", "minimum", "multipleOf", "properties", "required", "title", "type"}, Formats: []string{}, Root: "object_with_properties", Objects: "closed", RequiredProperties: "explicit", References: "local_nonrecursive", EnumValues: "primitive", ConstValues: "primitive", ArrayItems: "required", ArrayMinimum: "any", ReferenceAllOf: "allow", TextOnly: true, Limits: StructuredOutputLimits{NestingDepth: 10, ObjectProperties: 500, EnumValues: 500, CountedSchemaCharacters: 5000, LargeEnumThreshold: 250, LargeEnumCharacters: 7500}}
}

func TestCerebrasSchemaPolicy(t *testing.T) {
	for _, scenario := range []struct {
		name, schema string
		valid        bool
	}{
		{"missing-root-properties", `{"type":"object","additionalProperties":false}`, false},
		{"missing-array-items", `{"type":"object","properties":{"v":{"type":"array"}},"additionalProperties":false}`, false},
		{"array-items-and-union", `{"type":"object","properties":{"v":{"type":"array","items":{"anyOf":[{"type":"string"},{"type":"null"}]}}},"additionalProperties":false}`, true},
		{"optional", `{"type":"object","properties":{"v":{"type":"string"}},"additionalProperties":false}`, true},
		{"constraints", `{"type":"object","properties":{"v":{"type":"number","minimum":0,"maximum":5,"multipleOf":0.5},"n":{"const":null}},"additionalProperties":false}`, true},
		{"local-ref", `{"type":"object","properties":{"v":{"$ref":"#/$defs/v"}},"additionalProperties":false,"$defs":{"v":{"type":"string"}}}`, true},
		{"recursive", `{"type":"object","properties":{"v":{"$ref":"#"}},"additionalProperties":false}`, false},
		{"nested-recursive", `{"type":"object","properties":{"v":{"$ref":"#/$defs/v"}},"additionalProperties":false,"$defs":{"v":{"type":"object","properties":{"v":{"$ref":"#/$defs/v"}},"additionalProperties":false}}}`, false},
		{"pattern", `{"type":"object","properties":{"v":{"type":"string","pattern":"a"}},"additionalProperties":false}`, false},
		{"array-limit", `{"type":"object","properties":{"v":{"type":"array","items":{"type":"string"},"maxItems":1}},"additionalProperties":false}`, false},
		{"complex-const", `{"type":"object","properties":{"v":{"const":{"a":1}}},"additionalProperties":false}`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			schema := mustStructuredOutputSchema(t, scenario.schema)
			err := validateStructuredOutputRoute(textModelDefinition{structuredOutput: cerebrasTestPolicy()}, schema)
			if (err == nil) != scenario.valid {
				t.Fatalf("valid=%v err=%v", scenario.valid, err)
			}
		})
	}
}

func TestCerebrasChatUsagePresenceAndBounds(t *testing.T) {
	for _, scenario := range []struct {
		name, usage                string
		valid, measured, reasoning bool
	}{
		{"absent", `null`, true, false, false},
		{"empty", `{}`, true, false, false},
		{"zero", `{"prompt_tokens":0,"completion_tokens":0,"completion_tokens_details":{"reasoning_tokens":0}}`, true, true, true},
		{"partial", `{"prompt_tokens":9,"completion_tokens_details":{"reasoning_tokens":2}}`, true, false, true},
		{"negative", `{"prompt_tokens":9,"completion_tokens":5,"completion_tokens_details":{"reasoning_tokens":-1}}`, false, false, false},
		{"too-large", `{"prompt_tokens":9,"completion_tokens":5,"completion_tokens_details":{"reasoning_tokens":6}}`, false, false, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			result, err := parseChatCompletionResponse([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":`+scenario.usage+`}`), chatCompletionResponsePolicyDefault)
			if (err == nil) != scenario.valid {
				t.Fatalf("err=%v", err)
			}
			if !scenario.valid {
				return
			}
			if (completeTokenUsage(result.usage) != nil) != scenario.measured {
				t.Fatalf("usage=%+v", result.usage)
			}
			hasReasoning := result.usage != nil && result.usage.ReasoningTokens != nil
			if hasReasoning != scenario.reasoning {
				t.Fatalf("reasoning=%+v", result.usage)
			}
		})
	}
}

func TestCerebrasSchemaComplexityLimits(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		limit  func(*StructuredOutputPolicy)
		schema string
	}{
		{"properties", func(p *StructuredOutputPolicy) { p.Limits.ObjectProperties = 1 }, `{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}},"additionalProperties":false}`},
		{"enums", func(p *StructuredOutputPolicy) { p.Limits.EnumValues = 1 }, `{"type":"object","properties":{"a":{"enum":["a","b"]}},"additionalProperties":false}`},
		{"constant-characters", func(p *StructuredOutputPolicy) { p.Limits.CountedSchemaCharacters = 3 }, `{"type":"object","properties":{"a":{"const":"long"}},"additionalProperties":false}`},
		{"characters", func(p *StructuredOutputPolicy) { p.Limits.CountedSchemaCharacters = 2 }, `{"type":"object","properties":{"abc":{"type":"string"}},"additionalProperties":false}`},
		{"depth", func(p *StructuredOutputPolicy) { p.Limits.NestingDepth = 1 }, `{"type":"object","properties":{"a":{"type":"string"}},"additionalProperties":false}`},
		{"large-enum", func(p *StructuredOutputPolicy) { p.Limits.LargeEnumThreshold = 1; p.Limits.LargeEnumCharacters = 2 }, `{"type":"object","properties":{"a":{"enum":["aa","bb"]}},"additionalProperties":false}`},
		{"reference-depth", func(p *StructuredOutputPolicy) { p.Limits.NestingDepth = 2 }, `{"type":"object","properties":{"a":{"$ref":"#/$defs/a"}},"additionalProperties":false,"$defs":{"a":{"type":"object","properties":{"b":{"type":"string"}},"additionalProperties":false}}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			policy := cerebrasTestPolicy()
			scenario.limit(policy)
			schema := mustStructuredOutputSchema(t, scenario.schema)
			if err := validateStructuredOutputRoute(textModelDefinition{structuredOutput: policy}, schema); err == nil {
				t.Fatal("complexity limit admitted")
			}
		})
	}
}

func TestCerebrasPolicyCatalogAdmissionAndCloning(t *testing.T) {
	policy := cerebrasTestPolicy()
	if err := validateStructuredOutputPolicy(policy); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*StructuredOutputPolicy){func(p *StructuredOutputPolicy) { p.Root = "provider-name" },
		func(p *StructuredOutputPolicy) { p.Keywords = nil },
		func(p *StructuredOutputPolicy) { p.Keywords = append(p.Keywords, p.Keywords[0]) }, func(p *StructuredOutputPolicy) { p.Keywords = []string{"oneOf"} }, func(p *StructuredOutputPolicy) { p.References = "external" }, func(p *StructuredOutputPolicy) { p.Limits.EnumValues = -1 }} {
		candidate := cloneStructuredOutputPolicy(policy)
		mutate(candidate)
		if err := validateStructuredOutputPolicy(candidate); err == nil {
			t.Fatal("invalid catalog policy admitted")
		}
	}
	clone := cloneProviderOffering(ProviderOffering{StructuredOutput: policy})
	clone.StructuredOutput.Keywords[0] = "changed"
	clone.StructuredOutput.Formats = append(clone.StructuredOutput.Formats, "email")
	clone.StructuredOutput.Limits.EnumValues = 1
	if policy.Keywords[0] == "changed" || len(policy.Formats) != 0 || policy.Limits.EnumValues != 500 {
		t.Fatal("offering clone mutated policy")
	}
	document, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(document)
	old := "structured_output: *openai_schema"
	if !strings.Contains(text, old) {
		t.Fatal("expected reusable schema policy fixture")
	}
	if _, err := ParseProviderCatalog([]byte(strings.Replace(text, old, "structured_output: true", 1))); err == nil {
		t.Fatal("obsolete boolean structured_output admitted")
	}
}

func TestCerebrasReadOnlyConnectionVerification(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/models" || r.Header.Get("Authorization") != "Bearer provider-fixture" {
			t.Errorf("verification %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"id":"qwen-3.8-27b"}]}`)
	}))
	defer upstream.Close()
	schema := internalCanonicalProviderCatalog().Schema()
	for index := range schema.Models {
		if schema.Models[index].ID == "qwen3.8-27b" {
			schema.Models[index].Enabled = ModelEnabled
		}
	}
	for index := range schema.Providers {
		if schema.Providers[index].ID == "cerebras" {
			schema.Providers[index].Enabled = ModelEnabled
			schema.Providers[index].Offerings[0].Enabled = ModelEnabled
			schema.Providers[index].Offerings[0].DefaultOperations = []string{ModelOperationText}
		}
	}
	catalog, err := NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	endpoints := NewEndpoints()
	endpoints.SetProviderBaseURL("cerebras", upstream.URL)
	registry := newProviderRegistry(Configuration{ProviderCatalog: catalog, Endpoints: endpoints})
	provider := registry.definitions[providerID("cerebras")]
	provider, found := provider.resolvedTransport(provider.verification.Transport)
	if !found {
		t.Fatal("verification transport absent")
	}
	verifier := newOperationalProviderKeyVerifier(upstream.Client(), endpoints, time.Second, zap.NewNop().Sugar())
	if err := verifier.verify(context.Background(), provider, textModelDefinition{}, "provider-fixture"); err != nil || calls != 1 {
		t.Fatalf("verification calls=%d err=%v", calls, err)
	}
}

func TestCerebrasChatFailedFinishRetainsUsage(t *testing.T) {
	for _, finish := range []string{"length", "content_filter", "unknown"} {
		t.Run(finish, func(t *testing.T) {
			result, err := parseChatCompletionResponse([]byte(`{"choices":[{"message":{"content":"partial"},"finish_reason":"`+finish+`"}],"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14,"completion_tokens_details":{"reasoning_tokens":3}}}`), chatCompletionResponsePolicyDefault)
			if err == nil || result.usage == nil || result.usage.ReasoningTokens == nil || *result.usage.ReasoningTokens != 3 || result.usage.ResponseTokens != 5 || result.usage.TotalTokens != 14 {
				t.Fatalf("finish=%s usage=%+v err=%v", finish, result.usage, err)
			}
		})
	}
}

func TestCerebrasSchemaRepeatedReferencesRemainBounded(t *testing.T) {
	definitions := map[string]any{"d0": map[string]any{"type": "string"}}
	for depth := 1; depth <= 8; depth++ {
		properties := map[string]any{}
		for property := 0; property < 10; property++ {
			properties[fmt.Sprintf("p%d", property)] = map[string]any{"$ref": fmt.Sprintf("#/$defs/d%d", depth-1)}
		}
		definitions[fmt.Sprintf("d%d", depth)] = map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	}
	document := map[string]any{"type": "object", "properties": map[string]any{"result": map[string]any{"$ref": "#/$defs/d8"}}, "additionalProperties": false, "$defs": definitions}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	schema := mustStructuredOutputSchema(t, string(encoded))
	policy := cerebrasTestPolicy()
	if err := validateStructuredOutputRoute(textModelDefinition{structuredOutput: policy}, schema); err != nil {
		t.Fatal(err)
	}
	depth, err := structuredOutputGraphDepth(document)
	if err != nil || depth != 10 {
		t.Fatalf("graph depth=%d err=%v", depth, err)
	}
	policy.Limits.NestingDepth = 9
	if err := validateStructuredOutputRoute(textModelDefinition{structuredOutput: policy}, schema); err == nil {
		t.Fatal("reused subtree bypassed nesting depth limit")
	}
}

func TestCerebrasCatalogRejectsPreviousSchemaVersion(t *testing.T) {
	document, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	current := fmt.Sprintf("schema_version: %d", ProviderCatalogSchemaVersion)
	old := fmt.Sprintf("schema_version: %d", ProviderCatalogSchemaVersion-1)
	if !strings.Contains(string(document), current) {
		t.Fatal("catalog schema version mismatch")
	}
	if _, err := ParseProviderCatalog([]byte(strings.Replace(string(document), current, old, 1))); err == nil {
		t.Fatal("previous incompatible catalog schema admitted")
	}
}

func TestCerebrasSchemaGraphReferenceResolution(t *testing.T) {
	document := map[string]any{"type": "object", "properties": map[string]any{"a/b~": map[string]any{"type": "string"}}}
	for _, scenario := range []struct {
		reference string
		valid     bool
	}{{"#", true}, {"#/properties/a~1b~0", true}, {"https://example.invalid/schema", false}, {"#/missing", false}, {"#/properties/a~1b~0/type/child", false}} {
		t.Run(scenario.reference, func(t *testing.T) {
			_, err := structuredOutputReference(document, scenario.reference)
			if (err == nil) != scenario.valid {
				t.Fatalf("valid=%v err=%v", scenario.valid, err)
			}
		})
	}
	if _, err := structuredOutputGraphDepth(map[string]any{"$ref": "#/missing"}); err == nil {
		t.Fatal("graph accepted missing reference")
	}
	schema := mustStructuredOutputSchema(t, `{"type":"object","properties":{"values":{"type":"array","items":{"type":"string"}}},"additionalProperties":{"anyOf":[{"type":"number"},{"type":"null"}]}}`)
	policy := cerebrasTestPolicy()
	policy.Objects = "any"
	if err := validateStructuredOutputRoute(textModelDefinition{structuredOutput: policy}, schema); err != nil {
		t.Fatalf("bounded graph rejected schema-valued additional properties: %v", err)
	}
}

func TestCerebrasCatalogRejectsIncompatibleSchemaPolicies(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*ProviderOffering)
	}{
		{"invalid-policy", func(offering *ProviderOffering) { offering.StructuredOutput.Keywords = nil }},
		{"unsupported-codec", func(offering *ProviderOffering) { offering.WireContract = CatalogProtocolSystemOne }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			catalog := internalCanonicalProviderCatalog().ModelCatalog()
			found := false
			for index := range catalog.Offerings {
				offering := &catalog.Offerings[index]
				if offering.StructuredOutput != nil {
					scenario.mutate(offering)
					found = true
					break
				}
			}
			if !found {
				t.Fatal("schema policy offering absent")
			}
			if _, err := validateModelCatalogStructure(catalog); err == nil {
				t.Fatal("incompatible catalog policy admitted")
			}
		})
	}
}
