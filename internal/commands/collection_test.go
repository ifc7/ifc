package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPostman = `{"info":{"name":"Pets","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
"item":[{"id":"a","name":"List","request":{"method":"GET","url":"{{baseUrl}}/pets"}}]}`

const testOpenAPI = `openapi: 3.1.0
info: {title: Pets, version: 1.0.0}
paths:
  /cats:
    get: {operationId: listCats, responses: {'200': {description: ok}}}
`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCollectionValidate(t *testing.T) {
	t.Chdir(t.TempDir()) // no ifc.yaml
	good := writeTemp(t, "pets.postman.json", testPostman)
	oas := writeTemp(t, "openapi.yaml", testOpenAPI)

	stdout, _, err := runIfc(t, "collection", "validate", good)
	if err != nil {
		t.Fatalf("validate: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "POSTMAN_2_1") || !strings.Contains(stdout, "0 errors, 0 warnings") {
		t.Fatalf("stdout = %q", stdout)
	}

	// A request the interface doesn't have is a warning: it passes, unless --strict.
	stdout, _, err = runIfc(t, "collection", "validate", good, "--interface", oas, "--json")
	if err != nil {
		t.Fatalf("validate: %v\n%s", err, stdout)
	}
	var rep struct {
		Valid    bool `json:"valid"`
		Findings []struct {
			RuleID string `json:"ruleId"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	if !rep.Valid || len(rep.Findings) != 1 || rep.Findings[0].RuleID != "collection-unknown-operation" {
		t.Fatalf("report = %+v", rep)
	}
	_, _, err = runIfc(t, "collection", "validate", good, "--interface", oas, "--strict")
	wantExit(t, err, exitCollectionInvalid)

	bad := writeTemp(t, "bad.json", `{"info":{"name":"x","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},"item":[{"name":"r"}]}`)
	stdout, _, err = runIfc(t, "collection", "validate", bad)
	wantExit(t, err, exitCollectionInvalid)
	if !strings.Contains(stdout, "collection-schema") {
		t.Fatalf("stdout = %q", stdout)
	}

	unknown := writeTemp(t, "x.yaml", "openapi: 3.1.0\n")
	_, _, err = runIfc(t, "collection", "validate", unknown)
	wantExit(t, err, 1)
	_, _, err = runIfc(t, "collection", "validate", good, "--format", "raml")
	wantExit(t, err, 1)
	// --format overrides detection.
	_, _, err = runIfc(t, "collection", "validate", good, "--format", "arazzo_1_0")
	wantExit(t, err, exitCollectionInvalid)
}
