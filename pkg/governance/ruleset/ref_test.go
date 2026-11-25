package ruleset_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ifc7/ifc/pkg/governance/ruleset"
)

func TestParseRef(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		in      string
		want    ruleset.Ref
		ok      bool
		wantErr bool
	}{
		"pinned":         {in: "https://ifc7.dev/r/interface-7/owasp/v2", want: ruleset.Ref{Owner: "interface-7", Slug: "owasp", Version: 2}, ok: true},
		"unpinned":       {in: "https://ifc7.dev/r/acme/style", want: ruleset.Ref{Owner: "acme", Slug: "style"}, ok: true},
		"http, slash":    {in: "http://IFC7.dev/r/acme/style/v1/", want: ruleset.Ref{Owner: "acme", Slug: "style", Version: 1}, ok: true},
		"builtin":        {in: "spectral:oas"},
		"other host":     {in: "https://example.com/r/acme/style/v1"},
		"other path":     {in: "https://ifc7.dev/i/acme/api"},
		"missing slug":   {in: "https://ifc7.dev/r/acme", ok: true, wantErr: true},
		"bad version":    {in: "https://ifc7.dev/r/acme/style/v0", ok: true, wantErr: true},
		"leading zero":   {in: "https://ifc7.dev/r/acme/style/v01", ok: true, wantErr: true},
		"no v":           {in: "https://ifc7.dev/r/acme/style/3", ok: true, wantErr: true},
		"uppercase slug": {in: "https://ifc7.dev/r/acme/Style/v1", ok: true, wantErr: true},
		"query":          {in: "https://ifc7.dev/r/acme/style/v1?x=1", ok: true, wantErr: true},
		"too deep":       {in: "https://ifc7.dev/r/acme/style/v1/raw", ok: true, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, ok, err := ruleset.ParseRef(tc.in)
			if ok != tc.ok || (err != nil) != tc.wantErr {
				t.Fatalf("ParseRef(%q) = %v, %v, %v", tc.in, got, ok, err)
			}
			if err == nil && got != tc.want {
				t.Fatalf("ParseRef(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
	t.Run("string round trip", func(t *testing.T) {
		t.Parallel()
		r := ruleset.Ref{Owner: "acme", Slug: "style"}.WithVersion(4)
		if r.String() != "https://ifc7.dev/r/acme/style/v4" || !r.Pinned() {
			t.Fatalf("String() = %s", r)
		}
		back, ok, err := ruleset.ParseRef(r.String())
		if !ok || err != nil || back != r {
			t.Fatalf("round trip = %v %v %v", back, ok, err)
		}
	})
}

func TestRefResolver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := map[string]string{
		"https://ifc7.dev/r/interface-7/base/v1": "rules:\n  operation-tags: error\n",
		"https://ifc7.dev/r/acme/style/v1":       "extends: https://ifc7.dev/r/interface-7/base/v1\nrules:\n  info-contact: warn\n",
		"https://ifc7.dev/r/acme/loop-a/v1":      "extends: https://ifc7.dev/r/acme/loop-b/v1\n",
		"https://ifc7.dev/r/acme/loop-b/v1":      "extends: http://ifc7.dev/r/acme/loop-a/v1/\n",
	}
	loads := 0
	r := ruleset.RefResolver(func(_ context.Context, ref ruleset.Ref) ([]byte, error) {
		loads++
		s, ok := store[ref.String()]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ruleset.ErrUnresolved, ref)
		}
		return []byte(s), nil
	})

	t.Run("resolves a chain", func(t *testing.T) {
		eff, err := ruleset.Resolve(ctx, ruleset.MustParse([]byte("extends: https://ifc7.dev/r/acme/style/v1\n")), r)
		if err != nil {
			t.Fatal(err)
		}
		b, _, _ := ruleset.Canonical(eff)
		if !strings.Contains(string(b), `"operation-tags":"error"`) || !strings.Contains(string(b), `"info-contact":"warn"`) {
			t.Fatalf("effective = %s", b)
		}
	})
	for name, tc := range map[string]struct {
		doc  string
		want error
	}{
		"unpinned":       {doc: "extends: https://ifc7.dev/r/acme/style\n", want: ruleset.ErrUnpinned},
		"remote url":     {doc: "extends: https://example.com/ruleset.yaml\n", want: ruleset.ErrUnresolved},
		"local file":     {doc: "extends: ./ruleset.yaml\n", want: ruleset.ErrUnresolved},
		"disguised loop": {doc: "extends: https://ifc7.dev/r/acme/loop-a/v1\n", want: ruleset.ErrCycle},
		"malformed":      {doc: "extends: https://ifc7.dev/r/acme/style/vx\n", want: ruleset.ErrInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ruleset.Resolve(ctx, ruleset.MustParse([]byte(tc.doc)), r)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestResolveLimits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("depth", func(t *testing.T) {
		t.Parallel()
		deep := ruleset.ResolverFunc(func(_ context.Context, ref string) ([]byte, error) {
			var n int
			_, _ = fmt.Sscanf(ref, "r%d", &n)
			return []byte(fmt.Sprintf("extends: r%d\n", n+1)), nil
		})
		_, err := ruleset.Resolve(ctx, ruleset.MustParse([]byte("extends: r0\n")), deep)
		if !errors.Is(err, ruleset.ErrInvalid) || !strings.Contains(err.Error(), "deeper") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("documents", func(t *testing.T) {
		t.Parallel()
		// Each level extends the next level twice under different names, so
		// loads double per level without any chain getting deep.
		wide := ruleset.ResolverFunc(func(_ context.Context, ref string) ([]byte, error) {
			var level int
			var side string
			_, _ = fmt.Sscanf(ref, "l%d-%s", &level, &side)
			if level >= 10 {
				return []byte("rules: {}\n"), nil
			}
			return []byte(fmt.Sprintf("extends: [l%d-a, l%d-b]\n", level+1, level+1)), nil
		})
		_, err := ruleset.Resolve(ctx, ruleset.MustParse([]byte("extends: [l0-a]\n")), wide)
		if !errors.Is(err, ruleset.ErrInvalid) || !strings.Contains(err.Error(), "more than") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRefs(t *testing.T) {
	t.Parallel()
	refs, err := ruleset.Refs(ruleset.MustParse([]byte("extends: [spectral:oas, https://ifc7.dev/r/a/b/v1, [https://ifc7.dev/r/c/d/v2, recommended]]\n")))
	if err != nil || len(refs) != 2 || refs[0].Slug != "b" || refs[1].Version != 2 {
		t.Fatalf("Refs = %v, %v", refs, err)
	}
	if _, err := ruleset.Refs(ruleset.MustParse([]byte("extends: https://ifc7.dev/r/a\n"))); err == nil {
		t.Fatal("malformed ref should fail")
	}
}
