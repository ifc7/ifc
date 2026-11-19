package lint_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/governance/ruleset"
	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/openapi/lint"
)

func readSpec(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "specs", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func lintWith(t *testing.T, raw []byte, rs string) contract.LintOutput {
	t.Helper()
	in := contract.LintInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
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

func ruleIDs(out contract.LintOutput) []string {
	var ids []string
	for _, f := range out.Findings {
		ids = append(ids, f.RuleID)
	}
	return ids
}

func TestLintDefault(t *testing.T) {
	out := lintWith(t, readSpec(t, "openapi-missing-opid.yaml"), "")
	if out.Engine == nil || out.Engine.Name != "vacuum" || out.Engine.Version == "" {
		t.Fatalf("engine = %+v", out.Engine)
	}
	if !slices.Contains(ruleIDs(out), "operation-operationId") || !slices.Contains(ruleIDs(out), "oas3-api-servers") {
		t.Fatalf("findings = %v", ruleIDs(out))
	}
	if out.Score != findings.Score(out.Findings, nil) {
		t.Fatalf("score %d is not the ifc7 score of its findings", out.Score)
	}
	if out.Raw != findings.RenderText(out.Findings) {
		t.Fatal("raw should be rendered from findings")
	}
	for _, f := range out.Findings {
		if len(f.Fingerprint) != 64 || f.Range == nil || f.Range.Start.Line < 1 {
			t.Fatalf("incomplete finding %+v", f)
		}
		if f.Fingerprint != findings.Fingerprint(f.RuleID, f.Pointer, f.Message) {
			t.Fatalf("fingerprint mismatch for %+v", f)
		}
	}
	// No ruleset and the explicit default ruleset behave identically.
	explicit := lintWith(t, readSpec(t, "openapi-missing-opid.yaml"), "extends: ifc7:recommended\n")
	if explicit.Score != out.Score || explicit.Raw != out.Raw {
		t.Fatal("explicit default ruleset should match no ruleset")
	}
}

func TestLintRulesetTurnsRuleOffAndChangesSeverity(t *testing.T) {
	raw := readSpec(t, "openapi-missing-opid.yaml")
	base := lintWith(t, raw, "")
	tuned := lintWith(t, raw, "extends: spectral:oas\nrules:\n  oas3-api-servers: off\n  operation-tags: error\n")
	if slices.Contains(ruleIDs(tuned), "oas3-api-servers") {
		t.Fatalf("oas3-api-servers should be off: %v", ruleIDs(tuned))
	}
	for _, f := range tuned.Findings {
		if f.RuleID == "operation-tags" && f.Severity != contract.SeverityError {
			t.Fatalf("operation-tags severity = %s, want error", f.Severity)
		}
	}
	if len(tuned.Findings) != len(base.Findings)-1 {
		t.Fatalf("want one fewer finding: base=%v tuned=%v", ruleIDs(base), ruleIDs(tuned))
	}
}

func TestLintInlineRule(t *testing.T) {
	out := lintWith(t, readSpec(t, "openapi-ok.yaml"), `
extends: [[spectral:oas, off]]
rules:
  info-must-have-x-owner:
    description: info must name an owner
    message: info is missing x-owner
    given: $.info
    severity: error
    then:
      field: x-owner
      function: truthy
`)
	if got := ruleIDs(out); !slices.Equal(got, []string{"info-must-have-x-owner"}) {
		t.Fatalf("findings = %v", got)
	}
	if out.Findings[0].Pointer != "/info/x-owner" || out.Score != 90 {
		t.Fatalf("finding = %+v score=%d", out.Findings[0], out.Score)
	}
}

func TestFingerprintsSurviveReformattingAndKeyReordering(t *testing.T) {
	yamlOut := lintWith(t, readSpec(t, "openapi-missing-opid.yaml"), "")
	jsonOut := lintWith(t, readSpec(t, "openapi-missing-opid-reordered.json"), "")
	fp := func(out contract.LintOutput) []string {
		var s []string
		for _, f := range out.Findings {
			s = append(s, f.RuleID+" "+f.Pointer+" "+f.Fingerprint)
		}
		slices.Sort(s)
		return s
	}
	if !slices.Equal(fp(yamlOut), fp(jsonOut)) {
		t.Fatalf("fingerprints differ:\nyaml: %v\njson: %v", fp(yamlOut), fp(jsonOut))
	}
	if yamlOut.Score != jsonOut.Score {
		t.Fatalf("score differs: %d vs %d", yamlOut.Score, jsonOut.Score)
	}
}

func TestLintRejectsBadRuleset(t *testing.T) {
	raw := readSpec(t, "openapi-ok.yaml")
	_, err := lint.Plugin{}.Lint(contract.LintInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Document:      contract.NewSpecificationDocument(raw, contract.FileFormatYAML),
		Ruleset:       contract.NewRulesetDocument([]byte("foo: bar\n"), ""),
	})
	if err == nil {
		t.Fatal("expected error for invalid ruleset")
	}
}

func TestLintWrongType(t *testing.T) {
	_, err := lint.Plugin{}.Lint(contract.LintInput{InterfaceType: contract.InterfaceTypeJSONSchema})
	if err == nil {
		t.Fatal("expected error")
	}
}
