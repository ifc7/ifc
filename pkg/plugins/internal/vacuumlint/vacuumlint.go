// Package vacuumlint runs Vacuum with an effective OpenLint ruleset and turns
// its results into ifc7 findings. It is shared by the OpenAPI and JSON Schema
// lint plugins.
package vacuumlint

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/daveshanley/vacuum/model"
	"github.com/daveshanley/vacuum/motor"
	"github.com/daveshanley/vacuum/rulesets"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/governance/ruleset"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const (
	vacuumModule = "github.com/daveshanley/vacuum"
	// pinnedVacuumVersion is reported when build info is unavailable. Keep
	// in step with go.mod.
	pinnedVacuumVersion = "v0.41.0"
)

// Kind selects the built-in rules and execution mode.
type Kind int

const (
	OpenAPI Kind = iota
	JSONSchema
)

var (
	modelOnce sync.Once
	rsModel   rulesets.RuleSets

	engineOnce sync.Once
	engine     contract.Engine

	cacheMu sync.Mutex
	cache   = map[string]*rulesets.RuleSet{}
)

const maxCached = 64

func defaults() rulesets.RuleSets {
	modelOnce.Do(func() { rsModel = rulesets.BuildDefaultRuleSets() })
	return rsModel
}

// Engine returns the lint engine name and version.
func Engine() contract.Engine {
	engineOnce.Do(func() {
		engine = contract.Engine{Name: "vacuum", Version: pinnedVacuumVersion}
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, d := range bi.Deps {
				if d.Path == vacuumModule {
					v := d.Version
					if d.Replace != nil && d.Replace.Version != "" {
						v = d.Replace.Version
					}
					if v != "" && v != "(devel)" {
						engine.Version = v
					}
				}
			}
		}
	})
	return engine
}

// nativeBuiltin maps an ifc7 built-in name to Vacuum's name for kind, or ""
// when the built-in does not apply to kind.
func nativeBuiltin(name string, kind Kind) string {
	switch kind {
	case OpenAPI:
		switch name {
		case ruleset.BuiltinIfc7Recommended, ruleset.BuiltinSpectralOAS:
			return rulesets.SpectralOpenAPI
		}
	case JSONSchema:
		if name == ruleset.BuiltinIfc7Recommended {
			return rulesets.VacuumJSONSchema
		}
	}
	return ""
}

// BuildRuleSet turns an effective ruleset into a runnable Vacuum rule set
// for kind. Built-ins that don't apply to kind are dropped; if none apply,
// the kind's recommended set is used, so one org ruleset covers every
// interface type.
func BuildRuleSet(rs *contract.RulesetDocument, kind Kind) (*rulesets.RuleSet, string, error) {
	var eff *ruleset.Document
	if rs == nil {
		eff = ruleset.Default()
	} else {
		raw, err := rs.Decode()
		if err != nil {
			return nil, "", err
		}
		doc, err := ruleset.Parse(raw)
		if err != nil {
			return nil, "", err
		}
		// The plugin input is already effective; resolving again only
		// normalises it and rejects non-built-in extends.
		if eff, err = ruleset.Resolve(context.Background(), doc, ruleset.BuiltinsOnly); err != nil {
			return nil, "", err
		}
	}
	_, sum, err := ruleset.Canonical(eff)
	if err != nil {
		return nil, "", err
	}
	key := fmt.Sprintf("%d:%s", kind, sum)
	cacheMu.Lock()
	cached := cache[key]
	cacheMu.Unlock()
	if cached != nil {
		return cached, sum, nil
	}

	tree := eff.Map()
	var ext []any
	for _, e := range eff.Extends() {
		if n := nativeBuiltin(e.Name, kind); n != "" {
			ext = append(ext, []any{n, e.Mode})
		}
	}
	if len(ext) == 0 {
		ext = []any{[]any{nativeBuiltin(ruleset.BuiltinIfc7Recommended, kind), ruleset.ModeRecommended}}
	}
	tree["extends"] = ext

	native := &rulesets.RuleSet{
		Extends:          tree["extends"],
		RuleDefinitions:  mapOrEmpty(tree["rules"]),
		DocumentationURI: stringOr(tree["documentationUrl"]),
		Description:      stringOr(tree["description"]),
		Aliases:          mapOrNil(tree["aliases"]),
	}
	if f, ok := tree["formats"].([]any); ok {
		for _, x := range f {
			if s, ok := x.(string); ok {
				native.Formats = append(native.Formats, s)
			}
		}
	}
	if len(native.Aliases) > 0 {
		if native.ParsedAliases, err = rulesets.ParseAliases(native.Aliases); err != nil {
			return nil, "", fmt.Errorf("%w: aliases: %v", ruleset.ErrInvalid, err)
		}
	}
	built := defaults().GenerateRuleSetFromSuppliedRuleSet(native)
	if err := built.LoadError(); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ruleset.ErrInvalid, err)
	}
	cacheMu.Lock()
	if len(cache) >= maxCached {
		clear(cache)
	}
	cache[key] = built
	cacheMu.Unlock()
	return built, sum, nil
}

