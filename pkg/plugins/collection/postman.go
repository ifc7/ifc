package collection

import (
	"fmt"
	"strings"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// validatePostman checks a Postman 2.1 collection's structure, flags
// duplicate item ids and, with a bound OpenAPI document, requests that match
// no operation.
func validatePostman(in Input) (Result, error) {
	doc, _, bad := parseJSON(in.Source)
	if bad != nil {
		return Result{Findings: []contract.Finding{*bad}}, nil
	}
	schema, err := formatSchema("postman-2.1.schema.json")
	if err != nil {
		return Result{}, err
	}
	out := schemaFindings(schema, doc)
	if len(out) > 0 {
		return Result{Findings: out}, nil
	}
	root, _ := doc.(map[string]any)
	ops := indexOpenAPI(in.Interface)
	ids := map[string]string{}
	var walk func(items []any, base string)
	walk = func(items []any, base string) {
		for i, it := range items {
			m, _ := it.(map[string]any)
			p := pointer(base, i)
			if id, ok := m["id"].(string); ok && id != "" {
				if first, dup := ids[id]; dup {
					out = append(out, finding(RuleDuplicateID, contract.SeverityWarn, pointer(p, "id"), fmt.Sprintf("item id %q is already used at %s", id, first)))
				} else {
					ids[id] = p
				}
			}
			if children, ok := m["item"].([]any); ok {
				walk(children, pointer(p, "item"))
				continue
			}
			if ops == nil {
				continue
			}
			method, rawURL := postmanRequest(m["request"])
			if rawURL == "" {
				continue
			}
			if !ops.matchRequest(method, requestSegments(rawURL)) {
				out = append(out, finding(RuleUnknownOperation, contract.SeverityWarn, pointer(p, "request"),
					fmt.Sprintf("%s %s matches no operation in the interface's OpenAPI document", strings.ToUpper(orDefault(method, "GET")), rawURL)))
			}
		}
	}
	items, _ := root["item"].([]any)
	walk(items, "/item")
	return Result{Findings: out}, nil
}

// postmanRequest returns a request's method and URL. A string request is a
// GET of that URL.
func postmanRequest(v any) (string, string) {
	switch r := v.(type) {
	case string:
		return "GET", r
	case map[string]any:
		method, _ := r["method"].(string)
		if method == "" {
			method = "GET"
		}
		switch u := r["url"].(type) {
		case string:
			return method, u
		case map[string]any:
			if raw, ok := u["raw"].(string); ok && raw != "" {
				return method, raw
			}
			switch p := u["path"].(type) {
			case string:
				return method, "/" + strings.TrimPrefix(p, "/")
			case []any:
				var segs []string
				for _, s := range p {
					if str, ok := s.(string); ok {
						segs = append(segs, str)
					} else if m, ok := s.(map[string]any); ok {
						v, _ := m["value"].(string)
						segs = append(segs, v)
					}
				}
				return method, "/" + strings.Join(segs, "/")
			}
		}
	}
	return "", ""
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
