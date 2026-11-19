// Package ruleset parses, validates, resolves and canonicalises OpenLint
// rulesets (https://spec.openlint.org/).
//
// The library does no network or database I/O: rulesets named in extends that
// are not built in are fetched through a Resolver supplied by the caller.
// Phase 1 ships built-ins and inline rules only.
package ruleset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/daveshanley/vacuum/model"
	"github.com/daveshanley/vacuum/rulesets"

	"github.com/ifc7/ifc/pkg/spec"
)

// SourceFormat identifies the ruleset language a document is written in.
const SourceFormat = "OPENLINT_1"

// OpenLintContract is the OpenLint draft revision this package validates
// against. It is pinned by the Vacuum version in go.mod.
var OpenLintContract = string(model.OpenLintV1Draft)

// Built-in ruleset names accepted in extends.
const (
	BuiltinSpectralOAS      = "spectral:oas"
	BuiltinSpectralAsyncAPI = "spectral:asyncapi"
	BuiltinSpectralArazzo   = "spectral:arazzo"
	// BuiltinIfc7Recommended is the recommended built-in set for whichever
	// interface type is being linted (spectral:oas for OpenAPI, Vacuum's
	// JSON Schema rules for JSON Schema).
	BuiltinIfc7Recommended = "ifc7:recommended"
)

// Extends modes.
const (
	ModeRecommended = "recommended"
	ModeAll         = "all"
	ModeOff         = "off"
)

// Builtins lists every built-in ruleset name.
var Builtins = []string{BuiltinIfc7Recommended, BuiltinSpectralOAS, BuiltinSpectralAsyncAPI, BuiltinSpectralArazzo}

// IsBuiltin reports whether name is a built-in ruleset.
func IsBuiltin(name string) bool { return slices.Contains(Builtins, name) }

// Errors returned (wrapped) by this package.
var (
	ErrInvalid     = errors.New("invalid OpenLint ruleset")
	ErrUnsupported = errors.New("unsupported OpenLint feature")
	ErrUnresolved  = errors.New("cannot resolve ruleset")
	ErrCycle       = errors.New("ruleset extends cycle")
)

// unsupportedKeys are valid OpenLint but not yet supported by ifc7.
var unsupportedKeys = map[string]string{
	"functions":     "custom functions",
	"functionsDir":  "custom functions",
	"overrides":     "overrides",
	"parserOptions": "parserOptions",
}

// Document is a parsed, validated OpenLint ruleset held as a JSON tree.
type Document struct {
	tree map[string]any
}

// Extend is one entry of a ruleset's extends list.
type Extend struct {
	Name string
	Mode string
}

// Parse parses a JSON or YAML OpenLint ruleset and validates it against the
// OpenLint ruleset schema and ifc7's supported feature set.
func Parse(data []byte) (*Document, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%w: document is empty", ErrInvalid)
	}
	jsonBytes, err := spec.ToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var tree map[string]any
	if err := json.Unmarshal(jsonBytes, &tree); err != nil {
		return nil, fmt.Errorf("%w: ruleset must be an object: %v", ErrInvalid, err)
	}
	if tree == nil {
		return nil, fmt.Errorf("%w: ruleset must be an object", ErrInvalid)
	}
	if _, err := rulesets.CreateRuleSetFromDataWithContract(jsonBytes, model.OpenLintV1Draft); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalid, cleanSchemaError(err))
	}
	for _, k := range slices.Sorted(maps.Keys(unsupportedKeys)) {
		if _, ok := tree[k]; ok {
			return nil, fmt.Errorf("%w: %s (%q) is not supported yet", ErrUnsupported, unsupportedKeys[k], k)
		}
	}
	doc := &Document{tree: tree}
	for _, e := range doc.Extends() {
		switch e.Mode {
		case ModeRecommended, ModeAll, ModeOff:
		default:
			return nil, fmt.Errorf("%w: extends %q has unknown mode %q", ErrInvalid, e.Name, e.Mode)
		}
	}
	return doc, nil
}

// MustParse is Parse that panics on error. For built-in and test documents.
func MustParse(data []byte) *Document {
	d, err := Parse(data)
	if err != nil {
		panic(err)
	}
	return d
}

