package policy_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

func intp(n int) *int { return &n }

func TestValidateGates(t *testing.T) {
	t.Parallel()
	push := []policy.Stage{policy.StagePush}
	for name, tc := range map[string]struct {
		gates   []policy.Gate
		wantErr bool
	}{
		"none":            {},
		"minimal":         {gates: []policy.Gate{{Name: "g", On: push}}},
		"full":            {gates: []policy.Gate{{Name: "g", On: []policy.Stage{policy.StagePush, policy.StageRelease}, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnWarn, MinScore: intp(70), Breaking: policy.BreakingBlock}}},
		"no name":         {gates: []policy.Gate{{On: push}}, wantErr: true},
		"long name":       {gates: []policy.Gate{{Name: strings.Repeat("n", 101), On: push}}, wantErr: true},
		"no stage":        {gates: []policy.Gate{{Name: "g"}}, wantErr: true},
		"bad stage":       {gates: []policy.Gate{{Name: "g", On: []policy.Stage{"merge"}}}, wantErr: true},
		"bad enforcement": {gates: []policy.Gate{{Name: "g", On: push, Enforcement: "strict"}}, wantErr: true},
		"bad failOn":      {gates: []policy.Gate{{Name: "g", On: push, FailOn: "fatal"}}, wantErr: true},
		"score too high":  {gates: []policy.Gate{{Name: "g", On: push, MinScore: intp(101)}}, wantErr: true},
		"score negative":  {gates: []policy.Gate{{Name: "g", On: push, MinScore: intp(-1)}}, wantErr: true},
		"bad breaking":    {gates: []policy.Gate{{Name: "g", On: push, Breaking: "warn"}}, wantErr: true},
		"duplicate names": {gates: []policy.Gate{{Name: "g", On: push}, {Name: "g", On: push}}, wantErr: true},
		"too many gates":  {gates: make([]policy.Gate, policy.MaxGates+1), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := policy.ValidateGates(tc.gates)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, policy.ErrInvalid) {
				t.Fatalf("err should wrap ErrInvalid: %v", err)
			}
		})
	}
}

func codes(r policy.GateResult) string {
	var out []string
	for _, x := range r.Reasons {
		out = append(out, x.Code)
	}
	return strings.Join(out, ",")
}

