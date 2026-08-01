package project

import (
	"bytes"
	"fmt"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/ifc7/ifc/internal/pkg/fileio"
)

// DiffOwned returns a unified diff of an owned interface's working-tree file
// against the latest revision stored in the local manifest. An empty string
// means there are no differences. target may be an owned interface name or
// its manifest slug. It does not contact the remote hub.
func (p *Project) DiffOwned(target string) (string, error) {
	own, err := p.ownedByNameOrSlug(target)
	if err != nil {
		return "", err
	}
	if own.Path == "" {
		return "", fmt.Errorf("owned interface %q has empty path", target)
	}
	if !fileio.FileExists(own.Path) {
		return "", fmt.Errorf("file not found: %s", own.Path)
	}
	current, err := fileio.ReadFile(own.Path)
	if err != nil {
		return "", fmt.Errorf("error reading %s: %w", own.Path, err)
	}

	manifestBytes, err := p.manifestSpecification(own)
	if err != nil {
		return "", err
	}
	if bytes.Equal(current, manifestBytes) {
		return "", nil
	}

	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(manifestBytes)),
		B:        difflib.SplitLines(string(current)),
		FromFile: "a/" + own.Path + " (manifest)",
		ToFile:   "b/" + own.Path,
		Context:  3,
	}
	text, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return "", fmt.Errorf("error generating diff: %w", err)
	}
	return text, nil
}

// ownedByNameOrSlug finds an owned interface by ifc.yaml name or manifest slug.
// Name matches take precedence over slug matches.
func (p *Project) ownedByNameOrSlug(target string) (Owned, error) {
	for _, own := range p.config.Own {
		if own.Name == target {
			return own, nil
		}
	}
	for _, own := range p.config.Own {
		if slug := p.slugForOwned(own); slug != "" && slug == target {
			return own, nil
		}
	}
	return Owned{}, fmt.Errorf("unknown owned interface %q (name or slug)", target)
}

func (p *Project) manifestSpecification(own Owned) ([]byte, error) {
	key, ok := p.localManifestKeyForOwned(own)
	if !ok {
		return nil, fmt.Errorf("owned interface %q is not in the local manifest; nothing to diff against", own.Name)
	}
	_, manifestIfc, ok := p.manifest.findInterface(key)
	if !ok || manifestIfc == nil {
		return nil, fmt.Errorf("owned interface %q is not in the local manifest; nothing to diff against", own.Name)
	}
	if manifestIfc.LatestRevision == nil {
		return nil, fmt.Errorf("owned interface %q has no committed revision in the local manifest", own.Name)
	}
	if manifestIfc.LatestRevision.Specification == "" {
		return nil, fmt.Errorf("owned interface %q has no specification stored in the local manifest", own.Name)
	}
	b, err := base64Decode(manifestIfc.LatestRevision.Specification)
	if err != nil {
		return nil, fmt.Errorf("error decoding manifest specification for %q: %w", own.Name, err)
	}
	return b, nil
}
