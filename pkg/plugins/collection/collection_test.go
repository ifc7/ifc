package collection

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func validate(t *testing.T, f Format, src, iface []byte) Result {
	t.Helper()
	r, err := Validate(context.Background(), Input{Format: f, Source: src, Interface: iface})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return r
}

// rules returns "severity rule pointer" for each finding.
func rules(r Result) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, fmt.Sprintf("%s %s %s", f.Severity, f.RuleID, f.Pointer))
	}
	return out
}

func wantFindings(t *testing.T, r Result, want ...string) {
	t.Helper()
	got := rules(r)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("findings:\n got  %q\n want %q\nfull: %+v", got, want, r.Findings)
	}
}

func TestValidFixtures(t *testing.T) {
	t.Parallel()
	oas := read(t, "petstore.openapi.yaml")
	cases := []struct {
		format    Format
		file      string
		iface     []byte
		validator string
	}{
		{FormatArazzo10, "pets.arazzo.yaml", oas, "arazzo: vacuum "},
		{FormatPostman21, "pets.postman.json", oas, "postman-2.1-structure v1"},
		{FormatOpenCollection10, "pets.opencollection.yaml", oas, "opencollection-schema 1.0.0 (@opencollection/schema 0.15.0)"},
		{FormatJSONSchemaTestSuite, "pet.tests.json", read(t, "pet.schema.json"), "json-schema-test-suite v1"},
	}
	for _, c := range cases {
		t.Run(string(c.format), func(t *testing.T) {
			t.Parallel()
			r := validate(t, c.format, read(t, c.file), c.iface)
			if !r.Valid() || len(r.Findings) != 0 {
				t.Fatalf("want no findings, got %+v", r.Findings)
			}
			if !strings.HasPrefix(r.Validator, c.validator) {
				t.Fatalf("validator %q, want prefix %q", r.Validator, c.validator)
			}
			if r.Findings == nil {
				t.Fatal("findings must be an empty list, not nil")
			}
		})
	}
}

