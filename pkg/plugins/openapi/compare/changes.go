package compare

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	wcmodel "github.com/pb33f/libopenapi/what-changed/model"

	"github.com/ifc7/ifc/pkg/governance/changes"
	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/spec"
)

// StabilityExtension marks an operation's stability. Changes to an
// operation at an unstable level are never breaking.
const StabilityExtension = "x-stability-level"

// UnstableLevels are the x-stability-level values whose operations may
// change without a breaking change.
var UnstableLevels = []string{"draft", "alpha"}

var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true, "options": true,
	"head": true, "patch": true, "trace": true, "query": true,
}

var statusCode = regexp.MustCompile(`^([1-5][0-9X]{2}|default)$`)

// fieldSegments maps a what-changed field's json tag to the OpenAPI key it
// stands for. An empty segment means the field holds a map whose keys are
// the OpenAPI keys (paths, response codes).
var fieldSegments = map[string]string{
	"pathItems":            "",
	"response":             "",
	"requestBodies":        "requestBody",
	"externalDoc":          "externalDocs",
	"securityRequirements": "security",
	"extensions":           "",
	"itemSchemas":          "itemSchema",
}

// collectChanges walks libopenapi's change tree and flattens it into
// structured change records with JSON Pointers into the documents.
func collectChanges(root *wcmodel.DocumentChanges, before, after map[string]any) []contract.Change {
	w := &walker{before: before, after: after}
	w.walk(reflect.ValueOf(root), "", "document")
	changes.Sort(w.out)
	return w.out
}

type walker struct {
	before, after map[string]any
	out           []contract.Change
}

// walk visits v, a what-changed node at pointer ptr. object names the
// nearest enclosing *Changes struct, which owns any leaf Change found.
func (w *walker) walk(v reflect.Value, ptr, object string) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return
		}
		if c, ok := v.Interface().(*wcmodel.Change); ok {
			w.add(c, ptr, object)
			return
		}
		w.walk(v.Elem(), ptr, object)
	case reflect.Struct:
		t := v.Type()
		if name := t.Name(); strings.HasSuffix(name, "Changes") && name != "PropertyChanges" {
			object = lowerFirst(strings.TrimSuffix(name, "Changes"))
		}
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if f.Anonymous {
				w.walk(v.Field(i), ptr, object)
				continue
			}
			tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if tag == "" || tag == "-" {
				continue
			}
			if tag == "changes" {
				// PropertyChanges.Changes: the leaf changes of this object.
				w.walk(v.Field(i), ptr, object)
				continue
			}
			seg, mapped := fieldSegments[tag]
			if !mapped {
				seg = tag
			}
			if tag == "schemas" && object != "components" {
				seg = "schema"
			}
			child := ptr
			if seg != "" {
				child += "/" + findings.EscapePointerToken(seg)
			}
			w.walk(v.Field(i), child, object)
		}
	case reflect.Slice:
		for i := range v.Len() {
			el := v.Index(i)
			seg := fmt.Sprint(i)
			if p, ok := el.Interface().(*wcmodel.ParameterChanges); ok && p != nil && p.Name != "" {
				seg = p.Name
			}
			if _, leaf := el.Interface().(*wcmodel.Change); leaf {
				w.walk(el, ptr, object)
				continue
			}
			w.walk(el, ptr+"/"+findings.EscapePointerToken(seg), object)
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			w.walk(iter.Value(), ptr+"/"+findings.EscapePointerToken(fmt.Sprint(iter.Key().Interface())), object)
		}
	}
}

func (w *walker) add(c *wcmodel.Change, ptr, object string) {
	action := changes.ActionModified
	switch c.ChangeType {
	case wcmodel.PropertyAdded, wcmodel.ObjectAdded:
		action = changes.ActionAdded
	case wcmodel.PropertyRemoved, wcmodel.ObjectRemoved:
		action = changes.ActionRemoved
	}
	property := propertyKind(object, c.Property)
	pointer := ptr
	if c.Property != "" {
		pointer += "/" + findings.EscapePointerToken(c.Property)
	}
	// An added or removed schema property, or required entry, is named by
	// its value.
	if object == "schema" && (c.Property == "properties" || c.Property == "required") && action != changes.ActionModified {
		if name := firstNonEmpty(c.New, c.Original); name != "" {
			pointer += "/" + findings.EscapePointerToken(name)
		}
	}
	ch := changes.New(changes.Kind(object, property, action), pointer, c.Breaking, message(c, action))
	if ch.Breaking {
		doc := w.after
		if action == changes.ActionRemoved {
			doc = w.before
		}
		if level := unstableLevel(doc, pointer); level != "" {
			ch.Breaking = false
			ch.Level = contract.ChangeLevelWarn
			ch.Stability = StabilityExtension + ": " + level
		}
	}
	w.out = append(w.out, ch)
}

// propertyKind is the property segment of a change kind. Keys that are
// names rather than fixed fields (paths, methods, status codes, extension
// keys) are replaced by what they name, so kinds stay a small, fixed set.
func propertyKind(object, property string) string {
	switch {
	case property == "":
		return "value"
	case object == "paths" && strings.HasPrefix(property, "/"):
		return "path"
	case object == "pathItem" && httpMethods[strings.ToLower(property)]:
		return "operation"
	case object == "responses" && statusCode.MatchString(property):
		return "response"
	case strings.HasPrefix(property, "x-"):
		return "extension"
	}
	return property
}

func message(c *wcmodel.Change, action string) string {
	switch action {
	case changes.ActionAdded:
		if c.New != "" && c.New != c.Property {
			return fmt.Sprintf("added %s %q", c.Property, c.New)
		}
		return fmt.Sprintf("added %s", c.Property)
	case changes.ActionRemoved:
		if c.Original != "" && c.Original != c.Property {
			return fmt.Sprintf("removed %s %q", c.Property, c.Original)
		}
		return fmt.Sprintf("removed %s", c.Property)
	}
	return fmt.Sprintf("changed %s from %q to %q", c.Property, c.Original, c.New)
}

// unstableLevel returns the unstable x-stability-level that exempts the
// change at pointer, or "". A change inside an operation is exempt when the
// operation is unstable; a change to a whole path item is exempt when every
// operation in it is unstable.
func unstableLevel(doc map[string]any, pointer string) string {
	if doc == nil {
		return ""
	}
	rest, ok := strings.CutPrefix(pointer, "/paths/")
	if !ok {
		return ""
	}
	parts := strings.SplitN(rest, "/", 3)
	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths[unescape(parts[0])].(map[string]any)
	if item == nil {
		return ""
	}
	if len(parts) >= 2 && httpMethods[parts[1]] {
		op, _ := item[parts[1]].(map[string]any)
		return stability(op)
	}
	if len(parts) == 1 {
		level := ""
		for m, v := range item {
			if !httpMethods[m] {
				continue
			}
			op, _ := v.(map[string]any)
			l := stability(op)
			if l == "" {
				return ""
			}
			level = l
		}
		return level
	}
	return ""
}

func stability(op map[string]any) string {
	s, _ := op[StabilityExtension].(string)
	s = strings.ToLower(strings.TrimSpace(s))
	for _, u := range UnstableLevels {
		if s == u {
			return s
		}
	}
	return ""
}

func unescape(tok string) string {
	return strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
}

// parseTree decodes a JSON or YAML document into a generic tree, or nil.
func parseTree(b []byte) map[string]any {
	j, err := spec.ToJSON(b)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(j, &m) != nil {
		return nil
	}
	return m
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
