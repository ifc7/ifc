package project

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kaptinlin/jsonschema"
	"github.com/pb33f/libopenapi"
	"gopkg.in/yaml.v3"

	"github.com/ifc7/ifc/internal/client"
)

// DetectSpecificationType reports whether data is a valid OpenAPI or JSON Schema
// document. OpenAPI is checked first so OpenAPI documents are not classified as
// JSON Schema.
func DetectSpecificationType(data []byte) (client.InterfaceType, error) {
	doc, err := parseYAMLOrJSONMap(data)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidSpecification, err)
	}

	if looksLikeOpenAPI(doc) {
		if _, err := libopenapi.NewDocument(data); err != nil {
			return "", fmt.Errorf("%w: invalid OpenAPI document: %w", ErrInvalidSpecification, err)
		}
		return client.OPENAPI, nil
	}

	if looksLikeJSONSchema(doc) {
		jsonBytes, err := toJSONBytes(data)
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrInvalidSpecification, err)
		}
		if _, err := jsonschema.NewCompiler().Compile(jsonBytes); err != nil {
			return "", fmt.Errorf("%w: invalid JSON Schema document: %w", ErrInvalidSpecification, err)
		}
		return client.JSONSCHEMA, nil
	}

	return "", ErrInvalidSpecification
}

func parseYAMLOrJSONMap(data []byte) (map[string]any, error) {
	var doc map[string]any
	if json.Valid(data) {
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		return doc, nil
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("empty document")
	}
	return doc, nil
}

func toJSONBytes(data []byte) ([]byte, error) {
	if json.Valid(data) {
		return data, nil
	}
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func looksLikeOpenAPI(doc map[string]any) bool {
	if _, ok := doc["openapi"]; ok {
		return true
	}
	if _, ok := doc["swagger"]; ok {
		return true
	}
	return false
}

func looksLikeJSONSchema(doc map[string]any) bool {
	if _, ok := doc["$schema"]; ok {
		return true
	}
	// Common JSON Schema shapes without an explicit $schema keyword.
	if _, ok := doc["properties"]; ok {
		return true
	}
	if _, ok := doc["$defs"]; ok {
		return true
	}
	if _, ok := doc["definitions"]; ok {
		return true
	}
	if t, ok := doc["type"].(string); ok {
		switch t {
		case "object", "array", "string", "number", "integer", "boolean", "null":
			return true
		}
	}
	return false
}

func isSpecCandidateExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func defaultInterfaceName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	name = strings.TrimSuffix(name, ".schema")
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	if name == "" {
		return "interface"
	}
	return name
}
