package project

import (
	"os"
	"strings"
	"testing"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/testutils"
)

func TestProject_DiffOwned(t *testing.T) {
	testutils.UseSandbox(t)

	base := []byte("openapi: 3.0.0\ninfo:\n  title: t\n  version: 0.1.0\npaths: {}\n")
	path := "api.yaml"
	if err := os.WriteFile(path, base, 0o644); err != nil {
		t.Fatal(err)
	}

	manifestFor := func(content []byte) Manifest {
		return Manifest{
			Interfaces: map[string]*ManifestInterface{
				"api": {
					Interface: client.Interface{
						Name: "api",
						Slug: "api-slug",
						LatestRevision: &client.InterfaceRevision{
							Checksum:      sha256Checksum(content),
							Specification: base64Encode(content),
						},
					},
				},
			},
		}
	}

	t.Run("clean", func(t *testing.T) {
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(base), nil)
		if err != nil {
			t.Fatal(err)
		}
		diff, err := proj.DiffOwned("api")
		if err != nil {
			t.Fatal(err)
		}
		if diff != "" {
			t.Fatalf("expected empty diff, got %q", diff)
		}
	})

	t.Run("modified", func(t *testing.T) {
		modified := append(append([]byte{}, base...), []byte("  /pets: {}\n")...)
		if err := os.WriteFile(path, modified, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.WriteFile(path, base, 0o644)
		})

		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(base), nil)
		if err != nil {
			t.Fatal(err)
		}
		diff, err := proj.DiffOwned("api")
		if err != nil {
			t.Fatal(err)
		}
		if diff == "" {
			t.Fatal("expected non-empty diff")
		}
		if !strings.Contains(diff, "--- a/api.yaml (manifest)") {
			t.Fatalf("missing from-file header: %q", diff)
		}
		if !strings.Contains(diff, "+++ b/api.yaml") {
			t.Fatalf("missing to-file header: %q", diff)
		}
		if !strings.Contains(diff, "+  /pets: {}") {
			t.Fatalf("missing added line: %q", diff)
		}
	})

	t.Run("by slug", func(t *testing.T) {
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(base), nil)
		if err != nil {
			t.Fatal(err)
		}
		diff, err := proj.DiffOwned("api-slug")
		if err != nil {
			t.Fatal(err)
		}
		if diff != "" {
			t.Fatalf("expected empty diff, got %q", diff)
		}
	})

	t.Run("unknown name", func(t *testing.T) {
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(base), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = proj.DiffOwned("missing")
		if err == nil || !strings.Contains(err.Error(), "unknown owned interface") {
			t.Fatalf("expected unknown owned interface error, got %v", err)
		}
	})

	t.Run("not in manifest", func(t *testing.T) {
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, Manifest{Interfaces: map[string]*ManifestInterface{}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = proj.DiffOwned("api")
		if err == nil || !strings.Contains(err.Error(), "not in the local manifest") {
			t.Fatalf("expected not in manifest error, got %v", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: "does-not-exist.yaml"}},
		}, manifestFor(base), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = proj.DiffOwned("api")
		if err == nil || !strings.Contains(err.Error(), "file not found") {
			t.Fatalf("expected file not found error, got %v", err)
		}
	})

	t.Run("via interface id ref", func(t *testing.T) {
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path, Ref: "interface_abc"}},
		}, Manifest{
			Interfaces: map[string]*ManifestInterface{
				"interface_abc": {
					Interface: client.Interface{
						Id:   "interface_abc",
						Name: "api",
						LatestRevision: &client.InterfaceRevision{
							Checksum:      sha256Checksum(base),
							Specification: base64Encode(base),
						},
					},
				},
			},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		diff, err := proj.DiffOwned("api")
		if err != nil {
			t.Fatal(err)
		}
		if diff != "" {
			t.Fatalf("expected empty diff, got %q", diff)
		}
	})
}
