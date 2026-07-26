// Package runner resolves and invokes default lint/compare plugins by interface type.
package runner

import (
	"fmt"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	jsonschemacompare "github.com/ifc7/ifc/pkg/plugins/jsonschema/compare"
	jsonschemalint "github.com/ifc7/ifc/pkg/plugins/jsonschema/lint"
	openapicompare "github.com/ifc7/ifc/pkg/plugins/openapi/compare"
	openapilint "github.com/ifc7/ifc/pkg/plugins/openapi/lint"
)

// Action identifies a plugin role.
type Action string

const (
	ActionLint    Action = "lint"
	ActionCompare Action = "compare"
)

// Linter runs quality analysis on a single specification.
type Linter interface {
	ID() string
	Lint(input contract.LintInput) (contract.LintOutput, error)
}

// Comparator detects changes between two specifications.
type Comparator interface {
	ID() string
	Compare(input contract.CompareInput) (contract.CompareOutput, error)
}

// Registry maps interface types to default plugins.
type Registry struct {
	linters     map[contract.InterfaceType]Linter
	comparators map[contract.InterfaceType]Comparator
}

// DefaultRegistry returns the built-in default plugins for each type/action.
func DefaultRegistry() *Registry {
	return &Registry{
		linters: map[contract.InterfaceType]Linter{
			contract.InterfaceTypeOpenAPI:    openapilint.Plugin{},
			contract.InterfaceTypeJSONSchema: jsonschemalint.Plugin{},
		},
		comparators: map[contract.InterfaceType]Comparator{
			contract.InterfaceTypeOpenAPI:    openapicompare.Plugin{},
			contract.InterfaceTypeJSONSchema: jsonschemacompare.Plugin{},
		},
	}
}

// Lint invokes the default linter for the input's interface type.
func (r *Registry) Lint(input contract.LintInput) (contract.LintOutput, string, error) {
	linter, ok := r.linters[input.InterfaceType]
	if !ok {
		return contract.LintOutput{}, "", fmt.Errorf("no lint plugin for interface type %q", input.InterfaceType)
	}
	out, err := linter.Lint(input)
	if err != nil {
		return contract.LintOutput{}, linter.ID(), err
	}
	if out.Score < 0 || out.Score > 100 {
		return contract.LintOutput{}, linter.ID(), fmt.Errorf("lint plugin %s returned invalid score %d", linter.ID(), out.Score)
	}
	return out, linter.ID(), nil
}

// Compare invokes the default comparator for the input's interface type.
func (r *Registry) Compare(input contract.CompareInput) (contract.CompareOutput, string, error) {
	cmp, ok := r.comparators[input.InterfaceType]
	if !ok {
		return contract.CompareOutput{}, "", fmt.Errorf("no compare plugin for interface type %q", input.InterfaceType)
	}
	out, err := cmp.Compare(input)
	if err != nil {
		return contract.CompareOutput{}, cmp.ID(), err
	}
	return out, cmp.ID(), nil
}
