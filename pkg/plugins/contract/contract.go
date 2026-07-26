// Package contract defines the lint and compare plugin input/output types
// matching the published JSON Schema contracts at
// https://ifc7.dev/schemas/plugins/v0/.
package contract

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// InterfaceType identifies the kind of interface specification.
type InterfaceType string

const (
	InterfaceTypeOpenAPI    InterfaceType = "OPENAPI"
	InterfaceTypeJSONSchema InterfaceType = "JSON_SCHEMA"
)

// FileFormat is the serialization format of decoded specification bytes.
type FileFormat string

const (
	FileFormatJSON FileFormat = "json"
	FileFormatYAML FileFormat = "yaml"
)

// SpecificationDocument is a single interface specification payload.
type SpecificationDocument struct {
	Specification string     `json:"specification"`
	FileFormat    FileFormat `json:"fileFormat"`
}

// Decode returns the raw specification bytes.
func (d SpecificationDocument) Decode() ([]byte, error) {
	if d.Specification == "" {
		return nil, fmt.Errorf("specification is required")
	}
	b, err := base64.StdEncoding.DecodeString(d.Specification)
	if err != nil {
		return nil, fmt.Errorf("decode specification: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("specification is empty")
	}
	return b, nil
}

// NewSpecificationDocument encodes raw bytes as a SpecificationDocument.
func NewSpecificationDocument(raw []byte, format FileFormat) SpecificationDocument {
	return SpecificationDocument{
		Specification: base64.StdEncoding.EncodeToString(raw),
		FileFormat:    format,
	}
}

// DetectFileFormat returns json or yaml based on content.
func DetectFileFormat(raw []byte) FileFormat {
	if json.Valid(raw) {
		return FileFormatJSON
	}
	return FileFormatYAML
}

// LintInput is the input contract for linter plugins.
type LintInput struct {
	InterfaceType InterfaceType         `json:"interfaceType"`
	Document      SpecificationDocument `json:"document"`
	Options       map[string]any        `json:"options,omitempty"`
}

// LintOutput is the output contract for linter plugins.
type LintOutput struct {
	Score int            `json:"score"`
	Raw   string         `json:"raw"`
	Extra map[string]any `json:"extra,omitempty"`
}

// CompareInput is the input contract for change-detector plugins.
type CompareInput struct {
	InterfaceType InterfaceType         `json:"interfaceType"`
	Before        SpecificationDocument `json:"before"`
	After         SpecificationDocument `json:"after"`
	Options       map[string]any        `json:"options,omitempty"`
}

// CompareOutput is the output contract for change-detector plugins.
type CompareOutput struct {
	Breaking bool           `json:"breaking"`
	Raw      string         `json:"raw"`
	Extra    map[string]any `json:"extra,omitempty"`
}
