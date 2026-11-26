package check

import (
	"context"
	"testing"
	"time"

	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/governance/ruleset"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// strict raises operation-operationId to error, so each operation without an
// operationId is an error finding.
const strict = `extends:
  - [ifc7:recommended, recommended]
rules:
  operation-operationId: error
`

// scoped is strict with operation-operationId limited to changed parts.
const scoped = strict + `x-ifc-scope:
  changed: [operation-operationId]
`

func effectiveOf(t *testing.T, src string) Policy {
	t.Helper()
	_, b, sum, err := ruleset.Effective(context.Background(), []byte(src), ruleset.BuiltinsOnly)
	if err != nil {
		t.Fatal(err)
	}
	return Policy{Ruleset: b, SHA256: sum}
}

func gate(name string, enf policy.Enforcement, mod func(*policy.Gate)) policy.Gate {
	g := policy.Gate{Name: name, On: []policy.Stage{policy.StagePush, policy.StageRelease}, Enforcement: enf}
	if mod != nil {
		mod(&g)
	}
	return g
}

func run(t *testing.T, p Policy, in Input) *Result {
	t.Helper()
	if in.InterfaceType == "" {
		in.InterfaceType = contract.InterfaceTypeOpenAPI
	}
	if in.Document == nil {
		in.Document = []byte(after)
	}
	res, err := Run(context.Background(), nil, p, in)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func errorsAt(res *Result) []string {
	var out []string
	for _, f := range res.Findings {
		if f.RuleID == "operation-operationId" {
			out = append(out, f.Pointer+":"+string(f.Status))
		}
	}
	return out
}

func TestDefaultPolicyMatchesBuiltinDefault(t *testing.T) {
	res := run(t, Policy{}, Input{})
	_, _, sum, err := ruleset.Effective(context.Background(), nil, ruleset.BuiltinsOnly)
	if err != nil {
		t.Fatal(err)
	}
	if res.RulesetSHA256 != sum {
		t.Fatalf("sha = %s, want the default %s", res.RulesetSHA256, sum)
	}
	if res.Outcome != nil || res.Compare != nil || res.LintError != "" {
		t.Fatalf("unexpected outcome, compare or lint error: %+v", res)
	}
	if res.PluginID == "" || res.Score <= 0 || res.Score > 100 {
		t.Fatalf("plugin %q score %d", res.PluginID, res.Score)
	}
}

func TestUncanonicalRulesetIsCanonicalised(t *testing.T) {
	want := effectiveOf(t, strict)
	res := run(t, Policy{Ruleset: []byte(strict)}, Input{})
	if res.RulesetSHA256 != want.SHA256 {
		t.Fatalf("sha = %s, want %s", res.RulesetSHA256, want.SHA256)
	}
	if got := errorsAt(res); len(got) != 2 {
		t.Fatalf("operation-operationId findings = %v, want 2", got)
	}
}

func TestBlockingGateFailsAndAdvisoryDoesNot(t *testing.T) {
	p := effectiveOf(t, strict)
	p.Gates = []policy.Gate{
		gate("no-errors", policy.EnforcementBlocking, func(g *policy.Gate) { g.FailOn = policy.FailOnError }),
		gate("score", policy.EnforcementAdvisory, func(g *policy.Gate) { n := 100; g.MinScore = &n }),
	}
	res := run(t, p, Input{Stage: policy.StagePush})
	o := res.Outcome
	if o == nil || o.Passed || !o.AdvisoryFailed || len(o.Gates) != 2 {
		t.Fatalf("outcome = %+v, want a failed blocking and a failed advisory gate", o)
	}
	if o.Gates[0].Reasons[0].Code != policy.ReasonFindings || o.Gates[0].Reasons[0].Count != 2 {
		t.Fatalf("no-errors reasons = %+v", o.Gates[0].Reasons)
	}
	if o.Score != res.Score {
		t.Fatalf("outcome score %d != result score %d", o.Score, res.Score)
	}
}

func TestGatesForOtherStagesDoNotRun(t *testing.T) {
	p := effectiveOf(t, strict)
	p.Gates = []policy.Gate{{Name: "release-only", On: []policy.Stage{policy.StageRelease}, Enforcement: policy.EnforcementBlocking, FailOn: policy.FailOnError}}
	if res := run(t, p, Input{Stage: policy.StagePush}); res.Outcome != nil {
		t.Fatalf("outcome = %+v, want nil", res.Outcome)
	}
	if res := run(t, p, Input{Stage: policy.StageRelease}); res.Outcome == nil || res.Outcome.Passed {
		t.Fatalf("release outcome = %+v, want failed", res.Outcome)
	}
}

func TestWaiversKeepTheScoreButPassTheGate(t *testing.T) {
	p := effectiveOf(t, strict)
	p.Gates = []policy.Gate{gate("no-errors", policy.EnforcementBlocking, func(g *policy.Gate) { g.FailOn = policy.FailOnError })}
	unwaived := run(t, p, Input{Stage: policy.StagePush})
	expired := time.Now().Add(-time.Hour)
	p.Waivers = []policy.Waiver{
		{ID: "w1", RuleID: "operation-operationId", Pointer: "/paths/~1pets/get", Reason: "legacy"},
		{ID: "w2", RuleID: "operation-operationId", Pointer: "/paths/~1owners/get", Reason: "soon"},
		{ID: "w3", RuleID: "operation-operationId", Pointer: "/paths/~1owners/get", Reason: "old", ExpiresAt: &expired},
	}
	waived := run(t, p, Input{Stage: policy.StagePush})
	if waived.Score != unwaived.Score {
		t.Fatalf("score changed with waivers: %d -> %d", unwaived.Score, waived.Score)
	}
	if !waived.Outcome.Passed {
		t.Fatalf("outcome = %+v, want passed", waived.Outcome)
	}
	// The expired waiver is ignored, so only an active one can cover a finding.
	p.Waivers = p.Waivers[:1]
	p.Waivers = append(p.Waivers, policy.Waiver{ID: "w3", RuleID: "operation-operationId", Pointer: "/paths/~1owners/get", Reason: "old", ExpiresAt: &expired})
	if res := run(t, p, Input{Stage: policy.StagePush}); res.Outcome.Passed {
		t.Fatalf("outcome passed with an expired waiver: %v", errorsAt(res))
	}
}

func TestBreakingGateJudgesTheBaseline(t *testing.T) {
	p := Policy{Gates: []policy.Gate{gate("no-breaking", policy.EnforcementBlocking, func(g *policy.Gate) { g.Breaking = policy.BreakingBlock })}}
	base := &Baseline{Document: []byte(before), Version: "1.0.0"}

	res := run(t, p, Input{Stage: policy.StagePush, Baseline: base})
	if res.Compare == nil || !res.Compare.Breaking || res.Outcome.Passed || res.Outcome.BreakingChanges != 1 {
		t.Fatalf("compare = %+v outcome = %+v, want one blocking breaking change", res.Compare, res.Outcome)
	}
	if res.Outcome.Gates[0].Reasons[0].Code != policy.ReasonBreaking {
		t.Fatalf("reasons = %+v", res.Outcome.Gates[0].Reasons)
	}

	// Change settings can demote the kind, as on the hub.
	p.ChangeSettings = policy.ChangeSettings{Kinds: map[string]contract.ChangeLevel{"pathItem.operation.removed": contract.ChangeLevelWarn}}
	if res := run(t, p, Input{Stage: policy.StagePush, Baseline: base}); !res.Outcome.Passed {
		t.Fatalf("outcome = %+v, want passed once removal is a warning", res.Outcome)
	}

	// Without a baseline there is nothing to judge.
	p.ChangeSettings = policy.ChangeSettings{}
	if res := run(t, p, Input{Stage: policy.StagePush}); !res.Outcome.Passed || res.Compare != nil {
		t.Fatalf("outcome = %+v, want passed without a baseline", res.Outcome)
	}
}

func TestRequireMajorAtRelease(t *testing.T) {
	p := Policy{Gates: []policy.Gate{gate("semver", policy.EnforcementBlocking, func(g *policy.Gate) { g.Breaking = policy.BreakingRequireMajor })}}
	base := &Baseline{Document: []byte(before), Version: "1.0.0"}
	if res := run(t, p, Input{Stage: policy.StageRelease, Baseline: base, Version: "1.1.0"}); res.Outcome.Passed {
		t.Fatalf("1.0.0 -> 1.1.0 passed with a breaking change")
	}
	if res := run(t, p, Input{Stage: policy.StageRelease, Baseline: base, Version: "2.0.0"}); !res.Outcome.Passed {
		t.Fatalf("1.0.0 -> 2.0.0 failed: %+v", res.Outcome)
	}
	if res := run(t, p, Input{Stage: policy.StagePush, Baseline: base}); !res.Outcome.Passed {
		t.Fatalf("requireMajor failed at push: %+v", res.Outcome)
	}
}

func TestScopeNewCountsOnlyFindingsNotInTheBaseline(t *testing.T) {
	p := effectiveOf(t, strict)
	p.Gates = []policy.Gate{gate("new-errors", policy.EnforcementBlocking, func(g *policy.Gate) {
		g.FailOn = policy.FailOnError
		g.Scope = policy.ScopeNew
	})}
	res := run(t, p, Input{Stage: policy.StagePush, Baseline: &Baseline{Document: []byte(before), Version: "1.0.0"}})
	r := res.Outcome.Gates[0]
	if r.Passed || r.Reasons[0].Count != 1 {
		t.Fatalf("gate = %+v, want one new finding", r)
	}
	if want := "1 new unwaived finding(s) at error or above since release 1.0.0"; r.Reasons[0].Message != want {
		t.Fatalf("message = %q, want %q", r.Reasons[0].Message, want)
	}
	// Against itself nothing is new.
	res = run(t, p, Input{Stage: policy.StagePush, Baseline: &Baseline{Document: []byte(after), Version: "1.1.0"}})
	if !res.Outcome.Passed {
		t.Fatalf("outcome = %+v, want passed", res.Outcome)
	}
}

func TestChangeScopedRulesOnlyReportChangedParts(t *testing.T) {
	p := effectiveOf(t, scoped)
	res := run(t, p, Input{Baseline: &Baseline{Document: []byte(before), Version: "1.0.0"}})
	if got := errorsAt(res); len(got) != 1 || got[0] != "/paths/~1owners/get:open" {
		t.Fatalf("findings = %v, want only the added operation", got)
	}
	if len(res.ScopedOut) != 1 || res.ScopedOut[0].Pointer != "/paths/~1pets/get" {
		t.Fatalf("scopedOut = %+v", res.ScopedOut)
	}
	// Without a baseline the rule applies everywhere.
	if got := errorsAt(run(t, p, Input{})); len(got) != 2 {
		t.Fatalf("findings without a baseline = %v, want 2", got)
	}
}

func TestLintFailureFailsGatesClosed(t *testing.T) {
	p := Policy{Gates: []policy.Gate{gate("any", policy.EnforcementBlocking, nil)}}
	res := run(t, p, Input{InterfaceType: "graphql", Stage: policy.StagePush})
	if res.LintError == "" || res.Outcome == nil || res.Outcome.Passed {
		t.Fatalf("result = %+v, want a lint error and a failed gate", res)
	}
	if res.Outcome.Gates[0].Reasons[0].Code != policy.ReasonLintFailed {
		t.Fatalf("reasons = %+v", res.Outcome.Gates[0].Reasons)
	}
}

func TestInvalidRulesetIsAnError(t *testing.T) {
	if _, err := Run(context.Background(), nil, Policy{Ruleset: []byte("rules: [")}, Input{InterfaceType: contract.InterfaceTypeOpenAPI, Document: []byte(after)}); err == nil {
		t.Fatal("want an error for an invalid ruleset")
	}
}
