package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
	"github.com/ifc7/ifc/internal/ui"
	"github.com/ifc7/ifc/pkg/governance/check"
	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// exitGateFailed is the exit status when a governance gate blocks.
const exitGateFailed = 2

var lintCmd = &cobra.Command{
	Use:   "lint [name|path]...",
	Short: "Lint interface specifications and check governance gates",
	Long: `
Lint one or more specifications, score them and optionally check governance gates.

Arguments:
  name|path  Owned interface name from ifc.yaml, or a file path
             With no arguments, lints all owned interfaces
             File paths work even without an ifc.yaml project

Policy (pick at most one; the default is the built-in ruleset):
  --hub              Use each interface's effective policy on the hub: the ruleset of its
                     organization, group and interface, the score weights, the inline-ignore
                     setting, change-scoped rules and active waivers. Needs an owned
                     interface with a ref, and a login or IFC_TOKEN.
  --gate push|release
                     Like --hub, and also evaluate the gates the hub runs at that stage.
  --ruleset FILE     Lint with a local OpenLint ruleset. ifc7 ruleset references in its
                     extends (https://ifc7.dev/r/{owner}/{slug}/v{n}) are fetched from the
                     hub; only public rulesets can be fetched.

Baseline: with --hub or --gate the interface is compared with the release the hub would use:
the latest release, or with --gate release the highest release below --release-version. Breaking-change gates, scope: new gates and change-scoped rules (x-ifc-scope)
use it. --baseline FILE compares with a file instead, and --baseline none turns it off.

Gates from flags: --fail-on, --min-score, --breaking and --new-only add one blocking gate
named "cli", with or without a hub policy.

Exit status: 0 when every interface was linted and no blocking gate failed (with --strict,
no gate failed at all), 2 when a gate blocked, 1 for any other error. Without gates a low
score does not fail the command.

With --json, prints one JSON document to stdout, even on failure:
  {"passed", "interfaces": [{"name", "ref", "path", "policy", "baseline", "pluginId",
   "rulesetSha256", "score", "findings", "scopedOut", "compare", "outcome", "error"}]}
Each finding carries its status: open, waived, ignored or ignored-counted.
`,
	RunE: runLint,
}

type lintFlags struct {
	verbose         bool
	asJSON          bool
	hub             bool
	gate            string
	ruleset         string
	baseline        string
	baselineVersion string
	releaseVersion  string
	failOn          string
	minScore        int
	breaking        string
	newOnly         bool
	strict          bool
}

var lintOpts lintFlags

// lintReport is the --json document.
type lintReport struct {
	Passed     bool                   `json:"passed"`
	Interfaces []*project.CheckReport `json:"interfaces"`
}

