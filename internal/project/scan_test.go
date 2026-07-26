package project

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/testutils"
	"github.com/ifc7/ifc/internal/tui"
)

func TestFindUntrackedSpecs(t *testing.T) {
	testutils.UseSandbox(t)

	openapi := []byte("openapi: 3.0.0\ninfo:\n  title: t\n  version: 0.1.0\npaths: {}\n")
	schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
	noise := []byte(`{"foo":"bar"}`)

	if err := os.MkdirAll("nested", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("api.yaml", openapi, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("nested", "schema.json"), schema, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("noise.json", noise, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".git", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".git", "hidden.yaml"), openapi, 0o644); err != nil {
		t.Fatal(err)
	}

	proj, err := projectWithMockClient(t, Config{
		Own: []Owned{{Name: "api", Path: "api.yaml"}},
	}, Manifest{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	candidates, err := proj.FindUntrackedSpecs(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %#v", candidates)
	}
	if candidates[0].Path != "nested/schema.json" {
		t.Fatalf("unexpected path %q", candidates[0].Path)
	}
	if candidates[0].Type != client.JSONSCHEMA {
		t.Fatalf("unexpected type %q", candidates[0].Type)
	}

	// Subfolder scan should not see specs outside that folder.
	sub, err := proj.FindUntrackedSpecs("nested")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub) != 1 || sub[0].Path != "nested/schema.json" {
		t.Fatalf("expected only nested/schema.json, got %#v", sub)
	}
}

func TestFindUntrackedSpecs_invalidRoot(t *testing.T) {
	testutils.UseSandbox(t)
	proj, err := projectWithMockClient(t, Config{}, Manifest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proj.FindUntrackedSpecs("does-not-exist"); err == nil {
		t.Fatal("expected error for missing scan path")
	}
	if err := os.WriteFile("file.yaml", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := proj.FindUntrackedSpecs("file.yaml"); err == nil {
		t.Fatal("expected error when scan path is a file")
	}
}

func TestProject_Scan_addsSelected(t *testing.T) {
	testutils.UseSandbox(t)

	openapi := []byte("openapi: 3.0.0\ninfo:\n  title: t\n  version: 0.1.0\npaths: {}\n")
	if err := os.WriteFile("petstore.yaml", openapi, 0o644); err != nil {
		t.Fatal(err)
	}

	orig := promptScanAdd
	promptScanAdd = func(ctx context.Context, candidates []tui.ScanCandidate) ([]tui.ScanSelection, error) {
		_ = ctx
		if len(candidates) != 1 {
			t.Fatalf("expected 1 candidate, got %#v", candidates)
		}
		return []tui.ScanSelection{{
			Path: candidates[0].Path,
			Name: "petstore",
			Type: candidates[0].Type,
		}}, nil
	}
	t.Cleanup(func() { promptScanAdd = orig })

	proj, err := projectWithMockClient(t, Config{}, Manifest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := proj.Scan(t.Context(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %#v", messages)
	}
	want := Config{
		Own: []Owned{{Name: "petstore", Path: "petstore.yaml"}},
	}
	if !reflect.DeepEqual(proj.config, want) {
		t.Fatalf("expected config %#v, got %#v", want, proj.config)
	}
}

func TestProject_Scan_noneFound(t *testing.T) {
	testutils.UseSandbox(t)
	proj, err := projectWithMockClient(t, Config{}, Manifest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := proj.Scan(t.Context(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0] != "No untracked interface specifications found." {
		t.Fatalf("unexpected messages %#v", messages)
	}
}
