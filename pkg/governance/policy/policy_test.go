package policy_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/governance/ruleset"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

func rules(t *testing.T, e *policy.Effective) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.Bytes, &m); err != nil {
		t.Fatal(err)
	}
	r, _ := m["rules"].(map[string]any)
	return r
}

func TestMerge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	org := []byte("extends: spectral:oas\nrules:\n  oas3-api-servers: off\n  operation-tags: error\n")

	t.Run("no interface layer equals the org ruleset", func(t *testing.T) {
		t.Parallel()
		got, err := policy.Merge(ctx, org, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, want, err := ruleset.Effective(ctx, org, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.SHA256 != want {
			t.Fatalf("sha = %s, want %s", got.SHA256, want)
		}
	})

	t.Run("empty org means the built-in default", func(t *testing.T) {
		t.Parallel()
		got, err := policy.Merge(ctx, nil, []byte("  \n"), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, want, _ := ruleset.Effective(ctx, nil, nil)
		if got.SHA256 != want {
			t.Fatal("empty layers should equal the default ruleset")
		}
	})

	t.Run("interface overrides org rules", func(t *testing.T) {
		t.Parallel()
		got, err := policy.Merge(ctx, org, []byte("rules:\n  oas3-api-servers: warn\n  operation-tags: off\n  info-contact: error\n"), nil)
		if err != nil {
			t.Fatal(err)
		}
		r := rules(t, got)
		if r["oas3-api-servers"] != "warn" || r["operation-tags"] != "off" || r["info-contact"] != "error" {
			t.Fatalf("rules = %v", r)
		}
		if !strings.Contains(string(got.Bytes), "spectral:oas") {
			t.Fatalf("org extends lost: %s", got.Bytes)
		}
	})

	t.Run("interface may turn off an org built-in", func(t *testing.T) {
		t.Parallel()
		got, err := policy.Merge(ctx, org, []byte("extends: [[spectral:oas, off]]\nrules: {}\n"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got.Bytes), `["spectral:oas","off"]`) {
			t.Fatalf("extends = %s", got.Bytes)
		}
	})

	t.Run("invalid interface fragment", func(t *testing.T) {
		t.Parallel()
		_, err := policy.Merge(ctx, org, []byte("rules:\n  x: fatal\n"), nil)
		if !errors.Is(err, ruleset.ErrInvalid) || !strings.Contains(err.Error(), "interface ruleset") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("unsupported interface feature", func(t *testing.T) {
		t.Parallel()
		_, err := policy.Merge(ctx, org, []byte("extends: spectral:oas\nfunctions: [f]\n"), nil)
		if !errors.Is(err, ruleset.ErrUnsupported) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("interface cannot extend remote rulesets", func(t *testing.T) {
		t.Parallel()
		_, err := policy.Merge(ctx, org, []byte("extends: https://example.com/r.yaml\n"), nil)
		if !errors.Is(err, ruleset.ErrUnresolved) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("merge is deterministic", func(t *testing.T) {
		t.Parallel()
		frag := []byte("rules:\n  b: off\n  a: error\n")
		a, err := policy.Merge(ctx, org, frag, nil)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := policy.Merge(ctx, org, []byte(`{"rules":{"a":"error","b":"off"}}`), nil)
		if a.SHA256 != b.SHA256 {
			t.Fatal("YAML and JSON fragments with the same rules should merge identically")
		}
	})
}

func TestOrgPolicy_Validate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ok := 80
	bad := 120
	for name, tc := range map[string]struct {
		p       policy.OrgPolicy
		wantErr bool
	}{
		"default":         {p: policy.OrgPolicy{}},
		"full":            {p: policy.OrgPolicy{Ruleset: []byte("extends: spectral:oas\n"), Weights: findings.Weights{contract.SeverityError: 2}, Gates: []policy.Gate{{Name: "g", On: []policy.Stage{policy.StagePush}, MinScore: &ok}}, Waivers: policy.WaiverSettings{MaxExpiryDays: 90, InlineIgnores: policy.InlineIgnoresReport}}},
		"bad ruleset":     {p: policy.OrgPolicy{Ruleset: []byte("rules:\n  x: fatal\n")}, wantErr: true},
		"bad weight":      {p: policy.OrgPolicy{Weights: findings.Weights{contract.SeverityError: -1}}, wantErr: true},
		"bad gate":        {p: policy.OrgPolicy{Gates: []policy.Gate{{Name: "g", On: []policy.Stage{policy.StagePush}, MinScore: &bad}}}, wantErr: true},
		"bad inline":      {p: policy.OrgPolicy{Waivers: policy.WaiverSettings{InlineIgnores: "maybe"}}, wantErr: true},
		"bad expiry":      {p: policy.OrgPolicy{Waivers: policy.WaiverSettings{MaxExpiryDays: -1}}, wantErr: true},
		"change kinds":    {p: policy.OrgPolicy{Changes: policy.ChangeSettings{Kinds: map[string]contract.ChangeLevel{"paths.path.removed": contract.ChangeLevelWarn}}}},
		"bad change kind": {p: policy.OrgPolicy{Changes: policy.ChangeSettings{Kinds: map[string]contract.ChangeLevel{"paths.path.removed": "fatal"}}}, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := tc.p.Validate(ctx, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestWaiverSettings_Validate(t *testing.T) {
	t.Parallel()
	for _, s := range []policy.InlineIgnores{"", policy.InlineIgnoresAllow, policy.InlineIgnoresReport, policy.InlineIgnoresDeny} {
		if err := (policy.WaiverSettings{InlineIgnores: s}).Validate(); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
	}
	if err := (policy.WaiverSettings{InlineIgnores: "x"}).Validate(); !errors.Is(err, policy.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
}

// day is a fixed reference time for waiver tests.
var day = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
