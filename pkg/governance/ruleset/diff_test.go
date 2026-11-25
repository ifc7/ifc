package ruleset_test

import (
	"testing"

	"github.com/ifc7/ifc/pkg/governance/ruleset"
)

func TestDiffRulesets(t *testing.T) {
	t.Parallel()
	v1 := ruleset.MustParse([]byte(`
extends: [[spectral:oas, recommended]]
rules:
  operation-tags: warn
  info-contact: error
  legacy-rule: hint
  same: {given: "$.info", then: {function: truthy}, severity: warn}
  reshaped: {given: "$.info", then: {function: truthy}}
`))
	v2 := ruleset.MustParse([]byte(`
extends: [[spectral:oas, all], [spectral:asyncapi, recommended]]
rules:
  operation-tags: error
  info-contact: warn
  same: {given: "$.info", then: {function: truthy}, severity: warn}
  reshaped: {given: "$.paths", then: {function: truthy}}
  new-error: {given: "$", then: {function: truthy}, severity: error}
  new-off: false
`))
	d := ruleset.DiffRulesets(v1, v2)
	if !d.Tightening {
		t.Fatal("v2 should be tightening")
	}
	ext := map[string]ruleset.ExtendsChange{}
	for _, e := range d.Extends {
		ext[e.Name] = e
	}
	if e := ext["spectral:oas"]; e.Change != ruleset.DiffModified || e.OldMode != "recommended" || e.NewMode != "all" || !e.Tightening {
		t.Errorf("spectral:oas = %+v", e)
	}
	if e := ext["spectral:asyncapi"]; e.Change != ruleset.DiffAdded || !e.Tightening {
		t.Errorf("spectral:asyncapi = %+v", e)
	}
	rules := map[string]ruleset.RuleChange{}
	for _, r := range d.Rules {
		rules[r.RuleID] = r
	}
	for id, want := range map[string]ruleset.RuleChange{
		"operation-tags": {RuleID: "operation-tags", Change: ruleset.DiffModified, OldSeverity: "warn", NewSeverity: "error", Direction: ruleset.DirectionRaised, Tightening: true},
		"info-contact":   {RuleID: "info-contact", Change: ruleset.DiffModified, OldSeverity: "error", NewSeverity: "warn", Direction: ruleset.DirectionLowered},
		"legacy-rule":    {RuleID: "legacy-rule", Change: ruleset.DiffRemoved, OldSeverity: "hint"},
		"reshaped":       {RuleID: "reshaped", Change: ruleset.DiffModified, OldSeverity: "warn", NewSeverity: "warn"},
		"new-error":      {RuleID: "new-error", Change: ruleset.DiffAdded, NewSeverity: "error", Tightening: true},
		"new-off":        {RuleID: "new-off", Change: ruleset.DiffAdded, NewSeverity: "off"},
	} {
		if got := rules[id]; got != want {
			t.Errorf("%s = %+v, want %+v", id, got, want)
		}
	}
	if _, listed := rules["same"]; listed {
		t.Error("an unchanged rule should not be listed")
	}

	t.Run("loosening only", func(t *testing.T) {
		t.Parallel()
		d := ruleset.DiffRulesets(v2, ruleset.MustParse([]byte("extends: [[spectral:oas, all]]\nrules:\n  operation-tags: warn\n")))
		if d.Tightening {
			t.Fatalf("loosening should not be tightening: %+v", d)
		}
	})
	t.Run("identical", func(t *testing.T) {
		t.Parallel()
		if d := ruleset.DiffRulesets(v1, v1); !d.Empty() || d.Tightening {
			t.Fatalf("identical = %+v", d)
		}
		if d := ruleset.DiffRulesets(nil, ruleset.Default()); !d.Empty() {
			t.Fatalf("nil is Default: %+v", d)
		}
	})
}
