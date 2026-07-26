// Package lint implements the default JSON Schema linter plugin.
package lint

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kaptinlin/jsonschema"
	"gopkg.in/yaml.v3"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const pluginID = "jsonschema-lint@v0"

// Plugin is the default JSON Schema linter.
type Plugin struct{}

// ID returns the plugin identifier.
func (Plugin) ID() string { return pluginID }

// Lint analyzes a JSON Schema document and returns a quality score plus raw detail.
func (Plugin) Lint(input contract.LintInput) (contract.LintOutput, error) {
	if input.InterfaceType != contract.InterfaceTypeJSONSchema {
		return contract.LintOutput{}, fmt.Errorf("jsonschema lint: unexpected interface type %q", input.InterfaceType)
	}
	raw, err := input.Document.Decode()
	if err != nil {
		return contract.LintOutput{}, err
	}

	jsonBytes, err := toJSON(raw)
	if err != nil {
		return contract.LintOutput{
			Score: 0,
			Raw:   fmt.Sprintf("error: failed to parse document: %v\n", err),
			Extra: map[string]any{
				"findingCounts": map[string]int{"error": 1, "warning": 0, "info": 0},
			},
		}, nil
	}

	var findings []string
	errorCount := 0
	warningCount := 0
	infoCount := 0

	if _, err := jsonschema.NewCompiler().Compile(jsonBytes); err != nil {
		errorCount++
		findings = append(findings, fmt.Sprintf("error: invalid JSON Schema: %v", err))
	}

	var doc map[string]any
	if err := json.Unmarshal(jsonBytes, &doc); err == nil {
		if _, ok := doc["$schema"]; !ok {
			warningCount++
			findings = append(findings, "warning[schema-missing]: Document is missing $schema.")
		}
		if _, ok := doc["type"]; !ok {
			if _, hasProps := doc["properties"]; !hasProps {
				infoCount++
				findings = append(findings, "info[type-missing]: Document has no type or properties.")
			}
		}
		if title, ok := doc["title"].(string); !ok || strings.TrimSpace(title) == "" {
			infoCount++
			findings = append(findings, "info[title-missing]: Document is missing title.")
		}
	}

	score := 100 - (errorCount * 40) - (warningCount * 10) - (infoCount * 2)
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
				"info":    infoCount,
			},
		},
	}, nil
}

func toJSON(data []byte) ([]byte, error) {
	if json.Valid(data) {
		return data, nil
	}
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
