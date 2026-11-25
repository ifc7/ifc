package ruleset

import (
	"maps"
	"slices"
)

// Change kinds in a Diff.
const (
	DiffAdded    = "added"
	DiffRemoved  = "removed"
	DiffModified = "modified"
)

// Severity directions of a modified rule.
const (
	DirectionRaised  = "raised"
	DirectionLowered = "lowered"
)

// SeverityInherited is the severity reported for a rule entry that does not
// set one (for example `rule: true`, or an object without severity that
// adjusts a built-in rule): the rule keeps the severity it inherits.
const SeverityInherited = "inherited"

// RuleChange is one rule that differs between two rulesets.
type RuleChange struct {
	RuleID string `json:"ruleId"`
	// Change is added, removed or modified.
	Change      string `json:"change"`
	OldSeverity string `json:"oldSeverity,omitempty"`
	NewSeverity string `json:"newSeverity,omitempty"`
	// Direction is raised or lowered when the severity changed between two
	// known severities.
	Direction string `json:"direction,omitempty"`
	// Tightening is true when the change can add findings at error:
	// an error rule added, or a rule raised to error.
	Tightening bool `json:"tightening"`
}

// ExtendsChange is one built-in ruleset whose extends entry differs.
type ExtendsChange struct {
	Name    string `json:"name"`
	Change  string `json:"change"`
	OldMode string `json:"oldMode,omitempty"`
	NewMode string `json:"newMode,omitempty"`
	// Tightening is true when the built-in adds rules: added, or its mode
	// moved from off to recommended or from recommended to all.
	Tightening bool `json:"tightening"`
}

// Diff is the difference between two effective rulesets.
type Diff struct {
	Extends []ExtendsChange `json:"extends"`
	Rules   []RuleChange    `json:"rules"`
	// Tightening is true when any change is tightening: the new version
	// may fail documents that passed the old one.
	Tightening bool `json:"tightening"`
}

// Empty reports whether d has no changes.
func (d Diff) Empty() bool { return len(d.Extends) == 0 && len(d.Rules) == 0 }

// DiffRulesets compares two resolved rulesets (from Resolve or Effective)
// and lists built-ins whose extends changed and rules that were added,
// removed or changed severity. Pass resolved documents: entries of an
// unresolved extends chain are not expanded. A nil document means Default.
//
// Severities are read as OpenLint writes them: a name (error, warn, info,
// hint, off), a boolean (false is off, true keeps the inherited severity) or
// a rule object's severity. A rule
// object with given and then and no severity is warn, the OpenLint default.
func DiffRulesets(old, cur *Document) Diff {
	if old == nil {
		old = Default()
	}
	if cur == nil {
		cur = Default()
	}
	d := Diff{Extends: []ExtendsChange{}, Rules: []RuleChange{}}

	oldExt, curExt := extendModes(old), extendModes(cur)
	for _, name := range sortedUnion(oldExt, curExt) {
		om, inOld := oldExt[name]
		nm, inCur := curExt[name]
		var c ExtendsChange
		switch {
		case !inOld:
			c = ExtendsChange{Name: name, Change: DiffAdded, NewMode: nm, Tightening: nm != ModeOff}
		case !inCur:
			c = ExtendsChange{Name: name, Change: DiffRemoved, OldMode: om}
		case om != nm:
			c = ExtendsChange{Name: name, Change: DiffModified, OldMode: om, NewMode: nm, Tightening: modeRank(nm) > modeRank(om)}
		default:
			continue
		}
		d.Extends = append(d.Extends, c)
		d.Tightening = d.Tightening || c.Tightening
	}

	oldRules, curRules := old.Rules(), cur.Rules()
	for _, id := range sortedUnion(oldRules, curRules) {
		ov, inOld := oldRules[id]
		nv, inCur := curRules[id]
		var c RuleChange
		switch {
		case !inOld:
			ns := ruleSeverity(nv)
			c = RuleChange{RuleID: id, Change: DiffAdded, NewSeverity: ns, Tightening: ns == "error"}
		case !inCur:
			c = RuleChange{RuleID: id, Change: DiffRemoved, OldSeverity: ruleSeverity(ov)}
		default:
			os, ns := ruleSeverity(ov), ruleSeverity(nv)
			if os == ns && equalJSON(ov, nv) {
				continue
			}
			// A same-severity change (for example a new given) is listed
			// without a direction and is not judged tightening.
			c = RuleChange{RuleID: id, Change: DiffModified, OldSeverity: os, NewSeverity: ns}
			or, okO := severityRanks[os]
			nr, okN := severityRanks[ns]
			if okO && okN && or != nr {
				c.Direction = DirectionLowered
				if nr < or {
					c.Direction = DirectionRaised
				}
			}
			c.Tightening = os != ns && ns == "error"
		}
		d.Rules = append(d.Rules, c)
		d.Tightening = d.Tightening || c.Tightening
	}
	return d
}

// severityRanks orders severities, most severe first.
var severityRanks = map[string]int{"error": 0, "warn": 1, "info": 2, "hint": 3, ModeOff: 4}

func ruleSeverity(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if !t {
			return ModeOff
		}
		return SeverityInherited
	case map[string]any:
		if s, ok := t["severity"]; ok {
			return ruleSeverity(s)
		}
		if _, hasThen := t["then"]; hasThen {
			return "warn"
		}
		return SeverityInherited
	}
	return SeverityInherited
}

func modeRank(m string) int {
	switch m {
	case ModeOff:
		return 0
	case ModeRecommended:
		return 1
	case ModeAll:
		return 2
	}
	return 1
}

func extendModes(d *Document) map[string]string {
	out := map[string]string{}
	for _, e := range d.Extends() {
		out[e.Name] = e.Mode
	}
	return out
}

func sortedUnion[V any](a, b map[string]V) []string {
	keys := slices.Collect(maps.Keys(a))
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

func equalJSON(a, b any) bool {
	ab, _, errA := Canonical(&Document{tree: map[string]any{"v": a}})
	bb, _, errB := Canonical(&Document{tree: map[string]any{"v": b}})
	return errA == nil && errB == nil && string(ab) == string(bb)
}
