package ruleset_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ifc7/ifc/pkg/governance/ruleset"
)

func TestParse(t *testing.T) {
	for name, tc := range map[string]struct {
		in      string
		wantErr error
	}{
		"extends builtin":           {in: "extends: spectral:oas\n"},
		"extends tuple":             {in: "extends: [[spectral:oas, all]]\n"},
		"shorthand rules":           {in: "extends: ifc7:recommended\nrules:\n  oas3-api-servers: off\n  operation-tags: error\n  info-contact: false\n"},
		"numeric shorthand":         {in: "extends: spectral:oas\nrules:\n  operation-tags: 0\n", wantErr: ruleset.ErrInvalid},
		"inline rule json":          {in: `{"rules":{"no-x":{"given":"$.info","then":{"field":"x-foo","function":"falsy"},"severity":"warn"}}}`},
		"x- extension":              {in: "extends: spectral:oas\nx-owner: platform\n"},
		"empty":                     {in: "  \n", wantErr: ruleset.ErrInvalid},
		"not an object":             {in: "- a\n- b\n", wantErr: ruleset.ErrInvalid},
		"no extends or rules":       {in: "description: nothing\n", wantErr: ruleset.ErrInvalid},
		"unknown key":               {in: "extends: spectral:oas\nfoo: bar\n", wantErr: ruleset.ErrInvalid},
		"bad severity":              {in: "extends: spectral:oas\nrules:\n  operation-tags: fatal\n", wantErr: ruleset.ErrInvalid},
		"bad mode":                  {in: "extends: [[spectral:oas, some]]\n", wantErr: ruleset.ErrInvalid},
		"functions unsupported":     {in: "extends: spectral:oas\nfunctions: [myFn]\n", wantErr: ruleset.ErrUnsupported},
		"overrides unsupported":     {in: "extends: spectral:oas\noverrides:\n  - files: ['a.yaml']\n    rules:\n      operation-tags: off\n", wantErr: ruleset.ErrUnsupported},
		"parserOptions unsupported": {in: "extends: spectral:oas\nparserOptions:\n  duplicateKeys: warn\n", wantErr: ruleset.ErrUnsupported},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ruleset.Parse([]byte(tc.in))
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestExtends(t *testing.T) {
	d := ruleset.MustParse([]byte("extends:\n  - spectral:oas\n  - [spectral:asyncapi, off]\n"))
	got := fmt.Sprint(d.Extends())
	if got != "[{spectral:oas recommended} {spectral:asyncapi off}]" {
		t.Fatalf("Extends = %s", got)
	}
}

func TestCanonicalIsStable(t *testing.T) {
	a := ruleset.MustParse([]byte("extends: spectral:oas\nrules:\n  b: off\n  a: error\ndescription: x\n"))
	b := ruleset.MustParse([]byte(`{"description":"x","rules":{"a":"error","b":"off"},"extends":"spectral:oas"}`))
	ab, as, err := ruleset.Canonical(a)
	if err != nil {
		t.Fatal(err)
	}
	bb, bs, _ := ruleset.Canonical(b)
	if string(ab) != string(bb) || as != bs {
		t.Fatalf("canonical differs:\n%s\n%s", ab, bb)
	}
	if string(ab) != `{"description":"x","extends":"spectral:oas","rules":{"a":"error","b":"off"}}` {
		t.Fatalf("canonical = %s", ab)
	}
	if len(as) != 64 {
		t.Fatalf("sha256 = %q", as)
	}
}

func TestDefaultEffective(t *testing.T) {
	_, b, sum, err := ruleset.Effective(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"extends":[["ifc7:recommended","recommended"]]}` {
		t.Fatalf("default = %s", b)
	}
	// The default is itself a valid ruleset.
	if _, err := ruleset.Parse(b); err != nil {
		t.Fatal(err)
	}
	// Spelling the default out gives the same sha256.
	_, _, sum2, err := ruleset.Effective(context.Background(), []byte("extends: ifc7:recommended\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if sum != sum2 {
		t.Fatal("equivalent rulesets should hash the same once resolved")
	}
}

type mapResolver map[string]string

func (m mapResolver) Resolve(_ context.Context, ref string) ([]byte, error) {
	s, ok := m[ref]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ruleset.ErrUnresolved, ref)
	}
	return []byte(s), nil
}

func TestResolveChain(t *testing.T) {
	r := mapResolver{
		"org": `
extends: [[spectral:oas, all]]
description: org policy
aliases:
  Info: ["$.info"]
rules:
  operation-tags: error
  no-x-internal:
    given: "#Info"
    severity: error
    then: {field: x-internal, function: falsy}
`,
	}
	iface := ruleset.MustParse([]byte(`
extends: [org, [spectral:oas, recommended]]
rules:
  operation-tags: off
  no-x-internal: warn
  info-contact: false
`))
	eff, err := ruleset.Resolve(context.Background(), iface, r)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := ruleset.Canonical(eff)
	got := string(b)
	for _, want := range []string{
		`"extends":[["spectral:oas","recommended"]]`, // child entry wins over the inherited mode
		`"operation-tags":"off"`,                     // interface overrides org
		`"no-x-internal":{"given":"#Info","severity":"warn","then":{"field":"x-internal","function":"falsy"}}`,
		`"info-contact":false`,
		`"description":"org policy"`,
		`"aliases":{"Info":["$.info"]}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("effective ruleset missing %s\n%s", want, got)
		}
	}
	if strings.Contains(got, `"org"`) {
		t.Errorf("non-built-in extends should be flattened away: %s", got)
	}
	if _, err := ruleset.Parse(b); err != nil {
		t.Fatalf("effective ruleset should be valid OpenLint: %v", err)
	}
}

