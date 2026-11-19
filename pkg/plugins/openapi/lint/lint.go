// Package lint implements the default OpenAPI linter plugin.
package lint

import (
	"fmt"

	"github.com/daveshanley/vacuum/motor"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/internal/vacuumlint"
)

// PluginID identifies this plugin and its output contract.
const PluginID = "openapi-lint@v2"

// Plugin is the default OpenAPI linter. It lints with LintInput.Ruleset, or
// the recommended spectral:oas rules when no ruleset is given.
type Plugin struct{}

// ID returns the plugin identifier.
func (Plugin) ID() string { return PluginID }

// Lint analyzes an OpenAPI document and returns structured findings, the
// ifc7 score and raw text rendered from the findings.
func (Plugin) Lint(input contract.LintInput) (contract.LintOutput, error) {
	if input.InterfaceType != contract.InterfaceTypeOpenAPI {
		return contract.LintOutput{}, fmt.Errorf("openapi lint: unexpected interface type %q", input.InterfaceType)
	}
	raw, err := input.Document.Decode()
	if err != nil {
		return contract.LintOutput{}, err
	}
	rs, _, err := vacuumlint.BuildRuleSet(input.Ruleset, vacuumlint.OpenAPI)
	if err != nil {
		return contract.LintOutput{}, fmt.Errorf("openapi lint: ruleset: %w", err)
	}
	return vacuumlint.Run(&motor.RuleSetExecution{RuleSet: rs, Spec: raw}), nil
}
