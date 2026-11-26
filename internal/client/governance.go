package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ifc7/ifc/internal"
	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/governance/policy"
	"github.com/ifc7/ifc/pkg/governance/ruleset"
)

// EffectiveRuleset is the fully resolved ruleset an interface is linted with.
type EffectiveRuleset struct {
	// Document is the canonical JSON ruleset; SHA256 is over exactly these bytes.
	Document          string               `json:"document"`
	SHA256            string               `json:"sha256"`
	ScoreWeights      findings.Weights     `json:"scoreWeights"`
	IsDefault         bool                 `json:"isDefault"`
	HasInterfaceLayer bool                 `json:"hasInterfaceLayer,omitempty"`
	GroupID           string               `json:"groupId,omitempty"`
	ChangeScopedRules []string             `json:"changeScopedRules,omitempty"`
	InlineIgnores     policy.InlineIgnores `json:"inlineIgnores,omitempty"`
}

// EffectivePolicy is everything governance applies to an interface now, as
// GET /interfaces/{interfaceId}/governance/effective-policy returns it.
type EffectivePolicy struct {
	Ruleset        EffectiveRuleset      `json:"ruleset"`
	Gates          []policy.Gate         `json:"gates"`
	WaiverSettings policy.WaiverSettings `json:"waiverSettings"`
	ChangeSettings policy.ChangeSettings `json:"changeSettings"`
	Waivers        []policy.Waiver       `json:"waivers"`
}

// maxRulesetBytes bounds a ruleset document read from the hub. Rulesets are at most 256 KiB.
const maxRulesetBytes = 1 << 20

// GetEffectivePolicyWithResponse fetches an interface's effective governance policy. It returns
// the HTTP status, and a nil policy for any status but 200.
func (c *ClientWithResponses) GetEffectivePolicyWithResponse(ctx context.Context, interfaceID InterfaceId, reqEditors ...RequestEditorFn) (*EffectivePolicy, int, error) {
	underlying, ok := c.ClientInterface.(*Client)
	if !ok {
		return nil, 0, fmt.Errorf("effective policy requires *Client transport")
	}
	u, err := url.Parse(underlying.Server)
	if err != nil {
		return nil, 0, err
	}
	u = u.JoinPath("interfaces", interfaceID, "governance", "effective-policy")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	for _, e := range append(append([]RequestEditorFn{}, underlying.RequestEditors...), reqEditors...) {
		if err := e(ctx, req); err != nil {
			return nil, 0, err
		}
	}
	rsp, err := underlying.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, rsp.StatusCode, nil
	}
	var pol EffectivePolicy
	if err := json.NewDecoder(io.LimitReader(rsp.Body, 8*maxRulesetBytes)).Decode(&pol); err != nil {
		return nil, rsp.StatusCode, fmt.Errorf("decode effective policy: %w", err)
	}
	return &pol, rsp.StatusCode, nil
}

// FetchRulesetRef downloads a public ruleset version from its reference,
// https://ifc7.dev/r/{owner}/{slug}/v{n}. References on the default host are
// fetched from the hub at $IFC_HUB_URL. The request carries no credentials:
// the locator serves public rulesets only.
func FetchRulesetRef(ctx context.Context, doer HttpRequestDoer, ref ruleset.Ref) ([]byte, error) {
	hub, err := internal.HubURL()
	if err != nil {
		return nil, err
	}
	u := hub.JoinPath("r", ref.Owner, ref.Slug, fmt.Sprintf("v%d", ref.Version))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if doer == nil {
		doer = http.DefaultClient
	}
	rsp, err := doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch ruleset %s: %w", ref, err)
	}
	defer rsp.Body.Close()
	switch rsp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("ruleset %s not found on %s (only public rulesets can be fetched; lint with --hub to use a private one through the interface's policy)", ref, hub.Host)
	default:
		return nil, fmt.Errorf("fetch ruleset %s: HTTP %d", ref, rsp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(rsp.Body, maxRulesetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch ruleset %s: %w", ref, err)
	}
	if len(b) > maxRulesetBytes {
		return nil, fmt.Errorf("ruleset %s is larger than %d bytes", ref, maxRulesetBytes)
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil, fmt.Errorf("ruleset %s is empty", ref)
	}
	return b, nil
}