func TestResolveErrors(t *testing.T) {
	ctx := context.Background()
	_, err := ruleset.Resolve(ctx, ruleset.MustParse([]byte("extends: https://example.com/r.yaml\n")), nil)
	if !errors.Is(err, ruleset.ErrUnresolved) {
		t.Fatalf("builtins-only resolver: %v", err)
	}
	cyc := mapResolver{"a": "extends: b\n", "b": "extends: a\n"}
	_, err = ruleset.Resolve(ctx, ruleset.MustParse([]byte("extends: a\n")), cyc)
	if !errors.Is(err, ruleset.ErrCycle) {
		t.Fatalf("cycle: %v", err)
	}
	bad := mapResolver{"a": "foo: bar\n"}
	_, err = ruleset.Resolve(ctx, ruleset.MustParse([]byte("extends: a\n")), bad)
	if !errors.Is(err, ruleset.ErrInvalid) {
		t.Fatalf("invalid parent: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := ruleset.Resolve(cancelled, ruleset.Default(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

func TestResolveRulesOnly(t *testing.T) {
	d := ruleset.MustParse([]byte(`{"rules":{"r":{"given":"$","then":{"function":"truthy","field":"info"}}}}`))
	eff, err := ruleset.Resolve(context.Background(), d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(eff.Extends()) != 0 || len(eff.Rules()) != 1 {
		t.Fatalf("effective = %+v", eff.Map())
	}
}

func TestOverlay(t *testing.T) {
	ctx := context.Background()
	base := ruleset.MustParse([]byte("extends: spectral:oas\nrules:\n  oas3-api-servers: off\n  operation-tags: error\n"))

	t.Run("nil top is base resolved", func(t *testing.T) {
		got, err := ruleset.Overlay(ctx, base, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := ruleset.Resolve(ctx, base, nil)
		_, gs, _ := ruleset.Canonical(got)
		_, ws, _ := ruleset.Canonical(want)
		if gs != ws {
			t.Fatal("nil top should equal resolved base")
		}
	})

	t.Run("top overrides base", func(t *testing.T) {
		top := ruleset.MustParse([]byte("rules:\n  oas3-api-servers: warn\n"))
		got, err := ruleset.Overlay(ctx, base, top, nil)
		if err != nil {
			t.Fatal(err)
		}
		r := got.Rules()
		if r["oas3-api-servers"] != "warn" || r["operation-tags"] != "error" {
			t.Fatalf("rules = %v", r)
		}
		if fmt.Sprint(got.Extends()) != "[{spectral:oas recommended}]" {
			t.Fatalf("extends = %v", got.Extends())
		}
	})

	t.Run("top built-in extends come after base", func(t *testing.T) {
		top := ruleset.MustParse([]byte("extends: [[spectral:oas, all]]\n"))
		got, err := ruleset.Overlay(ctx, base, top, nil)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(got.Extends()) != "[{spectral:oas all}]" {
			t.Fatalf("extends = %v", got.Extends())
		}
	})

	t.Run("reserved name", func(t *testing.T) {
		top := ruleset.MustParse([]byte("extends: ifc7:internal/overlay-base\n"))
		if _, err := ruleset.Overlay(ctx, base, top, nil); !errors.Is(err, ruleset.ErrInvalid) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("top cannot resolve unknown rulesets", func(t *testing.T) {
		top := ruleset.MustParse([]byte("extends: acme/strict\n"))
		if _, err := ruleset.Overlay(ctx, base, top, nil); !errors.Is(err, ruleset.ErrUnresolved) {
			t.Fatalf("err = %v", err)
		}
	})
}
