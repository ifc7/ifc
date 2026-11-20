package project

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/spec"
)

// DetectSpecificationType reports whether data is a valid OpenAPI or JSON Schema
// document. Detection and validation are delegated to pkg/spec so the CLI and
// the hub accept the same documents.
func DetectSpecificationType(data []byte) (client.InterfaceType, error) {
	typ, err := spec.Detect(data)
	if err != nil {
		return "", err
	}
	switch typ {
	case contract.InterfaceTypeOpenAPI:
		return client.OPENAPI, nil
	case contract.InterfaceTypeJSONSchema:
		return client.JSONSCHEMA, nil
	default:
		return "", fmt.Errorf("%w: unsupported type %q", ErrInvalidSpecification, typ)
	}
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
