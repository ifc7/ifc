package collection

import (
	"embed"
	"fmt"
	"sync"

	"github.com/kaptinlin/jsonschema"
)

//go:embed schemas/*.json
var schemaFiles embed.FS

const (
	openCollectionSchemaVersion  = "1.0.0"
	openCollectionPackageVersion = "0.15.0"
)

var (
	schemaMu    sync.Mutex
	schemaCache = map[string]*jsonschema.Schema{}
)

// formatSchema compiles an embedded schema once.
func formatSchema(name string) (*jsonschema.Schema, error) {
	schemaMu.Lock()
	defer schemaMu.Unlock()
	if s, ok := schemaCache[name]; ok {
		return s, nil
	}
	raw, err := schemaFiles.ReadFile("schemas/" + name)
	if err != nil {
		return nil, fmt.Errorf("collection schema %s: %w", name, err)
	}
	s, err := offlineCompiler().Compile(raw)
	if err != nil {
		return nil, fmt.Errorf("compile collection schema %s: %w", name, err)
	}
	schemaCache[name] = s
	return s, nil
}