// Default returns the built-in default ruleset: extends ifc7:recommended.
func Default() *Document {
	return &Document{tree: map[string]any{
		"extends": []any{[]any{BuiltinIfc7Recommended, ModeRecommended}},
	}}
}

// Extends returns the document's extends entries in order. A bare name has
// mode "recommended".
func (d *Document) Extends() []Extend {
	var raw []any
	switch v := d.tree["extends"].(type) {
	case string:
		raw = []any{v}
	case []any:
		raw = v
	}
	out := make([]Extend, 0, len(raw))
	for _, item := range raw {
		switch v := item.(type) {
		case string:
			out = append(out, Extend{Name: v, Mode: ModeRecommended})
		case []any:
			e := Extend{Mode: ModeRecommended}
			if len(v) > 0 {
				e.Name, _ = v[0].(string)
			}
			if len(v) > 1 {
				e.Mode, _ = v[1].(string)
			}
			out = append(out, e)
		}
	}
	return out
}

// Rules returns a copy of the document's rules map.
func (d *Document) Rules() map[string]any {
	r, _ := d.tree["rules"].(map[string]any)
	if r == nil {
		return map[string]any{}
	}
	return deepCopy(r).(map[string]any)
}

// Map returns a deep copy of the document as a JSON tree.
func (d *Document) Map() map[string]any {
	return deepCopy(d.tree).(map[string]any)
}

// Resolver loads a ruleset named in extends that is not built in, such as a
// hub ruleset version or a local file. ref is the extends entry as written.
type Resolver interface {
	Resolve(ctx context.Context, ref string) ([]byte, error)
}

// ResolverFunc adapts a function to Resolver.
type ResolverFunc func(ctx context.Context, ref string) ([]byte, error)

// Resolve calls f.
func (f ResolverFunc) Resolve(ctx context.Context, ref string) ([]byte, error) { return f(ctx, ref) }

// BuiltinsOnly is a Resolver that resolves nothing; only built-ins and
// inline rules are allowed. It is the phase 1 resolver.
var BuiltinsOnly Resolver = ResolverFunc(func(_ context.Context, ref string) ([]byte, error) {
	return nil, fmt.Errorf("%w %q: only built-in rulesets (%v) can be extended", ErrUnresolved, ref, Builtins)
})

// MaxDepth bounds extends chains.
const MaxDepth = 16

// Resolve flattens doc's extends chain into a single effective document
// whose extends lists built-ins only. Non-built-in entries are loaded through
// r (nil means BuiltinsOnly), parsed, and merged parent-first: a child's
// rules, aliases and metadata override its parents'.
func Resolve(ctx context.Context, doc *Document, r Resolver) (*Document, error) {
	if doc == nil {
		return Default(), nil
	}
	if r == nil {
		r = BuiltinsOnly
	}
	out, err := resolve(ctx, doc, r, nil)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func resolve(ctx context.Context, doc *Document, r Resolver, stack []string) (*Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(stack) > MaxDepth {
		return nil, fmt.Errorf("%w: extends chain deeper than %d", ErrInvalid, MaxDepth)
	}
	merged := map[string]any{}
	var builtins []Extend
	addBuiltin := func(e Extend) {
		// Later entries win, matching OpenLint's ordered inheritance.
		builtins = slices.DeleteFunc(builtins, func(x Extend) bool { return x.Name == e.Name })
		builtins = append(builtins, e)
	}
	for _, e := range doc.Extends() {
		if IsBuiltin(e.Name) {
			addBuiltin(e)
			continue
		}
		if slices.Contains(stack, e.Name) {
			return nil, fmt.Errorf("%w: %v -> %s", ErrCycle, stack, e.Name)
		}
		if e.Mode != ModeRecommended {
			return nil, fmt.Errorf("%w: extends mode %q is only supported for built-in rulesets", ErrUnsupported, e.Mode)
		}
		data, err := r.Resolve(ctx, e.Name)
		if err != nil {
			return nil, fmt.Errorf("extends %q: %w", e.Name, err)
		}
		parentDoc, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("extends %q: %w", e.Name, err)
		}
		parent, err := resolve(ctx, parentDoc, r, append(stack, e.Name))
		if err != nil {
			return nil, err
		}
		for _, pe := range parent.Extends() {
			addBuiltin(pe)
		}
		mergeInto(merged, parent.tree)
	}
	mergeInto(merged, doc.tree)
	delete(merged, "extends")
	if len(builtins) > 0 {
		ext := make([]any, 0, len(builtins))
		for _, b := range builtins {
			ext = append(ext, []any{b.Name, b.Mode})
		}
		merged["extends"] = ext
	}
	if _, hasRules := merged["rules"]; !hasRules && len(builtins) == 0 {
		// An effective ruleset needs extends or rules to be valid OpenLint.
		merged["rules"] = map[string]any{}
	}
	return &Document{tree: merged}, nil
}

