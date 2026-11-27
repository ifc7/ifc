package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaptinlin/jsonschema"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	openapilint "github.com/ifc7/ifc/pkg/plugins/openapi/lint"
)

func TestLintOutputMatchesSchema(t *testing.T) {
	schema := loadSchema(t, "lint-output.json")
	out := contract.LintOutput{
		Score: 87,
		Raw:   "warning[operation-operationId]: missing\n",
		Extra: map[string]any{
			"findingCounts": map[string]int{"error": 0, "warning": 1, "info": 0},
		},
	}
	assertValid(t, schema, out)
}

func TestCompareOutputMatchesSchema(t *testing.T) {
	schema := loadSchema(t, "compare-output.json")
	out := contract.CompareOutput{
		Breaking: true,
		Raw:      "BREAKING: removed path\n",
		Extra: map[string]any{
			"changeCounts": map[string]int{"breaking": 1, "nonBreaking": 0},
		},
	}
	assertValid(t, schema, out)
}

func TestCompareOutputV1MatchesSchema(t *testing.T) {
	schema := loadSchema(t, "v1/compare-output.json")
	out := contract.CompareOutput{
		Breaking: true,
		Raw:      "BREAKING paths.path.removed /paths/~1pets: removed path\n",
		Changes: []contract.Change{{
			Kind:        "paths.path.removed",
			Pointer:     "/paths/~1pets",
			Breaking:    true,
			Level:       contract.ChangeLevelError,
			Message:     "removed path",
			Fingerprint: strings.Repeat("a", 64),
		}, {
			Kind:        "pathItem.operation.removed",
			Pointer:     "/paths/~1beta/get",
			Level:       contract.ChangeLevelWarn,
			Message:     "removed get",
			Stability:   "x-stability-level: alpha",
			Fingerprint: strings.Repeat("b", 64),
		}},
		Extra: map[string]any{"changeCounts": map[string]int{"breaking": 1, "nonBreaking": 1}},
	}
	assertValid(t, schema, out)
	// A v1 compare output without changes is a valid v0 output.
	out.Changes = nil
	assertValid(t, loadSchema(t, "compare-output.json"), out)
}

func TestLintInputMatchesSchema(t *testing.T) {
	schema := loadSchema(t, "lint-input.json")
	in := contract.LintInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Document:      contract.NewSpecificationDocument([]byte("openapi: \"3.0.3\"\n"), contract.FileFormatYAML),
		Options:       map[string]any{},
	}
	assertValid(t, schema, in)
}

func TestCompareInputMatchesSchema(t *testing.T) {
	schema := loadSchema(t, "compare-input.json")
	in := contract.CompareInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Before:        contract.NewSpecificationDocument([]byte("openapi: \"3.0.3\"\n"), contract.FileFormatYAML),
		After:         contract.NewSpecificationDocument([]byte("openapi: \"3.1.0\"\n"), contract.FileFormatYAML),
		Options:       map[string]any{},
	}
	assertValid(t, schema, in)
}

func TestLintOutputV1MatchesSchema(t *testing.T) {
	schema := loadSchema(t, "v1/lint-output.json")
	out := contract.LintOutput{
		Score: 95,
		Raw:   "warn[operation-operationId]: missing\n",
		Findings: []contract.Finding{{
			RuleID:           "operation-operationId",
			Severity:         contract.SeverityWarn,
			Message:          "missing",
			Pointer:          "/paths/~1pets/get",
			Range:            &contract.Range{Start: contract.Position{Line: 3, Column: 5}, End: contract.Position{Line: 3, Column: 8}},
			Fingerprint:      strings.Repeat("a", 64),
			DocumentationURL: "https://quobix.com/vacuum/rules/operations/operation-operationid",
		}},
		Engine: &contract.Engine{Name: "vacuum", Version: "v0.41.0"},
		Extra:  map[string]any{"findingCounts": map[string]int{"error": 0, "warn": 1, "info": 0, "hint": 0}},
	}
	assertValid(t, schema, out)
}

func TestLintInputV1MatchesSchema(t *testing.T) {
	schema := loadSchema(t, "v1/lint-input.json")
	in := contract.LintInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Document:      contract.NewSpecificationDocument([]byte("openapi: \"3.0.3\"\n"), contract.FileFormatYAML),
		Ruleset:       contract.NewRulesetDocument([]byte("extends: ifc7:recommended\n"), strings.Repeat("b", 64)),
	}
	assertValid(t, schema, in)
}

