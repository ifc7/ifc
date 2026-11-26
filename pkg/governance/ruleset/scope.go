package ruleset

import (
	"fmt"
	"slices"
)

// ScopeKey is the ifc7 extension that limits rules to the parts of a
// document that changed. It may appear in two places:
//
//   - on a rule object (one with given and then): `x-ifc-scope: changed`;
//   - at the top level, to scope rules defined elsewhere, such as built-in
//     ones: `x-ifc-scope: {changed: [rule-id, ...]}`.
//
// A change-scoped rule only reports findings whose JSON Pointer is inside a
// structured change (see findings.ScopeToChanges). Lint engines ignore the
// key; ifc7 applies it to the engine's findings.
const ScopeKey = "x-ifc-scope"

// ScopeChanged is the only scope ifc7 supports.
const ScopeChanged = "changed"

// validateScope checks both forms of x-ifc-scope.
func validateScope(tree map[string]any) error {
	if v, ok := tree[ScopeKey]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %s must be an object with a %q list", ErrInvalid, ScopeKey, ScopeChanged)
		}
		for k, list := range m {
			if k != ScopeChanged {
				return fmt.Errorf("%w: %s has unknown scope %q (only %q is supported)", ErrInvalid, ScopeKey, k, ScopeChanged)
			}
			items, ok := list.([]any)
			if !ok {
				return fmt.Errorf("%w: %s.%s must be a list of rule ids", ErrInvalid, ScopeKey, ScopeChanged)
			}
			for _, it := range items {
				if s, ok := it.(string); !ok || s == "" {
					return fmt.Errorf("%w: %s.%s must be a list of rule ids", ErrInvalid, ScopeKey, ScopeChanged)
				}
			}
		}
	}
	rules, _ := tree["rules"].(map[string]any)
	for id, rv := range rules {
		obj, ok := rv.(map[string]any)
		if !ok {
			continue
		}
		if s, ok := obj[ScopeKey]; ok && s != ScopeChanged {
			return fmt.Errorf("%w: rule %q has %s %v (only %q is supported)", ErrInvalid, id, ScopeKey, s, ScopeChanged)
		}
	}
	return nil
}

// ChangeScopedRules returns the sorted ids of the rules the document limits
// to changed parts of a document: those listed under the top-level
// x-ifc-scope.changed and rule objects tagged x-ifc-scope: changed. Call it on
// a resolved document so every layer's scope is included.
func (d *Document) ChangeScopedRules() []string {
	if d == nil {
		return nil
	}
	seen := map[string]bool{}
	if m, ok := d.tree[ScopeKey].(map[string]any); ok {
		items, _ := m[ScopeChanged].([]any)
		for _, it := range items {
			if s, ok := it.(string); ok && s != "" {
				seen[s] = true
			}
		}
	}
	rules, _ := d.tree["rules"].(map[string]any)
	for id, rv := range rules {
		if obj, ok := rv.(map[string]any); ok && obj[ScopeKey] == ScopeChanged {
			seen[id] = true
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// mergeScope unions src's top-level x-ifc-scope.changed list into dst's, so
// every layer's scoped rules stay scoped.
func mergeScope(dst map[string]any, v any) {
	src, _ := v.(map[string]any)
	ids := map[string]bool{}
	var out []any
	add := func(list any) {
		items, _ := list.([]any)
		for _, it := range items {
			if s, ok := it.(string); ok && !ids[s] {
				ids[s] = true
				out = append(out, s)
			}
		}
	}
	if cur, ok := dst[ScopeKey].(map[string]any); ok {
		add(cur[ScopeChanged])
	}
	add(src[ScopeChanged])
	slices.SortFunc(out, func(a, b any) int {
		as, bs := a.(string), b.(string)
		switch {
		case as < bs:
			return -1
		case as > bs:
			return 1
		}
		return 0
	})
	dst[ScopeKey] = map[string]any{ScopeChanged: out}
}
