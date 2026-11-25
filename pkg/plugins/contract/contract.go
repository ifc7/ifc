// Package contract defines the lint and compare plugin input/output types
// matching the published JSON Schema contracts at
// https://ifc7.dev/schemas/plugins/v0/ and https://ifc7.dev/schemas/plugins/v1/.
//
// v1 is a superset of v0: it adds LintInput.Ruleset, LintOutput.Findings,
// LintOutput.IgnoredFindings, LintOutput.Engine and CompareOutput.Changes. A
// v0 consumer that ignores those fields keeps working.
package contract

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// InterfaceType identifies the kind of interface specification.
type InterfaceType string

const (
	InterfaceTypeOpenAPI    InterfaceType = "OPENAPI"
	InterfaceTypeJSONSchema InterfaceType = "JSON_SCHEMA"
)

// FileFormat is the serialization format of decoded specification bytes.
type FileFormat string

const (
	FileFormatJSON FileFormat = "json"
	FileFormatYAML FileFormat = "yaml"
)

// SpecificationDocument is a single interface specification payload.
type SpecificationDocument struct {
	Specification string     `json:"specification"`
	FileFormat    FileFormat `json:"fileFormat"`
}

// Decode returns the raw specification bytes.
func (d SpecificationDocument) Decode() ([]byte, error) {
	if d.Specification == "" {
		return nil, fmt.Errorf("specification is required")
	}
	b, err := base64.StdEncoding.DecodeString(d.Specification)
	if err != nil {
		return nil, fmt.Errorf("decode specification: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("specification is empty")
	}
	return b, nil
}

// NewSpecificationDocument encodes raw bytes as a SpecificationDocument.
func NewSpecificationDocument(raw []byte, format FileFormat) SpecificationDocument {
	return SpecificationDocument{
		Specification: base64.StdEncoding.EncodeToString(raw),
		FileFormat:    format,
	}
}

// DetectFileFormat returns json or yaml based on content.
func DetectFileFormat(raw []byte) FileFormat {
	if json.Valid(raw) {
		return FileFormatJSON
	}
	return FileFormatYAML
}

// Contract versions.
const (
	VersionV0 = "v0"
	VersionV1 = "v1"
)

// LintInput is the input contract for linter plugins.
type LintInput struct {
	InterfaceType InterfaceType         `json:"interfaceType"`
	Document      SpecificationDocument `json:"document"`
	// Ruleset is the effective OpenLint ruleset to lint with (v1). Nil means
	// the plugin's built-in default.
	Ruleset *RulesetDocument `json:"ruleset,omitempty"`
	Options map[string]any   `json:"options,omitempty"`
}

// RulesetDocument carries an effective (fully resolved) OpenLint ruleset.
type RulesetDocument struct {
	// Document is the base64-encoded ruleset, JSON or YAML.
	Document string `json:"document"`
	// SHA256 is the lowercase hex sha256 of the canonical ruleset bytes.
	SHA256 string `json:"sha256"`
}

// NewRulesetDocument encodes ruleset bytes and their canonical sha256.
func NewRulesetDocument(raw []byte, sha256 string) *RulesetDocument {
	return &RulesetDocument{Document: base64.StdEncoding.EncodeToString(raw), SHA256: sha256}
}

// Decode returns the raw ruleset bytes.
func (r RulesetDocument) Decode() ([]byte, error) {
	if r.Document == "" {
		return nil, fmt.Errorf("ruleset document is required")
	}
	b, err := base64.StdEncoding.DecodeString(r.Document)
	if err != nil {
		return nil, fmt.Errorf("decode ruleset: %w", err)
	}
	return b, nil
}

// LintOutput is the output contract for linter plugins.
type LintOutput struct {
	// Score is the ifc7 quality score, 0 (worst) to 100 (best).
	Score int `json:"score"`
	// Raw is human-readable detail. v1 plugins render it from Findings.
	Raw string `json:"raw"`
	// Findings are the structured lint results (v1).
	Findings []Finding `json:"findings,omitempty"`
	// IgnoredFindings are results the document suppressed with an inline
	// x-lint-ignore (v1). They are not in Findings and do not affect Score;
	// a governance policy decides whether they count (see
	// pkg/governance/policy).
	IgnoredFindings []Finding `json:"ignoredFindings,omitempty"`
	// Engine names the lint engine that produced the findings (v1).
	Engine *Engine        `json:"engine,omitempty"`
	Extra  map[string]any `json:"extra,omitempty"`
}

// Severity is a finding severity.
type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
	SeverityInfo  Severity = "info"
	SeverityHint  Severity = "hint"
)

// Severities lists every severity, most severe first.
var Severities = []Severity{SeverityError, SeverityWarn, SeverityInfo, SeverityHint}

// Finding is a single structured lint result.
type Finding struct {
	RuleID   string   `json:"ruleId"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	// Pointer is a JSON Pointer (RFC 6901) to the offending node. Empty
	// string means the document root.
	Pointer string `json:"pointer"`
	// Range is the 1-based source location, when known.
	Range *Range `json:"range,omitempty"`
	// Fingerprint identifies the finding across formatting changes.
	Fingerprint      string `json:"fingerprint"`
	DocumentationURL string `json:"documentationUrl,omitempty"`
}

// Range is a source span. Lines and columns are 1-based.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position is a 1-based line and column.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Engine identifies a lint engine and its version.
type Engine struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// CompareInput is the input contract for change-detector plugins.
type CompareInput struct {
	InterfaceType InterfaceType         `json:"interfaceType"`
	Before        SpecificationDocument `json:"before"`
	After         SpecificationDocument `json:"after"`
	Options       map[string]any        `json:"options,omitempty"`
}

// CompareOutput is the output contract for change-detector plugins.
type CompareOutput struct {
	Breaking bool   `json:"breaking"`
	Raw      string `json:"raw"`
	// Changes are the structured change records (v1). Breaking is true
	// exactly when at least one change is breaking.
	Changes []Change       `json:"changes,omitempty"`
	Extra   map[string]any `json:"extra,omitempty"`
}

// ChangeLevel is how much a change matters to a gate.
type ChangeLevel string

const (
	// ChangeLevelError is a change a breaking-change gate judges: by
	// default every breaking change.
	ChangeLevelError ChangeLevel = "error"
	// ChangeLevelWarn is reported but never fails a gate: by default every
	// non-breaking change.
	ChangeLevelWarn ChangeLevel = "warn"
	// ChangeLevelIgnore drops the change. Plugins never emit it; a
	// governance policy may set it per change kind.
	ChangeLevelIgnore ChangeLevel = "ignore"
)

// Change is one structured difference between two documents.
type Change struct {
	// Kind classifies the change as {object}.{property}.{action}, for
	// example "pathItem.operation.removed" or "schema.type.modified".
	// action is added, removed or modified.
	Kind string `json:"kind"`
	// Pointer is a JSON Pointer to the changed location. It points into the
	// after document, or into the before document for a removal. Array
	// members that have a name (parameters) are addressed by that name.
	Pointer  string `json:"pointer"`
	Breaking bool   `json:"breaking"`
	// Level is error for a breaking change and warn otherwise.
	Level   ChangeLevel `json:"level"`
	Message string      `json:"message"`
	// Stability is set when a change that would be breaking is exempt
	// because its operation is marked x-stability-level (draft or alpha).
	Stability string `json:"stability,omitempty"`
	// Fingerprint identifies the change by kind and pointer.
	Fingerprint string `json:"fingerprint"`
}
