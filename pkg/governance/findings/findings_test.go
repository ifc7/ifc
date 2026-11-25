package findings_test

import (
	"testing"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

func fs(sevs ...contract.Severity) []contract.Finding {
	out := make([]contract.Finding, len(sevs))
	for i, s := range sevs {
		out[i] = contract.Finding{RuleID: "r" + string(s), Severity: s}
	}
	return out
}

func TestScore(t *testing.T) {
	E, W, I, H := contract.SeverityError, contract.SeverityWarn, contract.SeverityInfo, contract.SeverityHint
	for name, tc := range map[string]struct {
		findings []contract.Finding
		weights  findings.Weights
		want     int
	}{
		"none":                  {want: 100},
		"one error":             {findings: fs(E), want: 90},             // 100e^-0.1
		"one warn":              {findings: fs(W), want: 95},             // 100e^-0.05
		"one info":              {findings: fs(I), want: 98},             // 100e^-0.02
		"one hint":              {findings: fs(H), want: 99},             // 100e^-0.01
		"two errors three warn": {findings: fs(E, E, W, W, W), want: 70}, // 100e^-0.35
		"custom weights":        {findings: fs(E, W), weights: findings.Weights{contract.SeverityWarn: 0}, want: 90},
		"zero weights":          {findings: fs(E, E), weights: findings.Weights{E: 0}, want: 100},
	} {
		t.Run(name, func(t *testing.T) {
			if got := findings.Score(tc.findings, tc.weights); got != tc.want {
				t.Fatalf("Score = %d, want %d", got, tc.want)
			}
		})
	}
	many := make([]contract.Finding, 1000)
	for i := range many {
		many[i].Severity = contract.SeverityError
	}
	if got := findings.Score(many, nil); got != 0 {
		t.Fatalf("Score(1000 errors) = %d, want 0", got)
	}
}

func TestWeightsValidate(t *testing.T) {
	if err := findings.DefaultWeights().Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (findings.Weights{"fatal": 1}).Validate(); err == nil {
		t.Fatal("expected error for unknown severity")
	}
	if err := (findings.Weights{contract.SeverityError: -1}).Validate(); err == nil {
		t.Fatal("expected error for negative weight")
	}
}

func TestSummarise(t *testing.T) {
	in := []contract.Finding{
		{RuleID: "a", Severity: contract.SeverityError},
		{RuleID: "a", Severity: contract.SeverityError},
		{RuleID: "b", Severity: contract.SeverityWarn},
	}
	s := findings.Summarise(in)
	if s.Total != 3 || s.BySeverity[contract.SeverityError] != 2 || s.BySeverity[contract.SeverityWarn] != 1 ||
		s.BySeverity[contract.SeverityInfo] != 0 || s.BySeverity[contract.SeverityHint] != 0 {
		t.Fatalf("by severity = %+v", s)
	}
	if _, ok := s.BySeverity[contract.SeverityHint]; !ok {
		t.Fatal("every severity should be present")
	}
	if s.ByRule["a"] != 2 || s.ByRule["b"] != 1 {
		t.Fatalf("by rule = %+v", s.ByRule)
	}
}

func TestNormaliseMessage(t *testing.T) {
	for in, want := range map[string]string{
		"the `GET` operation does not contain an `operationId`": `the "*" operation does not contain an "*"`,
		"schema 'Pet' has 12 properties":                        `schema "*" has # properties`,
		`value "x"  is   1.5 too   big`:                         `value "*" is # too big`,
	} {
		if got := findings.NormaliseMessage(in); got != want {
			t.Errorf("NormaliseMessage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFingerprint(t *testing.T) {
	// Golden value: changing this breaks every stored fingerprint, waiver
	// and baseline. Only change it on purpose.
	const golden = "19c8b96300931ab915968840b816c6962a480c7494e142fce52275429fec9b51"
	got := findings.Fingerprint("operation-operationId", "/paths/~1pets/get", "the `GET` operation does not contain an `operationId`")
	if len(got) != 64 {
		t.Fatalf("fingerprint length %d", len(got))
	}
	if got != golden {
		t.Fatalf("fingerprint drifted: got %s", got)
	}
	same := findings.Fingerprint("operation-operationId", "/paths/~1pets/get", "the `POST`  operation does not contain an `operationId`")
	if same != got {
		t.Fatal("quoted values and whitespace should not change the fingerprint")
	}
	for _, other := range []string{
		findings.Fingerprint("operation-tags", "/paths/~1pets/get", "the `GET` operation does not contain an `operationId`"),
		findings.Fingerprint("operation-operationId", "/paths/~1dogs/get", "the `GET` operation does not contain an `operationId`"),
		findings.Fingerprint("operation-operationId", "/paths/~1pets/get", "something else"),
		findings.Fingerprint("operation-operationId/paths", "/~1pets/get", "the `GET` operation does not contain an `operationId`"),
	} {
		if other == got {
			t.Fatal("rule id, pointer and message must all contribute")
		}
	}
}

func TestPointerFromJSONPath(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "",
		"$":                    "",
		"$.info":               "/info",
		"$.info.contact.email": "/info/contact/email",
		"$.paths['/pets'].get": "/paths/~1pets/get",
		`$.paths["/pets/{id}"].get.responses['200']`: "/paths/~1pets~1{id}/get/responses/200",
		"$.servers[0].url":                           "/servers/0/url",
		"$.components.schemas['a~b']":                "/components/schemas/a~0b",
		"$['paths']":                                 "/paths",
		"/already/a/pointer":                         "/already/a/pointer",
		"#/components/schemas/Pet":                   "/components/schemas/Pet",
	} {
		if got := findings.PointerFromJSONPath(in); got != want {
			t.Errorf("PointerFromJSONPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSortAndRender(t *testing.T) {
	in := []contract.Finding{
		{RuleID: "b", Severity: contract.SeverityWarn, Message: "w", Pointer: "/x", Range: &contract.Range{Start: contract.Position{Line: 2, Column: 1}}},
		{RuleID: "a", Severity: contract.SeverityError, Message: "e", Pointer: ""},
	}
	findings.Sort(in)
	if in[0].RuleID != "a" {
		t.Fatalf("errors should sort first: %+v", in)
	}
	want := "error[a]: e\n  /\nwarn[b]: w\n  /x 2:1\n"
	if got := findings.RenderText(in); got != want {
		t.Fatalf("RenderText = %q, want %q", got, want)
	}
}

func TestNewSince(t *testing.T) {
	t.Parallel()
	f := func(fp string) contract.Finding { return contract.Finding{RuleID: "r", Fingerprint: fp} }
	got := findings.NewSince([]contract.Finding{f("a"), f("b"), f("a"), f("c")}, []contract.Finding{f("a"), f("c"), f("gone")})
	if len(got) != 2 || got[0].Fingerprint != "b" || got[1].Fingerprint != "a" {
		t.Fatalf("NewSince = %+v", got)
	}
	if got := findings.NewSince([]contract.Finding{f("a")}, nil); len(got) != 1 {
		t.Fatalf("no baseline: %+v", got)
	}
}
