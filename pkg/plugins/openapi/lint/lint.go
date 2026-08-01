// Package lint implements the default OpenAPI linter plugin.
package lint

import (
	"fmt"
	"strings"
	"sync"

	"github.com/daveshanley/vacuum/model"
	"github.com/daveshanley/vacuum/motor"
	"github.com/daveshanley/vacuum/rulesets"
	"github.com/daveshanley/vacuum/statistics"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const pluginID = "openapi-lint@v1"

// Plugin is the default OpenAPI linter.
type Plugin struct{}

// ID returns the plugin identifier.
func (Plugin) ID() string { return pluginID }

var (
	recommendedOnce sync.Once
	recommendedRS   *rulesets.RuleSet
)

func recommendedRuleSet() *rulesets.RuleSet {
	recommendedOnce.Do(func() {
		recommendedRS = rulesets.BuildDefaultRuleSets().GenerateOpenAPIRecommendedRuleSet()
	})
	return recommendedRS
}

// Lint analyzes an OpenAPI document and returns a quality score plus raw detail.
func (Plugin) Lint(input contract.LintInput) (contract.LintOutput, error) {
	if input.InterfaceType != contract.InterfaceTypeOpenAPI {
		return contract.LintOutput{}, fmt.Errorf("openapi lint: unexpected interface type %q", input.InterfaceType)
	}
	raw, err := input.Document.Decode()
	if err != nil {
		return contract.LintOutput{}, err
	}

	exec := motor.ApplyRulesToRuleSet(&motor.RuleSetExecution{
		RuleSet: recommendedRuleSet(),
		Spec:    raw,
	})
	defer exec.ReleaseOwnedResources()

	if len(exec.Errors) > 0 {
		return contract.LintOutput{
			Score: 0,
			Raw:   fmt.Sprintf("error: failed to lint OpenAPI document: %v\n", exec.Errors[0]),
			Extra: map[string]any{
				"findingCounts": map[string]int{"error": 1, "warning": 0, "info": 0},
			},
		}, nil
	}

	resultSet := model.NewRuleResultSet(exec.Results)
	sorted := resultSet.SortResultsByLineNumber()

	score := statistics.CalculateQualityScore(resultSet)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	var b strings.Builder
	for _, r := range sorted {
		if r == nil {
			continue
		}
		severity := displaySeverity(r.RuleSeverity)
		ruleID := r.RuleId
		if ruleID == "" && r.Rule != nil {
			ruleID = r.Rule.Id
		}
		fmt.Fprintf(&b, "%s[%s]: %s", severity, ruleID, r.Message)
		if loc := formatLocation(r); loc != "" {
			fmt.Fprintf(&b, "\n  %s", loc)
		}
		b.WriteByte('\n')
	}

	return contract.LintOutput{
		Score: score,
		Raw:   b.String(),
		Extra: map[string]any{
			"findingCounts": map[string]int{
				"error":   resultSet.GetErrorCount(),
				"warning": resultSet.GetWarnCount(),
				"info":    resultSet.GetInfoCount() + resultSet.GetHintCount(),
			},
		},
	}, nil
}

func displaySeverity(severity string) string {
	switch severity {
	case model.SeverityWarn:
		return "warning"
	case model.SeverityError, model.SeverityInfo, model.SeverityHint:
		return severity
	default:
		if severity == "" {
			return "warning"
		}
		return severity
	}
}

func formatLocation(r *model.RuleFunctionResult) string {
	if r == nil {
		return ""
	}
	if r.Path != "" {
		return r.Path
	}
	if r.StartNode != nil {
		return fmt.Sprintf("%d:%d", r.StartNode.Line, r.StartNode.Column)
	}
	if r.Range.Start.Line > 0 {
		return fmt.Sprintf("%d:%d", r.Range.Start.Line, r.Range.Start.Char)
	}
	return ""
}
