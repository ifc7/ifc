package spec_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/spec"
)

func read(t *testing.T, parts ...string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDetect(t *testing.T) {
	openapi := read(t, "..", "plugins", "testdata", "specs", "openapi-ok.yaml")
	schema := read(t, "..", "plugins", "testdata", "specs", "schema-before.json")
	for name, tc := range map[string]struct {
		in      []byte
		want    contract.InterfaceType
		wantErr bool
	}{
		"openapi":         {in: openapi, want: contract.InterfaceTypeOpenAPI},
		"json schema":     {in: schema, want: contract.InterfaceTypeJSONSchema},
		"yaml schema":     {in: []byte("type: object\nproperties:\n  a: {type: string}\n"), want: contract.InterfaceTypeJSONSchema},
		"random object":   {in: []byte(`{"foo":"bar"}`), wantErr: true},
		"invalid yaml":    {in: []byte(":\n  -"), wantErr: true},
		"bad json schema": {in: []byte(`{"type":"object","properties":{"a":{"type":12}}}`), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := spec.Detect(tc.in)
			if tc.wantErr {
				if !errors.Is(err, spec.ErrInvalidSpecification) {
					t.Fatalf("err = %v, want ErrInvalidSpecification", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("Detect = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	openapi := read(t, "..", "plugins", "testdata", "specs", "openapi-ok.yaml")
	if err := spec.Validate(contract.InterfaceTypeOpenAPI, openapi); err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(contract.InterfaceTypeJSONSchema, []byte(`{"type":"string"}`)); err != nil {
		t.Fatal(err)
	}
	err := spec.Validate(contract.InterfaceTypeOpenAPI, []byte("not: openapi\n"))
	var verr *spec.ValidationError
	if !errors.As(err, &verr) || verr.Type != contract.InterfaceTypeOpenAPI || !errors.Is(err, spec.ErrInvalidSpecification) {
		t.Fatalf("err = %v", err)
	}
	if err := spec.Validate(contract.InterfaceTypeOpenAPI, nil); !errors.Is(err, spec.ErrInvalidSpecification) {
		t.Fatalf("empty: %v", err)
	}
	if err := spec.Validate("GRAPHQL", []byte("x")); !errors.Is(err, spec.ErrUnsupportedType) {
		t.Fatalf("unsupported: %v", err)
	}
}

func TestDetectFileFormat(t *testing.T) {
	if spec.DetectFileFormat([]byte(`{"a":1}`)) != contract.FileFormatJSON || spec.DetectFileFormat([]byte("a: 1\n")) != contract.FileFormatYAML {
		t.Fatal("wrong format")
	}
}
