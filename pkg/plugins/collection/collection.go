// Package collection validates collection source documents: Arazzo 1.0
// workflows, Postman 2.1 collections, JSON Schema Test Suite files and
// OpenCollection 1.0 collections.
//
// Validate never touches the network or the filesystem. Structural problems
// are error findings; advice and cross-checks against the bound interface
// specification are warn findings or below. A source is valid when it has no
// error findings.
package collection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/kaptinlin/jsonschema"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/spec"
)

// Format is a collection source format. The values match ifc-server's
// collection source formats.
type Format string

const (
	FormatArazzo10            Format = "ARAZZO_1_0"
	FormatPostman21           Format = "POSTMAN_2_1"
	FormatJSONSchemaTestSuite Format = "JSON_SCHEMA_TEST_SUITE"
	FormatOpenCollection10    Format = "OPEN_COLLECTION_1_0"
)

// Formats lists every supported format.
func Formats() []Format {
	return []Format{FormatArazzo10, FormatPostman21, FormatJSONSchemaTestSuite, FormatOpenCollection10}
}

// MaxSourceBytes is the largest source Validate accepts.
const MaxSourceBytes = 10 << 20

// maxSchemaFindings caps the schema findings reported for one document. The
// rest are summed up in one extra finding.
const maxSchemaFindings = 50

// Rule ids of findings produced by this package. Arazzo findings also carry
// Vacuum's arazzo-* rule ids.
const (
	RuleParse             = "collection-parse"
	RuleSchema            = "collection-schema"
	RuleTooManyFindings   = "collection-too-many-findings"
	RuleUnknownOperation  = "collection-unknown-operation"
	RuleDuplicateID       = "postman-duplicate-id"
	RuleTestInvalidSchema = "json-schema-test-invalid-schema"
	RuleTestMismatch      = "json-schema-test-mismatch"
)

// ErrUnsupportedFormat is returned (wrapped) for a format this package has
// no validator for.
var ErrUnsupportedFormat = errors.New("unsupported collection format")

// Input is one collection source to validate.
type Input struct {
	Format Format
	// Source is the collection document, JSON or YAML.
	Source []byte
	// Interface is the bound interface specification (OpenAPI for Arazzo,
	// Postman and OpenCollection; JSON Schema for test suites). Optional:
	// without it, cross-checks are skipped.
	Interface []byte
}

// Result is the outcome of validating one source.
type Result struct {
	// Findings are sorted most severe first.
	Findings []contract.Finding `json:"findings"`
	// Validator names what checked the source and its pinned version, for
	// example "opencollection-schema 1.0.0 (@opencollection/schema 0.15.0)".
	Validator string `json:"validator"`
}

// Valid reports whether the source has no error findings.
func (r Result) Valid() bool { return len(r.Errors()) == 0 }

// Errors returns the error findings.
func (r Result) Errors() []contract.Finding {
	var out []contract.Finding
	for _, f := range r.Findings {
		if f.Severity == contract.SeverityError {
			out = append(out, f)
		}
	}
	return out
}

// Warnings returns every finding that is not an error.
func (r Result) Warnings() []contract.Finding {
	var out []contract.Finding
	for _, f := range r.Findings {
		if f.Severity != contract.SeverityError {
			out = append(out, f)
		}
	}
	return out
}

// Validate checks a collection source. The error is only for an unsupported
// format or a done context; a malformed source is reported in the findings.
func Validate(ctx context.Context, in Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	var (
		res Result
		err error
	)
	if len(in.Source) > MaxSourceBytes {
		res = Result{Findings: []contract.Finding{finding(RuleParse, contract.SeverityError, "", fmt.Sprintf("source is %d bytes, more than the %d allowed", len(in.Source), MaxSourceBytes))}}
		res.Validator = validatorName(in.Format)
		return res, nil
	}
	switch in.Format {
	case FormatArazzo10:
		res, err = validateArazzo(ctx, in)
	case FormatPostman21:
		res, err = validatePostman(in)
	case FormatJSONSchemaTestSuite:
		res, err = validateTestSuite(in)
	case FormatOpenCollection10:
		res, err = validateOpenCollection(in)
	default:
		return Result{}, fmt.Errorf("%w %q", ErrUnsupportedFormat, in.Format)
	}
	if err != nil {
		return Result{}, err
	}
	res.Validator = validatorName(in.Format)
	findings.Sort(res.Findings)
	if res.Findings == nil {
		res.Findings = []contract.Finding{}
	}
	return res, nil
}

