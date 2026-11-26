package collection

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// validateTestSuite checks a JSON Schema Test Suite file: its layout, that
// each group's schema compiles, and that each test's data validates the way
// its valid flag says. The bound interface schema is registered first, so a
// group schema can $ref it by its $id.
func validateTestSuite(in Input) (Result, error) {
	doc, _, bad := parseJSON(in.Source)
	if bad != nil {
		return Result{Findings: []contract.Finding{*bad}}, nil
	}
	layout, err := formatSchema("json-schema-test-suite.schema.json")
	if err != nil {
		return Result{}, err
	}
	out := schemaFindings(layout, doc)
	if len(out) > 0 {
		return Result{Findings: out}, nil
	}
	compiler := offlineCompiler()
	if len(in.Interface) > 0 {
		if _, iface, bad := parseJSON(in.Interface); bad == nil {
			// A broken interface schema only means refs to it won't resolve;
			// the group compile reports that.
			_, _ = compiler.Compile(iface)
		}
	}
	groups, _ := doc.([]any)
	for gi, g := range groups {
		gm, _ := g.(map[string]any)
		raw, err := json.Marshal(gm["schema"])
		if err != nil {
			return Result{}, err
		}
		schema, err := compiler.Compile(raw)
		if err != nil {
			out = append(out, finding(RuleTestInvalidSchema, contract.SeverityError, pointer("", gi, "schema"), "schema does not compile: "+err.Error()))
			continue
		}
		tests, _ := gm["tests"].([]any)
		for ti, t := range tests {
			tm, _ := t.(map[string]any)
			want, _ := tm["valid"].(bool)
			got := schema.Validate(tm["data"]).IsValid()
			if got != want {
				desc, _ := tm["description"].(string)
				out = append(out, finding(RuleTestMismatch, contract.SeverityWarn, pointer("", gi, "tests", ti),
					fmt.Sprintf("test %q expects valid: %t, but the data is %s", desc, want, validWord(got))))
			}
		}
	}
	return Result{Findings: out}, nil
}

func validWord(valid bool) string {
	if valid {
		return "valid"
	}
	return "invalid"
}

func sortedAnyKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
