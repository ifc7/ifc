// Package compare implements the default OpenAPI change-detector plugin.
package compare

import (
	"fmt"

	"github.com/pb33f/libopenapi"

	"github.com/ifc7/ifc/pkg/governance/changes"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const pluginID = "openapi-compare@v1"

// Plugin is the default OpenAPI change detector.
type Plugin struct{}

// ID returns the plugin identifier.
func (Plugin) ID() string { return pluginID }

// Compare detects breaking and non-breaking changes between two OpenAPI documents.
func (Plugin) Compare(input contract.CompareInput) (contract.CompareOutput, error) {
	if input.InterfaceType != contract.InterfaceTypeOpenAPI {
		return contract.CompareOutput{}, fmt.Errorf("openapi compare: unexpected interface type %q", input.InterfaceType)
	}
	beforeBytes, err := input.Before.Decode()
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("before: %w", err)
	}
	afterBytes, err := input.After.Decode()
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("after: %w", err)
	}

	beforeDoc, err := libopenapi.NewDocument(beforeBytes)
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("parse before document: %w", err)
	}
	afterDoc, err := libopenapi.NewDocument(afterBytes)
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("parse after document: %w", err)
	}

	diff, err := libopenapi.CompareDocuments(beforeDoc, afterDoc)
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("compare documents: %w", err)
	}
	var cs []contract.Change
	if diff != nil {
		cs = collectChanges(diff, parseTree(beforeBytes), parseTree(afterBytes))
	}
	return output(cs), nil
}

// output builds the compare output from structured changes. Breaking and
// the counts come from the changes, after x-stability-level exemptions.
func output(cs []contract.Change) contract.CompareOutput {
	if cs == nil {
		cs = []contract.Change{}
	}
	breaking, nonBreaking := changes.Counts(cs)
	raw := changes.RenderText(cs)
	if len(cs) > 0 {
		raw += fmt.Sprintf("total: %d change(s), %d breaking\n", len(cs), breaking)
	}
	return contract.CompareOutput{
		Breaking: breaking > 0,
		Raw:      raw,
		Changes:  cs,
		Extra: map[string]any{
			"changeCounts": map[string]int{"breaking": breaking, "nonBreaking": nonBreaking},
		},
	}
}
