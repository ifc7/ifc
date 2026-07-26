package project

import (
	"os"
	"testing"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/testutils"
)

func TestDetectSpecificationType(t *testing.T) {
	openapiYAML, err := os.ReadFile("internal/test/data/ifc7-rest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	jsonSchema, err := os.ReadFile("interfaces/ifc_yaml/ifc_yaml.schema.json")
	if err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		data    []byte
		expType client.InterfaceType
		expErr  error
	}{
		"openapi yaml": {
			data:    openapiYAML,
			expType: client.OPENAPI,
		},
		"json schema": {
			data:    jsonSchema,
			expType: client.JSONSCHEMA,
		},
		"random json object": {
			data:   []byte(`{"foo": "bar"}`),
			expErr: ErrInvalidSpecification,
		},
		"invalid yaml": {
			data:   []byte(":\n  -"),
			expErr: ErrInvalidSpecification,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := DetectSpecificationType(tc.data)
			if end := testutils.CheckErr(t, err, tc.expErr); end {
				return
			}
			if got != tc.expType {
				t.Fatalf("expected type %q, got %q", tc.expType, got)
			}
		})
	}
}

func TestDefaultInterfaceName(t *testing.T) {
	for name, tc := range map[string]struct {
		path string
		want string
	}{
		"yaml":        {path: "interfaces/device_auth/device_auth.yaml", want: "device-auth"},
		"schema json": {path: "foo.schema.json", want: "foo"},
		"simple":      {path: "api.yaml", want: "api"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := defaultInterfaceName(tc.path); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
