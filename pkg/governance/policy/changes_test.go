package policy_test

import (
	"errors"
	"testing"

	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

func TestChangeSettings_Validate(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		kinds   map[string]contract.ChangeLevel
		wantErr bool
	}{
		"empty":      {},
		"valid":      {kinds: map[string]contract.ChangeLevel{"schema.properties.added": "error", "info.version.modified": "ignore", "paths.path.removed": "warn"}},
		"bad kind":   {kinds: map[string]contract.ChangeLevel{"removed path": "warn"}, wantErr: true},
		"bad action": {kinds: map[string]contract.ChangeLevel{"paths.path.renamed": "warn"}, wantErr: true},
		"bad level":  {kinds: map[string]contract.ChangeLevel{"paths.path.removed": "fatal"}, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := policy.ChangeSettings{Kinds: tc.kinds}.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if err != nil && !errors.Is(err, policy.ErrInvalid) {
				t.Fatalf("err should wrap ErrInvalid: %v", err)
			}
		})
	}
}

func TestChangeSettings_Apply(t *testing.T) {
	t.Parallel()
	cs := []contract.Change{
		{Kind: "paths.path.removed", Breaking: true, Level: contract.ChangeLevelError},
		{Kind: "info.version.modified", Level: contract.ChangeLevelWarn},
		{Kind: "schema.properties.added", Level: contract.ChangeLevelWarn},
		{Kind: "schema.properties.added", Level: contract.ChangeLevelWarn, Stability: "x-stability-level: draft"},
	}
	got := policy.ChangeSettings{Kinds: map[string]contract.ChangeLevel{
		"paths.path.removed":      contract.ChangeLevelWarn,
		"info.version.modified":   contract.ChangeLevelIgnore,
		"schema.properties.added": contract.ChangeLevelError,
	}}.Apply(cs)
	if len(got) != 3 {
		t.Fatalf("ignored kind should be dropped: %+v", got)
	}
	if got[0].Level != contract.ChangeLevelWarn || got[1].Level != contract.ChangeLevelError || got[2].Level != contract.ChangeLevelWarn {
		t.Fatalf("levels = %+v", got)
	}
	if cs[0].Level != contract.ChangeLevelError {
		t.Fatal("Apply must not modify its input")
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]policy.Version{
		"1.2.3":        {Major: 1, Minor: 2, Patch: 3},
		"v0.4.0":       {Minor: 4},
		"2.0.0-rc.1":   {Major: 2},
		"3.1.4+build7": {Major: 3, Minor: 1, Patch: 4},
	} {
		got, err := policy.ParseVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %+v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "1.2", "1.x.0", "-1.0.0", "latest"} {
		if _, err := policy.ParseVersion(bad); err == nil {
			t.Errorf("ParseVersion(%q) should fail", bad)
		}
	}
}

func TestAllowsBreaking(t *testing.T) {
	t.Parallel()
	v := func(s string) policy.Version {
		x, err := policy.ParseVersion(s)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	for _, tc := range []struct {
		from, to string
		want     bool
	}{
		{"1.4.0", "2.0.0", true},
		{"1.4.0", "1.5.0", false},
		{"1.4.0", "1.4.1", false},
		{"0.3.0", "0.4.0", true},
		{"0.3.0", "0.3.9", false},
		{"0.3.0", "1.0.0", true},
	} {
		if got := policy.AllowsBreaking(v(tc.from), v(tc.to)); got != tc.want {
			t.Errorf("AllowsBreaking(%s, %s) = %v", tc.from, tc.to, got)
		}
	}
}
