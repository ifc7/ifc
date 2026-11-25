package policy

import (
	"encoding/json"
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
	// BreakingRequireMajor allows a breaking change only when the release
	// bumps the major version (the minor version for 0.x). It is judged at
	// release; at push there is no new version yet, so it does not fail.
	BreakingRequireMajor Breaking = "requireMajor"
)

// Scope decides which findings failOn counts.
type Scope string

const (
	// ScopeAll counts every unwaived finding. It is the default.
	ScopeAll Scope = "all"
	// ScopeNew counts only findings that are not in the baseline (the
	// latest release's findings, linted with the same ruleset), so a
	// stricter ruleset fails an interface only once it adds violations.
	ScopeNew Scope = "new"
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
	// severity or above. Empty means none. In JSON it is a severity, or an
	// object {"severity": "error", "scope": "new"} to set Scope too.
	FailOn FailOn `json:"failOn,omitempty"`
	// Scope is all (default) or new; see ScopeNew. In JSON it is part of
	// failOn.
	Scope Scope `json:"-"`
	// MinScore fails the gate when the score is below it.
	MinScore *int `json:"minScore,omitempty"`
	// Breaking is allow (default), block (fail on any breaking change) or
	// requireMajor (fail on a breaking change without a major bump).
	Breaking Breaking `json:"breaking,omitempty"`
}

// failOnObject is the object form of failOn.
type failOnObject struct {
	Severity FailOn `json:"severity"`
	Scope    Scope  `json:"scope,omitempty"`
}

// gateJSON is Gate without its JSON methods, with failOn left raw.
type gateJSON struct {
	Name        string          `json:"name"`
	On          []Stage         `json:"on"`
	Enforcement Enforcement     `json:"enforcement,omitempty"`
	FailOn      json.RawMessage `json:"failOn,omitempty"`
	MinScore    *int            `json:"minScore,omitempty"`
	Breaking    Breaking        `json:"breaking,omitempty"`
}

