package project

import (
	"context"
	"os"
	"testing"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/testutils"
	"github.com/ifc7/ifc/internal/tui"
)

func TestProject_Commit_detectsInterfaceType(t *testing.T) {
	testutils.UseSandbox(t)

	orig := promptNewInterfaceCommit
	promptNewInterfaceCommit = func(ctx context.Context, name string, ifaceType client.InterfaceType) (tui.NewInterface, error) {
		return tui.NewInterface{
			Name:          name,
			Description:   "detected type test",
			RevisionNotes: "initial",
			Type:          ifaceType,
		}, nil
	}
	t.Cleanup(func() { promptNewInterfaceCommit = orig })

	openapi := []byte("openapi: 3.0.0\ninfo:\n  title: t\n  version: 0.1.0\npaths: {}\n")
	schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)

	for name, tc := range map[string]struct {
		filename string
		content  []byte
		wantType client.InterfaceType
	}{
		"openapi": {
			filename: "api.yaml",
			content:  openapi,
			wantType: client.OPENAPI,
		},
		"json schema": {
			filename: "schema.json",
			content:  schema,
			wantType: client.JSONSCHEMA,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(tc.filename, tc.content, 0o644); err != nil {
				t.Fatal(err)
			}
			proj, err := projectWithMockClient(t, Config{
				Own: []Owned{{Name: "iface", Path: tc.filename}},
			}, Manifest{Interfaces: map[string]*ManifestInterface{}}, func(mock *client.MockClientWithResponsesIfc) {
				expectCurrentUserID(mock)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := proj.Commit(t.Context(), CommitParams{}); err != nil {
				t.Fatalf("Commit returned error: %v", err)
			}
			got := proj.manifest.Interfaces["iface"]
			if got == nil {
				t.Fatal("expected manifest entry for iface")
			}
			if got.Type != tc.wantType {
				t.Fatalf("expected type %q, got %q", tc.wantType, got.Type)
			}
		})
	}
}
