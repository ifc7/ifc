// Package lint implements the default OpenAPI linter plugin.
package lint

import (
	"fmt"
	"strings"

	"github.com/pb33f/libopenapi"
	v2high "github.com/pb33f/libopenapi/datamodel/high/v2"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const pluginID = "openapi-lint@v0"

// Plugin is the default OpenAPI linter.
type Plugin struct{}

// ID returns the plugin identifier.
func (Plugin) ID() string { return pluginID }

// Lint analyzes an OpenAPI document and returns a quality score plus raw detail.
func (Plugin) Lint(input contract.LintInput) (contract.LintOutput, error) {
	if input.InterfaceType != contract.InterfaceTypeOpenAPI {
		return contract.LintOutput{}, fmt.Errorf("openapi lint: unexpected interface type %q", input.InterfaceType)
	}
	raw, err := input.Document.Decode()
	if err != nil {
		return contract.LintOutput{}, err
	}

	doc, err := libopenapi.NewDocument(raw)
	if err != nil {
		return contract.LintOutput{
			Score: 0,
			Raw:   fmt.Sprintf("error: failed to parse OpenAPI document: %v\n", err),
			Extra: map[string]any{
				"findingCounts": map[string]int{"error": 1, "warning": 0, "info": 0},
			},
		}, nil
	}

	var findings []string
	errorCount := 0
	warningCount := 0

	version := doc.GetVersion()
	if strings.HasPrefix(version, "2") {
		model, buildErr := doc.BuildV2Model()
		if buildErr != nil {
			errorCount++
			findings = append(findings, fmt.Sprintf("error: %v", buildErr))
		} else if model != nil {
			warningCount += collectSwaggerWarnings(&model.Model, &findings)
		}
	} else {
		model, buildErr := doc.BuildV3Model()
		if buildErr != nil {
			errorCount++
			findings = append(findings, fmt.Sprintf("error: %v", buildErr))
		} else if model != nil {
			warningCount += collectOpenAPIWarnings(&model.Model, &findings)
		}
	}

	score := 100 - (errorCount * 40) - (warningCount * 5)
	if score < 0 {
		score = 0
	}

	rawOut := strings.Join(findings, "\n")
	if rawOut != "" {
		rawOut += "\n"
	}

	return contract.LintOutput{
		Score: score,
		Raw:   rawOut,
		Extra: map[string]any{
			"findingCounts": map[string]int{
				"error":   errorCount,
				"warning": warningCount,
				"info":    0,
			},
		},
	}, nil
}

func collectOpenAPIWarnings(doc *v3high.Document, findings *[]string) int {
	if doc == nil || doc.Paths == nil || doc.Paths.PathItems == nil {
		return 0
	}
	count := 0
	for pathPairs := doc.Paths.PathItems.First(); pathPairs != nil; pathPairs = pathPairs.Next() {
		path := pathPairs.Key()
		item := pathPairs.Value()
		if item == nil {
			continue
		}
		for _, op := range []*v3high.Operation{
			item.Get, item.Put, item.Post, item.Delete, item.Options, item.Head, item.Patch, item.Trace,
		} {
			if op == nil {
				continue
			}
			if op.OperationId == "" {
				count++
				*findings = append(*findings, fmt.Sprintf("warning[operation-operationId]: Operation is missing operationId.\n  #/paths/%s", escapeJSONPointer(path)))
			}
		}
	}
	return count
}

func collectSwaggerWarnings(doc *v2high.Swagger, findings *[]string) int {
	if doc == nil || doc.Paths == nil || doc.Paths.PathItems == nil {
		return 0
	}
	count := 0
	for pathPairs := doc.Paths.PathItems.First(); pathPairs != nil; pathPairs = pathPairs.Next() {
		path := pathPairs.Key()
		item := pathPairs.Value()
		if item == nil {
			continue
		}
		for _, op := range []*v2high.Operation{
			item.Get, item.Put, item.Post, item.Delete, item.Options, item.Head, item.Patch,
		} {
			if op == nil {
				continue
			}
			if op.OperationId == "" {
				count++
				*findings = append(*findings, fmt.Sprintf("warning[operation-operationId]: Operation is missing operationId.\n  #/paths/%s", escapeJSONPointer(path)))
			}
		}
	}
	return count
}

func escapeJSONPointer(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}
