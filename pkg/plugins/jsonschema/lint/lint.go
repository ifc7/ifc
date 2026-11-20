// Package lint implements the default JSON Schema linter plugin.
package lint

import (
	"fmt"

	schemautil "github.com/daveshanley/vacuum/jsonschema"
	"github.com/daveshanley/vacuum/motor"
	"github.com/pb33f/go-yaml"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/internal/vacuumlint"
)

// PluginID identifies this plugin and its output contract.
const PluginID = "jsonschema-lint@v2"

// Plugin is the default JSON Schema linter. It lints with LintInput.Ruleset,
// or Vacuum's recommended JSON Schema rules when no ruleset is given.
type Plugin struct{}

// ID returns the plugin identifier.
func (Plugin) ID() string { return PluginID }

// Lint analyzes a JSON Schema document and returns structured findings, the
// ifc7 score and raw text rendered from the findings.
func (Plugin) Lint(input contract.LintInput) (contract.LintOutput, error) {
	if input.InterfaceType != contract.InterfaceTypeJSONSchema {
		return contract.LintOutput{}, fmt.Errorf("jsonschema lint: unexpected interface type %q", input.InterfaceType)
	}
	raw, err := input.Document.Decode()
	if err != nil {
		return contract.LintOutput{}, err
	}
	rs, _, err := vacuumlint.BuildRuleSet(input.Ruleset, vacuumlint.JSONSchema)
	if err != nil {
		return contract.LintOutput{}, fmt.Errorf("jsonschema lint: ruleset: %w", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return vacuumlint.Failure(fmt.Sprintf("failed to parse JSON Schema document: %v", err)), nil
	}
	dialect := schemautil.DetectDialect(&root)
	return vacuumlint.Run(&motor.RuleSetExecution{
		RuleSet:           rs,
		Spec:              raw,
		SkipDocumentCheck: true,
		SpecFormat:        dialect.Format,
	}), nil
}
