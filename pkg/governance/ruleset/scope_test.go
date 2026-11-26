package ruleset

import (
	"errors"
	"fmt"
	"testing"
)

func TestDocument_ChangeScopedRules(t *testing.T) {
	t.Parallel()
	doc := MustParse([]byte(`extends: [[spectral:oas, recommended]]
x-ifc-scope:
  changed: [operation-tags, info-contact]
rules:
  my-rule:
    given: $.paths
    then: {function: truthy}
    x-ifc-scope: changed
  plain:
    given: $.info
    then: {function: truthy}
`))
	if got := fmt.Sprint(doc.ChangeScopedRules()); got != "[info-contact my-rule operation-tags]" {
		t.Fatalf("scoped = %s", got)
	}
	if got := Default().ChangeScopedRules(); len(got) != 0 {
		t.Fatalf("default scoped = %v", got)
	}
	var nilDoc *Document
	if nilDoc.ChangeScopedRules() != nil {
		t.Fatal("nil document should have no scoped rules")
	}
}

func TestParse_scope(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		doc  string
		ok   bool
	}{
		{"top-level list", "extends: [[spectral:oas, recommended]]\nx-ifc-scope:\n  changed: [operation-tags]\n", true},
		{"top-level not an object", "extends: [[spectral:oas, recommended]]\nx-ifc-scope: changed\n", false},
		{"unknown scope", "extends: [[spectral:oas, recommended]]\nx-ifc-scope:\n  new: [a]\n", false},
		{"list of non-strings", "extends: [[spectral:oas, recommended]]\nx-ifc-scope:\n  changed: [1]\n", false},
		{"rule tag", "rules:\n  r:\n    given: $.paths\n    then: {function: truthy}\n    x-ifc-scope: changed\n", true},
		{"bad rule tag", "rules:\n  r:\n    given: $.paths\n    then: {function: truthy}\n    x-ifc-scope: all\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(tt.doc))
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestOverlay_scopeUnion(t *testing.T) {
	t.Parallel()
	base := MustParse([]byte("extends: [[spectral:oas, recommended]]\nx-ifc-scope:\n  changed: [operation-tags]\n"))
	top := MustParse([]byte("x-ifc-scope:\n  changed: [info-contact]\nrules:\n  info-contact: warn\n"))
	got, err := Overlay(t.Context(), base, top, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := fmt.Sprint(got.ChangeScopedRules()); s != "[info-contact operation-tags]" {
		t.Fatalf("scoped = %s", s)
	}
}
