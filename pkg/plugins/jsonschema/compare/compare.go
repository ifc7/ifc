// Package compare implements the default JSON Schema change-detector plugin.
package compare

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/ifc7/ifc/pkg/governance/changes"
	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const pluginID = "jsonschema-compare@v1"

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

	var cs []contract.Change
	add := func(kind, pointer string, breaking bool, msg string) {
		cs = append(cs, changes.New(kind, pointer, breaking, msg))
	}

	beforeType, _ := before["type"].(string)
	afterType, _ := after["type"].(string)
	if beforeType != "" && afterType != "" && beforeType != afterType {
		add("schema.type.modified", "/type", true, fmt.Sprintf("changed type from %q to %q", beforeType, afterType))
	}

	beforeRequired := stringSet(asStringSlice(before["required"]))
	afterRequired := stringSet(asStringSlice(after["required"]))
	for name := range afterRequired {
		if !beforeRequired[name] {
			add("schema.required.added", "/required/"+findings.EscapePointerToken(name), true, fmt.Sprintf("added required property %q", name))
		}
	}
	for name := range beforeRequired {
		if !afterRequired[name] {
			add("schema.required.removed", "/required/"+findings.EscapePointerToken(name), false, fmt.Sprintf("removed required constraint on %q", name))
		}
	}

	beforeProps := asObjectMap(before["properties"])
	afterProps := asObjectMap(after["properties"])
	for name := range beforeProps {
		if _, ok := afterProps[name]; !ok {
			add("schema.properties.removed", "/properties/"+findings.EscapePointerToken(name), true, fmt.Sprintf("removed property %q", name))
		}
	}
	for name := range afterProps {
		if _, ok := beforeProps[name]; !ok {
			add("schema.properties.added", "/properties/"+findings.EscapePointerToken(name), false, fmt.Sprintf("added property %q", name))
		}
	}
	if cs == nil {
		cs = []contract.Change{}
	}
	changes.Sort(cs)
	breaking, nonBreaking := changes.Counts(cs)

	return contract.CompareOutput{
		Breaking: breaking > 0,
		Raw:      changes.RenderText(cs),
		Changes:  cs,
		Extra: map[string]any{
			"changeCounts": map[string]int{
				"breaking":    breaking,
				"nonBreaking": nonBreaking,
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