func validatorName(f Format) string {
	switch f {
	case FormatArazzo10:
		return "arazzo: " + vacuumEngine()
	case FormatPostman21:
		return "postman-2.1-structure v1"
	case FormatJSONSchemaTestSuite:
		return "json-schema-test-suite v1"
	case FormatOpenCollection10:
		return "opencollection-schema " + openCollectionSchemaVersion + " (@opencollection/schema " + openCollectionPackageVersion + ")"
	}
	return ""
}

func finding(rule string, sev contract.Severity, pointer, msg string) contract.Finding {
	f := contract.Finding{RuleID: rule, Severity: sev, Message: msg, Pointer: pointer}
	f.Fingerprint = findings.Fingerprint(f.RuleID, f.Pointer, f.Message)
	return f
}

// parseJSON converts a JSON or YAML source to a decoded JSON value and its
// JSON bytes. It returns a parse finding when the source can't be read.
func parseJSON(raw []byte) (any, []byte, *contract.Finding) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		f := finding(RuleParse, contract.SeverityError, "", "source is empty")
		return nil, nil, &f
	}
	jsonBytes, err := spec.ToJSON(raw)
	if err != nil {
		f := finding(RuleParse, contract.SeverityError, "", "source is not valid JSON or YAML: "+err.Error())
		return nil, nil, &f
	}
	var v any
	if err := json.Unmarshal(jsonBytes, &v); err != nil {
		f := finding(RuleParse, contract.SeverityError, "", "source is not valid JSON or YAML: "+err.Error())
		return nil, nil, &f
	}
	return v, jsonBytes, nil
}

// offlineCompiler returns a JSON Schema compiler that can't load anything
// from the network or the filesystem.
func offlineCompiler() *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	refuse := func(url string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("loading %s is not allowed: only references inside the document resolve", url)
	}
	for _, scheme := range []string{"http", "https", "file"} {
		c.RegisterLoader(scheme, refuse)
	}
	return c
}

// aggregateKeywords only summarise their subschemas' errors, which are
// reported where they happen instead.
var aggregateKeywords = map[string]bool{
	"properties": true, "items": true, "prefixItems": true, "$ref": true, "$dynamicRef": true,
	"allOf": true, "anyOf": true, "oneOf": true, "patternProperties": true, "dependentSchemas": true,
	"then": true, "else": true, "contains": true, "additionalItems": true,
}

type schemaIssue struct{ pointer, msg string }

// schemaFindings validates instance against schema and turns each failing
// keyword into an error finding at its instance location. Of an anyOf or
// oneOf, only the branch with the fewest problems is reported.
func schemaFindings(schema *jsonschema.Schema, instance any) []contract.Finding {
	res := schema.Validate(instance)
	if res.IsValid() {
		return nil
	}
	issues := collectIssues(*res.ToList(true), "", instance)
	seen := map[string]bool{}
	var out []contract.Finding
	n := 0
	for _, is := range issues {
		key := is.pointer + "\x00" + is.msg
		if seen[key] {
			continue
		}
		seen[key] = true
		if n == maxSchemaFindings {
			n++
			continue
		}
		if n > maxSchemaFindings {
			n++
			continue
		}
		n++
		out = append(out, finding(RuleSchema, contract.SeverityError, is.pointer, is.msg))
	}
	if n > maxSchemaFindings {
		out = append(out, finding(RuleTooManyFindings, contract.SeverityError, "", fmt.Sprintf("%d more schema problems are not listed", n-maxSchemaFindings)))
	}
	if len(out) == 0 {
		out = append(out, finding(RuleSchema, contract.SeverityError, "", "document does not match the format's schema"))
	}
	return out
}

