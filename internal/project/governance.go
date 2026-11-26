package project

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver/v3"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/pkg/governance/check"
	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/governance/ruleset"
)

// BaselineNone turns the baseline off: nothing is compared.
const BaselineNone = "none"

// PolicySource says where a check's policy came from.
type PolicySource string

const (
	// PolicyDefault is the built-in default ruleset with no gates of its own.
	PolicyDefault PolicySource = "default"
	// PolicyLocal is a ruleset file given with --ruleset.
	PolicyLocal PolicySource = "local"
	// PolicyHub is the interface's effective policy on the hub.
	PolicyHub PolicySource = "hub"
)

// CheckOptions selects the policy, baseline and gates of a governance check.
type CheckOptions struct {
	// Hub uses each interface's effective policy from the hub: its ruleset,
	// weights, inline-ignore setting, change-scoped rules and waivers.
	Hub bool
	// Stage evaluates the hub's gates at this stage. It implies Hub.
	Stage policy.Stage
	// Ruleset is a local OpenLint ruleset file. ifc7 ruleset references in
	// its extends are fetched from the hub. Not with Hub.
	Ruleset string
	// Baseline is a specification file to compare against, BaselineNone,
	// or empty for the default with Hub, as on the hub: the latest release
	// at push, and the highest release below Version at release. Without
	// Hub the default is nothing.
	Baseline string
	// BaselineVersion is the version of a Baseline file, for
	// breaking: requireMajor.
	BaselineVersion string
	// Version is the version being released, for breaking: requireMajor.
	Version string
	// Gates are extra gates from the command line. They run at Stage, or at
	// push when Stage is empty.
	Gates []policy.Gate
}

// PolicyInfo describes the policy a check used.
type PolicyInfo struct {
	Source            PolicySource `json:"source"`
	RulesetSHA256     string       `json:"rulesetSha256"`
	Ruleset           string       `json:"ruleset,omitempty"`
	IsDefault         bool         `json:"isDefault,omitempty"`
	HasInterfaceLayer bool         `json:"hasInterfaceLayer,omitempty"`
	GroupID           string       `json:"groupId,omitempty"`
	Waivers           int          `json:"waivers"`
	Gates             int          `json:"gates"`
}

