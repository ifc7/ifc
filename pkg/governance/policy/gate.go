package policy

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// Stage is where a gate runs.
type Stage string

const (
	StagePush    Stage = "push"
	StageRelease Stage = "release"
)

// Stages lists every stage.
var Stages = []Stage{StagePush, StageRelease}

// Enforcement decides what a failing gate does.
type Enforcement string

const (
	// EnforcementAdvisory records a failure without blocking. It is the
	// default, so a new gate never starts rejecting work unannounced.
	EnforcementAdvisory Enforcement = "advisory"
	// EnforcementBlocking rejects the push or release.
	EnforcementBlocking Enforcement = "blocking"
)

// FailOn is the lowest severity that fails a gate. none never fails on
// findings.
type FailOn string

const (
	FailOnError FailOn = "error"
	FailOnWarn  FailOn = "warn"
	FailOnInfo  FailOn = "info"
	FailOnHint  FailOn = "hint"
	FailOnNone  FailOn = "none"
)

// Breaking decides what a breaking change does to a gate.
type Breaking string

const (
	BreakingAllow Breaking = "allow"
	BreakingBlock Breaking = "block"
)

// Gate is one governance gate.
type Gate struct {
	// Name identifies the gate in results. Unique within a policy.
	Name string `json:"name"`
	// On lists the stages the gate runs at.
	On []Stage `json:"on"`
	// Enforcement is advisory (default) or blocking.
	Enforcement Enforcement `json:"enforcement,omitempty"`
	// FailOn fails the gate when any counted, unwaived finding is at this
	// severity or above. Empty means none.
	FailOn FailOn `json:"failOn,omitempty"`
	// MinScore fails the gate when the score is below it.
	MinScore *int `json:"minScore,omitempty"`
	// Breaking block fails the gate on a breaking change. Empty means allow.
	Breaking Breaking `json:"breaking,omitempty"`
}

// RunsAt reports whether g runs at stage s.
func (g Gate) RunsAt(s Stage) bool { return slices.Contains(g.On, s) }

// Blocking reports whether a failure of g blocks.
func (g Gate) Blocking() bool { return g.Enforcement == EnforcementBlocking }

// MaxGates bounds the gates in one policy.
const MaxGates = 20

// ValidateGates checks every gate and that names are unique.
func ValidateGates(gs []Gate) error {
	if len(gs) > MaxGates {
		return fmt.Errorf("%w: at most %d gates", ErrInvalid, MaxGates)
	}
	seen := map[string]bool{}
	for i, g := range gs {
		if err := g.Validate(); err != nil {
			return fmt.Errorf("gate %d: %w", i, err)
		}
		if seen[g.Name] {
			return fmt.Errorf("%w: gate name %q is used twice", ErrInvalid, g.Name)
		}
		seen[g.Name] = true
	}
	return nil
}

// Validate checks one gate.
func (g Gate) Validate() error {
	if strings.TrimSpace(g.Name) == "" || len(g.Name) > 100 {
		return fmt.Errorf("%w: gate name is required (at most 100 characters)", ErrInvalid)
	}
	if len(g.On) == 0 {
		return fmt.Errorf("%w: gate %q needs at least one stage in on (push, release)", ErrInvalid, g.Name)
	}
	for _, s := range g.On {
		if !validStage(s) {
			return fmt.Errorf("%w: gate %q has unknown stage %q (push, release)", ErrInvalid, g.Name, s)
		}
	}
	switch g.Enforcement {
	case "", EnforcementAdvisory, EnforcementBlocking:
	default:
		return fmt.Errorf("%w: gate %q enforcement must be advisory or blocking", ErrInvalid, g.Name)
	}
	switch g.FailOn {
	case "", FailOnError, FailOnWarn, FailOnInfo, FailOnHint, FailOnNone:
	default:
		return fmt.Errorf("%w: gate %q failOn must be error, warn, info, hint or none", ErrInvalid, g.Name)
	}
	if g.MinScore != nil && (*g.MinScore < 0 || *g.MinScore > 100) {
		return fmt.Errorf("%w: gate %q minScore must be 0 to 100", ErrInvalid, g.Name)
	}
	switch g.Breaking {
	case "", BreakingAllow, BreakingBlock:
	default:
		return fmt.Errorf("%w: gate %q breaking must be allow or block", ErrInvalid, g.Name)
	}
	return nil
}

// severityRank is 0 for error, rising to hint.
func severityRank(s contract.Severity) int {
	if i := slices.Index(contract.Severities, s); i >= 0 {
		return i
	}
	return len(contract.Severities)
}

// failsOn reports whether a finding of severity s fails failOn f.
func failsOn(f FailOn, s contract.Severity) bool {
	switch f {
	case "", FailOnNone:
		return false
	}
	return severityRank(s) <= severityRank(contract.Severity(f))
}

// Change is the compare result a gate judges breaking changes on.
type Change struct {
	// Breaking is the compare plugin's verdict.
	Breaking bool `json:"breaking"`
	// ComparedTo names the baseline, for example the superseded release.
	ComparedTo string `json:"comparedTo,omitempty"`
	// Failed means compare could not run.
	Failed bool `json:"failed,omitempty"`
}

