// Package findings fingerprints, scores and summarises structured lint
// findings. The score formula is owned by ifc7 and is the same for every
// lint engine, so the CLI and the hub always agree.
package findings

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// Weights maps a severity to its score weight.
type Weights map[contract.Severity]float64

// DefaultWeights are the phase 1 default score weights.
func DefaultWeights() Weights {
	return Weights{
		contract.SeverityError: 1.0,
		contract.SeverityWarn:  0.5,
		contract.SeverityInfo:  0.2,
		contract.SeverityHint:  0.1,
	}
}

// Validate reports an error for unknown severities or negative weights.
func (w Weights) Validate() error {
	for sev, v := range w {
		if !IsSeverity(sev) {
			return fmt.Errorf("unknown severity %q in score weights", sev)
		}
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("score weight for %q must be a finite number >= 0", sev)
		}
	}
	return nil
}

// withDefaults returns w with any missing severity filled from DefaultWeights.
func (w Weights) withDefaults() Weights {
	out := DefaultWeights()
	for k, v := range w {
		out[k] = v
	}
	return out
}

// ScoreDecay is the constant k in score = round(100 * e^(-k * penalty)).
const ScoreDecay = 0.1

// Score returns round(100 × e^(−0.1 × Σ weight × count)), clamped to 0..100.
// Nil or partial weights fall back to DefaultWeights for missing severities.
func Score(fs []contract.Finding, w Weights) int {
	w = w.withDefaults()
	var penalty float64
	for _, f := range fs {
		penalty += w[f.Severity]
	}
	score := int(math.Round(100 * math.Exp(-ScoreDecay*penalty)))
	return max(0, min(100, score))
}

// IsSeverity reports whether s is a known finding severity.
func IsSeverity(s contract.Severity) bool {
	switch s {
	case contract.SeverityError, contract.SeverityWarn, contract.SeverityInfo, contract.SeverityHint:
		return true
	}
	return false
}

// Summary counts findings by severity and by rule id.
type Summary struct {
	Total      int                       `json:"total"`
	BySeverity map[contract.Severity]int `json:"bySeverity"`
	ByRule     map[string]int            `json:"byRule"`
}

// Summarise counts findings by severity and by rule id. Every known severity
// is present in BySeverity, with zero when there are none.
func Summarise(fs []contract.Finding) Summary {
	s := Summary{
		Total:      len(fs),
		BySeverity: make(map[contract.Severity]int, len(contract.Severities)),
		ByRule:     map[string]int{},
	}
	for _, sev := range contract.Severities {
		s.BySeverity[sev] = 0
	}
	for _, f := range fs {
		s.BySeverity[f.Severity]++
		s.ByRule[f.RuleID]++
	}
	return s
}

var (
	quotedRe = regexp.MustCompile("'[^']*'|\"[^\"]*\"|`[^`]*`")
	numberRe = regexp.MustCompile(`\d+(\.\d+)?`)
	spaceRe  = regexp.MustCompile(`\s+`)
)

// NormaliseMessage replaces quoted values with "*" and numbers with "#" and
// collapses whitespace, so a message that only differs in a value or count
// keeps its fingerprint.
func NormaliseMessage(msg string) string {
	msg = quotedRe.ReplaceAllString(msg, `"*"`)
	msg = numberRe.ReplaceAllString(msg, "#")
	msg = spaceRe.ReplaceAllString(msg, " ")
	return strings.TrimSpace(msg)
}

// Fingerprint is the sha256 of the rule id, JSON Pointer and normalised
// message. Line numbers are excluded so reformatting a document or reordering
// its keys does not change it.
func Fingerprint(ruleID, pointer, message string) string {
	h := sha256.New()
	// NUL separators keep ("a","bc") and ("ab","c") distinct.
	h.Write([]byte(ruleID))
	h.Write([]byte{0})
	h.Write([]byte(pointer))
	h.Write([]byte{0})
	h.Write([]byte(NormaliseMessage(message)))
	return hex.EncodeToString(h.Sum(nil))
}

// Sort orders findings by severity (most severe first), then source line,
// rule id, pointer and fingerprint, so output is deterministic.
func Sort(fs []contract.Finding) {
	rank := map[contract.Severity]int{}
	for i, s := range contract.Severities {
		rank[s] = i
	}
	line := func(f contract.Finding) int {
		if f.Range == nil {
			return 0
		}
		return f.Range.Start.Line
	}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		if line(a) != line(b) {
			return line(a) < line(b)
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.Pointer != b.Pointer {
			return a.Pointer < b.Pointer
		}
		return a.Fingerprint < b.Fingerprint
	})
}

// RenderText renders findings as the plain-text raw output used by
// LintOutput.Raw: one "severity[rule]: message" line per finding, followed by
// an indented pointer and line:column when known.
func RenderText(fs []contract.Finding) string {
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "%s[%s]: %s\n", f.Severity, f.RuleID, f.Message)
		loc := f.Pointer
		if loc == "" {
			loc = "/"
		}
		if f.Range != nil && f.Range.Start.Line > 0 {
			loc = fmt.Sprintf("%s %d:%d", loc, f.Range.Start.Line, f.Range.Start.Column)
		}
		fmt.Fprintf(&b, "  %s\n", loc)
	}
	return b.String()
}
