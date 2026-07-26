package project

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ifc7/ifc/internal"
	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/fileio"
	"github.com/ifc7/ifc/internal/tui"
)

const maxSpecFileBytes = 10 << 20 // 10 MiB

var (
	promptScanAdd = tui.PromptScanAdd

	skipScanDirs = map[string]bool{
		".git":         true,
		".ifc":         true,
		".idea":        true,
		".vscode":      true,
		"node_modules": true,
		"vendor":       true,
		"bin":          true,
		"coverage":     true,
	}
)

// ScanCandidate is an untracked OpenAPI or JSON Schema file discovered by Scan.
type ScanCandidate struct {
	Path string
	Type client.InterfaceType
}

// Scan recursively searches root for valid OpenAPI / JSON Schema documents that
// are not already listed in ifc.yaml, presents them in a TUI, and adds selected
// files using the same logic as Add. root defaults to the current directory.
func (p *Project) Scan(ctx context.Context, root string) ([]string, error) {
	if root == "" {
		root = "."
	}
	if err := validateScanRoot(root); err != nil {
		return nil, err
	}
	candidates, err := p.FindUntrackedSpecs(root)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []string{"No untracked interface specifications found."}, nil
	}

	usedNames := map[string]bool{}
	for _, o := range p.config.Own {
		usedNames[o.Name] = true
	}
	tuiCandidates := make([]tui.ScanCandidate, len(candidates))
	for i, c := range candidates {
		name := uniqueName(usedNames, defaultInterfaceName(c.Path), i)
		usedNames[name] = true
		tuiCandidates[i] = tui.ScanCandidate{
			Path:        c.Path,
			Type:        c.Type,
			DefaultName: name,
		}
	}

	selections, err := promptScanAdd(ctx, tuiCandidates)
	if err != nil {
		return nil, err
	}
	if len(selections) == 0 {
		return []string{"No interfaces selected."}, nil
	}

	var messages []string
	for _, sel := range selections {
		err := p.Add(ctx, AddParams{
			Name: sel.Name,
			Path: sel.Path,
		})
		if err != nil {
			return messages, fmt.Errorf("error adding %q (%s): %w", sel.Name, sel.Path, err)
		}
		messages = append(messages, fmt.Sprintf("Added %q (%s) at %s", sel.Name, sel.Type, sel.Path))
	}
	return messages, nil
}

// FindUntrackedSpecs walks root for valid OpenAPI / JSON Schema files not already
// tracked as owned interfaces in ifc.yaml. Returned paths are relative to the
// current working directory so they can be passed directly to Add.
func (p *Project) FindUntrackedSpecs(root string) ([]ScanCandidate, error) {
	if root == "" {
		root = "."
	}
	if err := validateScanRoot(root); err != nil {
		return nil, err
	}
	tracked := make(map[string]bool, len(p.config.Own))
	for _, o := range p.config.Own {
		tracked[normalizeScanPath(o.Path)] = true
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error resolving working directory: %w", err)
	}

	var candidates []ScanCandidate
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipScanDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !isSpecCandidateExt(path) {
			return nil
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil
		}
		relPath, err := filepath.Rel(cwd, absPath)
		if err != nil {
			return nil
		}
		if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
			return nil
		}
		rel := normalizeScanPath(relPath)
		if rel == internal.IfcConfigFile || filepath.Base(rel) == internal.IfcConfigFile {
			return nil
		}
		if tracked[rel] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() == 0 || info.Size() > maxSpecFileBytes {
			return nil
		}
		data, err := fileio.ReadFile(path)
		if err != nil {
			return nil
		}
		specType, err := DetectSpecificationType(data)
		if err != nil {
			return nil
		}
		candidates = append(candidates, ScanCandidate{
			Path: rel,
			Type: specType,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error scanning for interfaces: %w", err)
	}
	return candidates, nil
}

func validateScanRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("scan path %q: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("scan path %q is not a directory", root)
	}
	return nil
}

func normalizeScanPath(path string) string {
	cleaned := filepath.Clean(path)
	cleaned = strings.TrimPrefix(cleaned, "."+string(filepath.Separator))
	if cleaned == "." {
		return cleaned
	}
	return filepath.ToSlash(cleaned)
}

func uniqueName(used map[string]bool, base string, index int) string {
	name := base
	if name == "" {
		name = fmt.Sprintf("interface-%d", index+1)
	}
	if !used[name] {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if !used[candidate] {
			return candidate
		}
	}
}
