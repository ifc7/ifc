// Package compare implements the default OpenAPI change-detector plugin.
package compare

import (
	"fmt"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/what-changed/reports"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const pluginID = "openapi-compare@v0"

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

	changes, err := libopenapi.CompareDocuments(beforeDoc, afterDoc)
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("compare documents: %w", err)
	}
	if changes == nil {
		return contract.CompareOutput{
			Breaking: false,
			Raw:      "",
			Extra: map[string]any{
				"changeCounts": map[string]int{"breaking": 0, "nonBreaking": 0},
			},
		}, nil
	}

	breakingCount := changes.TotalBreakingChanges()
	totalCount := changes.TotalChanges()
	nonBreaking := totalCount - breakingCount
	if nonBreaking < 0 {
		nonBreaking = 0
	}

	var b strings.Builder
	report := reports.CreateOverallReport(changes)
	if report != nil {
		for label, ch := range report.ChangeReport {
			if ch == nil || ch.Total == 0 {
				continue
			}
			fmt.Fprintf(&b, "%s: %d change(s) (%d breaking)\n", label, ch.Total, ch.Breaking)
		}
	}
	fmt.Fprintf(&b, "total: %d change(s), %d breaking\n", totalCount, breakingCount)

	return contract.CompareOutput{
		Breaking: breakingCount > 0,
		Raw:      b.String(),
		Extra: map[string]any{
			"changeCounts": map[string]int{
				"breaking":    breakingCount,
				"nonBreaking": nonBreaking,
			},
		},
	}, nil
}
