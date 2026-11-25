// Package policy holds ifc7 governance policy: organization and interface
// layers, waivers, inline-ignore handling and gates. It merges the layers into
// one effective OpenLint ruleset, applies waivers to lint findings and
// evaluates gates, so the hub and the CLI reach the same pass or fail for the
// same revision.
//
// Like the rest of pkg/governance it does no network or database I/O.
package policy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/governance/ruleset"
)

// ErrInvalid is returned (wrapped) for an invalid policy, gate or waiver.
var ErrInvalid = errors.New("invalid governance policy")

// InlineIgnores decides what an inline x-lint-ignore in a document does.
type InlineIgnores string

const (
	// InlineIgnoresAllow drops ignored results: they are shown as ignored
	// and count toward neither the score nor any gate. This is the default
	// and matches how phase 1 scored.
	InlineIgnoresAllow InlineIgnores = "allow"
	// InlineIgnoresReport shows ignored results as ignored but still counts
	// them toward the score and the gates.
	InlineIgnoresReport InlineIgnores = "report"
	// InlineIgnoresDeny disregards x-lint-ignore: ignored results are
	// ordinary findings.
	InlineIgnoresDeny InlineIgnores = "deny"
)

// Valid reports whether i is a known setting. Empty means allow.
func (i InlineIgnores) Valid() bool {
	switch i {
	case "", InlineIgnoresAllow, InlineIgnoresReport, InlineIgnoresDeny:
		return true
	}
	return false
}

func (i InlineIgnores) orDefault() InlineIgnores {
	if i == "" {
		return InlineIgnoresAllow
	}
	return i
}

// WaiverSettings are an organization's rules for waivers and inline ignores.
type WaiverSettings struct {
	// MaxExpiryDays caps how far ahead a waiver may expire. Zero means no
	// cap and allows waivers without an expiry.
	MaxExpiryDays int `json:"maxExpiryDays,omitempty"`
	// InlineIgnores decides what x-lint-ignore does. Empty means allow.
	InlineIgnores InlineIgnores `json:"inlineIgnores,omitempty"`
}

// Validate reports an error for an unknown inline-ignore setting or a
// negative expiry cap.
func (w WaiverSettings) Validate() error {
	if w.MaxExpiryDays < 0 {
		return fmt.Errorf("%w: maxExpiryDays must be 0 or more", ErrInvalid)
	}
	if !w.InlineIgnores.Valid() {
		return fmt.Errorf("%w: inlineIgnores must be allow, report or deny, not %q", ErrInvalid, w.InlineIgnores)
	}
	return nil
}

// OrgPolicy is an organization's governance policy.
type OrgPolicy struct {
	// Ruleset is the organization's OpenLint document. Empty means the
	// built-in default.
	Ruleset []byte
	// Weights are the score weights. Missing severities use the defaults.
	Weights findings.Weights
	// Gates are evaluated on push, release or both.
	Gates []Gate
	// Waivers holds the waiver and inline-ignore settings.
	Waivers WaiverSettings
}

// Validate checks the ruleset (through ruleset.Effective with r), the
// weights, the gates and the waiver settings.
func (p *OrgPolicy) Validate(ctx context.Context, r ruleset.Resolver) error {
	if _, _, _, err := ruleset.Effective(ctx, p.Ruleset, r); err != nil {
		return err
	}
	if err := p.Weights.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	if err := ValidateGates(p.Gates); err != nil {
		return err
	}
	return p.Waivers.Validate()
}

// InterfacePolicy is one interface's layer over its organization's policy.
type InterfacePolicy struct {
	// Ruleset is an OpenLint fragment layered over the organization's
	// effective ruleset. Empty means no overrides.
	Ruleset []byte
}

// ParseInterfaceRuleset parses an interface ruleset fragment. Empty input
// returns nil (no overrides).
func ParseInterfaceRuleset(data []byte) (*ruleset.Document, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	return ruleset.Parse(data)
}

// Effective is a resolved ruleset: canonical bytes and their sha256.
type Effective struct {
	Document *ruleset.Document
	Bytes    []byte
	SHA256   string
}

// Merge builds the effective ruleset for an interface: the organization's
// ruleset (empty means the built-in default), with the interface's fragment
// on top using plain extends semantics, so the interface may override any
// organization rule. A nil or empty interface layer gives exactly the
// organization's effective ruleset (same bytes and sha256).
func Merge(ctx context.Context, org []byte, iface []byte, r ruleset.Resolver) (*Effective, error) {
	base := ruleset.Default()
	if len(strings.TrimSpace(string(org))) > 0 {
		var err error
		if base, err = ruleset.Parse(org); err != nil {
			return nil, fmt.Errorf("organization ruleset: %w", err)
		}
	}
	top, err := ParseInterfaceRuleset(iface)
	if err != nil {
		return nil, fmt.Errorf("interface ruleset: %w", err)
	}
	doc, err := ruleset.Overlay(ctx, base, top, r)
	if err != nil {
		return nil, err
	}
	b, sum, err := ruleset.Canonical(doc)
	if err != nil {
		return nil, err
	}
	return &Effective{Document: doc, Bytes: b, SHA256: sum}, nil
}

func validStage(s Stage) bool { return slices.Contains(Stages, s) }