// mergeInto overlays src onto dst. rules and aliases merge per key; for a
// rule, a shorthand (severity or boolean) adjusts an inherited rule object
// and a rule object is shallow-merged over an inherited one. Other keys are
// replaced.
func mergeInto(dst, src map[string]any) {
	for k, v := range src {
		switch k {
		case "extends":
			continue
		case "rules":
			dr, _ := dst["rules"].(map[string]any)
			if dr == nil {
				dr = map[string]any{}
			}
			sr, _ := v.(map[string]any)
			for name, rv := range sr {
				dr[name] = mergeRule(dr[name], deepCopy(rv))
			}
			dst["rules"] = dr
		case "aliases":
			da, _ := dst["aliases"].(map[string]any)
			if da == nil {
				da = map[string]any{}
			}
			sa, _ := v.(map[string]any)
			for name, av := range sa {
				da[name] = deepCopy(av)
			}
			dst["aliases"] = da
		default:
			dst[k] = deepCopy(v)
		}
	}
}

func mergeRule(parent, child any) any {
	po, parentIsObject := parent.(map[string]any)
	if !parentIsObject {
		return child
	}
	po = deepCopy(po).(map[string]any)
	switch c := child.(type) {
	case map[string]any:
		maps.Copy(po, c)
		return po
	case string, float64:
		po["severity"] = c
		return po
	case bool:
		if !c {
			po["severity"] = ModeOff
		} else if po["severity"] == ModeOff || po["severity"] == float64(-1) {
			delete(po, "severity")
		}
		return po
	}
	return child
}

// Canonical returns stable bytes for doc (JSON with sorted keys, no HTML
// escaping, no insignificant whitespace) and their lowercase hex sha256.
func Canonical(doc *Document) ([]byte, string, error) {
	if doc == nil {
		doc = Default()
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc.tree); err != nil {
		return nil, "", err
	}
	b := bytes.TrimRight(buf.Bytes(), "\n")
	sum := sha256.Sum256(b)
	return b, hex.EncodeToString(sum[:]), nil
}

// Effective parses data (nil or empty means Default), resolves it through r,
// and returns the effective document with its canonical bytes and sha256.
func Effective(ctx context.Context, data []byte, r Resolver) (*Document, []byte, string, error) {
	doc := Default()
	if len(bytes.TrimSpace(data)) > 0 {
		var err error
		if doc, err = Parse(data); err != nil {
			return nil, nil, "", err
		}
	}
	eff, err := Resolve(ctx, doc, r)
	if err != nil {
		return nil, nil, "", err
	}
	b, sum, err := Canonical(eff)
	if err != nil {
		return nil, nil, "", err
	}
	return eff, b, sum, nil
}

// cleanSchemaError drops Vacuum's repeated prefix and the schema file URL
// (which embeds the working directory) from validation errors.
func cleanSchemaError(err error) string {
	msg := strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")
	if first, rest, ok := strings.Cut(msg, "\n"); ok && strings.HasPrefix(first, "jsonschema validation failed") {
		lines := strings.Split(rest, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimSpace(l)
		}
		return strings.Join(lines, "; ")
	}
	return msg
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = deepCopy(x)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, x := range t {
			s[i] = deepCopy(x)
		}
		return s
	default:
		return t
	}
}
