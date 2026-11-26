package collection

import (
	"fmt"
	"strings"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// validateOpenCollection checks an OpenCollection 1.0 document against the
// pinned OpenCollection schema and, with a bound OpenAPI document, flags HTTP
// requests that match no operation.
func validateOpenCollection(in Input) (Result, error) {
	doc, _, bad := parseJSON(in.Source)
	if bad != nil {
		return Result{Findings: []contract.Finding{*bad}}, nil
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return Result{Findings: []contract.Finding{finding(RuleSchema, contract.SeverityError, "", "an OpenCollection document must be an object")}}, nil
	}
	if v, ok := root["opencollection"].(string); !ok || !strings.HasPrefix(v, "1.") {
		return Result{Findings: []contract.Finding{finding(RuleSchema, contract.SeverityError, "/opencollection", fmt.Sprintf("opencollection must be a 1.x version, got %q", fmt.Sprint(root["opencollection"])))}}, nil
	}
	schema, err := formatSchema("opencollection-1.0.0.schema.json")
	if err != nil {
		return Result{}, err
	}
	out := schemaFindings(schema, doc)
	if len(out) > 0 {
		return Result{Findings: out}, nil
	}
	ops := indexOpenAPI(in.Interface)
	if ops == nil {
		return Result{Findings: out}, nil
	}
	var walk func(v any, p string)
	walk = func(v any, p string) {
		switch t := v.(type) {
		case map[string]any:
			if h, ok := t["http"].(map[string]any); ok {
				method, _ := h["method"].(string)
				u, _ := h["url"].(string)
				if u != "" && !ops.matchRequest(method, requestSegments(u)) {
					out = append(out, finding(RuleUnknownOperation, contract.SeverityWarn, pointer(p, "http"),
						fmt.Sprintf("%s %s matches no operation in the interface's OpenAPI document", strings.ToUpper(orDefault(method, "GET")), u)))
				}
			}
			for _, k := range sortedAnyKeys(t) {
				if k == "http" || k == "examples" {
					continue
				}
				walk(t[k], pointer(p, k))
			}
		case []any:
			for i, x := range t {
				walk(x, pointer(p, i))
			}
		}
	}
	walk(root["items"], "/items")
	walk(root["request"], "/request")
	return Result{Findings: out}, nil
}
