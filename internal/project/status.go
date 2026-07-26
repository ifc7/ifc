package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ifc7/ifc/internal/client"
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
	id, ok := p.localManifestIDForOwned(own)
	if !ok {
		st.Kind = StatusNew
		st.Detail = "not in local manifest"
		return st
	}
	manifestIfc, ok := p.manifest.Interfaces[id]
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

// localManifestIDForOwned resolves the manifest map key for an owned interface
// using only local config/manifest data (no network).
func (p *Project) localManifestIDForOwned(own Owned) (client.InterfaceId, bool) {
	if own.Ref == "" {
		return client.InterfaceId(own.Name), true
	}
	if strings.HasPrefix(own.Ref, "interface_") {
		return client.InterfaceId(own.Ref), true
	}
	for id, iface := range p.manifest.Interfaces {
		if iface == nil || iface.CanonicalUrl == "" {
			continue
		}
		cfgRef, err := configRefFromCanonicalURL(iface.CanonicalUrl)
		if err != nil {
			continue
		}
		if refsEquivalent(cfgRef, own.Ref) {
			return client.InterfaceId(id), true
		}
	}
	if _, ok := p.manifest.Interfaces[own.Name]; ok {
		return client.InterfaceId(own.Name), true
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
	var b strings.Builder
	fmt.Fprintln(&b, "Owned interface status:")
	fmt.Fprintln(&b)
	counts := map[InterfaceStatusKind]int{}
	for _, st := range statuses {
		counts[st.Kind]++
		kind := formatStatusKind(st.Kind)
		line := fmt.Sprintf("  %s %-24s %s", kind, st.Name, st.Path)
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