func TestFormats(t *testing.T) {
	t.Parallel()
	if len(Formats()) != 4 {
		t.Fatalf("formats: %v", Formats())
	}
	_, err := Validate(context.Background(), Input{Format: "RAML", Source: []byte("{}")})
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Validate(ctx, Input{Format: FormatPostman21, Source: []byte("{}")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	for _, f := range Formats() {
		for _, src := range []string{"", "  \n", "{", "a: [b"} {
			r := validate(t, f, []byte(src), nil)
			wantFindings(t, r, "error collection-parse ")
		}
	}
	big := make([]byte, MaxSourceBytes+1)
	r := validate(t, FormatPostman21, big, nil)
	wantFindings(t, r, "error collection-parse ")
}

func TestArazzo(t *testing.T) {
	t.Parallel()
	oas := read(t, "petstore.openapi.yaml")
	good := string(read(t, "pets.arazzo.yaml"))

	t.Run("unknown operations are warnings", func(t *testing.T) {
		t.Parallel()
		src := strings.Replace(good, "operationId: listPets", "operationId: listCats", 1)
		src = strings.Replace(src, "~1pets~1{petId}/get", "~1pets~1{petId}/delete", 1)
		r := validate(t, FormatArazzo10, []byte(src), oas)
		wantFindings(t, r,
			"warn collection-unknown-operation /workflows/0/steps/0/operationId",
			"warn collection-unknown-operation /workflows/0/steps/2/operationPath")
		if !r.Valid() || len(r.Warnings()) != 2 {
			t.Fatal("warnings must not make the source invalid")
		}
	})
	t.Run("no cross-check without an interface", func(t *testing.T) {
		t.Parallel()
		src := strings.Replace(good, "operationId: listPets", "operationId: listCats", 1)
		wantFindings(t, validate(t, FormatArazzo10, []byte(src), nil))
	})
	t.Run("no cross-check with two OpenAPI sources", func(t *testing.T) {
		t.Parallel()
		src := strings.Replace(good, "    type: openapi\n", "    type: openapi\n  - name: other\n    url: ./other.yaml\n    type: openapi\n", 1)
		src = strings.Replace(src, "operationId: listPets", "operationId: $sourceDescriptions.pets.listCats", 1)
		for _, f := range validate(t, FormatArazzo10, []byte(src), oas).Findings {
			if f.RuleID == RuleUnknownOperation {
				t.Fatalf("unexpected %+v", f)
			}
		}
	})
	t.Run("steps naming another source are skipped", func(t *testing.T) {
		t.Parallel()
		src := strings.Replace(good, "    type: openapi\n", "    type: openapi\n  - name: flows\n    url: ./flows.arazzo.yaml\n    type: arazzo\n", 1)
		src = strings.Replace(src, "$sourceDescriptions.pets.getPet", "$sourceDescriptions.other.getCat", 1)
		r := validate(t, FormatArazzo10, []byte(src), oas)
		for _, f := range r.Findings {
			if f.RuleID == RuleUnknownOperation {
				t.Fatalf("unexpected %+v", f)
			}
		}
	})
	t.Run("structural errors", func(t *testing.T) {
		t.Parallel()
		r := validate(t, FormatArazzo10, []byte("arazzo: 1.0.1\ninfo: {title: x, summary: s, description: d}\nworkflows: []\n"), nil)
		if r.Valid() {
			t.Fatalf("want invalid, got %+v", r.Findings)
		}
		for _, f := range r.Errors() {
			if f.RuleID != "arazzo-structure" {
				t.Fatalf("unexpected error %+v", f)
			}
		}
	})
	t.Run("version", func(t *testing.T) {
		t.Parallel()
		for _, src := range []string{"arazzo: 2.0.0\n", "openapi: 3.1.0\n", "[]"} {
			if validate(t, FormatArazzo10, []byte(src), nil).Valid() {
				t.Fatalf("%q: want invalid", src)
			}
		}
	})
	t.Run("never loads source descriptions", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
		defer srv.Close()
		src := strings.Replace(good, "./petstore.openapi.yaml", srv.URL+"/openapi.yaml", 1)
		validate(t, FormatArazzo10, []byte(src), nil)
		if hits.Load() != 0 {
			t.Fatalf("fetched the source description %d times", hits.Load())
		}
	})
}

func TestPostman(t *testing.T) {
	t.Parallel()
	oas := read(t, "petstore.openapi.yaml")

	t.Run("structure", func(t *testing.T) {
		t.Parallel()
		src := `{"info":{"name":"x","schema":"https://schema.getpostman.com/json/collection/v2.0.0/collection.json"},
			"item":[{"name":"f","item":[{"request":{"method":1}}]},{"name":"r"}]}`
		r := validate(t, FormatPostman21, []byte(src), nil)
		wantFindings(t, r,
			"error collection-schema /info/schema",
			"error collection-schema /item/0/item/0/request/method",
			"error collection-schema /item/1")
	})
	t.Run("missing info and item", func(t *testing.T) {
		t.Parallel()
		wantFindings(t, validate(t, FormatPostman21, []byte(`{"info":{"name":"x"}}`), nil),
			"error collection-schema ", "error collection-schema /info")
	})
	t.Run("duplicate ids and unknown requests are warnings", func(t *testing.T) {
		t.Parallel()
		src := strings.Replace(string(read(t, "pets.postman.json")), `"id": "c"`, `"id": "a"`, 1)
		src = strings.Replace(src, `"name": "Simple", "request": "{{baseUrl}}/pets/{{petId}}"`, `"name": "Simple", "request": {"method": "DELETE", "url": "{{baseUrl}}/pets/{{petId}}"}`, 1)
		r := validate(t, FormatPostman21, []byte(src), oas)
		wantFindings(t, r,
			"warn postman-duplicate-id /item/0/item/2/id",
			"warn collection-unknown-operation /item/1/request")
		if !r.Valid() {
			t.Fatal("want valid")
		}
	})
	t.Run("caps schema findings", func(t *testing.T) {
		t.Parallel()
		items := strings.Repeat(`{"request":{"method":1}},`, maxSchemaFindings+10)
		src := `{"info":{"name":"x","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},"item":[` + strings.TrimSuffix(items, ",") + `]}`
		r := validate(t, FormatPostman21, []byte(src), nil)
		if len(r.Findings) != maxSchemaFindings+1 {
			t.Fatalf("got %d findings", len(r.Findings))
		}
		last := false
		for _, f := range r.Findings {
			if f.RuleID == RuleTooManyFindings && f.Message == "10 more schema problems are not listed" {
				last = true
			}
		}
		if !last {
			t.Fatalf("no summary finding: %+v", r.Findings)
		}
	})
}

func TestOpenCollection(t *testing.T) {
	t.Parallel()
	oas := read(t, "petstore.openapi.yaml")
	good := string(read(t, "pets.opencollection.yaml"))
	t.Run("unknown request", func(t *testing.T) {
		t.Parallel()
		src := strings.Replace(good, "{method: GET, url: '{{baseUrl}}/pets/{{petId}}'}", "{method: PUT, url: '{{baseUrl}}/pets/{{petId}}'}", 1)
		wantFindings(t, validate(t, FormatOpenCollection10, []byte(src), oas), "warn collection-unknown-operation /items/1/http")
	})
	t.Run("schema error", func(t *testing.T) {
		t.Parallel()
		src := strings.Replace(good, "{method: GET, url: '{{baseUrl}}/pets'}", "{method: 7, url: '{{baseUrl}}/pets'}", 1)
		r := validate(t, FormatOpenCollection10, []byte(src), oas)
		if r.Valid() {
			t.Fatalf("want invalid: %+v", r.Findings)
		}
	})
	t.Run("version", func(t *testing.T) {
		t.Parallel()
		wantFindings(t, validate(t, FormatOpenCollection10, []byte(strings.Replace(good, "opencollection: 1.0.0", "opencollection: 2.0.0", 1)), nil),
			"error collection-schema /opencollection")
	})
}

func TestTestSuite(t *testing.T) {
	t.Parallel()
	pet := read(t, "pet.schema.json")
	good := string(read(t, "pet.tests.json"))
	t.Run("mismatched tests are warnings", func(t *testing.T) {
		t.Parallel()
		src := strings.Replace(good, `"data": {"age": 2}, "valid": false`, `"data": {"age": 2}, "valid": true`, 1)
		r := validate(t, FormatJSONSchemaTestSuite, []byte(src), pet)
		wantFindings(t, r, "warn json-schema-test-mismatch /0/tests/1")
		if !strings.Contains(r.Findings[0].Message, `test "no name" expects valid: true, but the data is invalid`) {
			t.Fatalf("message %q", r.Findings[0].Message)
		}
	})
	t.Run("ref to the interface needs the interface", func(t *testing.T) {
		t.Parallel()
		r := validate(t, FormatJSONSchemaTestSuite, []byte(good), nil)
		wantFindings(t, r, "error json-schema-test-invalid-schema /0/schema")
	})
	t.Run("layout", func(t *testing.T) {
		t.Parallel()
		wantFindings(t, validate(t, FormatJSONSchemaTestSuite, []byte(`[{"description":"x","schema":{},"tests":[{"data":1}]}]`), nil),
			"error collection-schema /0/tests/0")
		wantFindings(t, validate(t, FormatJSONSchemaTestSuite, []byte(`[]`), nil), "error collection-schema ")
		wantFindings(t, validate(t, FormatJSONSchemaTestSuite, []byte(`{}`), nil), "error collection-schema ")
	})
	t.Run("invalid group schema", func(t *testing.T) {
		t.Parallel()
		src := `[{"description":"x","schema":{"type":5},"tests":[{"description":"t","data":1,"valid":true}]}]`
		wantFindings(t, validate(t, FormatJSONSchemaTestSuite, []byte(src), nil), "error json-schema-test-invalid-schema /0/schema")
	})
	t.Run("never loads remote references", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			_, _ = w.Write([]byte(`{"type":"integer"}`))
		}))
		defer srv.Close()
		src := `[{"description":"x","schema":{"$ref":"` + srv.URL + `/s.json"},"tests":[{"description":"t","data":1,"valid":true}]}]`
		r := validate(t, FormatJSONSchemaTestSuite, []byte(src), nil)
		if hits.Load() != 0 {
			t.Fatalf("fetched the reference %d times", hits.Load())
		}
		if r.Valid() {
			t.Fatalf("an unresolvable reference must be an error: %+v", r.Findings)
		}
	})
}

