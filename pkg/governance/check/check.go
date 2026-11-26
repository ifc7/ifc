// Package check runs a full governance check on one specification: it
// compares the document with a baseline release, lints it with an effective
// ruleset, sets aside change-scoped findings outside the changes, applies
// waivers and inline-ignore settings, scores it, and evaluates gates. It is
// the analysis the ifc7 hub runs on a push or a release, packaged so that
// `ifc lint --gate` reaches the same pass or fail for the same document and
// policy.
//
// Like the rest of pkg/governance it does no network or database I/O: the
// caller supplies the policy and the baseline document.
package check

import (
	"context"
	"fmt"
	"time"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/governance/ruleset"
	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/runner"
)

// Policy is everything governance applies to one interface: what the hub's
// effective-policy endpoint returns, or a local ruleset with ad-hoc gates.
type Policy struct {
	// Ruleset is the effective (fully resolved) OpenLint ruleset, as
	// canonical bytes. Empty means the built-in default.
	Ruleset []byte
	// SHA256 is the sha256 of Ruleset. When empty, Ruleset is parsed and
	// canonicalised first; it must then extend built-ins only (resolve ifc7
	// references with ruleset.Effective before calling Run).
	SHA256 string
	// Weights are the score weights. Nil means the defaults.
	Weights findings.Weights
	// InlineIgnores decides what x-lint-ignore does. Empty means allow.
	InlineIgnores policy.InlineIgnores
	// ChangeScoped lists the rules limited to changed parts of a document
	// (x-ifc-scope: changed). Nil means read them from Ruleset.
	ChangeScoped []string
	// Gates are evaluated at Input.Stage.
	Gates []policy.Gate
	// ChangeSettings set per-kind change levels.
	ChangeSettings policy.ChangeSettings
	// Waivers are the interface's waivers. Only those active at Input.Now
	// apply.
	Waivers []policy.Waiver
}

// Baseline is the release a document is compared with: the latest release
// for a push, the release being superseded for a release.
type Baseline struct {
	// Document is the release's specification.
	Document []byte
	// Version is the release's semantic version, for example "1.4.0".
	// Gates with breaking: requireMajor need it.
	Version string
	// Name labels the baseline in results, for a baseline that is not a
	// release (such as a file). Empty means "release {Version}".
	Name string
}

// label returns what results call b, and what scope-new messages say the
// findings are new since.
func (b *Baseline) label() (comparedTo, since string) {
	if b.Name != "" {
		return b.Name, b.Name
	}
	return b.Version, "release " + b.Version
}

// Input is one document to check.
type Input struct {
	InterfaceType contract.InterfaceType
	Document      []byte
	// Baseline is compared against, or nil when there is none (no release
	// yet). Without one, change-scoped rules apply everywhere, breaking
	// gates have nothing to judge and every finding is new.
	Baseline *Baseline
	// Stage selects the gates to evaluate. Empty evaluates none.
	Stage policy.Stage
	// Version is the version being released, for breaking: requireMajor
	// at the release stage.
	Version string
	// Now decides which waivers are active. Zero means time.Now().
	Now time.Time
}

// Compared is the result of comparing the document with its baseline.
type Compared struct {
	PluginID string `json:"pluginId"`
	// Against is the baseline's version, or its name.
	Against  string            `json:"against"`
	Breaking bool              `json:"breaking"`
	Changes  []contract.Change `json:"changes"`
	// Error is set when compare could not run; gates that judge breaking
	// changes then fail closed.
	Error string `json:"error,omitempty"`
}

// Result is a checked document.
type Result struct {
	PluginID string           `json:"pluginId"`
	Engine   *contract.Engine `json:"engine,omitempty"`
	// RulesetSHA256 is the sha256 of the effective ruleset linted with.
	RulesetSHA256 string `json:"rulesetSha256"`
	// Score is the quality score, as the hub stores it: waived findings
	// count, inline ignores the policy allows do not.
	Score int `json:"score"`
	// Findings are every finding with its governance status.
	Findings []policy.Evaluated `json:"findings"`
	// ScopedOut are findings of change-scoped rules outside the changes.
	// They count toward nothing.
	ScopedOut []contract.Finding `json:"scopedOut,omitempty"`
	// Compare is nil when there was no baseline.
	Compare *Compared `json:"compare,omitempty"`
	// LintError is set when lint could not run. Every gate at the stage
	// then fails closed.
	LintError string `json:"lintError,omitempty"`
	// Outcome is nil when no gate runs at the stage.
	Outcome *policy.Outcome `json:"outcome,omitempty"`
	// Raw is the lint plugin's human-readable report.
	Raw string `json:"-"`
}

