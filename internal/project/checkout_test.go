package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/testutils"
)

func TestProject_Checkout(t *testing.T) {
	testutils.UseSandbox(t)

	base := []byte("openapi: 3.0.0\ninfo:\n  title: t\n  version: 0.1.0\npaths: {}\n")
	newer := []byte("openapi: 3.0.0\ninfo:\n  title: newer\n  version: 0.2.0\npaths: {}\n")
	path := "api.yaml"

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

	t.Run("missing file created", func(t *testing.T) {
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(base), nil)
		if err != nil {
			t.Fatal(err)
		}
		messages, err := proj.Checkout(t.Context(), CheckoutParams{Targets: []string{"api"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != 1 || !strings.Contains(messages[0], `Updated "api"`) {
			t.Fatalf("unexpected messages: %v", messages)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(base) {
			t.Fatalf("file content = %q, want %q", got, base)
		}
	})

	t.Run("clean is no-op", func(t *testing.T) {
		if err := os.WriteFile(path, base, 0o644); err != nil {
			t.Fatal(err)
		}
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(base), nil)
		if err != nil {
			t.Fatal(err)
		}
		messages, err := proj.Checkout(t.Context(), CheckoutParams{Targets: []string{"api"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != 1 || !strings.Contains(messages[0], "already up to date") {
			t.Fatalf("unexpected messages: %v", messages)
		}
	})

	t.Run("modified without force is skipped", func(t *testing.T) {
		if err := os.WriteFile(path, base, 0o644); err != nil {
			t.Fatal(err)
		}
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(newer), nil)
		if err != nil {
			t.Fatal(err)
		}
		messages, err := proj.Checkout(t.Context(), CheckoutParams{Targets: []string{"api"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != 1 || !strings.Contains(messages[0], "Skipped") || !strings.Contains(messages[0], "--force") {
			t.Fatalf("unexpected messages: %v", messages)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(base) {
			t.Fatalf("file should be unchanged, got %q", got)
		}
	})

	t.Run("modified with force overwrites", func(t *testing.T) {
		if err := os.WriteFile(path, base, 0o644); err != nil {
			t.Fatal(err)
		}
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(newer), nil)
		if err != nil {
			t.Fatal(err)
		}
		messages, err := proj.Checkout(t.Context(), CheckoutParams{
			Targets: []string{"api"},
			Force:   true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != 1 || !strings.Contains(messages[0], `Updated "api"`) {
			t.Fatalf("unexpected messages: %v", messages)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(newer) {
			t.Fatalf("file content = %q, want %q", got, newer)
		}
	})

	t.Run("by slug", func(t *testing.T) {
		_ = os.Remove(path)
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(base), nil)
		if err != nil {
			t.Fatal(err)
		}
		messages, err := proj.Checkout(t.Context(), CheckoutParams{Targets: []string{"api-slug"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != 1 || !strings.Contains(messages[0], `Updated "api"`) {
			t.Fatalf("unexpected messages: %v", messages)
		}
	})

	t.Run("unknown name", func(t *testing.T) {
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, manifestFor(base), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = proj.Checkout(t.Context(), CheckoutParams{Targets: []string{"missing"}})
		if err == nil || !strings.Contains(err.Error(), "unknown owned interface") {
			t.Fatalf("expected unknown owned interface error, got %v", err)
		}
	})

	t.Run("no committed revision", func(t *testing.T) {
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{{Name: "api", Path: path}},
		}, Manifest{
			Interfaces: map[string]*ManifestInterface{
				"api": {
					Interface: client.Interface{Name: "api"},
				},
			},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = proj.Checkout(t.Context(), CheckoutParams{Targets: []string{"api"}})
		if err == nil || !strings.Contains(err.Error(), "no committed revision") {
			t.Fatalf("expected no committed revision error, got %v", err)
		}
	})

	t.Run("all owned when no targets", func(t *testing.T) {
		otherPath := filepath.Join("nested", "other.yaml")
		_ = os.Remove(path)
		_ = os.RemoveAll("nested")
		proj, err := projectWithMockClient(t, Config{
			Own: []Owned{
				{Name: "api", Path: path},
				{Name: "other", Path: otherPath},
			},
		}, Manifest{
			Interfaces: map[string]*ManifestInterface{
				"api": {
					Interface: client.Interface{
						Name: "api",
						LatestRevision: &client.InterfaceRevision{
							Checksum:      sha256Checksum(base),
							Specification: base64Encode(base),
						},
					},
				},
				"other": {
					Interface: client.Interface{
						Name: "other",
						LatestRevision: &client.InterfaceRevision{
							Checksum:      sha256Checksum(newer),
							Specification: base64Encode(newer),
						},
					},
				},
			},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		messages, err := proj.Checkout(t.Context(), CheckoutParams{})
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %v", messages)
		}
		gotAPI, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		gotOther, err := os.ReadFile(otherPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotAPI) != string(base) {
			t.Fatalf("api content = %q, want %q", gotAPI, base)
		}
		if string(gotOther) != string(newer) {
			t.Fatalf("other content = %q, want %q", gotOther, newer)
		}
	})
}
