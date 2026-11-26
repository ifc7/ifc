package commands

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/project"
	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/governance/ruleset"
)

// govBefore is the released document: two operations, one without an operationId.
const govBefore = `openapi: 3.0.3
info:
  title: Pets
  version: 1.0.0
  description: Pet store.
  contact:
    name: Team
servers:
  - url: https://api.example.com
tags:
  - name: pets
    description: Pets.
paths:
  /pets:
    get:
      summary: List pets
      tags: [pets]
      responses:
        '200':
          description: OK
    delete:
      operationId: deletePets
      summary: Delete pets
      description: Deletes pets.
      tags: [pets]
      responses:
        '204':
          description: Deleted
`

// govAfter removes the delete operation (breaking) and adds an operation without an
// operationId (a new error).
const govAfter = `openapi: 3.0.3
info:
  title: Pets
  version: 1.0.0
  description: Pet store.
  contact:
    name: Team
servers:
  - url: https://api.example.com
tags:
  - name: pets
    description: Pets.
paths:
  /pets:
    get:
      summary: List pets
      tags: [pets]
      responses:
        '200':
          description: OK
  /owners:
    get:
      summary: List owners
      tags: [pets]
      responses:
        '200':
          description: OK
`

// hubRuleset is the recommended ruleset without description-duplication, so
// govAfter has exactly its two operationId errors.
const hubRuleset = `extends:
  - [ifc7:recommended, recommended]
rules:
  description-duplication: "off"
`

const govIfcYAML = `use: []
own:
    - name: rest-api
      ref: ifc7.dev/i/interface-7/rest-api
      path: api/v0.yaml
    - name: new-api
      ref: ""
      path: api/new.yaml
`

func effectiveRuleset(t *testing.T, src string) client.EffectiveRuleset {
	t.Helper()
	doc, b, sum, err := ruleset.Effective(context.Background(), []byte(src), ruleset.BuiltinsOnly)
	if err != nil {
		t.Fatal(err)
	}
	return client.EffectiveRuleset{
		Document:          string(b),
		SHA256:            sum,
		ScoreWeights:      findings.DefaultWeights(),
		ChangeScopedRules: doc.ChangeScopedRules(),
	}
}

// govHub serves rest-api with govBefore released as 1.0.0, under hubRuleset and gates.
func govHub(t *testing.T, gates ...policy.Gate) *fakeHub {
	t.Helper()
	hub := newFakeHub()
	hub.addInterface(restID, "rest-api", govBefore)
	fi := hub.interfaces[restID]
	hub.appendRevision(fi, []byte(govAfter), nil)
	// 1.0.0 is govBefore; 3.0.0 is govAfter, so only a release between them has a breaking baseline.
	fi.releases = []client.InterfaceRelease{
		{InterfaceId: restID, InterfaceRevisionId: fi.revisions[0].Id, SemanticVersion: "1.0.0"},
		{InterfaceId: restID, InterfaceRevisionId: fi.revisions[1].Id, SemanticVersion: "3.0.0"},
	}
	// The latest release on push is 1.0.0, as if 3.0.0 were not there yet.
	fi.ifc.LatestRelease = &fi.releases[0]
	if gates == nil {
		gates = []policy.Gate{}
	}
	hub.policies[restID] = &client.EffectivePolicy{
		Ruleset: effectiveRuleset(t, hubRuleset),
		Gates:   gates,
		Waivers: []policy.Waiver{},
	}
	startHub(t, hub)
	freshCheckout(t, govIfcYAML, map[string]string{"api/v0.yaml": govAfter, "api/new.yaml": govAfter})
	return hub
}

type lintJSON struct {
	Passed     bool                  `json:"passed"`
	Interfaces []project.CheckReport `json:"interfaces"`
}

func decodeLint(t *testing.T, stdout string) lintJSON {
	t.Helper()
	var rep lintJSON
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&rep); err != nil {
		t.Fatalf("stdout is not a lint report: %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Fatalf("stdout has more than one JSON document:\n%s", stdout)
	}
	return rep
}

func wantExit(t *testing.T, err error, code int) {
	t.Helper()
	if got := exitCode(err); err == nil || got != code {
		t.Fatalf("err = %v (exit %d), want exit %d", err, exitCode(err), code)
	}
}