func runLint(cmd *cobra.Command, args []string) error {
	f := lintOpts
	opts, err := f.checkOptions(cmd)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	messages := out
	if f.asJSON {
		messages = cmd.ErrOrStderr()
	}

	reports, runErr := checkAll(cmd, args, opts)
	passed := runErr == nil
	blocked := false
	for _, r := range reports {
		if r.Error != "" {
			passed = false
		}
		if r.Blocked(f.strict) {
			passed, blocked = false, true
		}
	}
	if f.asJSON {
		if reports == nil {
			reports = []*project.CheckReport{}
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(lintReport{Passed: passed, Interfaces: reports}); err != nil {
			return fmt.Errorf("error writing JSON report: %w", err)
		}
	} else {
		for _, r := range reports {
			printCheck(out, r, f.verbose)
		}
	}
	if runErr != nil {
		return runErr
	}
	var failed []string
	for _, r := range reports {
		if r.Error != "" {
			_, _ = fmt.Fprintln(messages, ui.Apply(ui.Error, fmt.Sprintf("%s: %s", r.Path, r.Error)))
			failed = append(failed, r.Path)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("lint failed for %s", strings.Join(failed, ", "))
	}
	if blocked {
		return &ExitError{Code: exitGateFailed, Err: project.ErrGateBlocked}
	}
	return nil
}

// checkAll resolves the targets and checks each one. The error is for problems
// that stop every target (bad project or ruleset); per-target errors are in
// the reports.
func checkAll(cmd *cobra.Command, args []string, opts project.CheckOptions) ([]*project.CheckReport, error) {
	ctx := cmd.Context()
	hub := opts.Hub || opts.Stage != ""
	proj, err := project.Load()
	var targets []project.CheckTarget
	if err != nil {
		// Bare file paths can be linted without a project, but have no hub policy.
		if len(args) == 0 || hub {
			return nil, fmt.Errorf("error loading project config: %w", err)
		}
		if proj, err = project.New(); err != nil {
			return nil, err
		}
		for _, a := range args {
			targets = append(targets, project.CheckTarget{Path: a})
		}
	} else if targets, err = proj.ResolveCheckTargets(args); err != nil {
		return nil, err
	}

	local := check.Policy{}
	source := project.PolicyDefault
	if opts.Ruleset != "" {
		if local, err = project.LocalRuleset(ctx, opts.Ruleset, http.DefaultClient); err != nil {
			return nil, err
		}
		source = project.PolicyLocal
	}
	reports := make([]*project.CheckReport, 0, len(targets))
	for _, t := range targets {
		reports = append(reports, proj.Check(ctx, t, local, source, opts))
	}
	return reports, nil
}

// checkOptions validates the flags and turns them into check options.
func (f lintFlags) checkOptions(cmd *cobra.Command) (project.CheckOptions, error) {
	opts := project.CheckOptions{
		Hub:             f.hub,
		Ruleset:         f.ruleset,
		Baseline:        f.baseline,
		BaselineVersion: f.baselineVersion,
		Version:         f.releaseVersion,
	}
	switch f.gate {
	case "":
	case string(policy.StagePush), string(policy.StageRelease):
		opts.Stage = policy.Stage(f.gate)
	default:
		return opts, fmt.Errorf("--gate must be push or release, not %q", f.gate)
	}
	if f.ruleset != "" && (f.hub || f.gate != "") {
		return opts, errors.New("--ruleset cannot be combined with --hub or --gate, which lint with the hub's ruleset")
	}
	if f.releaseVersion != "" && opts.Stage != policy.StageRelease {
		return opts, errors.New("--release-version needs --gate release")
	}
	if opts.Stage == policy.StageRelease && f.releaseVersion == "" {
		return opts, errors.New("--gate release needs --release-version, the version being released")
	}
	if f.baselineVersion != "" && (f.baseline == "" || f.baseline == project.BaselineNone) {
		return opts, errors.New("--baseline-version needs --baseline FILE")
	}
	if f.baseline != "" && f.baseline != project.BaselineNone && f.baselineVersion == "" && f.releaseVersion != "" {
		return opts, errors.New("--baseline FILE with --release-version needs --baseline-version, to judge the version bump")
	}

	g := policy.Gate{Name: "cli", On: []policy.Stage{policy.StagePush}, Enforcement: policy.EnforcementBlocking}
	set := false
	if f.failOn != "" {
		g.FailOn, set = policy.FailOn(f.failOn), true
	}
	if f.newOnly {
		if f.failOn == "" {
			return opts, errors.New("--new-only needs --fail-on")
		}
		g.Scope = policy.ScopeNew
	}
	if cmd.Flags().Changed("min-score") {
		n := f.minScore
		g.MinScore, set = &n, true
	}
	if f.breaking != "" {
		g.Breaking, set = policy.Breaking(f.breaking), true
	}
	if set {
		if err := g.Validate(); err != nil {
			return opts, fmt.Errorf("invalid gate flags: %w", err)
		}
		opts.Gates = []policy.Gate{g}
	}
	return opts, nil
}

func printCheck(w io.Writer, r *project.CheckReport, verbose bool) {
	_, _ = fmt.Fprintln(w, ui.Apply(ui.Emphasis, r.Path))
	if r.Error != "" {
		_, _ = fmt.Fprintf(w, "  %s %s\n", ui.KeyHints("error:   "), ui.Apply(ui.Error, r.Error))
		return
	}
	res := r.Result
	_, _ = fmt.Fprintf(w, "  %s %s\n", ui.KeyHints("plugin:  "), res.PluginID)
	if r.Policy != nil && r.Policy.Source != project.PolicyDefault {
		_, _ = fmt.Fprintf(w, "  %s %s\n", ui.KeyHints("ruleset: "), describePolicy(r.Policy))
	}
	if res.LintError != "" {
		_, _ = fmt.Fprintf(w, "  %s %s\n", ui.KeyHints("lint:    "), ui.Apply(ui.Error, res.LintError))
	} else {
		_, _ = fmt.Fprintf(w, "  %s %s\n", ui.KeyHints("score:   "), ui.FormatScore(res.Score))
		if r.Policy != nil && r.Policy.Source != project.PolicyDefault || res.Outcome != nil {
			_, _ = fmt.Fprintf(w, "  %s %s\n", ui.KeyHints("findings:"), describeFindings(res))
		}
	}
	if c := res.Compare; c != nil {
		_, _ = fmt.Fprintf(w, "  %s %s\n", ui.KeyHints("compared:"), describeCompare(r.Baseline, c))
	}
	if o := res.Outcome; o != nil {
		_, _ = fmt.Fprintf(w, "  %s\n", ui.KeyHints(fmt.Sprintf("gates (%s):", o.Stage)))
		for _, g := range o.Gates {
			mark, style := "✓", ui.Success
			if !g.Passed {
				mark, style = "✗", ui.Error
				if g.Enforcement != policy.EnforcementBlocking {
					style = ui.Warning
				}
			}
			line := fmt.Sprintf("%s %s (%s)", mark, g.Gate, g.Enforcement)
			var reasons []string
			for _, reason := range g.Reasons {
				reasons = append(reasons, reason.Message)
			}
			if len(reasons) > 0 {
				line += ": " + strings.Join(reasons, "; ")
			}
			_, _ = fmt.Fprintf(w, "    %s\n", ui.Apply(style, line))
		}
	}
	if verbose {
		if res.Raw != "" {
			_, _ = fmt.Fprintf(w, "\n%s", res.Raw)
		}
		for _, e := range res.Findings {
			if e.Status == policy.StatusWaived {
				_, _ = fmt.Fprintf(w, "  waived %s %s %s\n", e.Severity, e.RuleID, pointerOrRoot(e.Pointer))
			}
		}
		for _, sf := range res.ScopedOut {
			_, _ = fmt.Fprintf(w, "  outside changes %s %s %s\n", sf.Severity, sf.RuleID, pointerOrRoot(sf.Pointer))
		}
		if c := res.Compare; c != nil {
			for _, ch := range c.Changes {
				_, _ = fmt.Fprintf(w, "  change %s %s %s\n", ch.Level, ch.Kind, pointerOrRoot(ch.Pointer))
			}
		}
	}
}

func pointerOrRoot(p string) string {
	if p == "" {
		return "(root)"
	}
	return p
}

func describePolicy(p *project.PolicyInfo) string {
	sha := p.RulesetSHA256
	if len(sha) > 12 {
		sha = sha[:12]
	}
	switch p.Source {
	case project.PolicyLocal:
		return fmt.Sprintf("%s (%s)", p.Ruleset, sha)
	case project.PolicyHub:
		var parts []string
		if p.IsDefault {
			parts = append(parts, "hub default")
		} else {
			parts = append(parts, "hub organization policy")
		}
		if p.GroupID != "" {
			parts = append(parts, "group")
		}
		if p.HasInterfaceLayer {
			parts = append(parts, "interface layer")
		}
		s := fmt.Sprintf("%s (%s)", strings.Join(parts, " + "), sha)
		if p.Waivers > 0 {
			s += fmt.Sprintf(", %d active waiver(s)", p.Waivers)
		}
		return s
	}
	return sha
}

func describeFindings(res *check.Result) string {
	open := map[contract.Severity]int{}
	waived, ignored := 0, 0
	for _, e := range res.Findings {
		switch e.Status {
		case policy.StatusWaived:
			waived++
		case policy.StatusIgnored, policy.StatusIgnoredCounted:
			ignored++
		default:
			open[e.Severity]++
		}
	}
	var parts []string
	for _, s := range contract.Severities {
		if open[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", open[s], s))
		}
	}
	s := "none open"
	if len(parts) > 0 {
		s = strings.Join(parts, ", ")
	}
	var extra []string
	if waived > 0 {
		extra = append(extra, fmt.Sprintf("%d waived", waived))
	}
	if ignored > 0 {
		extra = append(extra, fmt.Sprintf("%d ignored inline", ignored))
	}
	if n := len(res.ScopedOut); n > 0 {
		extra = append(extra, fmt.Sprintf("%d outside changes", n))
	}
	if len(extra) > 0 {
		s += " (" + strings.Join(extra, ", ") + ")"
	}
	return s
}

func describeCompare(b *project.BaselineInfo, c *check.Compared) string {
	against := c.Against
	if b != nil && b.Source == "release" {
		against = "release " + b.Version
	}
	if c.Error != "" {
		return fmt.Sprintf("%s, %s", against, ui.Apply(ui.Error, "compare failed: "+c.Error))
	}
	n := 0
	for _, ch := range c.Changes {
		if ch.Level == contract.ChangeLevelError {
			n++
		}
	}
	if c.Changes == nil || (len(c.Changes) == 0 && c.Breaking) {
		return fmt.Sprintf("%s, breaking=%s", against, ui.FormatBreaking(c.Breaking))
	}
	return fmt.Sprintf("%s, %d change(s), %d breaking", against, len(c.Changes), n)
}

func init() {
	f := lintCmd.Flags()
	f.BoolVarP(&lintOpts.verbose, "verbose", "v", false, "print the plugin's raw findings, waived and out-of-scope findings, and changes")
	f.BoolVar(&lintOpts.asJSON, "json", false, "print a JSON report to stdout")
	f.BoolVar(&lintOpts.hub, "hub", false, "lint with each interface's effective policy on the hub")
	f.StringVar(&lintOpts.gate, "gate", "", "evaluate the hub's gates for a stage: push or release (implies --hub)")
	f.StringVar(&lintOpts.ruleset, "ruleset", "", "lint with a local OpenLint ruleset file")
	f.StringVar(&lintOpts.baseline, "baseline", "", "specification file to compare against, or none (default with --hub: the latest release)")
	f.StringVar(&lintOpts.baselineVersion, "baseline-version", "", "version of the --baseline file, for breaking: requireMajor")
	f.StringVar(&lintOpts.releaseVersion, "release-version", "", "version being released, with --gate release")
	f.StringVar(&lintOpts.failOn, "fail-on", "", "fail when an unwaived finding is at this severity or above: error, warn, info or hint")
	f.IntVar(&lintOpts.minScore, "min-score", 0, "fail when the score is below this (0-100)")
	f.StringVar(&lintOpts.breaking, "breaking", "", "fail on breaking changes against the baseline: block, or requireMajor (with --gate release)")
	f.BoolVar(&lintOpts.newOnly, "new-only", false, "with --fail-on, count only findings that are not in the baseline")
	f.BoolVar(&lintOpts.strict, "strict", false, "also fail when an advisory gate fails")
	rootCmd.AddCommand(lintCmd)
}
