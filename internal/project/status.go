package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ifc7/ifc/internal/pkg/fileio"
)

// InterfaceStatusKind describes how an owned interface compares to the local manifest.
type InterfaceStatusKind string

const (
	StatusClean    InterfaceStatusKind = "clean"
	StatusModified InterfaceStatusKind = "modified"
	StatusNew      InterfaceStatusKind = "new"
	StatusMissing  InterfaceStatusKind = "missing"
	StatusError    InterfaceStatusKind = "error"
)

// InterfaceStatus is the working-tree status of one owned interface.
type InterfaceStatus struct {
	Name   string
	Slug   string
	Path   string
	Ref    string
	Kind   InterfaceStatusKind
	Detail string
}

// Status compares each owned interface on disk against the local manifest.
// It does not contact the remote hub.
func (p *Project) Status(ctx context.Context) ([]InterfaceStatus, error) {
	_ = ctx
	if len(p.config.Own) == 0 {
		return nil, nil
	}
	results := make([]InterfaceStatus, 0, len(p.config.Own))
	for _, own := range p.config.Own {
		results = append(results, p.statusForOwned(own))
	}
	return results, nil
}

func (p *Project) statusForOwned(own Owned) InterfaceStatus {
	st := InterfaceStatus{
		Name: own.Name,
		Path: own.Path,
		Ref:  own.Ref,
		Slug: p.slugForOwned(own),
	}
	if own.Path == "" {
		st.Kind = StatusError
		st.Detail = "path is empty"
		return st
	}
	if !fileio.FileExists(own.Path) {
		st.Kind = StatusMissing
		st.Detail = "file not found"
		return st
	}
	b, err := fileio.ReadFile(own.Path)
	if err != nil {
		st.Kind = StatusError
		st.Detail = err.Error()
		return st
	}
	sha := sha256Checksum(b)
	key, ok := p.localManifestKeyForOwned(own)
	if !ok {
		st.Kind = StatusNew
		st.Detail = "not in local manifest"
		return st
	}
	_, manifestIfc, ok := p.manifest.findInterface(key)
	if !ok || manifestIfc == nil {
		st.Kind = StatusNew
		st.Detail = "not in local manifest"
		return st
	}
	if manifestIfc.LatestRevision == nil || manifestIfc.LatestRevision.Checksum != sha {
		st.Kind = StatusModified
		st.Detail = "local file differs from latest committed revision"
		return st
	}
	st.Kind = StatusClean
	return st
}

// slugForOwned returns the manifest slug for an owned interface, if known.
func (p *Project) slugForOwned(own Owned) string {
	key, ok := p.localManifestKeyForOwned(own)
	if !ok {
		return ""
	}
	_, ifc, ok := p.manifest.findInterface(key)
	if !ok || ifc == nil {
		return ""
	}
	return ifc.Slug
}

// localManifestKeyForOwned resolves the manifest map key for an owned interface
// using only local config/manifest data (no network).
func (p *Project) localManifestKeyForOwned(own Owned) (string, bool) {
	if own.Ref == "" {
		if mapKey, _, ok := p.manifest.findInterface(own.Name); ok {
			return mapKey, true
		}
		return own.Name, true
	}
	if path, ok := canonicalURLPath(own.Ref); ok {
		if mapKey, _, ok := p.manifest.findInterface(path); ok {
			return mapKey, true
		}
	}
	if strings.HasPrefix(own.Ref, "interface_") {
		if mapKey, _, ok := p.manifest.findInterface(own.Ref); ok {
			return mapKey, true
		}
		return own.Ref, true
	}
	if mapKey, _, ok := p.manifest.findInterface(own.Name); ok {
		return mapKey, true
	}
	return "", false
}

var (
	statusKindNew = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // green
	statusKindMod = lipgloss.NewStyle().Foreground(lipgloss.Color("129")) // purple
)

// FormatStatusReport renders a human-readable status report.
func FormatStatusReport(statuses []InterfaceStatus) string {
	if len(statuses) == 0 {
		return "No owned interfaces tracked in ifc.yaml.\n"
	}
	nameWidth, slugWidth := 4, 4 // "name", "slug"
	for _, st := range statuses {
		if len(st.Name) > nameWidth {
			nameWidth = len(st.Name)
		}
		slug := st.Slug
		if slug == "" {
			slug = "-"
		}
		if len(slug) > slugWidth {
			slugWidth = len(slug)
		}
	}
	var b strings.Builder
	fmt.Fprintln(&b, "Owned interface status:")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "  %-10s %-*s %-*s %s\n", "status", nameWidth, "name", slugWidth, "slug", "path")
	counts := map[InterfaceStatusKind]int{}
	for _, st := range statuses {
		counts[st.Kind]++
		kind := formatStatusKind(st.Kind)
		slug := st.Slug
		if slug == "" {
			slug = "-"
		}
		line := fmt.Sprintf("  %s %-*s %-*s %s", kind, nameWidth, st.Name, slugWidth, slug, st.Path)
		if st.Detail != "" && st.Kind != StatusClean {
			line = fmt.Sprintf("%s  (%s)", line, st.Detail)
		}
		fmt.Fprintln(&b, line)
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "%d owned interface(s): %d clean, %d modified, %d new, %d missing, %d error\n",
		len(statuses),
		counts[StatusClean],
		counts[StatusModified],
		counts[StatusNew],
		counts[StatusMissing],
		counts[StatusError],
	)
	return b.String()
}

func formatStatusKind(kind InterfaceStatusKind) string {
	padded := fmt.Sprintf("%-10s", kind)
	switch kind {
	case StatusNew:
		return statusKindNew.Render(padded)
	case StatusModified:
		return statusKindMod.Render(padded)
	default:
		return padded
	}
}