func blocking(name string, mod func(*policy.Gate)) policy.Gate {
	g := policy.Gate{Name: name, On: []policy.Stage{policy.StagePush}, Enforcement: policy.EnforcementBlocking}
	mod(&g)
	return g
}

func TestLintWithoutGatesStillPasses(t *testing.T) {
	govHub(t)
	stdout, _, err := runIfc(t, "lint", "rest-api")
	if err != nil {
		t.Fatalf("lint: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "score:") || strings.Contains(stdout, "gates") {
		t.Fatalf("stdout = %q, want a score and no gates", stdout)
	}
}

func TestLintGatePushBlocksOnTheHubsGates(t *testing.T) {
	hub := govHub(t,
		blocking("no-errors", func(g *policy.Gate) { g.FailOn = policy.FailOnError }),
		blocking("no-breaking", func(g *policy.Gate) { g.Breaking = policy.BreakingBlock }),
		policy.Gate{Name: "release-only", On: []policy.Stage{policy.StageRelease}, FailOn: policy.FailOnError},
	)
	stdout, _, err := runIfc(t, "lint", "rest-api", "--gate", "push", "--json")
	wantExit(t, err, exitGateFailed)
	if !errors.Is(err, project.ErrGateBlocked) {
		t.Fatalf("err = %v, want ErrGateBlocked", err)
	}
	rep := decodeLint(t, stdout)
	if rep.Passed || len(rep.Interfaces) != 1 {
		t.Fatalf("report = %+v", rep)
	}
	r := rep.Interfaces[0]
	if r.Policy == nil || r.Policy.Source != project.PolicyHub || r.Policy.RulesetSHA256 != hub.policies[restID].Ruleset.SHA256 {
		t.Fatalf("policy = %+v, want the hub's ruleset", r.Policy)
	}
	if r.Baseline == nil || r.Baseline.Source != "release" || r.Baseline.Version != "1.0.0" {
		t.Fatalf("baseline = %+v, want release 1.0.0", r.Baseline)
	}
	o := r.Outcome
	if o == nil || o.Passed || len(o.Gates) != 2 || o.BreakingChanges != 1 {
		t.Fatalf("outcome = %+v, want two failed push gates and one breaking change", o)
	}
	if o.Gates[0].Reasons[0].Count != 2 || o.Gates[1].Reasons[0].Code != policy.ReasonBreaking {
		t.Fatalf("gates = %+v", o.Gates)
	}

	// The human report names each gate.
	stdout, _, err = runIfc(t, "lint", "rest-api", "--gate", "push")
	wantExit(t, err, exitGateFailed)
	for _, want := range []string{"gates (push):", "✗ no-errors (blocking): 2 unwaived finding(s) at error or above", "✗ no-breaking (blocking): 1 breaking change(s) against 1.0.0", "compared: release 1.0.0"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestLintGateHonoursWaiversAndScopeNew(t *testing.T) {
	hub := govHub(t, blocking("new-errors", func(g *policy.Gate) {
		g.FailOn = policy.FailOnError
		g.Scope = policy.ScopeNew
	}))
	// One error is in release 1.0.0 already; the other is new.
	stdout, _, err := runIfc(t, "lint", "--gate", "push", "--json", "rest-api")
	wantExit(t, err, exitGateFailed)
	if r := decodeLint(t, stdout).Interfaces[0]; r.Outcome.Gates[0].Reasons[0].Message != "1 new unwaived finding(s) at error or above since release 1.0.0" {
		t.Fatalf("reasons = %+v", r.Outcome.Gates[0].Reasons)
	}

	future := time.Now().Add(24 * time.Hour)
	hub.policies[restID].Waivers = []policy.Waiver{{ID: "waiver_1", RuleID: "operation-operationId", Pointer: "/paths/~1owners/get", Reason: "tracked", ExpiresAt: &future}}
	stdout, _, err = runIfc(t, "lint", "--gate", "push", "--json", "rest-api")
	if err != nil {
		t.Fatalf("lint: %v\n%s", err, stdout)
	}
	rep := decodeLint(t, stdout)
	if !rep.Passed || rep.Interfaces[0].Policy.Waivers != 1 {
		t.Fatalf("report = %+v, want passed with one waiver", rep)
	}
	waived := 0
	for _, f := range rep.Interfaces[0].Findings {
		if f.Status == policy.StatusWaived {
			waived++
		}
	}
	if waived != 1 {
		t.Fatalf("waived findings = %d, want 1", waived)
	}
}

func TestLintAdvisoryGateFailsOnlyWithStrict(t *testing.T) {
	govHub(t, policy.Gate{Name: "advice", On: []policy.Stage{policy.StagePush}, FailOn: policy.FailOnError})
	stdout, _, err := runIfc(t, "lint", "--gate", "push", "rest-api")
	if err != nil {
		t.Fatalf("lint: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "✗ advice (advisory)") {
		t.Fatalf("stdout = %s", stdout)
	}
	_, _, err = runIfc(t, "lint", "--gate", "push", "--strict", "rest-api")
	wantExit(t, err, exitGateFailed)
}

func TestLintBaselineFlags(t *testing.T) {
	govHub(t, blocking("no-breaking", func(g *policy.Gate) { g.Breaking = policy.BreakingBlock }))
	// Against itself there is no breaking change.
	stdout, _, err := runIfc(t, "lint", "--gate", "push", "--baseline", "api/v0.yaml", "rest-api")
	if err != nil {
		t.Fatalf("lint: %v\n%s", err, stdout)
	}
	// With no baseline there is nothing to judge.
	if _, _, err := runIfc(t, "lint", "--gate", "push", "--baseline", "none", "rest-api"); err != nil {
		t.Fatalf("lint --baseline none: %v", err)
	}
}

func TestLintGateRelease(t *testing.T) {
	govHub(t, policy.Gate{Name: "semver", On: []policy.Stage{policy.StageRelease}, Enforcement: policy.EnforcementBlocking, Breaking: policy.BreakingRequireMajor})
	// The baseline is the highest release below the new version, as on the hub.
	stdout, _, err := runIfc(t, "lint", "--gate", "release", "--release-version", "1.1.0", "--json", "rest-api")
	wantExit(t, err, exitGateFailed)
	if r := decodeLint(t, stdout).Interfaces[0]; r.Baseline == nil || r.Baseline.Version != "1.0.0" || r.Outcome.Gates[0].Reasons[0].Code != policy.ReasonMajorRequired {
		t.Fatalf("report = %+v %+v", r.Baseline, r.Outcome)
	}
	if _, _, err := runIfc(t, "lint", "--gate", "release", "--release-version", "2.0.0", "rest-api"); err != nil {
		t.Fatalf("2.0.0: %v", err)
	}
	// Against 3.0.0 (the same document) nothing breaks.
	stdout, _, err = runIfc(t, "lint", "--gate", "release", "--release-version", "3.1.0", "--json", "rest-api")
	if err != nil {
		t.Fatalf("3.1.0: %v\n%s", err, stdout)
	}
	if r := decodeLint(t, stdout).Interfaces[0]; r.Baseline == nil || r.Baseline.Version != "3.0.0" {
		t.Fatalf("baseline = %+v, want 3.0.0", r.Baseline)
	}
	// A first release has no baseline.
	stdout, _, err = runIfc(t, "lint", "--gate", "release", "--release-version", "0.9.0", "--json", "rest-api")
	if err != nil {
		t.Fatalf("0.9.0: %v", err)
	}
	if r := decodeLint(t, stdout).Interfaces[0]; r.Baseline != nil || r.Compare != nil {
		t.Fatalf("report = %+v, want no baseline", r)
	}
	if _, _, err := runIfc(t, "lint", "--gate", "release", "--release-version", "next", "rest-api"); err == nil || exitCode(err) != 1 {
		t.Fatalf("invalid version: err = %v", err)
	}
}

func TestLintCommandLineGatesWithoutAHub(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NO_COLOR", "1")
	t.Setenv("HOME", t.TempDir())
	freshCheckout(t, "use: []\nown: []\n", map[string]string{"before.yaml": govBefore, "after.yaml": govAfter})

	if _, _, err := runIfc(t, "lint", "after.yaml", "--min-score", "50"); err != nil {
		t.Fatalf("lint --min-score 50: %v", err)
	}
	// The default ruleset reports operations without an operationId at error.
	_, _, err := runIfc(t, "lint", "after.yaml", "--fail-on", "error")
	wantExit(t, err, exitGateFailed)
	// Only one of them is new since before.yaml.
	stdout, _, err := runIfc(t, "lint", "after.yaml", "--fail-on", "error", "--new-only", "--baseline", "before.yaml")
	wantExit(t, err, exitGateFailed)
	if !strings.Contains(stdout, "1 new unwaived finding(s) at error or above since before.yaml") {
		t.Fatalf("stdout = %s", stdout)
	}
	stdout, _, err = runIfc(t, "lint", "after.yaml", "--min-score", "100")
	wantExit(t, err, exitGateFailed)
	if !strings.Contains(stdout, "✗ cli (blocking): score") {
		t.Fatalf("stdout = %s", stdout)
	}
	stdout, _, err = runIfc(t, "lint", "after.yaml", "--breaking", "block", "--baseline", "before.yaml", "--json")
	wantExit(t, err, exitGateFailed)
	r := decodeLint(t, stdout).Interfaces[0]
	if r.Baseline == nil || r.Baseline.Source != "file" || r.Compare == nil || r.Compare.Against != "before.yaml" {
		t.Fatalf("report = %+v", r)
	}
}

func TestLintLocalRulesetFetchesPublicReferences(t *testing.T) {
	hub := newFakeHub()
	hub.rulesets["/r/acme/strict/v1"] = hubRuleset
	startHub(t, hub)
	freshCheckout(t, "use: []\nown: []\n", map[string]string{
		"after.yaml":   govAfter,
		"ruleset.yaml": "extends:\n  - https://ifc7.dev/r/acme/strict/v1\n",
		"missing.yaml": "extends:\n  - https://ifc7.dev/r/acme/missing/v1\n",
	})
	stdout, _, err := runIfc(t, "lint", "after.yaml", "--ruleset", "ruleset.yaml", "--fail-on", "error", "--json")
	wantExit(t, err, exitGateFailed)
	r := decodeLint(t, stdout).Interfaces[0]
	if r.Policy.Source != project.PolicyLocal || r.Outcome.Gates[0].Reasons[0].Count != 2 {
		t.Fatalf("report = %+v %+v", r.Policy, r.Outcome)
	}
	_, _, err = runIfc(t, "lint", "after.yaml", "--ruleset", "missing.yaml")
	if err == nil || !strings.Contains(err.Error(), "not found") || exitCode(err) != 1 {
		t.Fatalf("err = %v, want a not found error with exit 1", err)
	}
}

func TestLintHubErrors(t *testing.T) {
	hub := govHub(t)
	// An owned interface without a ref has no hub policy; the other one is still checked.
	stdout, _, err := runIfc(t, "lint", "--hub", "--json")
	wantExit(t, err, 1)
	rep := decodeLint(t, stdout)
	if rep.Passed || len(rep.Interfaces) != 2 || rep.Interfaces[0].Error != "" || !strings.Contains(rep.Interfaces[1].Error, "not an owned interface with a ref") {
		t.Fatalf("report = %+v", rep)
	}

	hub.policyStatus[restID] = 403
	_, _, err = runIfc(t, "lint", "--hub", "rest-api")
	if err == nil || exitCode(err) != 1 {
		t.Fatalf("err = %v, want exit 1", err)
	}
	stdout, _, _ = runIfc(t, "lint", "--hub", "rest-api")
	if !strings.Contains(stdout, "HTTP 403") {
		t.Fatalf("stdout = %s, want the 403 explained", stdout)
	}
}

func TestLintFlagValidation(t *testing.T) {
	govHub(t)
	for _, args := range [][]string{
		{"--gate", "deploy"},
		{"--gate", "release"},
		{"--release-version", "1.0.0"},
		{"--ruleset", "x.yaml", "--hub"},
		{"--new-only"},
		{"--fail-on", "fatal"},
		{"--breaking", "sometimes"},
		{"--baseline-version", "1.0.0"},
	} {
		if _, _, err := runIfc(t, append([]string{"lint", "rest-api"}, args...)...); err == nil || exitCode(err) != 1 {
			t.Errorf("lint %v: err = %v, want a usage error", args, err)
		}
	}
}
