package policy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ifc7/ifc/pkg/governance/ruleset"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// ErrRefused is returned (wrapped, in a *GuardrailError) when a layer above
// the organization changes a rule the organization's guardrails protect.
var ErrRefused = errors.New("override refused by organization guardrails")

// Guardrails are an organization's limits on the layers above it (groups and
// interfaces). Without guardrails a layer may override any organization rule.
type Guardrails struct {
	// Locked rules cannot be changed by any layer above the organization: not
	// their severity, not their definition, and not their scope.
	Locked []string `json:"locked,omitempty"`
	// MinSeverity sets a floor per rule: a layer above the organization may
	// raise the rule, or lower it down to the floor, but not below it or off.
	// Values are error, warn, info or hint.
	MinSeverity map[string]contract.Severity `json:"minSeverity,omitempty"`
}

// Empty reports whether g protects nothing.
func (g Guardrails) Empty() bool { return len(g.Locked) == 0 && len(g.MinSeverity) == 0 }

// Validate reports an error for an empty rule id, an unknown floor severity,
// or a rule that is both locked and floored.
func (g Guardrails) Validate() error {
	for _, id := range g.Locked {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("%w: guardrails.locked has an empty rule id", ErrInvalid)
		}
	}
	for id, sev := range g.MinSeverity {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("%w: guardrails.minSeverity has an empty rule id", ErrInvalid)
		}
		if _, ok := floorRank[string(sev)]; !ok || sev == ruleset.ModeOff {
			return fmt.Errorf("%w: guardrails.minSeverity[%s] must be error, warn, info or hint, not %q", ErrInvalid, id, sev)
		}
		if slices.Contains(g.Locked, id) {
			return fmt.Errorf("%w: rule %q is both locked and has a severity floor", ErrInvalid, id)
		}
	}
	return nil
}

// Refusal reasons.
const (
	RefusedLocked  = "locked"
	RefusedFloor   = "below_floor"
	RefusedExtends = "extends_lowered"
	RefusedScope   = "scoped"
)

// Refusal is one override a layer may not make.
type Refusal struct {
	// Layer names the layer that made the override (for example "interface"
	// or a group's name).
	Layer string `json:"layer"`
	// RuleID is the protected rule; empty for a built-in ruleset change.
	RuleID string `json:"ruleId,omitempty"`
	// Extends names the built-in ruleset whose mode was lowered.
	Extends string `json:"extends,omitempty"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// GuardrailError lists every refused override. errors.Is(err, ErrRefused)
// and errors.Is(err, ErrInvalid) are both true.
type GuardrailError struct {
	Refusals []Refusal
}

func (e *GuardrailError) Error() string {
	msgs := make([]string, 0, len(e.Refusals))
	for _, r := range e.Refusals {
		msgs = append(msgs, r.Message)
	}
	return fmt.Sprintf("%s: %s", ErrRefused, strings.Join(msgs, "; "))
}

// Is matches ErrRefused and ErrInvalid.
func (e *GuardrailError) Is(target error) bool { return target == ErrRefused || target == ErrInvalid }

// floorRank orders severities, most severe first, with off last.
var floorRank = map[string]int{"error": 0, "warn": 1, "info": 2, "hint": 3, ruleset.ModeOff: 4}

// Check compares a layer's result (after) with the ruleset beneath it
// (before), both resolved, and returns every override the guardrails refuse.
// Lowering a built-in ruleset's mode (or removing it) is refused whenever g
// protects anything, because it can switch off a protected built-in rule.
func (g Guardrails) Check(layer string, before, after *ruleset.Document) []Refusal {
	if g.Empty() {
		return nil
	}
	var out []Refusal
	diff := ruleset.DiffRulesets(before, after)
	for _, c := range diff.Extends {
		if c.Change == ruleset.DiffAdded || c.Tightening {
			continue
		}
		mode := c.NewMode
		if c.Change == ruleset.DiffRemoved {
			mode = "removed"
		}
		out = append(out, Refusal{Layer: layer, Extends: c.Name, Reason: RefusedExtends,
			Message: fmt.Sprintf("%s may not lower built-in ruleset %s (%s to %s) while the organization has guardrails", layer, c.Name, c.OldMode, mode)})
	}
	for _, c := range diff.Rules {
		if slices.Contains(g.Locked, c.RuleID) {
			out = append(out, Refusal{Layer: layer, RuleID: c.RuleID, Reason: RefusedLocked,
				Message: fmt.Sprintf("%s may not change locked rule %s", layer, c.RuleID)})
			continue
		}
		floor, ok := g.MinSeverity[c.RuleID]
		if !ok {
			continue
		}
		sev := c.NewSeverity
		if c.Change == ruleset.DiffRemoved {
			sev = ruleset.ModeOff
		}
		rank, known := floorRank[sev]
		if known && rank > floorRank[string(floor)] {
			out = append(out, Refusal{Layer: layer, RuleID: c.RuleID, Reason: RefusedFloor,
				Message: fmt.Sprintf("%s may not set %s to %s, below the organization's floor of %s", layer, c.RuleID, sev, floor)})
		}
	}
	wasScoped := before.ChangeScopedRules()
	for _, id := range after.ChangeScopedRules() {
		if slices.Contains(wasScoped, id) {
			continue
		}
		if _, floored := g.MinSeverity[id]; floored || slices.Contains(g.Locked, id) {
			out = append(out, Refusal{Layer: layer, RuleID: id, Reason: RefusedScope,
				Message: fmt.Sprintf("%s may not limit protected rule %s to changed parts of a document", layer, id)})
		}
	}
	return out
}

// Layer is one policy layer above the organization, such as a group or an
// interface. Layers apply in order, each on top of the one before.
type Layer struct {
	// Name identifies the layer in refusals, for example "group platform"
	// or "interface".
	Name string
	// Ruleset is an OpenLint fragment. Empty means the layer adds nothing.
	Ruleset []byte
}

// MergeLayers builds an interface's effective ruleset from the
// organization's ruleset (empty means the built-in default) and the layers
// above it, in order, with plain extends semantics. Each layer is checked
// against the guardrails as it is applied; if any override is refused it
// returns a *GuardrailError listing all of them. With no non-empty layers the
// result is exactly the organization's effective ruleset, and with one layer
// and no guardrails it is exactly Merge.
func MergeLayers(ctx context.Context, org []byte, layers []Layer, g Guardrails, r ruleset.Resolver) (*Effective, error) {
	base := ruleset.Default()
	if len(strings.TrimSpace(string(org))) > 0 {
		var err error
		if base, err = ruleset.Parse(org); err != nil {
			return nil, fmt.Errorf("organization ruleset: %w", err)
		}
	}
	cur, err := ruleset.Overlay(ctx, base, nil, r)
	if err != nil {
		return nil, err
	}
	var refused []Refusal
	for _, l := range layers {
		top, err := ParseInterfaceRuleset(l.Ruleset)
		if err != nil {
			return nil, fmt.Errorf("%s ruleset: %w", l.Name, err)
		}
		if top == nil {
			continue
		}
		next, err := ruleset.Overlay(ctx, cur, top, r)
		if err != nil {
			return nil, err
		}
		refused = append(refused, g.Check(l.Name, cur, next)...)
		cur = next
	}
	if len(refused) > 0 {
		return nil, &GuardrailError{Refusals: refused}
	}
	b, sum, err := ruleset.Canonical(cur)
	if err != nil {
		return nil, err
	}
	return &Effective{Document: cur, Bytes: b, SHA256: sum}, nil
}