// Input is everything a stage's gates are evaluated on.
type Input struct {
	Stage Stage
	// Findings are the lint findings after ApplyWaivers.
	Findings []Evaluated
	Weights  findings.Weights
	// LintError is set when lint could not produce findings (for example
	// it timed out). Every gate that runs at the stage then fails closed.
	LintError string
	// Change is the compare result, or nil when there is no baseline (for
	// example the first release).
	Change *Change
}

// Reason codes.
const (
	ReasonFindings      = "findings"
	ReasonMinScore      = "min_score"
	ReasonBreaking      = "breaking_change"
	ReasonLintFailed    = "lint_failed"
	ReasonCompareFailed = "compare_failed"
)

// Reason explains one gate failure.
type Reason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Count is the number of findings behind a findings reason.
	Count int `json:"count,omitempty"`
}

// GateResult is one gate's outcome.
type GateResult struct {
	Gate        string      `json:"gate"`
	Enforcement Enforcement `json:"enforcement"`
	Passed      bool        `json:"passed"`
	Reasons     []Reason    `json:"reasons,omitempty"`
}

// Counts are evaluated findings by status and severity.
type Counts struct {
	Open    map[contract.Severity]int `json:"open"`
	Waived  map[contract.Severity]int `json:"waived"`
	Ignored map[contract.Severity]int `json:"ignored"`
}

// Outcome is a stage's result: the score, counts and every gate that ran.
type Outcome struct {
	Stage Stage `json:"stage"`
	// Passed is false when any blocking gate failed.
	Passed bool `json:"passed"`
	// AdvisoryFailed is true when an advisory gate failed.
	AdvisoryFailed bool         `json:"advisoryFailed"`
	Score          int          `json:"score"`
	Counts         Counts       `json:"counts"`
	Gates          []GateResult `json:"gates"`
}

// Failed returns the gates that failed, blocking first.
func (o *Outcome) Failed() []GateResult {
	var out []GateResult
	for _, g := range o.Gates {
		if !g.Passed && g.Enforcement == EnforcementBlocking {
			out = append(out, g)
		}
	}
	for _, g := range o.Gates {
		if !g.Passed && g.Enforcement != EnforcementBlocking {
			out = append(out, g)
		}
	}
	return out
}

// Evaluate runs every gate in gs that runs at in.Stage. Gates for other
// stages are skipped and not reported.
func Evaluate(gs []Gate, in Input) *Outcome {
	o := &Outcome{
		Stage:  in.Stage,
		Passed: true,
		Score:  ScoreOf(in.Findings, in.Weights),
		Counts: countsOf(in.Findings),
		Gates:  []GateResult{},
	}
	for _, g := range gs {
		if !g.RunsAt(in.Stage) {
			continue
		}
		r := evaluateGate(g, in, o.Score)
		if !r.Passed {
			if g.Blocking() {
				o.Passed = false
			} else {
				o.AdvisoryFailed = true
			}
		}
		o.Gates = append(o.Gates, r)
	}
	return o
}

func evaluateGate(g Gate, in Input, score int) GateResult {
	enf := g.Enforcement
	if enf == "" {
		enf = EnforcementAdvisory
	}
	r := GateResult{Gate: g.Name, Enforcement: enf}
	if in.LintError != "" {
		r.Reasons = append(r.Reasons, Reason{Code: ReasonLintFailed, Message: "lint did not complete: " + in.LintError})
		return r
	}
	if g.FailOn != "" && g.FailOn != FailOnNone {
		n := 0
		for _, e := range in.Findings {
			if e.countsForGate() && failsOn(g.FailOn, e.Severity) {
				n++
			}
		}
		if n > 0 {
			r.Reasons = append(r.Reasons, Reason{
				Code:    ReasonFindings,
				Message: fmt.Sprintf("%d unwaived finding(s) at %s or above", n, g.FailOn),
				Count:   n,
			})
		}
	}
	if g.MinScore != nil && score < *g.MinScore {
		r.Reasons = append(r.Reasons, Reason{
			Code:    ReasonMinScore,
			Message: fmt.Sprintf("score %d is below the minimum %d", score, *g.MinScore),
		})
	}
	if g.Breaking == BreakingBlock && in.Change != nil {
		switch {
		case in.Change.Failed:
			r.Reasons = append(r.Reasons, Reason{Code: ReasonCompareFailed, Message: "breaking-change check could not run" + against(in.Change)})
		case in.Change.Breaking:
			r.Reasons = append(r.Reasons, Reason{Code: ReasonBreaking, Message: "breaking change" + against(in.Change)})
		}
	}
	r.Passed = len(r.Reasons) == 0
	return r
}

func against(c *Change) string {
	if c.ComparedTo == "" {
		return ""
	}
	return " against " + c.ComparedTo
}

func countsOf(es []Evaluated) Counts {
	c := Counts{Open: zero(), Waived: zero(), Ignored: zero()}
	for _, e := range es {
		switch e.Status {
		case StatusWaived:
			c.Waived[e.Severity]++
		case StatusIgnored, StatusIgnoredCounted:
			c.Ignored[e.Severity]++
		default:
			c.Open[e.Severity]++
		}
	}
	return c
}

func zero() map[contract.Severity]int {
	m := make(map[contract.Severity]int, len(contract.Severities))
	for _, s := range contract.Severities {
		m[s] = 0
	}
	return m
}