// Run executes rs against spec and returns the plugin output.
func Run(exec *motor.RuleSetExecution) contract.LintOutput {
	result := motor.ApplyRulesToRuleSet(exec)
	defer result.ReleaseOwnedResources()
	eng := Engine()
	if len(result.Errors) > 0 {
		return failure(fmt.Sprintf("failed to lint document: %v", result.Errors[0]))
	}
	fs := make([]contract.Finding, 0, len(result.Results))
	for i := range result.Results {
		if f, ok := toFinding(&result.Results[i]); ok {
			fs = append(fs, f)
		}
	}
	out := Output(fs, &eng)
	if len(result.IgnoredResults) > 0 {
		ignored := make([]contract.Finding, 0, len(result.IgnoredResults))
		for i := range result.IgnoredResults {
			if f, ok := toFinding(&result.IgnoredResults[i]); ok {
				ignored = append(ignored, f)
			}
		}
		findings.Sort(ignored)
		out.IgnoredFindings = ignored
	}
	return out
}

// Failure returns the output for a document that could not be linted at
// all: one error finding and its score.
func failure(msg string) contract.LintOutput {
	eng := Engine()
	f := contract.Finding{RuleID: "ifc7-lint-failed", Severity: contract.SeverityError, Message: msg}
	f.Fingerprint = findings.Fingerprint(f.RuleID, f.Pointer, f.Message)
	return Output([]contract.Finding{f}, &eng)
}

// Failure is the exported form of failure for plugins that fail before
// running Vacuum (for example, unparsable input).
func Failure(msg string) contract.LintOutput { return failure(msg) }

// Output sorts findings and builds the plugin output: ifc7 score with
// default weights, raw text rendered from findings, and finding counts.
func Output(fs []contract.Finding, eng *contract.Engine) contract.LintOutput {
	findings.Sort(fs)
	sum := findings.Summarise(fs)
	counts := map[string]int{}
	for k, v := range sum.BySeverity {
		counts[string(k)] = v
	}
	return contract.LintOutput{
		Score:    findings.Score(fs, nil),
		Raw:      findings.RenderText(fs),
		Findings: fs,
		Engine:   eng,
		Extra: map[string]any{
			"findingCounts": counts,
			"ruleCounts":    sum.ByRule,
		},
	}
}

func toFinding(r *model.RuleFunctionResult) (contract.Finding, bool) {
	if r == nil || r.RepairOnly {
		return contract.Finding{}, false
	}
	ruleID := r.RuleId
	if ruleID == "" && r.Rule != nil {
		ruleID = r.Rule.Id
	}
	sev := severity(r.RuleSeverity)
	if sev == "" && r.Rule != nil {
		sev = severity(r.Rule.Severity)
	}
	if sev == "" {
		sev = contract.SeverityWarn
	}
	f := contract.Finding{
		RuleID:   ruleID,
		Severity: sev,
		Message:  r.Message,
		Pointer:  findings.PointerFromJSONPath(r.Path),
		Range:    rangeOf(r),
	}
	if r.Rule != nil {
		f.DocumentationURL = r.Rule.DocumentationURL
	}
	f.Fingerprint = findings.Fingerprint(f.RuleID, f.Pointer, f.Message)
	return f, true
}

func severity(s string) contract.Severity {
	switch s {
	case model.SeverityError:
		return contract.SeverityError
	case model.SeverityWarn, "warning":
		return contract.SeverityWarn
	case model.SeverityInfo:
		return contract.SeverityInfo
	case model.SeverityHint:
		return contract.SeverityHint
	}
	return ""
}

func rangeOf(r *model.RuleFunctionResult) *contract.Range {
	if r.StartNode != nil && r.StartNode.Line > 0 {
		rg := &contract.Range{
			Start: contract.Position{Line: r.StartNode.Line, Column: max(1, r.StartNode.Column)},
		}
		rg.End = rg.Start
		if r.EndNode != nil && r.EndNode.Line > 0 {
			rg.End = contract.Position{Line: r.EndNode.Line, Column: max(1, r.EndNode.Column)}
		}
		return rg
	}
	if r.Range.Start.Line > 0 || r.RangeIsZeroBased {
		off := 0
		if r.RangeIsZeroBased {
			off = 1
		}
		return &contract.Range{
			Start: contract.Position{Line: r.Range.Start.Line + off, Column: max(1, r.Range.Start.Char+off)},
			End:   contract.Position{Line: max(1, r.Range.End.Line+off), Column: max(1, r.Range.End.Char+off)},
		}
	}
	return nil
}

func mapOrEmpty(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func mapOrNil(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func stringOr(v any) string {
	s, _ := v.(string)
	return s
}
