package runner_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/runner"
)

func TestDefaultRegistry_LintOpenAPI(t *testing.T) {
	reg := runner.DefaultRegistry()
	raw := readSpec(t, "openapi-ok.yaml")
	out, id, err := reg.Lint(contract.LintInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Document:      contract.NewSpecificationDocument(raw, contract.FileFormatYAML),
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "openapi-lint@v2" {
		t.Fatalf("plugin id = %q", id)
	}
	if out.Score < 0 || out.Score > 100 {
		t.Fatalf("score = %d, want 0-100; raw=%q", out.Score, out.Raw)
	}
}

func TestDefaultRegistry_LintOpenAPIMissingOpID(t *testing.T) {
	reg := runner.DefaultRegistry()
	raw := readSpec(t, "openapi-missing-opid.yaml")
	out, _, err := reg.Lint(contract.LintInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Document:      contract.NewSpecificationDocument(raw, contract.FileFormatYAML),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Score >= 100 {
		t.Fatalf("expected score < 100, got %d", out.Score)
	}
	if out.Raw == "" {
		t.Fatal("expected raw findings")
	}
}

func TestDefaultRegistry_CompareOpenAPIBreaking(t *testing.T) {
	reg := runner.DefaultRegistry()
	before := readSpec(t, "openapi-ok.yaml")
	after := readSpec(t, "openapi-breaking-after.yaml")
	out, id, err := reg.Compare(contract.CompareInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Before:        contract.NewSpecificationDocument(before, contract.FileFormatYAML),
		After:         contract.NewSpecificationDocument(after, contract.FileFormatYAML),
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "openapi-compare@v1" {
		t.Fatalf("plugin id = %q", id)
	}
	if !out.Breaking {
		t.Fatalf("expected breaking=true; raw=%q", out.Raw)
	}
}

func TestDefaultRegistry_LintJSONSchema(t *testing.T) {
	reg := runner.DefaultRegistry()
	raw := readSpec(t, "schema-before.json")
	out, id, err := reg.Lint(contract.LintInput{
		InterfaceType: contract.InterfaceTypeJSONSchema,
		Document:      contract.NewSpecificationDocument(raw, contract.FileFormatJSON),
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "jsonschema-lint@v2" {
		t.Fatalf("plugin id = %q", id)
	}
	if out.Score < 0 || out.Score > 100 {
		t.Fatalf("score = %d, want 0-100; raw=%q", out.Score, out.Raw)
	}
}

func TestDefaultRegistry_CompareJSONSchemaBreaking(t *testing.T) {
	reg := runner.DefaultRegistry()
	before := readSpec(t, "schema-before.json")
	after := readSpec(t, "schema-after.json")
	out, _, err := reg.Compare(contract.CompareInput{
		InterfaceType: contract.InterfaceTypeJSONSchema,
		Before:        contract.NewSpecificationDocument(before, contract.FileFormatJSON),
		After:         contract.NewSpecificationDocument(after, contract.FileFormatJSON),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Breaking {
		t.Fatalf("expected breaking=true; raw=%q", out.Raw)
	}
}

func readSpec(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "specs", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRegistry_Generate(t *testing.T) {
	t.Parallel()
	reg := runner.DefaultRegistry()
	raw := readSpec(t, "openapi-ok.yaml")
	in := func(typ contract.InterfaceType) contract.GenerationInput {
		return contract.GenerationInput{
			InterfaceType: typ,
			Document:      contract.NewSpecificationDocument(raw, contract.FileFormatYAML),
			Target:        contract.GenerationTarget{Language: contract.SdkLanguageGo},
			Package:       contract.GenerationPackage{Name: "example.com/pets", Version: "v1.0.0"},
		}
	}
	t.Run("openapi go", func(t *testing.T) {
		t.Parallel()
		out, id, err := reg.Generate(in(contract.InterfaceTypeOpenAPI))
		if err != nil {
			t.Fatal(err)
		}
		if id != "openapi-go@v1" {
			t.Fatalf("plugin id = %q", id)
		}
		if len(out.Files) == 0 {
			t.Fatal("no files")
		}
	})
	t.Run("json schema has no generator", func(t *testing.T) {
		t.Parallel()
		_, _, err := reg.Generate(in(contract.InterfaceTypeJSONSchema))
		if !errors.Is(err, runner.ErrNoGenerator) {
			t.Fatalf("err = %v, want ErrNoGenerator", err)
		}
	})
}

func TestRegistry_HasGenerator(t *testing.T) {
	t.Parallel()
	reg := runner.DefaultRegistry()
	tests := []struct {
		name string
		typ  contract.InterfaceType
		lang contract.SdkLanguage
		want bool
	}{
		{"openapi go", contract.InterfaceTypeOpenAPI, contract.SdkLanguageGo, true},
		{"json schema go", contract.InterfaceTypeJSONSchema, contract.SdkLanguageGo, false},
		{"openapi unknown language", contract.InterfaceTypeOpenAPI, "ts", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := reg.HasGenerator(tt.typ, tt.lang); got != tt.want {
				t.Fatalf("HasGenerator = %v, want %v", got, tt.want)
			}
		})
	}
}
