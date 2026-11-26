package collection

import (
	"net/url"
	"regexp"
	"strings"
)

var httpMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query"}

// operations indexes the operations of an OpenAPI document for cross-checks.
type operations struct {
	ids   map[string]bool
	paths []openAPIPath
	root  map[string]any
}

type openAPIPath struct {
	key      string   // the paths key, e.g. /users/{id}
	segments []string // with templates replaced by "*"
	methods  map[string]bool
}

// indexOpenAPI returns the operations of an OpenAPI document, or nil when raw
// is empty or not an OpenAPI document.
func indexOpenAPI(raw []byte) *operations {
	if len(raw) == 0 {
		return nil
	}
	v, _, bad := parseJSON(raw)
	if bad != nil {
		return nil
	}
	root, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if _, ok := root["openapi"]; !ok {
		if _, ok := root["swagger"]; !ok {
			return nil
		}
	}
	ops := &operations{ids: map[string]bool{}, root: root}
	paths, _ := root["paths"].(map[string]any)
	for key, item := range paths {
		p := openAPIPath{key: key, segments: templateSegments(key), methods: map[string]bool{}}
		if m, ok := item.(map[string]any); ok {
			for _, method := range httpMethods {
				op, ok := m[method].(map[string]any)
				if !ok {
					continue
				}
				p.methods[method] = true
				if id, ok := op["operationId"].(string); ok && id != "" {
					ops.ids[id] = true
				}
			}
		}
		ops.paths = append(ops.paths, p)
	}
	return ops
}

var templateParam = regexp.MustCompile(`^\{[^}]*\}$`)

func templateSegments(path string) []string {
	var out []string
	for _, s := range strings.Split(strings.Trim(path, "/"), "/") {
		if s == "" {
			continue
		}
		if templateParam.MatchString(s) {
			s = "*"
		}
		out = append(out, s)
	}
	return out
}

// variableSegment matches a request path segment that is a variable in any of
// the collection formats: :id (Postman), {{id}} (Postman, Bruno) or {id}.
var variableSegment = regexp.MustCompile(`^(:[A-Za-z0-9_.-]+|\{\{[^}]*\}\}|\{[^}]*\})$`)

// requestSegments returns the path segments of a request URL, with variable
// segments replaced by "*". A leading variable host such as {{baseUrl}} is
// dropped, along with the scheme and host of an absolute URL.
func requestSegments(raw string) []string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		raw = u.Path
	} else if strings.Contains(raw, "://") {
		raw = raw[strings.Index(raw, "://")+3:]
		if i := strings.Index(raw, "/"); i >= 0 {
			raw = raw[i:]
		} else {
			raw = ""
		}
	}
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	var out []string
	for i, s := range parts {
		if s == "" {
			continue
		}
		if i == 0 && (strings.HasPrefix(s, "{{") || strings.Contains(s, ".")) && !strings.HasPrefix(raw, "/") {
			continue // a host or {{baseUrl}} before the first slash
		}
		if variableSegment.MatchString(s) {
			s = "*"
		}
		out = append(out, s)
	}
	return out
}

// matchRequest reports whether method and the request path match an
// operation. The OpenAPI path may match the end of the request path, so an
// unknown server base path such as /api/v1 doesn't matter. A "*" on either
// side matches any segment. An empty method matches any method.
func (o *operations) matchRequest(method string, segments []string) bool {
	method = strings.ToLower(strings.TrimSpace(method))
	for _, p := range o.paths {
		if method != "" && !p.methods[method] {
			continue
		}
		if len(p.segments) > len(segments) {
			continue
		}
		tail := segments[len(segments)-len(p.segments):]
		ok := true
		for i, s := range p.segments {
			if s != "*" && tail[i] != "*" && s != tail[i] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// resolvePointer reports whether a JSON Pointer resolves in the document.
func (o *operations) resolvePointer(ptr string) bool {
	if ptr == "" {
		return true
	}
	if !strings.HasPrefix(ptr, "/") {
		return false
	}
	var cur any = o.root
	for _, tok := range strings.Split(ptr[1:], "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		if cur, ok = m[tok]; !ok {
			return false
		}
	}
	return true
}
