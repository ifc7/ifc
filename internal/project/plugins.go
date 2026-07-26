package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/fileio"
	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/runner"
)

// LintResult is the outcome of running a lint plugin on a local specification.
type LintResult struct {
	Target   string
	PluginID string
	Output   contract.LintOutput
}

// CompareResult is the outcome of running a compare plugin on two specifications.
type CompareResult struct {
	Before   string
	After    string
	PluginID string
	Output   contract.CompareOutput
}

// LintFile runs the default linter for a specification file.
func LintFile(path string) (LintResult, error) {
	raw, typ, format, err := readSpecFile(path)
	if err != nil {
		return LintResult{}, err
	}
	reg := runner.DefaultRegistry()
	out, pluginID, err := reg.Lint(contract.LintInput{
		InterfaceType: toPluginType(typ),
		Document:      contract.NewSpecificationDocument(raw, format),
	})
	if err != nil {
		return LintResult{}, err
	}
	return LintResult{Target: path, PluginID: pluginID, Output: out}, nil
}

// CompareFiles runs the default change detector on two specification files.
func CompareFiles(beforePath, afterPath string) (CompareResult, error) {
	beforeRaw, beforeType, beforeFormat, err := readSpecFile(beforePath)
	if err != nil {
		return CompareResult{}, fmt.Errorf("before: %w", err)
	}
	afterRaw, afterType, afterFormat, err := readSpecFile(afterPath)
	if err != nil {
		return CompareResult{}, fmt.Errorf("after: %w", err)
	}
	if beforeType != afterType {
		return CompareResult{}, fmt.Errorf("interface type mismatch: before=%s after=%s", beforeType, afterType)
	}
	reg := runner.DefaultRegistry()
	out, pluginID, err := reg.Compare(contract.CompareInput{
		InterfaceType: toPluginType(beforeType),
		Before:        contract.NewSpecificationDocument(beforeRaw, beforeFormat),
		After:         contract.NewSpecificationDocument(afterRaw, afterFormat),
	})
	if err != nil {
		return CompareResult{}, err
	}
	return CompareResult{
		Before:   beforePath,
		After:    afterPath,
		PluginID: pluginID,
		Output:   out,
	}, nil
}

// ResolveLintTargets resolves CLI targets to file paths.
// Empty targets means all owned interfaces. Named targets match owned names or file paths.
func (p *Project) ResolveLintTargets(targets []string) ([]string, error) {
	if len(targets) == 0 {
		if len(p.config.Own) == 0 {
			return nil, fmt.Errorf("no owned interfaces to lint; pass a file path or add interfaces with ifc add")
		}
		paths := make([]string, 0, len(p.config.Own))
		for _, own := range p.config.Own {
			paths = append(paths, own.Path)
		}
		return paths, nil
	}
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		path, err := p.resolveSpecPath(target)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// ResolveComparePath resolves a CLI target (owned name or file path) to a file path.
func (p *Project) ResolveComparePath(target string) (string, error) {
	return p.resolveSpecPath(target)
}

func (p *Project) resolveSpecPath(target string) (string, error) {
	for _, own := range p.config.Own {
		if own.Name == target {
			if own.Path == "" {
				return "", fmt.Errorf("owned interface %q has empty path", target)
			}
			return own.Path, nil
		}
	}
	if fileio.FileExists(target) {
		return target, nil
	}
	abs, err := filepath.Abs(target)
	if err == nil && fileio.FileExists(abs) {
		return abs, nil
	}
	return "", fmt.Errorf("unknown interface or file %q", target)
}

func readSpecFile(path string) ([]byte, client.InterfaceType, contract.FileFormat, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("read %s: %w", path, err)
	}
	typ, err := DetectSpecificationType(raw)
	if err != nil {
		return nil, "", "", fmt.Errorf("%s: %w", path, err)
	}
	return raw, typ, contract.DetectFileFormat(raw), nil
}

func toPluginType(t client.InterfaceType) contract.InterfaceType {
	switch t {
	case client.OPENAPI:
		return contract.InterfaceTypeOpenAPI
	case client.JSONSCHEMA:
		return contract.InterfaceTypeJSONSchema
	default:
		return contract.InterfaceType(t)
	}
}