func TestRequestSegments(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"{{baseUrl}}/pets/:petId":          "pets/*",
		"{{baseUrl}}/pets/{{petId}}?x=1#h": "pets/*",
		"https://api.example.com/v1/pets":  "v1/pets",
		"api.example.com/pets/{id}":        "pets/*",
		"/pets":                            "pets",
		"https://api.example.com":          "",
		"http://{{host}}/v1/pets/{{id}}":   "v1/pets/*",
	}
	for in, want := range cases {
		if got := strings.Join(requestSegments(in), "/"); got != want {
			t.Errorf("requestSegments(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResultHelpers(t *testing.T) {
	t.Parallel()
	r := Result{Findings: []contract.Finding{{Severity: contract.SeverityError}, {Severity: contract.SeverityWarn}, {Severity: contract.SeverityHint}}}
	if r.Valid() || len(r.Errors()) != 1 || len(r.Warnings()) != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()
	cases := map[string]Format{
		"pets.arazzo.yaml":         FormatArazzo10,
		"pets.postman.json":        FormatPostman21,
		"pets.opencollection.yaml": FormatOpenCollection10,
		"pet.tests.json":           FormatJSONSchemaTestSuite,
	}
	for file, want := range cases {
		if got, ok := Detect(read(t, file)); !ok || got != want {
			t.Errorf("%s: got %q %v", file, got, ok)
		}
	}
	for _, src := range []string{"", "{", "openapi: 3.1.0", `{"info":{"schema":"x"}}`, "42"} {
		if f, ok := Detect([]byte(src)); ok {
			t.Errorf("%q: detected %q", src, f)
		}
	}
}
