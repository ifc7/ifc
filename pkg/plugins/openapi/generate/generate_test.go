package generate_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/openapi/generate"
)

func petstore(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "petstore.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func input(spec []byte, mutate func(*contract.GenerationInput)) contract.GenerationInput {
	in := contract.GenerationInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Document:      contract.NewSpecificationDocument(spec, contract.DetectFileFormat(spec)),
		Target:        contract.GenerationTarget{Language: contract.SdkLanguageGo},
		Package: contract.GenerationPackage{
			Name:    "ifc7.dev/go/org/acme/pet-store/v2",
			Version: "v2.0.0",
			Details: map[string]any{"packageName": "petstore", "goVersion": "1.24"},
		},
	}
	if mutate != nil {
		mutate(&in)
	}
	return in
}

func fileMap(out contract.GenerationOutput) map[string]string {
	m := map[string]string{}
	for _, f := range out.Files {
		m[f.Path] = f.Contents
	}
	return m
}

func TestPlugin_Generate(t *testing.T) {
	t.Parallel()
	spec := petstore(t)
	externalRef := []byte(`openapi: "3.0.3"
info: {title: x, version: "1"}
paths:
  /a:
    get:
      operationId: a
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "https://example.com/schemas.yaml#/Pet"
`)

	tests := []struct {
		name    string
		in      contract.GenerationInput
		wantErr string
		check   func(t *testing.T, files map[string]string)
	}{
		{
			name: "models, strict server and client with a go.mod",
			in:   input(spec, nil),
			check: func(t *testing.T, files map[string]string) {
				for _, p := range []string{generate.TypesFile, generate.ServerFile, generate.ClientFile, generate.GoModFile} {
					if _, ok := files[p]; !ok {
						t.Fatalf("missing %s", p)
					}
				}
				if !strings.Contains(files[generate.TypesFile], "package petstore") {
					t.Errorf("types file has wrong package clause")
				}
				if !strings.Contains(files[generate.ServerFile], "ListPetsRequestObject") {
					t.Errorf("server file is not a strict server")
				}
				if !strings.Contains(files[generate.ClientFile], "ClientWithResponses") {
					t.Errorf("client file has no client")
				}
				mod := files[generate.GoModFile]
				for _, want := range []string{"module ifc7.dev/go/org/acme/pet-store/v2", "go 1.24", "github.com/google/uuid v1.6.0"} {
					if !strings.Contains(mod, want) {
						t.Errorf("go.mod missing %q:\n%s", want, mod)
					}
				}
			},
		},
		{
			name: "package name and go version default from the module path",
			in: input(spec, func(in *contract.GenerationInput) {
				in.Package.Details = nil
			}),
			check: func(t *testing.T, files map[string]string) {
				if !strings.Contains(files[generate.TypesFile], "package petstore\n") {
					t.Errorf("want package petstore derived from pet-store/v2")
				}
				if !strings.Contains(files[generate.GoModFile], "go "+generate.DefaultGoVersion) {
					t.Errorf("want default go version")
				}
			},
		},
		{
			name:    "json schema is not supported",
			in:      input(spec, func(in *contract.GenerationInput) { in.InterfaceType = contract.InterfaceTypeJSONSchema }),
			wantErr: "unsupported interface type",
		},
		{
			name:    "only go is supported",
			in:      input(spec, func(in *contract.GenerationInput) { in.Target.Language = "ts" }),
			wantErr: "unsupported language",
		},
		{
			name:    "package name must be a Go identifier",
			in:      input(spec, func(in *contract.GenerationInput) { in.Package.Details["packageName"] = "pet-store" }),
			wantErr: "not a Go identifier",
		},
		{
			name:    "module path is required",
			in:      input(spec, func(in *contract.GenerationInput) { in.Package.Name = "" }),
			wantErr: "package.name is required",
		},
		{
			name:    "external refs fail before anything is fetched",
			in:      input(externalRef, nil),
			wantErr: "https://example.com/schemas.yaml#/Pet",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := generate.Plugin{}.Generate(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, fileMap(out))
		})
	}
}

func TestPlugin_Generate_externalRefSentinel(t *testing.T) {
	t.Parallel()
	spec := []byte(`{"openapi":"3.0.3","info":{"title":"x","version":"1"},"paths":{},"components":{"schemas":{"A":{"$ref":"./a.json"}}}}`)
	_, err := generate.Plugin{}.Generate(input(spec, nil))
	if !errors.Is(err, generate.ErrExternalRef) {
		t.Fatalf("err = %v, want ErrExternalRef", err)
	}
}

func TestPlugin_ID(t *testing.T) {
	t.Parallel()
	if got := (generate.Plugin{}).ID(); got != generate.PluginID {
		t.Fatalf("ID() = %q", got)
	}
}

// TestGeneratedModuleBuilds checks the generated module compiles. It downloads the required
// modules, so it is skipped with -short.
func TestGeneratedModuleBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("needs the Go module proxy")
	}
	t.Parallel()
	out, err := generate.Plugin{}.Generate(input(petstore(t), nil))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, f := range out.Files {
		if err := os.WriteFile(filepath.Join(dir, f.Path), []byte(f.Contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"mod", "download", "all"}, {"vet", "-mod=mod", "./..."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, b)
		}
	}
}