func TestEvaluate(t *testing.T) {
	t.Parallel()
	both := []policy.Stage{policy.StagePush, policy.StageRelease}
	errs := policy.ApplyWaivers([]contract.Finding{
		finding("a", "/x", contract.SeverityError),
		finding("b", "/y", contract.SeverityWarn),
	}, nil, nil, "", day)
	waivedErr := policy.ApplyWaivers([]contract.Finding{
		finding("a", "/x", contract.SeverityError),
		finding("b", "/y", contract.SeverityWarn),
	}, nil, []policy.Waiver{{ID: "w", RuleID: "a", Pointer: "/x"}}, "", day)

	for name, tc := range map[string]struct {
		gate      policy.Gate
		in        policy.Input
		wantPass  bool
		wantCodes string
		wantStage bool // whether the stage passes (blocking failures only)
	}{
		"failOn error with an error": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnError},
			in:   policy.Input{Stage: policy.StagePush, Findings: errs}, wantCodes: "findings",
		},
		"waiver clears failOn error": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnError},
			in:   policy.Input{Stage: policy.StagePush, Findings: waivedErr}, wantPass: true, wantStage: true,
		},
		"failOn warn counts errors and warnings": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnWarn},
			in:   policy.Input{Stage: policy.StagePush, Findings: waivedErr}, wantCodes: "findings",
		},
		"failOn none": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnNone},
			in:   policy.Input{Stage: policy.StagePush, Findings: errs}, wantPass: true, wantStage: true,
		},
		"min score fails and waivers do not raise the score": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, MinScore: intp(95)},
			in:   policy.Input{Stage: policy.StagePush, Findings: waivedErr}, wantCodes: "min_score",
		},
		"min score passes": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, MinScore: intp(80)},
			in:   policy.Input{Stage: policy.StagePush, Findings: errs}, wantPass: true, wantStage: true,
		},
		"breaking blocked": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, Breaking: policy.BreakingBlock},
			in:   policy.Input{Stage: policy.StageRelease, Change: &policy.Change{Breaking: true, ComparedTo: "v1.2.0"}}, wantCodes: "breaking_change",
		},
		"breaking allowed": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, Breaking: policy.BreakingAllow},
			in:   policy.Input{Stage: policy.StageRelease, Change: &policy.Change{Breaking: true}}, wantPass: true, wantStage: true,
		},
		"no baseline passes breaking": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, Breaking: policy.BreakingBlock},
			in:   policy.Input{Stage: policy.StageRelease}, wantPass: true, wantStage: true,
		},
		"compare failure fails closed": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, Breaking: policy.BreakingBlock},
			in:   policy.Input{Stage: policy.StageRelease, Change: &policy.Change{Failed: true}}, wantCodes: "compare_failed",
		},
		"lint failure fails closed": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking},
			in:   policy.Input{Stage: policy.StagePush, LintError: "lint took longer than 10s"}, wantCodes: "lint_failed",
		},
		"advisory failure does not block": {
			gate: policy.Gate{Name: "g", On: both, FailOn: policy.FailOnError},
			in:   policy.Input{Stage: policy.StagePush, Findings: errs}, wantCodes: "findings", wantStage: true,
		},
		"several reasons": {
			gate: policy.Gate{Name: "g", On: both, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnError, MinScore: intp(99), Breaking: policy.BreakingBlock},
			in:   policy.Input{Stage: policy.StageRelease, Findings: errs, Change: &policy.Change{Breaking: true}}, wantCodes: "findings,min_score,breaking_change",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			o := policy.Evaluate([]policy.Gate{tc.gate}, tc.in)
			if len(o.Gates) != 1 {
				t.Fatalf("gates = %d", len(o.Gates))
			}
			r := o.Gates[0]
			if r.Passed != tc.wantPass || codes(r) != tc.wantCodes {
				t.Fatalf("passed=%v codes=%q, want %v %q (%+v)", r.Passed, codes(r), tc.wantPass, tc.wantCodes, r.Reasons)
			}
			if o.Passed != tc.wantStage {
				t.Fatalf("stage passed = %v, want %v", o.Passed, tc.wantStage)
			}
			if o.AdvisoryFailed != (!r.Passed && !tc.gate.Blocking()) {
				t.Fatalf("advisoryFailed = %v", o.AdvisoryFailed)
			}
		})
	}

	t.Run("gates for other stages are skipped", func(t *testing.T) {
		t.Parallel()
		o := policy.Evaluate([]policy.Gate{
			{Name: "push-only", On: []policy.Stage{policy.StagePush}, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnError},
			{Name: "release-only", On: []policy.Stage{policy.StageRelease}, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnError},
		}, policy.Input{Stage: policy.StageRelease, Findings: errs})
		if len(o.Gates) != 1 || o.Gates[0].Gate != "release-only" || o.Passed {
			t.Fatalf("outcome = %+v", o)
		}
	})

	t.Run("no gates passes with score and counts", func(t *testing.T) {
		t.Parallel()
		o := policy.Evaluate(nil, policy.Input{Stage: policy.StagePush, Findings: waivedErr})
		if !o.Passed || len(o.Gates) != 0 {
			t.Fatalf("outcome = %+v", o)
		}
		if o.Counts.Open[contract.SeverityWarn] != 1 || o.Counts.Waived[contract.SeverityError] != 1 || o.Counts.Open[contract.SeverityError] != 0 {
			t.Fatalf("counts = %+v", o.Counts)
		}
		if o.Score != policy.ScoreOf(waivedErr, nil) {
			t.Fatal("score mismatch")
		}
	})

	t.Run("failed lists blocking first", func(t *testing.T) {
		t.Parallel()
		o := policy.Evaluate([]policy.Gate{
			{Name: "advisory", On: []policy.Stage{policy.StagePush}, FailOn: policy.FailOnError},
			{Name: "blocking", On: []policy.Stage{policy.StagePush}, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnError},
		}, policy.Input{Stage: policy.StagePush, Findings: errs})
		f := o.Failed()
		if len(f) != 2 || f[0].Gate != "blocking" || f[1].Gate != "advisory" {
			t.Fatalf("failed = %+v", f)
		}
	})
}
