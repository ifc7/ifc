package findings

import (
	"strings"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// ScopeToChanges applies change-scoped rules (ruleset.ChangeScopedRules) to
// lint findings. A finding of a scoped rule is kept only when its JSON Pointer
// overlaps a change's pointer: one is the other or lies inside it. Findings of
// other rules are always kept. Both slices keep their input order.
//
// compared says whether the document was compared with a baseline at all.
// Without one (a first push, or `ifc lint` without --baseline) everything is
// new, so nothing is dropped. With one and no changes, every finding of a
// scoped rule is dropped.
func ScopeToChanges(fs []contract.Finding, scoped []string, changes []contract.Change, compared bool) (kept, dropped []contract.Finding) {
	if len(scoped) == 0 || !compared {
		return fs, nil
	}
	isScoped := make(map[string]bool, len(scoped))
	for _, id := range scoped {
		isScoped[id] = true
	}
	kept = make([]contract.Finding, 0, len(fs))
	for _, f := range fs {
		if !isScoped[f.RuleID] || touchesChange(f.Pointer, changes) {
			kept = append(kept, f)
			continue
		}
		dropped = append(dropped, f)
	}
	return kept, dropped
}

func touchesChange(pointer string, changes []contract.Change) bool {
	for _, c := range changes {
		if PointerContains(c.Pointer, pointer) || PointerContains(pointer, c.Pointer) {
			return true
		}
	}
	return false
}

// PointerContains reports whether JSON Pointer inner is outer or lies inside
// it, comparing whole segments ("/paths/~1a" does not contain "/paths/~1ab").
// The root pointer ("") contains every pointer.
func PointerContains(outer, inner string) bool {
	if outer == "" || outer == inner {
		return true
	}
	return strings.HasPrefix(inner, outer+"/")
}