func TestV0LintOutputStillValidForV0Consumers(t *testing.T) {
	// A v1 output without findings and engine is a valid v0 output.
	schema := loadSchema(t, "lint-output.json")
	assertValid(t, schema, contract.LintOutput{Score: 100, Raw: ""})
}

func TestRulesetDocumentRoundTrip(t *testing.T) {
	r := contract.NewRulesetDocument([]byte("rules: {}\n"), "x")
	b, err := r.Decode()
	if err != nil || string(b) != "rules: {}\n" {
		t.Fatalf("got %q, %v", b, err)
	}
	if _, err := (contract.RulesetDocument{}).Decode(); err == nil {
		t.Fatal("expected error for empty ruleset")
	}
}

func loadSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	dir := filepath.Join("..", "testdata", "schemas")
	compiler := jsonschema.NewCompiler()
	// Load defs first so relative $refs resolve when compiling siblings.
	defsPath := filepath.Join(dir, filepath.Dir(name), "defs.json")
	defsBytes, err := os.ReadFile(defsPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.Compile(defsBytes); err != nil {
		// defs may not be a root schema for validation; ignore compile of defs alone
		_ = err
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	// Inline $refs from defs for standalone validation.
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var defsRoot map[string]any
	if err := json.Unmarshal(defsBytes, &defsRoot); err != nil {
		t.Fatal(err)
	}
	defs, _ := defsRoot["$defs"].(map[string]any)
	inlineRefs(doc, defs)
	inlined, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := jsonschema.NewCompiler().Compile(inlined)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}
	return schema
}

func inlineRefs(node any, defs map[string]any) {
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok {
			if name, ok := defRefName(ref); ok {
				if def, ok := defs[name].(map[string]any); ok {
					// Sibling keywords (for example description) stay; $ref is replaced
					// by the shared definition. Nested #/$defs refs are expanded on the
					// copy first so they are not missed when new map keys are added.
					delete(n, "$ref")
					copied, _ := deepCopyJSON(def).(map[string]any)
					inlineRefs(copied, defs)
					for k, v := range copied {
						if _, exists := n[k]; !exists {
							n[k] = v
						}
					}
				}
			}
		}
		for _, v := range n {
			inlineRefs(v, defs)
		}
	case []any:
		for _, v := range n {
			inlineRefs(v, defs)
		}
	}
}

// defRefName returns the $defs key for a shared-definition reference.
// External refs look like defs.json#/$defs/FileFormat. After a definition is
// inlined, its internal refs look like #/$defs/FileFormat.
func defRefName(ref string) (string, bool) {
	const (
		filePrefix  = "defs.json#/$defs/"
		localPrefix = "#/$defs/"
	)
	var name string
	switch {
	case strings.HasPrefix(ref, filePrefix):
		name = ref[len(filePrefix):]
	case strings.HasPrefix(ref, localPrefix):
		name = ref[len(localPrefix):]
	default:
		return "", false
	}
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}

func deepCopyJSON(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}

func assertValid(t *testing.T, schema *jsonschema.Schema, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	result := schema.Validate(decoded)
	if !result.IsValid() {
		t.Fatalf("schema validation failed: %v\nvalue=%s", result.Errors, string(b))
	}
}

func TestRealLintOutputMatchesV1Schema(t *testing.T) {
	schema := loadSchema(t, "v1/lint-output.json")
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "specs", "openapi-missing-opid.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := openapilint.Plugin{}.Lint(contract.LintInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Document:      contract.NewSpecificationDocument(raw, contract.FileFormatYAML),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Findings) == 0 {
		t.Fatal("expected findings")
	}
	assertValid(t, schema, out)
}

func TestGenerationInputMatchesSchema(t *testing.T) {
	schema := loadSchema(t, "generation-input.json")
	in := contract.GenerationInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Document:      contract.NewSpecificationDocument([]byte("openapi: \"3.0.3\"\n"), contract.FileFormatYAML),
		Target:        contract.GenerationTarget{Language: contract.SdkLanguageGo},
		Package: contract.GenerationPackage{
			Name:    "ifc7.dev/go/org/acme/payments/v2",
			Version: "v2.1.0",
			Details: map[string]any{"packageName": "payments", "goVersion": "1.27"},
		},
		Options: map[string]any{},
	}
	assertValid(t, schema, in)
}

func TestGenerationOutputMatchesSchema(t *testing.T) {
	schema := loadSchema(t, "generation-output.json")
	out := contract.GenerationOutput{
		Files: []contract.GeneratedFile{{Path: "client.gen.go", Contents: "package payments\n"}},
		Raw:   "generated 1 file\n",
		Extra: map[string]any{"operations": 3},
	}
	assertValid(t, schema, out)
}
