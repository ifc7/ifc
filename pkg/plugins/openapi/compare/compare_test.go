package compare_test

import (
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/openapi/compare"
)

const before = `openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /pets:
    get:
      parameters:
        - {name: limit, in: query, schema: {type: integer}}
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema: {type: object, properties: {id: {type: string}, name: {type: string}}, required: [id]}
  /old:
    get: {responses: {"200": {description: ok}}}
  /beta:
    get:
      x-stability-level: alpha
      parameters:
        - {name: q, in: query, schema: {type: integer}}
      responses: {"200": {description: ok}}
  /draft:
    get: {x-stability-level: draft, responses: {"200": {description: ok}}}
`

const after = `openapi: 3.0.3
info: {title: t, version: "2"}
paths:
  /pets:
    get:
      parameters:
        - {name: limit, in: query, schema: {type: string}}
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema: {type: object, properties: {id: {type: string}}, required: [id]}
    post: {responses: {"201": {description: ok}}}
  /beta:
    get:
      x-stability-level: alpha
      parameters:
        - {name: q, in: query, schema: {type: string}}
      responses: {"200": {description: ok}}
`

func doc(s string) contract.SpecificationDocument {
	return contract.NewSpecificationDocument([]byte(s), contract.FileFormatYAML)
}

func TestPlugin_Compare(t *testing.T) {
	t.Parallel()
	out, err := compare.Plugin{}.Compare(contract.CompareInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Before:        doc(before),
		After:         doc(after),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Breaking {
		t.Fatal("expected a breaking change")
	}
	byPointer := map[string]contract.Change{}
	for _, c := range out.Changes {
		byPointer[c.Pointer] = c
		if len(c.Fingerprint) != 64 {
			t.Errorf("%s: fingerprint %q", c.Pointer, c.Fingerprint)
		}
	}
	for _, want := range []struct {
		pointer, kind string
		breaking      bool
		level         contract.ChangeLevel
		stability     string
	}{
		{"/paths/~1old", "paths.path.removed", true, contract.ChangeLevelError, ""},
		{"/paths/~1draft", "paths.path.removed", false, contract.ChangeLevelWarn, "x-stability-level: draft"},
		{"/paths/~1pets/post", "pathItem.operation.added", false, contract.ChangeLevelWarn, ""},
		{"/paths/~1pets/get/parameters/limit/schema/type", "schema.type.modified", true, contract.ChangeLevelError, ""},
		{"/paths/~1beta/get/parameters/q/schema/type", "schema.type.modified", false, contract.ChangeLevelWarn, "x-stability-level: alpha"},
		{"/paths/~1pets/get/responses/200/content/application~1json/schema/properties/name", "schema.properties.removed", true, contract.ChangeLevelError, ""},
		{"/info/version", "info.version.modified", false, contract.ChangeLevelWarn, ""},
	} {
		got, ok := byPointer[want.pointer]
		if !ok {
			t.Errorf("no change at %s; got %v", want.pointer, out.Changes)
			continue
		}
		if got.Kind != want.kind || got.Breaking != want.breaking || got.Level != want.level || got.Stability != want.stability {
			t.Errorf("%s = %+v", want.pointer, got)
		}
	}
	counts := out.Extra["changeCounts"].(map[string]int)
	if counts["breaking"]+counts["nonBreaking"] != len(out.Changes) || counts["breaking"] != 3 {
		t.Errorf("changeCounts = %v for %d changes", counts, len(out.Changes))
	}
	if out.Changes[0].Breaking != true {
		t.Error("breaking changes sort first")
	}

	t.Run("deterministic", func(t *testing.T) {
		t.Parallel()
		again, _ := compare.Plugin{}.Compare(contract.CompareInput{InterfaceType: contract.InterfaceTypeOpenAPI, Before: doc(before), After: doc(after)})
		if again.Raw != out.Raw {
			t.Fatalf("raw differs between runs:\n%s\n---\n%s", out.Raw, again.Raw)
		}
	})
	t.Run("no changes", func(t *testing.T) {
		t.Parallel()
		same, err := compare.Plugin{}.Compare(contract.CompareInput{InterfaceType: contract.InterfaceTypeOpenAPI, Before: doc(before), After: doc(before)})
		if err != nil || same.Breaking || len(same.Changes) != 0 || same.Changes == nil {
			t.Fatalf("same = %+v, %v", same, err)
		}
	})
	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()
		if _, err := (compare.Plugin{}).Compare(contract.CompareInput{InterfaceType: contract.InterfaceTypeJSONSchema}); err == nil {
			t.Fatal("expected error")
		}
	})
}
