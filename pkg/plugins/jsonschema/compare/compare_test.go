package compare_test

import (
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/jsonschema/compare"
)

func TestPlugin_Compare(t *testing.T) {
	t.Parallel()
	before := `{"type":"object","required":["a","b"],"properties":{"a":{},"b":{},"gone":{}}}`
	after := `{"type":"array","required":["a","c"],"properties":{"a":{},"b":{},"c":{}}}`
	out, err := compare.Plugin{}.Compare(contract.CompareInput{
		InterfaceType: contract.InterfaceTypeJSONSchema,
		Before:        contract.NewSpecificationDocument([]byte(before), contract.FileFormatJSON),
		After:         contract.NewSpecificationDocument([]byte(after), contract.FileFormatJSON),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]contract.Change{}
	for _, c := range out.Changes {
		got[c.Kind+" "+c.Pointer] = c
	}
	for key, breaking := range map[string]bool{
		"schema.type.modified /type":                 true,
		"schema.required.added /required/c":          true,
		"schema.required.removed /required/b":        false,
		"schema.properties.removed /properties/gone": true,
		"schema.properties.added /properties/c":      false,
	} {
		c, ok := got[key]
		if !ok {
			t.Errorf("missing %s in %v", key, out.Changes)
			continue
		}
		if c.Breaking != breaking {
			t.Errorf("%s breaking = %v", key, c.Breaking)
		}
	}
	if !out.Breaking || len(out.Changes) != 5 {
		t.Fatalf("out = %+v", out)
	}
	again, _ := compare.Plugin{}.Compare(contract.CompareInput{
		InterfaceType: contract.InterfaceTypeJSONSchema,
		Before:        contract.NewSpecificationDocument([]byte(before), contract.FileFormatJSON),
		After:         contract.NewSpecificationDocument([]byte(after), contract.FileFormatJSON),
	})
	if again.Raw != out.Raw {
		t.Fatal("raw output should be deterministic")
	}
}
