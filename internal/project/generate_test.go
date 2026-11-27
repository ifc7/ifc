package project

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

const generateSpec = `openapi: "3.0.3"
info: {title: Pets, version: "1"}
paths:
  /pets/{id}:
    get:
      operationId: getPet
      parameters:
        - {name: id, in: path, required: true, schema: {type: string, format: uuid}}
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema: {$ref: "#/components/schemas/Pet"}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`

func TestGenerateFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	spec := filepath.Join(dir, "pets.yaml")
	if err := os.WriteFile(spec, []byte(generateSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	schema := filepath.Join(dir, "pet.schema.json")
	if err := os.WriteFile(schema, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		path        string
		opts        GenerateOptions
		wantErr     string
		wantFiles   []string
		wantPackage string
	}{
		{
			name:        "into an existing module",
			path:        spec,
			opts:        GenerateOptions{OutDir: filepath.Join(dir, "pet-client")},
			wantFiles:   []string{"client.gen.go", "server.gen.go", "types.gen.go"},
			wantPackage: "petclient",
		},
		{
			name:        "as its own module",
			path:        spec,
			opts:        GenerateOptions{OutDir: filepath.Join(dir, "mod"), Module: "example.com/pets", Package: "pets"},
			wantFiles:   []string{"client.gen.go", "go.mod", "server.gen.go", "types.gen.go"},
			wantPackage: "pets",
		},
		{
			name:    "json schema is not supported",
			path:    schema,
			opts:    GenerateOptions{OutDir: filepath.Join(dir, "schema")},
			wantErr: "no generate plugin",
		},
		{
			name:    "an output directory is required",
			path:    spec,
			wantErr: "output directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := GenerateFile(tt.path, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.Package != tt.wantPackage {
				t.Errorf("package = %q, want %q", res.Package, tt.wantPackage)
			}
			entries, err := os.ReadDir(tt.opts.OutDir)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, e := range entries {
				got = append(got, e.Name())
			}
			if !slices.Equal(got, tt.wantFiles) {
				t.Errorf("files = %v, want %v", got, tt.wantFiles)
			}
			if res.Requires["github.com/google/uuid"] == "" {
				t.Errorf("requires = %v, want github.com/google/uuid", res.Requires)
			}
		})
	}
}

func TestWriteGenerated(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"nested path", "a/b.go", false},
		{"parent escape", "../x.go", true},
		{"absolute path", "/tmp/x.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			_, err := writeGenerated(dir, []contract.GeneratedFile{{Path: tt.path, Contents: "x"}})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
