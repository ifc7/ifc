package runner_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/runner"
	"github.com/ifc7/ifc/pkg/spec"
)

// TestNothingFetchesRemoteReferences guards every path a pushed document takes on the hub: none
// of them may load a $ref over the network.
func TestNothingFetchesRemoteReferences(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"type":"string"}`))
	}))
	defer srv.Close()
	ref := srv.URL + "/s.json"
	oas := []byte(`{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/a":{"get":{"responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"` + ref + `"}}}}}}}}}`)
	js := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"a":{"$ref":"` + ref + `"}}}`)
	reg := runner.DefaultRegistry()
	doc := func(b []byte) contract.SpecificationDocument {
		return contract.NewSpecificationDocument(b, contract.FileFormatJSON)
	}
	steps := map[string]func(){
		"spec.ValidateOpenAPI":    func() { _ = spec.ValidateOpenAPI(oas) },
		"spec.ValidateJSONSchema": func() { _ = spec.ValidateJSONSchema(js) },
		"lint OpenAPI": func() {
			_, _, _ = reg.Lint(contract.LintInput{InterfaceType: contract.InterfaceTypeOpenAPI, Document: doc(oas)})
		},
		"lint JSON Schema": func() {
			_, _, _ = reg.Lint(contract.LintInput{InterfaceType: contract.InterfaceTypeJSONSchema, Document: doc(js)})
		},
		"compare OpenAPI": func() {
			_, _, _ = reg.Compare(contract.CompareInput{InterfaceType: contract.InterfaceTypeOpenAPI, Before: doc(oas), After: doc(oas)})
		},
		"compare JSON Schema": func() {
			_, _, _ = reg.Compare(contract.CompareInput{InterfaceType: contract.InterfaceTypeJSONSchema, Before: doc(js), After: doc(js)})
		},
	}
	for name, step := range steps {
		before := hits.Load()
		step()
		if n := hits.Load() - before; n != 0 {
			t.Errorf("%s fetched the remote $ref %d times", name, n)
		}
	}
}
