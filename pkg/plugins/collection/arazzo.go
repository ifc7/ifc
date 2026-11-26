package collection

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/daveshanley/vacuum/model"
	"github.com/daveshanley/vacuum/motor"
	"github.com/daveshanley/vacuum/rulesets"

	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/internal/vacuumlint"
)

var (
	arazzoOnce sync.Once
	arazzoRS   *rulesets.RuleSet
)

func arazzoRuleSet() *rulesets.RuleSet {
	arazzoOnce.Do(func() { arazzoRS = rulesets.BuildDefaultRuleSets().GenerateArazzoRecommendedRuleSet() })
	return arazzoRS
}

func vacuumEngine() string {
	e := vacuumlint.Engine()
	return e.Name + " " + e.Version
}

// unavailableSource is the message suffix Vacuum uses when a step's source
// description can't be loaded. Validate never loads sources, so these are
// replaced by the cross-check against the bound interface.
const unavailableSource = "source document is unavailable"

// validateArazzo lints an Arazzo document with Vacuum's recommended Arazzo
// rules. Source descriptions are never fetched.
func validateArazzo(ctx context.Context, in Input) (Result, error) {
	doc, _, bad := parseJSON(in.Source)
	if bad != nil {
		return Result{Findings: []contract.Finding{*bad}}, nil
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return Result{Findings: []contract.Finding{finding(RuleParse, contract.SeverityError, "", "an Arazzo document must be an object")}}, nil
	}
	if v, _ := root["arazzo"].(string); !strings.HasPrefix(v, "1.0.") {
		return Result{Findings: []contract.Finding{finding(RuleSchema, contract.SeverityError, "/arazzo", fmt.Sprintf("arazzo must be a 1.0.x version, got %q", fmt.Sprint(root["arazzo"])))}}, nil
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	res := motor.ApplyRulesToRuleSet(&motor.RuleSetExecution{
		RuleSet:     arazzoRuleSet(),
		Spec:        in.Source,
		SilenceLogs: true,
		AllowLookup: false,
	})
	defer res.ReleaseOwnedResources()
	var out []contract.Finding
	if len(res.Errors) > 0 {
		out = append(out, finding(RuleParse, contract.SeverityError, "", fmt.Sprintf("failed to validate the Arazzo document: %v", res.Errors[0])))
	}
	for i := range res.Results {
		r := &res.Results[i]
		if r.RepairOnly || strings.HasSuffix(r.Message, unavailableSource) {
			continue
		}
		out = append(out, arazzoFinding(r))
	}
	out = append(out, crossCheckArazzo(root, indexOpenAPI(in.Interface))...)
	return Result{Findings: out}, nil
}

func arazzoFinding(r *model.RuleFunctionResult) contract.Finding {
	rule := r.RuleId
	if rule == "" && r.Rule != nil {
		rule = r.Rule.Id
	}
	sev := contract.SeverityWarn
	switch r.RuleSeverity {
	case model.SeverityError:
		sev = contract.SeverityError
	case model.SeverityInfo:
		sev = contract.SeverityInfo
	case model.SeverityHint:
		sev = contract.SeverityHint
	}
	f := finding(rule, sev, findings.PointerFromJSONPath(r.Path), r.Message)
	if r.StartNode != nil && r.StartNode.Line > 0 {
		pos := contract.Position{Line: r.StartNode.Line, Column: max(1, r.StartNode.Column)}
		f.Range = &contract.Range{Start: pos, End: pos}
	}
	if r.Rule != nil {
		f.DocumentationURL = r.Rule.DocumentationURL
	}
	return f
}

// crossCheckArazzo checks that each step's operationId or operationPath
// exists in the bound OpenAPI document. It only runs when the document has
// exactly one OpenAPI source description, since that is the one bound to the
// interface; steps that name another source are skipped.
func crossCheckArazzo(root map[string]any, ops *operations) []contract.Finding {
	if ops == nil {
		return nil
	}
	sources, _ := root["sourceDescriptions"].([]any)
	var openapiSource string
	n := 0
	for _, s := range sources {
		m, _ := s.(map[string]any)
		typ, _ := m["type"].(string)
		if typ == "" || typ == "openapi" {
			n++
			openapiSource, _ = m["name"].(string)
		}
	}
	if n != 1 {
		return nil
	}
	var out []contract.Finding
	workflows, _ := root["workflows"].([]any)
	for wi, w := range workflows {
		wm, _ := w.(map[string]any)
		steps, _ := wm["steps"].([]any)
		for si, s := range steps {
			sm, _ := s.(map[string]any)
			if id, ok := sm["operationId"].(string); ok && id != "" {
				name, opID := splitSourceRef(id)
				if name != "" && name != openapiSource {
					continue
				}
				if !ops.ids[opID] {
					out = append(out, finding(RuleUnknownOperation, contract.SeverityWarn, pointer("", "workflows", wi, "steps", si, "operationId"),
						fmt.Sprintf("operationId %q is not in the interface's OpenAPI document", opID)))
				}
			}
			if p, ok := sm["operationPath"].(string); ok && p != "" {
				name, frag := splitOperationPath(p)
				if name != "" && name != openapiSource {
					continue
				}
				if frag != "" && !ops.resolvePointer(frag) {
					out = append(out, finding(RuleUnknownOperation, contract.SeverityWarn, pointer("", "workflows", wi, "steps", si, "operationPath"),
						fmt.Sprintf("operationPath %q does not resolve in the interface's OpenAPI document", p)))
				}
			}
		}
	}
	return out
}

// splitSourceRef splits "$sourceDescriptions.name.opId" into name and opId.
// A plain operationId has no name.
func splitSourceRef(id string) (string, string) {
	const prefix = "$sourceDescriptions."
	if !strings.HasPrefix(id, prefix) {
		return "", id
	}
	rest := id[len(prefix):]
	if i := strings.Index(rest, "."); i > 0 {
		return rest[:i], rest[i+1:]
	}
	return "", id
}

// splitOperationPath splits "{$sourceDescriptions.name.url}#/paths/~1x/get"
// into the source name and the JSON Pointer fragment.
func splitOperationPath(p string) (string, string) {
	name := ""
	if strings.HasPrefix(p, "{$sourceDescriptions.") {
		if end := strings.Index(p, "}"); end > 0 {
			ref := p[len("{$sourceDescriptions."):end]
			if i := strings.Index(ref, "."); i > 0 {
				name = ref[:i]
			}
		}
	}
	frag := ""
	if i := strings.Index(p, "#"); i >= 0 {
		frag = p[i+1:]
	}
	return name, frag
}
