// Package compare implements the default JSON Schema change-detector plugin.
package compare

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const pluginID = "jsonschema-compare@v0"

// Plugin is the default JSON Schema change detector.
type Plugin struct{}

// ID returns the plugin identifier.
func (Plugin) ID() string { return pluginID }

// Compare detects breaking and non-breaking changes between two JSON Schema documents.
func (Plugin) Compare(input contract.CompareInput) (contract.CompareOutput, error) {
	if input.InterfaceType != contract.InterfaceTypeJSONSchema {
		return contract.CompareOutput{}, fmt.Errorf("jsonschema compare: unexpected interface type %q", input.InterfaceType)
	}
	beforeBytes, err := input.Before.Decode()
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("before: %w", err)
	}
	afterBytes, err := input.After.Decode()
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("after: %w", err)
	}

	before, err := parseObject(beforeBytes)
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("parse before: %w", err)
	}
	after, err := parseObject(afterBytes)
	if err != nil {
		return contract.CompareOutput{}, fmt.Errorf("parse after: %w", err)
	}

	var lines []string
	breakingCount := 0
	nonBreakingCount := 0

	beforeType, _ := before["type"].(string)
	afterType, _ := after["type"].(string)
	if beforeType != "" && afterType != "" && beforeType != afterType {
		breakingCount++
		lines = append(lines, fmt.Sprintf("BREAKING: type changed from %q to %q", beforeType, afterType))
	}

	beforeRequired := stringSet(asStringSlice(before["required"]))
	afterRequired := stringSet(asStringSlice(after["required"]))
	for name := range afterRequired {
		if !beforeRequired[name] {
			breakingCount++
			lines = append(lines, fmt.Sprintf("BREAKING: added required property %q", name))
		}
	}
	for name := range beforeRequired {
		if !afterRequired[name] {
			nonBreakingCount++
			lines = append(lines, fmt.Sprintf("NON-BREAKING: removed required constraint on %q", name))
		}
	}

	beforeProps := asObjectMap(before["properties"])
	afterProps := asObjectMap(after["properties"])
	for name := range beforeProps {
		if _, ok := afterProps[name]; !ok {
			breakingCount++
			lines = append(lines, fmt.Sprintf("BREAKING: removed property %q", name))
		}
	}
	for name := range afterProps {
		if _, ok := beforeProps[name]; !ok {
			nonBreakingCount++
			lines = append(lines, fmt.Sprintf("NON-BREAKING: added property %q", name))
		}
	}

	rawOut := strings.Join(lines, "\n")
	if rawOut != "" {
		rawOut += "\n"
	}

	return contract.CompareOutput{
		Breaking: breakingCount > 0,
		Raw:      rawOut,
		Extra: map[string]any{
			"changeCounts": map[string]int{
				"breaking":    breakingCount,
				"nonBreaking": nonBreakingCount,
			},
		},
	}, nil
}

func parseObject(data []byte) (map[string]any, error) {
	jsonBytes := data
	if !json.Valid(data) {
		var v any
		if err := yaml.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		jsonBytes = b
	}
	var doc map[string]any
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("empty document")
	}
	return doc, nil
}

func asStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, s := range items {
		out[s] = true
	}
	return out
}

func asObjectMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return m
}