// BaselineInfo describes what a check compared against.
type BaselineInfo struct {
	// Source is release (the hub's latest release) or file.
	Source  string `json:"source"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
}

// CheckReport is one interface's governance check.
type CheckReport struct {
	Name     string        `json:"name,omitempty"`
	Ref      string        `json:"ref,omitempty"`
	Path     string        `json:"path"`
	Policy   *PolicyInfo   `json:"policy,omitempty"`
	Baseline *BaselineInfo `json:"baseline,omitempty"`
	*check.Result
	Error string `json:"error,omitempty"`
}

// Blocked reports whether a blocking gate failed, or with strict any gate.
func (r *CheckReport) Blocked(strict bool) bool {
	if r.Result == nil || r.Outcome == nil {
		return false
	}
	return !r.Outcome.Passed || (strict && r.Outcome.AdvisoryFailed)
}

// CheckTarget is a specification to check: a file, and the owned interface it belongs to.
type CheckTarget struct {
	Path  string
	Owned *Owned
}

// ResolveCheckTargets resolves lint arguments like ResolveLintTargets, and also
// returns the owned interface each one belongs to. A file path matches an owned
// interface with the same path.
func (p *Project) ResolveCheckTargets(targets []string) ([]CheckTarget, error) {
	paths, err := p.ResolveLintTargets(targets)
	if err != nil {
		return nil, err
	}
	out := make([]CheckTarget, 0, len(paths))
	for _, path := range paths {
		t := CheckTarget{Path: path}
		for i := range p.config.Own {
			if samePath(p.config.Own[i].Path, path) {
				t.Owned = &p.config.Own[i]
				break
			}
		}
		out = append(out, t)
	}
	return out, nil
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && aa == bb
}

// LocalRuleset reads a ruleset file and resolves it into an effective ruleset,
// fetching ifc7 ruleset references from the hub.
func LocalRuleset(ctx context.Context, path string, doer client.HttpRequestDoer) (check.Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return check.Policy{}, fmt.Errorf("read ruleset: %w", err)
	}
	resolver := ruleset.RefResolver(func(ctx context.Context, ref ruleset.Ref) ([]byte, error) {
		return client.FetchRulesetRef(ctx, doer, ref)
	})
	doc, b, sum, err := ruleset.Effective(ctx, data, resolver)
	if err != nil {
		return check.Policy{}, fmt.Errorf("ruleset %s: %w", path, err)
	}
	return check.Policy{Ruleset: b, SHA256: sum, ChangeScoped: doc.ChangeScopedRules()}, nil
}

// Check runs a governance check on one target. local is the policy to use
// when opts selects no hub policy (the zero Policy is the built-in default).
// Errors that concern only this target are returned in the report.
func (p *Project) Check(ctx context.Context, t CheckTarget, local check.Policy, localSource PolicySource, opts CheckOptions) *CheckReport {
	rep := &CheckReport{Path: t.Path}
	if t.Owned != nil {
		rep.Name, rep.Ref = t.Owned.Name, t.Owned.Ref
	}
	fail := func(err error) *CheckReport {
		rep.Error = err.Error()
		return rep
	}
	raw, typ, _, err := readSpecFile(t.Path)
	if err != nil {
		return fail(err)
	}

	pol := local
	info := &PolicyInfo{Source: localSource}
	if localSource == PolicyLocal {
		info.Ruleset = opts.Ruleset
	}
	var ifcID client.InterfaceId
	hub := opts.Hub || opts.Stage != ""
	if hub {
		if t.Owned == nil || t.Owned.Ref == "" {
			return fail(fmt.Errorf("%s is not an owned interface with a ref in ifc.yaml, so it has no hub policy (push it first, or lint without --hub and --gate)", t.Path))
		}
		ifcID, err = p.resolveRefToID(ctx, t.Owned.Ref)
		if err != nil {
			return fail(err)
		}
		ep, status, err := p.client.GetEffectivePolicyWithResponse(ctx, ifcID)
		if err != nil {
			return fail(fmt.Errorf("fetch effective policy for %s: %w", t.Owned.Ref, err))
		}
		if status != http.StatusOK {
			return fail(fmt.Errorf("fetch effective policy for %s: %s", t.Owned.Ref, policyStatusHint(status)))
		}
		pol = check.Policy{
			Ruleset:        []byte(ep.Ruleset.Document),
			SHA256:         ep.Ruleset.SHA256,
			Weights:        ep.Ruleset.ScoreWeights,
			InlineIgnores:  ep.Ruleset.InlineIgnores,
			ChangeScoped:   ep.Ruleset.ChangeScopedRules,
			ChangeSettings: ep.ChangeSettings,
			Waivers:        ep.Waivers,
		}
		if pol.ChangeScoped == nil {
			pol.ChangeScoped = []string{}
		}
		if opts.Stage != "" {
			pol.Gates = ep.Gates
		}
		info = &PolicyInfo{
			Source:            PolicyHub,
			IsDefault:         ep.Ruleset.IsDefault,
			HasInterfaceLayer: ep.Ruleset.HasInterfaceLayer,
			GroupID:           ep.Ruleset.GroupID,
			Waivers:           len(ep.Waivers),
		}
	}
	stage := opts.Stage
	if stage == "" && len(opts.Gates) > 0 {
		stage = policy.StagePush
	}
	for _, g := range opts.Gates {
		g.On = []policy.Stage{stage}
		pol.Gates = append(pol.Gates, g)
	}
	info.Gates = 0
	for _, g := range pol.Gates {
		if g.RunsAt(stage) {
			info.Gates++
		}
	}

	in := check.Input{InterfaceType: toPluginType(typ), Document: raw, Stage: stage, Version: opts.Version}
	switch {
	case opts.Baseline == BaselineNone:
	case opts.Baseline != "":
		b, btyp, _, err := readSpecFile(opts.Baseline)
		if err != nil {
			return fail(fmt.Errorf("baseline: %w", err))
		}
		if btyp != typ {
			return fail(fmt.Errorf("baseline %s is %s but %s is %s", opts.Baseline, btyp, t.Path, typ))
		}
		in.Baseline = &check.Baseline{Document: b, Version: opts.BaselineVersion, Name: opts.Baseline}
		rep.Baseline = &BaselineInfo{Source: "file", Path: opts.Baseline, Version: opts.BaselineVersion}
	case hub:
		base, version, err := p.baselineRelease(ctx, ifcID, stage, opts.Version)
		if err != nil {
			return fail(fmt.Errorf("baseline for %s: %w", t.Owned.Ref, err))
		}
		if base != nil {
			in.Baseline = &check.Baseline{Document: base, Version: version}
			rep.Baseline = &BaselineInfo{Source: "release", Version: version}
		}
	}

	res, err := check.Run(ctx, nil, pol, in)
	if err != nil {
		return fail(err)
	}
	info.RulesetSHA256 = res.RulesetSHA256
	rep.Policy = info
	rep.Result = res
	return rep
}

// baselineRelease returns the specification and version of the release the hub
// compares against at stage: the latest release on push, and on release the
// highest release below the version being released. It returns nils when
// there is none.
func (p *Project) baselineRelease(ctx context.Context, id client.InterfaceId, stage policy.Stage, version string) ([]byte, string, error) {
	var rel *client.InterfaceRelease
	if stage == policy.StageRelease {
		target, err := semver.NewVersion(version)
		if err != nil {
			return nil, "", fmt.Errorf("release version %q: %w", version, err)
		}
		rels, err := p.client.ListInterfaceReleasesWithResponse(ctx, id)
		if err != nil {
			return nil, "", err
		}
		if rels.StatusCode() != http.StatusOK || rels.JSON200 == nil {
			return nil, "", fmt.Errorf("list releases: HTTP %d", rels.StatusCode())
		}
		var best *semver.Version
		for i, r := range *rels.JSON200 {
			v, err := semver.NewVersion(r.SemanticVersion)
			if err != nil || !v.LessThan(target) || (best != nil && !best.LessThan(v)) {
				continue
			}
			best, rel = v, &(*rels.JSON200)[i]
		}
	} else {
		ifc, err := p.client.GetInterfaceWithResponse(ctx, id)
		if err != nil {
			return nil, "", err
		}
		if ifc.StatusCode() != http.StatusOK || ifc.JSON200 == nil {
			return nil, "", fmt.Errorf("fetch interface: HTTP %d", ifc.StatusCode())
		}
		rel = ifc.JSON200.LatestRelease
	}
	if rel == nil {
		return nil, "", nil
	}
	rev, err := p.client.GetInterfaceRevisionWithResponse(ctx, id, rel.InterfaceRevisionId)
	if err != nil {
		return nil, "", err
	}
	if rev.StatusCode() != http.StatusOK || rev.JSON200 == nil {
		return nil, "", fmt.Errorf("fetch release %s: HTTP %d", rel.SemanticVersion, rev.StatusCode())
	}
	spec, err := base64Decode(rev.JSON200.Specification)
	if err != nil {
		return nil, "", fmt.Errorf("decode release %s: %w", rel.SemanticVersion, err)
	}
	return spec, rel.SemanticVersion, nil
}

func policyStatusHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "HTTP 401 (log in with ifc login, or set IFC_TOKEN)"
	case http.StatusForbidden:
		return "HTTP 403 (this login or token cannot read the interface's governance policy)"
	case http.StatusNotFound:
		return "HTTP 404 (the interface does not exist, or the hub does not support governance)"
	}
	return fmt.Sprintf("HTTP %d", status)
}

// ErrGateBlocked is returned when a gate blocks a checked interface.
var ErrGateBlocked = errors.New("governance gate failed")
