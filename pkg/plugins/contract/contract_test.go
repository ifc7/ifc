package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaptinlin/jsonschema"

	"github.com/ifc7/ifc/pkg/plugins/contract"
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

func loadSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	dir := filepath.Join("..", "testdata", "schemas")
	compiler := jsonschema.NewCompiler()
	// Load defs first so relative $refs resolve when compiling siblings.
	defsPath := filepath.Join(dir, "defs.json")
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
			const prefix = "defs.json#/$defs/"
			if len(ref) > len(prefix) && ref[:len(prefix)] == prefix {
				name := ref[len(prefix):]
				if def, ok := defs[name]; ok {
					// replace map contents with a shallow copy of def
					delete(n, "$ref")
					if defMap, ok := def.(map[string]any); ok {
						for k, v := range defMap {
							if _, exists := n[k]; !exists {
								n[k] = deepCopyJSON(v)
							}
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
