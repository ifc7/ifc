package policy

import (
	"fmt"
	"strings"
	"time"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// Waiver accepts a finding on one interface. It matches a finding with the
// same rule id and either the same fingerprint (when set) or the same JSON
// Pointer, until it expires or is revoked.
type Waiver struct {
	ID          string `json:"id"`
	RuleID      string `json:"ruleId"`
	Pointer     string `json:"pointer"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Reason      string `json:"reason"`
	// ExpiresAt is when the waiver stops applying. Nil means never, which
	// only an organization without maxExpiryDays allows.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

// ActiveAt reports whether w applies at now: not revoked and not expired.
func (w Waiver) ActiveAt(now time.Time) bool {
	if w.RevokedAt != nil && !now.Before(*w.RevokedAt) {
		return false
	}
	return w.ExpiresAt == nil || now.Before(*w.ExpiresAt)
}

// Matches reports whether w covers f (ignoring time).
func (w Waiver) Matches(f contract.Finding) bool {
	if w.RuleID != f.RuleID {
		return false
	}
	if w.Fingerprint != "" {
		return w.Fingerprint == f.Fingerprint
	}
	return w.Pointer == f.Pointer
}

// MaxReasonLength bounds a waiver reason.
const MaxReasonLength = 2000

// ValidateWaiver checks a new waiver against the organization's settings at
// now: a rule id and reason are required, the pointer must be a JSON Pointer,
// a fingerprint must be a sha256, and the expiry must be in the future and
// within maxExpiryDays when the organization sets one.
func ValidateWaiver(w Waiver, settings WaiverSettings, now time.Time) error {
	if strings.TrimSpace(w.RuleID) == "" {
		return fmt.Errorf("%w: waiver ruleId is required", ErrInvalid)
	}
	if strings.TrimSpace(w.Reason) == "" {
		return fmt.Errorf("%w: waiver reason is required", ErrInvalid)
	}
	if len(w.Reason) > MaxReasonLength {
		return fmt.Errorf("%w: waiver reason is longer than %d characters", ErrInvalid, MaxReasonLength)
	}
	if w.Pointer != "" && !strings.HasPrefix(w.Pointer, "/") {
		return fmt.Errorf("%w: waiver pointer must be a JSON Pointer starting with /", ErrInvalid)
	}
	if w.Fingerprint != "" && !isSHA256(w.Fingerprint) {
		return fmt.Errorf("%w: waiver fingerprint must be a lowercase hex sha256", ErrInvalid)
	}
	if w.ExpiresAt != nil && !w.ExpiresAt.After(now) {
		return fmt.Errorf("%w: waiver expiresAt must be in the future", ErrInvalid)
	}
	if settings.MaxExpiryDays > 0 {
		if w.ExpiresAt == nil {
			return fmt.Errorf("%w: waiver expiresAt is required (at most %d days ahead)", ErrInvalid, settings.MaxExpiryDays)
		}
		if w.ExpiresAt.After(now.AddDate(0, 0, settings.MaxExpiryDays)) {
			return fmt.Errorf("%w: waiver expiresAt is more than %d days ahead", ErrInvalid, settings.MaxExpiryDays)
		}
	}
	return nil
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Status is what governance made of one finding.
type Status string

const (
	// StatusOpen counts toward the score and the gates.
	StatusOpen Status = "open"
	// StatusWaived is covered by an active waiver. It counts toward the
	// score but not toward failOn.
	StatusWaived Status = "waived"
	// StatusIgnored was suppressed inline with x-lint-ignore and the policy
	// allows that. It counts toward neither the score nor the gates.
	StatusIgnored Status = "ignored"
	// StatusIgnoredCounted was suppressed inline, but the policy reports
	// inline ignores without honouring them, so it counts like an open
	// finding.
	StatusIgnoredCounted Status = "ignored-counted"
)

// Evaluated is a finding with its governance status.
type Evaluated struct {
	contract.Finding
	Status Status `json:"status"`
	// WaiverID is the waiver that covers the finding, when Status is waived.
	WaiverID string `json:"waiverId,omitempty"`
}

// countsForScore reports whether e lowers the score.
func (e Evaluated) countsForScore() bool { return e.Status != StatusIgnored }

// countsForGate reports whether e counts toward a gate's failOn.
func (e Evaluated) countsForGate() bool {
	return e.Status == StatusOpen || e.Status == StatusIgnoredCounted
}

// ApplyWaivers combines a lint run's findings and inline-ignored findings
// under the inline-ignore setting, then marks each counted finding covered by
// a waiver active at now as waived. Every finding stays in the output, in the
// order given (findings first, then ignored ones), and the first matching
// waiver wins.
func ApplyWaivers(fs, ignored []contract.Finding, waivers []Waiver, inline InlineIgnores, now time.Time) []Evaluated {
	active := make([]Waiver, 0, len(waivers))
	for _, w := range waivers {
		if w.ActiveAt(now) {
			active = append(active, w)
		}
	}
	out := make([]Evaluated, 0, len(fs)+len(ignored))
	waive := func(e Evaluated) Evaluated {
		for _, w := range active {
			if w.Matches(e.Finding) {
				e.Status = StatusWaived
				e.WaiverID = w.ID
				break
			}
		}
		return e
	}
	for _, f := range fs {
		out = append(out, waive(Evaluated{Finding: f, Status: StatusOpen}))
	}
	for _, f := range ignored {
		switch inline.orDefault() {
		case InlineIgnoresDeny:
			out = append(out, waive(Evaluated{Finding: f, Status: StatusOpen}))
		case InlineIgnoresReport:
			out = append(out, waive(Evaluated{Finding: f, Status: StatusIgnoredCounted}))
		default:
			out = append(out, Evaluated{Finding: f, Status: StatusIgnored})
		}
	}
	return out
}

// ScoreOf scores evaluated findings with w. Waived findings still count, so
// the score describes the document; inline ignores the policy allows do not.
func ScoreOf(es []Evaluated, w findings.Weights) int {
	return findings.Score(scored(es), w)
}

func scored(es []Evaluated) []contract.Finding {
	out := make([]contract.Finding, 0, len(es))
	for _, e := range es {
		if e.countsForScore() {
			out = append(out, e.Finding)
		}
	}
	return out
}
