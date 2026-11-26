package policy

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ifc7/ifc/pkg/governance/ruleset"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const guardOrg = `extends: [[spectral:oas, recommended]]
rules:
  operation-tags: error
  info-contact: warn
`

func TestGuardrails_Validate(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		g    Guardrails
		ok   bool
	}{
		{"empty", Guardrails{}, true},
		{"lock and floor", Guardrails{Locked: []string{"a"}, MinSeverity: map[string]contract.Severity{"b": "warn"}}, true},
		{"empty lock", Guardrails{Locked: []string{" "}}, false},
		{"empty floor id", Guardrails{MinSeverity: map[string]contract.Severity{"": "warn"}}, false},
		{"off floor", Guardrails{MinSeverity: map[string]contract.Severity{"a": "off"}}, false},
		{"unknown floor", Guardrails{MinSeverity: map[string]contract.Severity{"a": "fatal"}}, false},
		{"locked and floored", Guardrails{Locked: []string{"a"}, MinSeverity: map[string]contract.Severity{"a": "warn"}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.g.Validate()
			if tt.ok != (err == nil) {
				t.Fatalf("Validate = %v", err)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestGuardrails_Empty(t *testing.T) {
	t.Parallel()
	if !(Guardrails{}).Empty() || (Guardrails{Locked: []string{"a"}}).Empty() {
		t.Fatal("Empty is wrong")
	}
}

func TestGuardrails_Check(t *testing.T) {
	t.Parallel()
	g := Guardrails{Locked: []string{"operation-tags"}, MinSeverity: map[string]contract.Severity{"info-contact": "warn"}}
	before, err := ruleset.Resolve(t.Context(), ruleset.MustParse([]byte(guardOrg)), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		layer string
		want  string
	}{
		{"unrelated rule", "rules:\n  oas3-api-servers: off\n", "[]"},
		{"locked rule lowered", "rules:\n  operation-tags: warn\n", "[locked:operation-tags]"},
		{"locked rule raised is still a change", "rules:\n  operation-tags:\n    given: $.paths\n    then: {function: truthy}\n    severity: error\n", "[locked:operation-tags]"},
		{"floor kept", "rules:\n  info-contact: warn\n", "[]"},
		{"floor raised", "rules:\n  info-contact: error\n", "[]"},
		{"below floor", "rules:\n  info-contact: hint\n", "[below_floor:info-contact]"},
		{"floored rule off", "rules:\n  info-contact: off\n", "[below_floor:info-contact]"},
		{"built-in lowered", "extends: [[spectral:oas, off]]\n", "[extends_lowered:spectral:oas]"},
		{"built-in raised", "extends: [[spectral:oas, all]]\n", "[]"},
		{"protected rule scoped", "rules: {}\nx-ifc-scope:\n  changed: [operation-tags, oas3-api-servers]\n", "[scoped:operation-tags]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			after, err := ruleset.Overlay(t.Context(), before, ruleset.MustParse([]byte(tt.layer)), nil)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range g.Check("interface", before, after) {
				id := r.RuleID
				if id == "" {
					id = r.Extends
				}
				got = append(got, r.Reason+":"+id)
				if r.Layer != "interface" || r.Message == "" {
					t.Fatalf("refusal = %+v", r)
				}
			}
			if got == nil {
				got = []string{}
			}
			if s := fmt.Sprint(got); s != tt.want {
				t.Fatalf("refusals = %s, want %s", s, tt.want)
			}
		})
	}
	t.Run("no guardrails refuse nothing", func(t *testing.T) {
		t.Parallel()
		after, _ := ruleset.Overlay(t.Context(), before, ruleset.MustParse([]byte("extends: [[spectral:oas, off]]\n")), nil)
		if got := (Guardrails{}).Check("interface", before, after); got != nil {
			t.Fatalf("refusals = %+v", got)
		}
	})
}

func TestMergeLayers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	t.Run("matches Merge for one layer", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct{ org, iface string }{
			{"", ""},
			{"", "rules:\n  oas3-api-servers: off\n"},
			{guardOrg, ""},
			{guardOrg, "rules:\n  operation-tags: warn\n"},
			{guardOrg, "extends: [[spectral:oas, all]]\nrules:\n  info-contact: off\n"},
		} {
			want, err := Merge(ctx, []byte(tt.org), []byte(tt.iface), nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := MergeLayers(ctx, []byte(tt.org), []Layer{{Name: "interface", Ruleset: []byte(tt.iface)}}, Guardrails{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.SHA256 != want.SHA256 || string(got.Bytes) != string(want.Bytes) {
				t.Fatalf("org %q iface %q:\n got %s\nwant %s", tt.org, tt.iface, got.Bytes, want.Bytes)
			}
		}
	})
	t.Run("applies layers in order", func(t *testing.T) {
		t.Parallel()
		got, err := MergeLayers(ctx, []byte(guardOrg), []Layer{
			{Name: "group", Ruleset: []byte("rules:\n  info-contact: hint\n  operation-tags: warn\n")},
			{Name: "empty"},
			{Name: "interface", Ruleset: []byte("rules:\n  info-contact: info\n")},
		}, Guardrails{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		rules := got.Document.Rules()
		if rules["info-contact"] != "info" || rules["operation-tags"] != "warn" {
			t.Fatalf("rules = %v", rules)
		}
	})
	t.Run("collects refusals from every layer", func(t *testing.T) {
		t.Parallel()
		g := Guardrails{Locked: []string{"operation-tags"}, MinSeverity: map[string]contract.Severity{"info-contact": "warn"}}
		_, err := MergeLayers(ctx, []byte(guardOrg), []Layer{
			{Name: "group platform", Ruleset: []byte("rules:\n  operation-tags: warn\n")},
			{Name: "interface", Ruleset: []byte("rules:\n  info-contact: hint\n")},
		}, g, nil)
		var ge *GuardrailError
		if !errors.As(err, &ge) || !errors.Is(err, ErrRefused) || !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v", err)
		}
		if len(ge.Refusals) != 2 || ge.Refusals[0].Layer != "group platform" || ge.Refusals[1].Reason != RefusedFloor {
			t.Fatalf("refusals = %+v", ge.Refusals)
		}
	})
	t.Run("reports invalid layers and organization rulesets", func(t *testing.T) {
		t.Parallel()
		if _, err := MergeLayers(ctx, []byte("rules: 5\n"), nil, Guardrails{}, nil); err == nil {
			t.Fatal("expected an organization ruleset error")
		}
		if _, err := MergeLayers(ctx, nil, []Layer{{Name: "group x", Ruleset: []byte("rules: 5\n")}}, Guardrails{}, nil); err == nil {
			t.Fatal("expected a layer error")
		}
	})
}
