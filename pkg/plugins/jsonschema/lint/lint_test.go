package lint_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ifc7/ifc/pkg/governance/ruleset"
	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/jsonschema/lint"
)

func lintWith(t *testing.T, raw []byte, rs string) contract.LintOutput {
	t.Helper()
	in := contract.LintInput{
		InterfaceType: contract.InterfaceTypeJSONSchema,
		Document:      contract.NewSpecificationDocument(raw, contract.DetectFileFormat(raw)),
	}
	if rs != "" {
		_, b, sum, err := ruleset.Effective(context.Background(), []byte(rs), nil)
		if err != nil {
			t.Fatal(err)
		}
		in.Ruleset = contract.NewRulesetDocument(b, sum)
	}
	out, err := lint.Plugin{}.Lint(in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLintJSONSchema(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "specs", "schema-before.json"))
	if err != nil {
		t.Fatal(err)
	}
	def := lintWith(t, raw, "")
	if def.Engine == nil || def.Score < 0 || def.Score > 100 {
		t.Fatalf("output = %+v", def)
	}
	// An org ruleset written for OpenAPI still lints JSON Schema interfaces
	// with the recommended JSON Schema rules; OpenAPI rule names are ignored.
	oasOnly := lintWith(t, raw, "extends: spectral:oas\nrules:\n  oas3-api-servers: off\n")
	if oasOnly.Score != def.Score || oasOnly.Raw != def.Raw {
		t.Fatalf("OpenAPI-only ruleset changed JSON Schema results:\n%s\nvs\n%s", oasOnly.Raw, def.Raw)
	}
}

func TestLintUnparsable(t *testing.T) {
	out := lintWith(t, []byte("{: ["), "")
	if len(out.Findings) != 1 || out.Findings[0].Severity != contract.SeverityError || out.Score != 90 {
		t.Fatalf("output = %+v", out)
	}
}
