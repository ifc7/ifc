// Package spec parses, detects and validates interface specifications.
//
// It is the single place in ifc7 where OpenAPI documents are parsed with
// libopenapi and JSON Schema documents are compiled. The CLI and ifc-server
// both call it instead of importing those libraries directly, so a document
// accepted by one is accepted by the other.
package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/kaptinlin/jsonschema"
	"github.com/pb33f/libopenapi"
	"gopkg.in/yaml.v3"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// ErrInvalidSpecification is returned (wrapped) for any document that cannot
// be detected or does not validate as its declared type.
var ErrInvalidSpecification = errors.New("invalid interface specification")

// ErrUnsupportedType is returned (wrapped) by Validate for an interface type
// that this package has no validator for.
var ErrUnsupportedType = errors.New("unsupported interface type")

// ValidationError describes why a document failed validation.
type ValidationError struct {
	// Type is the interface type the document was validated as. Empty when
	// the type could not be detected.
	Type contract.InterfaceType
	// Err is the underlying parser or compiler error.
	Err error
}

func (e *ValidationError) Error() string {
	switch e.Type {
	case contract.InterfaceTypeOpenAPI:
		return fmt.Sprintf("%s: invalid OpenAPI document: %v", ErrInvalidSpecification, e.Err)
	case contract.InterfaceTypeJSONSchema:
		return fmt.Sprintf("%s: invalid JSON Schema document: %v", ErrInvalidSpecification, e.Err)
	default:
		if e.Err == nil {
			return ErrInvalidSpecification.Error()
		}
		return fmt.Sprintf("%s: %v", ErrInvalidSpecification, e.Err)
	}
}

// Unwrap lets errors.Is match both ErrInvalidSpecification and the cause.
func (e *ValidationError) Unwrap() []error {
	if e.Err == nil {
		return []error{ErrInvalidSpecification}
	}
	return []error{ErrInvalidSpecification, e.Err}
}

// DetectFileFormat returns json or yaml based on content.
func DetectFileFormat(raw []byte) contract.FileFormat {
	return contract.DetectFileFormat(raw)
}

// Validate checks that raw is a well-formed document of the given type.
func Validate(typ contract.InterfaceType, raw []byte) error {
	switch typ {
	case contract.InterfaceTypeOpenAPI:
		return ValidateOpenAPI(raw)
	case contract.InterfaceTypeJSONSchema:
		return ValidateJSONSchema(raw)
	default:
		return fmt.Errorf("%w %q", ErrUnsupportedType, typ)
	}
}

// ValidateOpenAPI checks that raw parses as an OpenAPI (or Swagger) document.
func ValidateOpenAPI(raw []byte) error {
	if len(raw) == 0 {
		return &ValidationError{Type: contract.InterfaceTypeOpenAPI, Err: errors.New("document is empty")}
	}
	m, err := parseMap(raw)
	if err != nil {
		return &ValidationError{Type: contract.InterfaceTypeOpenAPI, Err: err}
	}
	if !looksLikeOpenAPI(m) {
		return &ValidationError{Type: contract.InterfaceTypeOpenAPI, Err: errors.New("document has no openapi or swagger version")}
	}
	if _, err := libopenapi.NewDocument(raw); err != nil {
		return &ValidationError{Type: contract.InterfaceTypeOpenAPI, Err: err}
	}
	return nil
}

// ValidateJSONSchema checks that raw (JSON or YAML) compiles as a JSON Schema.
func ValidateJSONSchema(raw []byte) error {
	if len(raw) == 0 {
		return &ValidationError{Type: contract.InterfaceTypeJSONSchema, Err: errors.New("document is empty")}
	}
	jsonBytes, err := ToJSON(raw)
	if err != nil {
		return &ValidationError{Type: contract.InterfaceTypeJSONSchema, Err: err}
	}
	if _, err := NewOfflineCompiler().Compile(jsonBytes); err != nil {
		return &ValidationError{Type: contract.InterfaceTypeJSONSchema, Err: err}
	}
	return nil
}

// ErrExternalReference is returned (wrapped) when a JSON Schema refers to a document outside
// itself. Validation never loads anything over the network or from disk, so a hub can't be made
// to fetch URLs by the documents pushed to it.
var ErrExternalReference = errors.New("external references are not loaded")

// NewOfflineCompiler returns a JSON Schema compiler that refuses to load http, https and file
// references. References inside the document, and the standard meta-schemas, still resolve.
func NewOfflineCompiler() *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	refuse := func(url string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("%w: %s (only references inside the document resolve)", ErrExternalReference, url)
	}
	for _, scheme := range []string{"http", "https", "file"} {
		c.RegisterLoader(scheme, refuse)
	}
	return c
}

// Detect reports whether raw is a valid OpenAPI or JSON Schema document.
// OpenAPI is checked first so OpenAPI documents are not classified as JSON
// Schema.
func Detect(raw []byte) (contract.InterfaceType, error) {
	doc, err := parseMap(raw)
	if err != nil {
		return "", &ValidationError{Err: err}
	}
	if looksLikeOpenAPI(doc) {
		if err := ValidateOpenAPI(raw); err != nil {
			return "", err
		}
		return contract.InterfaceTypeOpenAPI, nil
	}
	if looksLikeJSONSchema(doc) {
		if err := ValidateJSONSchema(raw); err != nil {
			return "", err
		}
		return contract.InterfaceTypeJSONSchema, nil
	}
	return "", &ValidationError{}
}

// ToJSON converts a JSON or YAML document to JSON bytes. JSON input is
// returned unchanged.
func ToJSON(raw []byte) ([]byte, error) {
	if json.Valid(raw) {
		return raw, nil
	}
	var v any
	if err := yaml.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func parseMap(raw []byte) (map[string]any, error) {
	var doc map[string]any
	if json.Valid(raw) {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, err
		}
		return doc, nil
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("empty document")
	}
	return doc, nil
}

func looksLikeOpenAPI(doc map[string]any) bool {
	for _, k := range []string{"openapi", "swagger"} {
		if v, ok := doc[k]; ok && v != nil {
			return true
		}
	}
	return false
}

func looksLikeJSONSchema(doc map[string]any) bool {
	for _, k := range []string{"$schema", "properties", "$defs", "definitions"} {
		if _, ok := doc[k]; ok {
			return true
		}
	}
	if t, ok := doc["type"].(string); ok {
		switch t {
		case "object", "array", "string", "number", "integer", "boolean", "null":
			return true
		}
	}
	return false
}
