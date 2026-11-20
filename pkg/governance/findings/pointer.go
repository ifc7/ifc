package findings

import (
	"strings"
)

// PointerFromJSONPath converts a normalised JSONPath such as
// $.paths['/pets'].get.responses['200'] or $.servers[0] into a JSON Pointer
// (RFC 6901) such as /paths/~1pets/get/responses/200. The root ("$" or "")
// becomes "". Wildcards and filters cannot be represented and are kept as
// literal segments, so the result is still stable for fingerprinting.
func PointerFromJSONPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "$" {
		return ""
	}
	// Already a pointer.
	if strings.HasPrefix(path, "/") {
		return path
	}
	if strings.HasPrefix(path, "#/") {
		return path[1:]
	}
	path = strings.TrimPrefix(path, "$")

	var segs []string
	for i := 0; i < len(path); {
		switch path[i] {
		case '.':
			i++
			if i < len(path) && path[i] == '.' { // recursive descent, keep literal
				segs = append(segs, "..")
				i++
			}
			j := i
			for j < len(path) && path[j] != '.' && path[j] != '[' {
				j++
			}
			if j > i {
				segs = append(segs, path[i:j])
			}
			i = j
		case '[':
			i++
			if i < len(path) && (path[i] == '\'' || path[i] == '"') {
				q := path[i]
				i++
				var b strings.Builder
				for i < len(path) && path[i] != q {
					if path[i] == '\\' && i+1 < len(path) {
						i++
					}
					b.WriteByte(path[i])
					i++
				}
				segs = append(segs, b.String())
				// skip closing quote and bracket
				for i < len(path) && path[i] != ']' {
					i++
				}
				i++
			} else {
				j := strings.IndexByte(path[i:], ']')
				if j < 0 {
					segs = append(segs, path[i:])
					i = len(path)
				} else {
					segs = append(segs, path[i:i+j])
					i += j + 1
				}
			}
		default:
			j := i
			for j < len(path) && path[j] != '.' && path[j] != '[' {
				j++
			}
			segs = append(segs, path[i:j])
			i = j
		}
	}
	var b strings.Builder
	for _, s := range segs {
		b.WriteByte('/')
		b.WriteString(EscapePointerToken(s))
	}
	return b.String()
}

// EscapePointerToken escapes a JSON Pointer reference token.
func EscapePointerToken(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}
