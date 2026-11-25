package policy_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

func finding(rule, ptr string, sev contract.Severity) contract.Finding {
	msg := rule + " failed"
	return contract.Finding{RuleID: rule, Pointer: ptr, Severity: sev, Message: msg, Fingerprint: findings.Fingerprint(rule, ptr, msg)}
}

func at(d time.Duration) *time.Time {
	t := day.Add(d)
	return &t
}

func TestWaiver_ActiveAt(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		w    policy.Waiver
		want bool
	}{
		"no expiry":       {w: policy.Waiver{}, want: true},
		"future expiry":   {w: policy.Waiver{ExpiresAt: at(time.Hour)}, want: true},
		"expired":         {w: policy.Waiver{ExpiresAt: at(-time.Hour)}, want: false},
		"expires exactly": {w: policy.Waiver{ExpiresAt: at(0)}, want: false},
		"revoked":         {w: policy.Waiver{RevokedAt: at(-time.Minute)}, want: false},
		"revoked later":   {w: policy.Waiver{RevokedAt: at(time.Minute)}, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tc.w.ActiveAt(day); got != tc.want {
				t.Fatalf("ActiveAt = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWaiver_Matches(t *testing.T) {
	t.Parallel()
	f := finding("operation-tags", "/paths/~1pets/get", contract.SeverityWarn)
	for name, tc := range map[string]struct {
		w    policy.Waiver
		want bool
	}{
		"rule and pointer":          {w: policy.Waiver{RuleID: "operation-tags", Pointer: "/paths/~1pets/get"}, want: true},
		"other pointer":             {w: policy.Waiver{RuleID: "operation-tags", Pointer: "/paths/~1pets/post"}, want: false},
		"other rule":                {w: policy.Waiver{RuleID: "info-contact", Pointer: "/paths/~1pets/get"}, want: false},
		"fingerprint wins":          {w: policy.Waiver{RuleID: "operation-tags", Pointer: "/elsewhere", Fingerprint: f.Fingerprint}, want: true},
		"fingerprint mismatch":      {w: policy.Waiver{RuleID: "operation-tags", Pointer: "/paths/~1pets/get", Fingerprint: strings.Repeat("0", 64)}, want: false},
		"fingerprint needs rule id": {w: policy.Waiver{RuleID: "x", Fingerprint: f.Fingerprint}, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tc.w.Matches(f); got != tc.want {
				t.Fatalf("Matches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateWaiver(t *testing.T) {
	t.Parallel()
	base := policy.Waiver{RuleID: "r", Pointer: "/a", Reason: "accepted", ExpiresAt: at(24 * time.Hour)}
	capped := policy.WaiverSettings{MaxExpiryDays: 30}
	for name, tc := range map[string]struct {
		edit     func(w *policy.Waiver)
		settings policy.WaiverSettings
		wantErr  bool
	}{
		"valid":                 {edit: func(*policy.Waiver) {}},
		"no expiry without cap": {edit: func(w *policy.Waiver) { w.ExpiresAt = nil }},
		"no expiry with cap":    {edit: func(w *policy.Waiver) { w.ExpiresAt = nil }, settings: capped, wantErr: true},
		"within cap":            {edit: func(w *policy.Waiver) { w.ExpiresAt = at(29 * 24 * time.Hour) }, settings: capped},
		"beyond cap":            {edit: func(w *policy.Waiver) { w.ExpiresAt = at(31 * 24 * time.Hour) }, settings: capped, wantErr: true},
		"past expiry":           {edit: func(w *policy.Waiver) { w.ExpiresAt = at(-time.Hour) }, wantErr: true},
		"no rule":               {edit: func(w *policy.Waiver) { w.RuleID = " " }, wantErr: true},
		"no reason":             {edit: func(w *policy.Waiver) { w.Reason = "" }, wantErr: true},
		"long reason":           {edit: func(w *policy.Waiver) { w.Reason = strings.Repeat("x", policy.MaxReasonLength+1) }, wantErr: true},
		"bad pointer":           {edit: func(w *policy.Waiver) { w.Pointer = "paths" }, wantErr: true},
		"root pointer":          {edit: func(w *policy.Waiver) { w.Pointer = "" }},
		"bad fingerprint":       {edit: func(w *policy.Waiver) { w.Fingerprint = "ABC" }, wantErr: true},
		"good fingerprint":      {edit: func(w *policy.Waiver) { w.Fingerprint = strings.Repeat("a", 64) }},
		"uppercase fingerprint": {edit: func(w *policy.Waiver) { w.Fingerprint = strings.Repeat("A", 64) }, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := base
			tc.edit(&w)
			err := policy.ValidateWaiver(w, tc.settings, day)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, policy.ErrInvalid) {
				t.Fatalf("err should wrap ErrInvalid: %v", err)
			}
		})
	}
}

func statuses(es []policy.Evaluated) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.RuleID + ":" + string(e.Status)
	}
	return out
}

func TestApplyWaivers(t *testing.T) {
	t.Parallel()
	fs := []contract.Finding{
		finding("a", "/x", contract.SeverityError),
		finding("b", "/y", contract.SeverityWarn),
	}
	ignored := []contract.Finding{finding("c", "/z", contract.SeverityError)}
	waivers := []policy.Waiver{
		{ID: "w-expired", RuleID: "a", Pointer: "/x", ExpiresAt: at(-time.Hour)},
		{ID: "w-a", RuleID: "a", Pointer: "/x", ExpiresAt: at(time.Hour)},
		{ID: "w-a2", RuleID: "a", Pointer: "/x"},
		{ID: "w-c", RuleID: "c", Pointer: "/z"},
	}
	for name, tc := range map[string]struct {
		inline policy.InlineIgnores
		want   string
	}{
		"allow (default)": {inline: "", want: "a:waived b:open c:ignored"},
		"report":          {inline: policy.InlineIgnoresReport, want: "a:waived b:open c:waived"},
		"deny":            {inline: policy.InlineIgnoresDeny, want: "a:waived b:open c:waived"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := policy.ApplyWaivers(fs, ignored, waivers, tc.inline, day)
			if s := strings.Join(statuses(got), " "); s != tc.want {
				t.Fatalf("statuses = %s, want %s", s, tc.want)
			}
			if got[0].WaiverID != "w-a" {
				t.Fatalf("first active waiver should win, got %q", got[0].WaiverID)
			}
		})
	}

	t.Run("report without a waiver counts the ignored finding", func(t *testing.T) {
		t.Parallel()
		got := policy.ApplyWaivers(nil, ignored, nil, policy.InlineIgnoresReport, day)
		if got[0].Status != policy.StatusIgnoredCounted {
			t.Fatalf("status = %s", got[0].Status)
		}
	})

	t.Run("expired waivers stop applying without a re-lint", func(t *testing.T) {
		t.Parallel()
		w := []policy.Waiver{{ID: "w", RuleID: "a", Pointer: "/x", ExpiresAt: at(time.Hour)}}
		if got := policy.ApplyWaivers(fs, nil, w, "", day); got[0].Status != policy.StatusWaived {
			t.Fatal("waiver should apply before expiry")
		}
		if got := policy.ApplyWaivers(fs, nil, w, "", day.Add(2*time.Hour)); got[0].Status != policy.StatusOpen {
			t.Fatal("waiver should stop applying after expiry")
		}
	})

	t.Run("does not modify its input", func(t *testing.T) {
		t.Parallel()
		in := []contract.Finding{finding("a", "/x", contract.SeverityError)}
		_ = policy.ApplyWaivers(in, nil, waivers, "", day)
		if in[0].RuleID != "a" {
			t.Fatal("input changed")
		}
	})
}

func TestScoreOf(t *testing.T) {
	t.Parallel()
	fs := []contract.Finding{finding("a", "/x", contract.SeverityError), finding("b", "/y", contract.SeverityError)}
	ignored := []contract.Finding{finding("c", "/z", contract.SeverityError)}
	waivers := []policy.Waiver{{ID: "w", RuleID: "a", Pointer: "/x"}}

	allow := policy.ApplyWaivers(fs, ignored, waivers, policy.InlineIgnoresAllow, day)
	if got, want := policy.ScoreOf(allow, nil), findings.Score(fs, nil); got != want {
		t.Fatalf("allow: score = %d, want %d (waived counts, ignored does not)", got, want)
	}
	deny := policy.ApplyWaivers(fs, ignored, waivers, policy.InlineIgnoresDeny, day)
	if got, want := policy.ScoreOf(deny, nil), findings.Score(append(append([]contract.Finding{}, fs...), ignored...), nil); got != want {
		t.Fatalf("deny: score = %d, want %d", got, want)
	}
	report := policy.ApplyWaivers(fs, ignored, waivers, policy.InlineIgnoresReport, day)
	if policy.ScoreOf(report, nil) != policy.ScoreOf(deny, nil) {
		t.Fatal("report and deny should score the same")
	}
}
