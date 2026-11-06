package proxy

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/message"
)

type diagnosticTestKind struct{ path []string }

func (kind diagnosticTestKind) KeywordPath() []string { return kind.path }
func (kind diagnosticTestKind) LocalizedString(*message.Printer) string {
	return "private-validator-message-do-not-log"
}

func diagnosticValidation(keyword string) *jsonschema.ValidationError {
	return &jsonschema.ValidationError{SchemaURL: "https://private-schema-do-not-log/#/$defs/private-name", InstanceLocation: []string{"private-value"}, ErrorKind: diagnosticTestKind{path: []string{keyword, "private-property"}}}
}

func TestStructuredOutputDiagnosticMetadataPreservesErrorAndPrivacy(t *testing.T) {
	for _, reason := range []structuredOutputFailureReason{structuredOutputNotJSON, structuredOutputMultipleJSON, structuredOutputSchemaViolation} {
		failure := newStructuredOutputResponseFailure(reason, nil)
		if !errors.Is(failure, ErrProviderAPI) || failure.Error() != ErrProviderAPI.Error()+": "+structuredOutputFailureMessages[reason] {
			t.Fatal("provider error identity changed")
		}
		status, retryAfter, retryable, hasStatus, codes := providerFailureMetadata(failure)
		if status != 0 || retryAfter != "" || retryable || hasStatus || !reflect.DeepEqual(codes, []string{string(reason)}) {
			t.Fatalf("metadata=%v", codes)
		}
	}
	root := diagnosticValidation("required")
	root.Causes = []*jsonschema.ValidationError{diagnosticValidation("type"), diagnosticValidation("enum"), diagnosticValidation("type"), diagnosticValidation("private-keyword"), diagnosticValidation(""), nil, {}}
	codes := newStructuredOutputResponseFailure(structuredOutputSchemaViolation, root).(providerTerminalFailure).providerFailureCodes()
	expected := []string{"structured_output.schema_violation", "json_schema.enum", "json_schema.required", "json_schema.type"}
	if !reflect.DeepEqual(codes, expected) {
		t.Fatalf("codes=%v", codes)
	}
	for _, code := range codes {
		if strings.Contains(code, "private") {
			t.Fatal("private validator data entered diagnostic metadata")
		}
	}
}

func TestStructuredOutputDiagnosticTraversalAndOutputAreBounded(t *testing.T) {
	t.Run("node count", func(t *testing.T) {
		root := diagnosticValidation("required")
		for i := 0; i < structuredOutputDiagnosticNodeLimit-1; i++ {
			root.Causes = append(root.Causes, diagnosticValidation("type"))
		}
		root.Causes = append(root.Causes, diagnosticValidation("enum"))
		if got := structuredOutputFailureKeywords(root); !reflect.DeepEqual(got, []string{"json_schema.required", "json_schema.type"}) {
			t.Fatalf("node limit crossed: %v", got)
		}
	})
	t.Run("depth", func(t *testing.T) {
		root := diagnosticValidation("required")
		leaf := root
		for i := 0; i < structuredOutputDiagnosticDepthLimit; i++ {
			child := diagnosticValidation("type")
			leaf.Causes = []*jsonschema.ValidationError{child}
			leaf = child
		}
		leaf.Causes = []*jsonschema.ValidationError{diagnosticValidation("enum"), nil}
		if got := structuredOutputFailureKeywords(root); !reflect.DeepEqual(got, []string{"json_schema.required", "json_schema.type"}) {
			t.Fatalf("depth limit crossed: %v", got)
		}
	})
	t.Run("keyword count", func(t *testing.T) {
		root := &jsonschema.ValidationError{}
		for _, keyword := range []string{"required", "type", "enum", "const", "minimum", "maximum", "minLength", "maxLength", "pattern"} {
			root.Causes = append(root.Causes, diagnosticValidation(keyword))
		}
		codes := structuredOutputFailureKeywords(root)
		if len(codes) != structuredOutputDiagnosticKeywordLimit {
			t.Fatalf("keyword count=%d", len(codes))
		}
		for _, code := range codes {
			if code == "json_schema.pattern" {
				t.Fatal("keyword output limit crossed")
			}
		}
	})
}