// MarshalJSON writes failOn as a string, or as an object when Scope is new.
func (g Gate) MarshalJSON() ([]byte, error) {
	j := gateJSON{Name: g.Name, On: g.On, Enforcement: g.Enforcement, MinScore: g.MinScore, Breaking: g.Breaking}
	var err error
	switch {
	case g.Scope != "" && g.Scope != ScopeAll:
		j.FailOn, err = json.Marshal(failOnObject{Severity: g.FailOn, Scope: g.Scope})
	case g.FailOn != "":
		j.FailOn, err = json.Marshal(g.FailOn)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(j)
}

// UnmarshalJSON accepts failOn as a severity string or as an object with
// severity and scope.
func (g *Gate) UnmarshalJSON(b []byte) error {
	var j gateJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*g = Gate{Name: j.Name, On: j.On, Enforcement: j.Enforcement, MinScore: j.MinScore, Breaking: j.Breaking}
	raw := strings.TrimSpace(string(j.FailOn))
	switch {
	case raw == "" || raw == "null":
	case strings.HasPrefix(raw, "{"):
		var o failOnObject
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return fmt.Errorf("%w: gate %q failOn: %v", ErrInvalid, j.Name, err)
		}
		g.FailOn, g.Scope = o.Severity, o.Scope
	default:
		if err := json.Unmarshal(j.FailOn, &g.FailOn); err != nil {
			return fmt.Errorf("%w: gate %q failOn must be a severity or {severity, scope}", ErrInvalid, j.Name)
		}
	}
	return nil
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
	switch g.Scope {
	case "", ScopeAll:
	case ScopeNew:
		if g.FailOn == "" || g.FailOn == FailOnNone {
			return fmt.Errorf("%w: gate %q failOn scope new needs a severity", ErrInvalid, g.Name)
		}
	default:
		return fmt.Errorf("%w: gate %q failOn scope must be all or new", ErrInvalid, g.Name)
	}
	switch g.Breaking {
	case "", BreakingAllow, BreakingBlock, BreakingRequireMajor:
	default:
		return fmt.Errorf("%w: gate %q breaking must be allow, block or requireMajor", ErrInvalid, g.Name)
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
	// Breaking is the compare plugin's verdict. It is used only when
	// Changes is nil (a record from before structured changes).
	Breaking bool `json:"breaking"`
	// Changes are the structured change records. When set, a breaking
	// change is one whose level, after the policy's ChangeSettings, is
	// error.
	Changes []contract.Change `json:"changes,omitempty"`
	// ComparedTo names the baseline, for example the superseded release.
	ComparedTo string `json:"comparedTo,omitempty"`
	// FromVersion and ToVersion are the superseded and new release
	// versions, for requireMajor. ToVersion is empty at push.
	FromVersion string `json:"fromVersion,omitempty"`
	ToVersion   string `json:"toVersion,omitempty"`
	// Failed means compare could not run.
	Failed bool `json:"failed,omitempty"`
}

// Baseline is the findings scope-new gates compare against.
type Baseline struct {
	// Findings are the baseline document's findings, linted with the same
	// effective ruleset as the findings being judged.
	Findings []contract.Finding
	// From names the baseline, for example "release 1.4.0".
	From string
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
	// ChangeSettings sets per-kind change levels.
	ChangeSettings ChangeSettings
	// Baseline is what scope-new gates compare against. Nil means there is
	// no baseline (no release yet), so every finding is new.
	Baseline *Baseline
}

// Reason codes.
const (
	ReasonFindings      = "findings"
	ReasonMinScore      = "min_score"
	ReasonBreaking      = "breaking_change"
	ReasonLintFailed    = "lint_failed"
	ReasonCompareFailed = "compare_failed"
	// ReasonMajorRequired is a breaking change released without a major
	// version bump under breaking: requireMajor.
	ReasonMajorRequired = "major_bump_required"
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
	AdvisoryFailed bool   `json:"advisoryFailed"`
	Score          int    `json:"score"`
	Counts         Counts `json:"counts"`
	// BreakingChanges counts the changes at level error after the policy's
	// change settings.
	BreakingChanges int          `json:"breakingChanges"`
	Gates           []GateResult `json:"gates"`
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
	breaking := breakingCount(in.Change, in.ChangeSettings)
	o.BreakingChanges = breaking
	isNew := newMask(in.Findings, in.Baseline)
	for _, g := range gs {
		if !g.RunsAt(in.Stage) {
			continue
		}
		r := evaluateGate(g, in, o.Score, breaking, isNew)
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

// breakingCount is the number of breaking changes in c under settings.
func breakingCount(c *Change, settings ChangeSettings) int {
	if c == nil || c.Failed {
		return 0
	}
	if c.Changes == nil {
		if c.Breaking {
			return 1
		}
		return 0
	}
	return len(breakingChanges(settings.Apply(c.Changes)))
}

// newMask marks each finding that is not in the baseline, matching by
// fingerprint and counting occurrences like findings.NewSince.
func newMask(es []Evaluated, b *Baseline) []bool {
	mask := make([]bool, len(es))
	seen := map[string]int{}
	if b != nil {
		for _, f := range b.Findings {
			seen[f.Fingerprint]++
		}
	}
	for i, e := range es {
		if seen[e.Fingerprint] > 0 {
			seen[e.Fingerprint]--
			continue
		}
		mask[i] = true
	}
	return mask
}

func evaluateGate(g Gate, in Input, score, breaking int, isNew []bool) GateResult {
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
		onlyNew := g.Scope == ScopeNew
		n := 0
		for i, e := range in.Findings {
			if onlyNew && !isNew[i] {
				continue
			}
			if e.countsForGate() && failsOn(g.FailOn, e.Severity) {
				n++
			}
		}
		if n > 0 {
			msg := fmt.Sprintf("%d unwaived finding(s) at %s or above", n, g.FailOn)
			if onlyNew {
				msg = fmt.Sprintf("%d new unwaived finding(s) at %s or above", n, g.FailOn)
				if in.Baseline != nil && in.Baseline.From != "" {
					msg += " since " + in.Baseline.From
				}
			}
			r.Reasons = append(r.Reasons, Reason{Code: ReasonFindings, Message: msg, Count: n})
		}
	}
	if g.MinScore != nil && score < *g.MinScore {
		r.Reasons = append(r.Reasons, Reason{
			Code:    ReasonMinScore,
			Message: fmt.Sprintf("score %d is below the minimum %d", score, *g.MinScore),
		})
	}
	if (g.Breaking == BreakingBlock || g.Breaking == BreakingRequireMajor) && in.Change != nil {
		if reason, failed := judgeBreaking(g.Breaking, in.Change, breaking); failed {
			r.Reasons = append(r.Reasons, reason)
		}
	}
	r.Passed = len(r.Reasons) == 0
	return r
}

func judgeBreaking(mode Breaking, c *Change, n int) (Reason, bool) {
	if c.Failed {
		return Reason{Code: ReasonCompareFailed, Message: "breaking-change check could not run" + against(c)}, true
	}
	if n == 0 {
		return Reason{}, false
	}
	what := "breaking change"
	if c.Changes != nil {
		what = fmt.Sprintf("%d breaking change(s)", n)
	}
	if mode == BreakingBlock {
		return Reason{Code: ReasonBreaking, Message: what + against(c), Count: n}, true
	}
	// requireMajor: nothing to judge until there is a new version.
	if c.ToVersion == "" {
		return Reason{}, false
	}
	if c.FromVersion == "" {
		// No earlier release: a first release may contain anything.
		return Reason{}, false
	}
	from, errFrom := ParseVersion(c.FromVersion)
	to, errTo := ParseVersion(c.ToVersion)
	if errFrom != nil || errTo != nil {
		return Reason{Code: ReasonMajorRequired, Message: fmt.Sprintf("%s%s, and the version bump %s to %s cannot be judged", what, against(c), c.FromVersion, c.ToVersion), Count: n}, true
	}
	if AllowsBreaking(from, to) {
		return Reason{}, false
	}
	need := "a major version bump"
	if from.Major == 0 {
		need = "a minor version bump (0.x)"
	}
	return Reason{
		Code:    ReasonMajorRequired,
		Message: fmt.Sprintf("%s%s needs %s, but %s to %s is not one", what, against(c), need, c.FromVersion, c.ToVersion),
		Count:   n,
	}, true
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