func collectIssues(l jsonschema.List, base string, instance any) []schemaIssue {
	loc := base + l.InstanceLocation
	var out []schemaIssue
	for _, k := range sortedKeys(l.Errors) {
		if aggregateKeywords[k] {
			continue
		}
		// kaptinlin evaluates properties that are absent as null; the
		// required keyword already reports those.
		if k == "type" && !pointerExists(instance, loc) {
			continue
		}
		out = append(out, schemaIssue{loc, l.Errors[k]})
	}
	var best []schemaIssue
	var tied []string // messages of branches that tie with best on one issue at loc
	branched := false
	for _, d := range l.Details {
		if d.Valid {
			continue
		}
		child := collectIssues(d, loc, instance)
		if strings.HasPrefix(d.EvaluationPath, "/anyOf/") || strings.HasPrefix(d.EvaluationPath, "/oneOf/") {
			switch {
			case len(child) == 0:
			case !branched || len(child) < len(best) || len(child) == len(best) && depth(child) > depth(best):
				best = child
				tied = nil
				if len(child) == 1 && child[0].pointer == loc {
					tied = []string{child[0].msg}
				}
				branched = true
			case len(child) == 1 && len(best) == 1 && child[0].pointer == loc && best[0].pointer == loc:
				tied = append(tied, child[0].msg)
			}
			continue
		}
		out = append(out, child...)
	}
	if len(tied) > 1 {
		// No branch got further than another: say what each would need.
		best = []schemaIssue{{loc, "matches none of the allowed shapes: " + strings.Join(tied, ", or ")}}
	}
	out = append(out, best...)
	if len(out) == 0 && pointerExists(instance, loc) {
		for _, k := range []string{"oneOf", "anyOf"} {
			if msg, ok := l.Errors[k]; ok {
				out = append(out, schemaIssue{loc, msg})
			}
		}
	}
	return out
}

// depth is the deepest pointer among issues: a branch that fails deeper in
// the document matched more of it.
func depth(issues []schemaIssue) int {
	d := 0
	for _, is := range issues {
		d = max(d, strings.Count(is.pointer, "/"))
	}
	return d
}

// pointerExists reports whether a JSON Pointer resolves in a decoded JSON
// value.
func pointerExists(v any, ptr string) bool {
	if ptr == "" {
		return true
	}
	cur := v
	for _, tok := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		switch c := cur.(type) {
		case map[string]any:
			next, ok := c[tok]
			if !ok {
				return false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(c) {
				return false
			}
			cur = c[i]
		default:
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// pointer appends JSON Pointer tokens to base.
func pointer(base string, tokens ...any) string {
	var b strings.Builder
	b.WriteString(base)
	for _, t := range tokens {
		b.WriteByte('/')
		switch v := t.(type) {
		case int:
			b.WriteString(strconv.Itoa(v))
		case string:
			b.WriteString(strings.ReplaceAll(strings.ReplaceAll(v, "~", "~0"), "/", "~1"))
		default:
			fmt.Fprint(&b, v)
		}
	}
	return b.String()
}

// Detect guesses a source's format from its top-level shape: an arazzo or
// opencollection version key, a Postman info.schema URL, or a top-level
// array (JSON Schema Test Suite). It reports false when nothing fits.
func Detect(raw []byte) (Format, bool) {
	v, _, bad := parseJSON(raw)
	if bad != nil {
		return "", false
	}
	switch t := v.(type) {
	case []any:
		return FormatJSONSchemaTestSuite, true
	case map[string]any:
		if _, ok := t["arazzo"]; ok {
			return FormatArazzo10, true
		}
		if _, ok := t["opencollection"]; ok {
			return FormatOpenCollection10, true
		}
		if info, ok := t["info"].(map[string]any); ok {
			if s, _ := info["schema"].(string); strings.Contains(s, "postman.com/json/collection/") {
				return FormatPostman21, true
			}
		}
	}
	return "", false
}
