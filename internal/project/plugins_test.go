package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ifc7/ifc/internal/project"
)

func TestLintFileOpenAPI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.yaml")
	content := []byte(`openapi: "3.0.3"
info:
  title: Test
  version: 1.0.0
paths:
  /ok:
    get:
      operationId: getOk
      responses:
        "200":
          description: ok
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := project.LintFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output.Score != 100 {
		t.Fatalf("score=%d raw=%q", result.Output.Score, result.Output.Raw)
	}
}

func TestCompareFilesJSONSchema(t *testing.T) {
	dir := t.TempDir()
	before := filepath.Join(dir, "before.json")
	after := filepath.Join(dir, "after.json")
	if err := os.WriteFile(before, []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {"a": {"type": "string"}},
  "required": ["a"]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(after, []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {},
  "required": ["a", "b"]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := project.CompareFiles(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Output.Breaking {
		t.Fatalf("expected breaking; raw=%q", result.Output.Raw)
	}
}