// Run checks in under p with the plugins in reg (nil means the default
// registry). It returns an error only for an unusable policy; a lint or
// compare failure is reported in the result and fails the gates closed,
// as on the hub.
func Run(ctx context.Context, reg *runner.Registry, p Policy, in Input) (*Result, error) {
	if reg == nil {
		reg = runner.DefaultRegistry()
	}
	rs, sum, scoped, err := effective(ctx, p)
	if err != nil {
		return nil, err
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	weights := p.Weights
	if weights == nil {
		weights = findings.DefaultWeights()
	}
	res := &Result{RulesetSHA256: sum, Findings: []policy.Evaluated{}}

	var change *policy.Change
	var changes []contract.Change
	compared := false
	if in.Baseline != nil {
		out, id, cerr := reg.CompareContext(ctx, contract.CompareInput{
			InterfaceType: in.InterfaceType,
			Before:        contract.NewSpecificationDocument(in.Baseline.Document, contract.DetectFileFormat(in.Baseline.Document)),
			After:         contract.NewSpecificationDocument(in.Document, contract.DetectFileFormat(in.Document)),
		})
		against, _ := in.Baseline.label()
		res.Compare = &Compared{PluginID: id, Against: against, Changes: []contract.Change{}}
		change = &policy.Change{ComparedTo: against, FromVersion: in.Baseline.Version, ToVersion: in.Version}
		if cerr != nil {
			res.Compare.Error = cerr.Error()
			change.Failed = true
		} else {
			res.Compare.Breaking = out.Breaking
			if out.Changes != nil {
				res.Compare.Changes = p.ChangeSettings.Apply(out.Changes)
			}
			change.Breaking, change.Changes = out.Breaking, out.Changes
			changes, compared = out.Changes, true
		}
	}

	ruleDoc := contract.NewRulesetDocument(rs, sum)
	fs, ignored, scopedOut, lerr := lint(ctx, reg, in.InterfaceType, in.Document, ruleDoc, p.InlineIgnores, scoped, changes, compared, res)
	if lerr != nil {
		res.LintError = lerr.Error()
	} else {
		res.ScopedOut = scopedOut
		res.Findings = policy.ApplyWaivers(fs, ignored, p.Waivers, p.InlineIgnores, now)
		// Waivers never change the score.
		res.Score = policy.ScoreOf(policy.ApplyWaivers(fs, ignored, nil, p.InlineIgnores, time.Time{}), weights)
	}

	if in.Stage == "" || !runsAt(p.Gates, in.Stage) {
		return res, nil
	}
	gin := policy.Input{
		Stage:          in.Stage,
		Findings:       res.Findings,
		Weights:        weights,
		LintError:      res.LintError,
		Change:         change,
		ChangeSettings: p.ChangeSettings,
	}
	if in.Baseline != nil && lerr == nil && scopesNew(p.Gates, in.Stage) {
		// Like the hub, the baseline is linted with the same ruleset. When
		// it can't be, every finding counts as new, so the gate stays strict.
		if bfs, _, _, berr := lint(ctx, reg, in.InterfaceType, in.Baseline.Document, ruleDoc, p.InlineIgnores, scoped, nil, false, nil); berr == nil {
			_, since := in.Baseline.label()
			gin.Baseline = &policy.Baseline{Findings: bfs, From: since}
		}
	}
	res.Outcome = policy.Evaluate(p.Gates, gin)
	return res, nil
}

// lint runs the linter and splits its results the way the hub does. When
// res is set it records the plugin id, engine and raw report on it.
func lint(ctx context.Context, reg *runner.Registry, typ contract.InterfaceType, doc []byte, rs *contract.RulesetDocument, inline policy.InlineIgnores, scoped []string, changes []contract.Change, compared bool, res *Result) (fs, ignored, scopedOut []contract.Finding, err error) {
	out, id, err := reg.LintContext(ctx, contract.LintInput{
		InterfaceType: typ,
		Document:      contract.NewSpecificationDocument(doc, contract.DetectFileFormat(doc)),
		Ruleset:       rs,
	})
	if res != nil {
		res.PluginID = id
	}
	if err != nil {
		return nil, nil, nil, err
	}
	fs = out.Findings
	if fs == nil {
		fs = []contract.Finding{}
	}
	ignored = out.IgnoredFindings
	// Under deny, x-lint-ignore is disregarded: ignored results are
	// ordinary findings.
	if inline == policy.InlineIgnoresDeny {
		fs = append(fs, ignored...)
		ignored = nil
	}
	fs, scopedOut = findings.ScopeToChanges(fs, scoped, changes, compared)
	ignored, scopedIgnored := findings.ScopeToChanges(ignored, scoped, changes, compared)
	scopedOut = append(scopedOut, scopedIgnored...)
	findings.Sort(fs)
	findings.Sort(ignored)
	findings.Sort(scopedOut)
	if res != nil {
		res.Engine = out.Engine
		res.Raw = out.Raw
		if res.Raw == "" {
			res.Raw = findings.RenderText(fs)
		}
	}
	return fs, ignored, scopedOut, nil
}

// effective returns the ruleset bytes, their sha256 and the change-scoped
// rules. An empty ruleset is the built-in default.
func effective(ctx context.Context, p Policy) ([]byte, string, []string, error) {
	if len(p.Ruleset) == 0 {
		doc, b, sum, err := ruleset.Effective(ctx, nil, ruleset.BuiltinsOnly)
		if err != nil {
			return nil, "", nil, err
		}
		return b, sum, doc.ChangeScopedRules(), nil
	}
	if p.SHA256 == "" {
		// Not yet canonical: parse it. The effective ruleset lists
		// built-ins only, so it resolves without a resolver.
		doc, b, sum, err := ruleset.Effective(ctx, p.Ruleset, ruleset.BuiltinsOnly)
		if err != nil {
			return nil, "", nil, fmt.Errorf("effective ruleset: %w", err)
		}
		scoped := p.ChangeScoped
		if scoped == nil {
			scoped = doc.ChangeScopedRules()
		}
		return b, sum, scoped, nil
	}
	scoped := p.ChangeScoped
	if scoped == nil {
		doc, err := ruleset.Parse(p.Ruleset)
		if err != nil {
			return nil, "", nil, fmt.Errorf("effective ruleset: %w", err)
		}
		scoped = doc.ChangeScopedRules()
	}
	return p.Ruleset, p.SHA256, scoped, nil
}

func runsAt(gs []policy.Gate, s policy.Stage) bool {
	for _, g := range gs {
		if g.RunsAt(s) {
			return true
		}
	}
	return false
}

func scopesNew(gs []policy.Gate, s policy.Stage) bool {
	for _, g := range gs {
		if g.RunsAt(s) && g.Scope == policy.ScopeNew {
			return true
		}
	}
	return false
}
