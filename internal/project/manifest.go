package project

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/fileio"
)

var (
	ErrInvalidManifest = fmt.Errorf("invalid manifest")
)

// Manifest is the structure of the local project manifest file (manifest.json)
// This holds local copies and version history of all the interfaces tracked by the project.
type Manifest struct {
	Interfaces map[string]*ManifestInterface `json:"interfaces"` // map of interface ID to interface data
}

// ManifestInterface holds the state of a single interface in the project manifest.
type ManifestInterface struct {
	client.Interface
	Revisions map[string]*client.InterfaceRevision `json:"revisions"` // map of revision ID to revision data
	Releases  map[string]*client.InterfaceRelease  `json:"releases"`  // map of release version to release data
}

// NewManifest creates a new empty Manifest struct
func NewManifest() *Manifest {
	return &Manifest{
		Interfaces: map[string]*ManifestInterface{},
	}
}

// ReadManifest reads the manifest from a file
func ReadManifest(path string) (*Manifest, error) {
	b, err := fileio.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: error reading manifest file: %w", ErrInvalidManifest, err)
	}
	manifest := &Manifest{}
	err = json.Unmarshal(b, manifest)
	if err != nil {
		return nil, fmt.Errorf("%w: error unmarshaling manifest: %w", ErrInvalidManifest, err)
	}
	return manifest, nil
}

// Write writes the manifest to a file
func (m *Manifest) Write(path string) error {
	b := []byte(m.String())
	return fileio.WriteFile(b, path)
}

// String converts a manifest into string format
func (m *Manifest) String() string {
	if m == nil {
		return ""
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(b)
}

// upsertInterface adds or updates an interface in the manifest
func (m *Manifest) upsertInterface(ifc client.Interface) error {
	var ok bool
	var manifestIfc *ManifestInterface
	if m.Interfaces == nil {
		m.Interfaces = map[client.InterfaceId]*ManifestInterface{}
	} else {
		manifestIfc, ok = m.Interfaces[ifc.Id]
	}
	if !ok {
		// create new interface entry
		m.Interfaces[ifc.Id] = &ManifestInterface{
			Interface: ifc,
			Revisions: map[string]*client.InterfaceRevision{},
			Releases:  map[string]*client.InterfaceRelease{},
		}
	} else {
		// update existing interface entry
		if manifestIfc.Name != ifc.Name {
			slog.Warn(fmt.Sprintf("interface %s name changed from %s to %s", manifestIfc.Id, manifestIfc.Name, ifc.Name))
			manifestIfc.Name = ifc.Name
		}
		if manifestIfc.Type != ifc.Type {
			return fmt.Errorf("interface %s type changed from %s to %s", manifestIfc.Id, manifestIfc.Type, ifc.Type)
		}
		desc := ""
		if ifc.Description != nil {
			desc = *ifc.Description
		}
		manifestDesc := ""
		if manifestIfc.Description != nil {
			manifestDesc = *manifestIfc.Description
		}
		if manifestDesc != desc {
			slog.Warn(fmt.Sprintf("interface %s description changed from %s to %s", manifestIfc.Id, manifestDesc, desc))
			manifestIfc.Description = &desc
		}
		if manifestIfc.LatestRevision.Id != ifc.LatestRevision.Id {
			slog.Warn(fmt.Sprintf("interface %s current revision changed from %s to %s", manifestIfc.Id, manifestIfc.LatestRevision.Id, ifc.LatestRevision.Id))
			manifestIfc.LatestRevision = ifc.LatestRevision
		}
	}
	return nil
}

// upsertRevision adds or updates a revision to an interface in the manifest
func (m *Manifest) upsertRevision(ifcId client.InterfaceId, rev client.InterfaceRevision) error {
	if _, ok := m.Interfaces[ifcId]; !ok {
		return fmt.Errorf("interface %s not found in manifest", ifcId)
	}
	notes := ""
	if rev.Notes != nil {
		notes = *rev.Notes
	}
	manifestRev, ok := m.Interfaces[ifcId].Revisions[rev.Id]
	if !ok {
		// create new revision entry
		m.Interfaces[ifcId].Revisions[rev.Id] = &rev
	} else {
		manifestNotes := ""
		if manifestRev.Notes != nil {
			manifestNotes = *manifestRev.Notes
		}
		if manifestNotes != notes {
			slog.Warn(fmt.Sprintf("revision %s notes changed from %s to %s", manifestRev.Id, manifestNotes, notes))
			manifestRev.Notes = &notes
		}
		if manifestRev.Checksum != rev.Checksum {
			return fmt.Errorf("revision %s checksum changed from %s to %s", manifestRev.Id, manifestRev.Checksum, rev.Checksum)
		}
		if manifestRev.Definition != rev.Definition {
			return fmt.Errorf("revision %s definition changed from %s to %s", manifestRev.Id, manifestRev.Definition, rev.Definition)
		}
		// TODO: should we verify the checksum of the revision definition here?
		if manifestRev.CreatedAt != rev.CreatedAt {
			return fmt.Errorf("revision %s created at changed from %s to %s", manifestRev.Id, manifestRev.CreatedAt, rev.CreatedAt)
		}
	}
	return nil
}

// upsertRelease adds or updates an interface release in the manifest
func (m *Manifest) upsertRelease(ifcId client.InterfaceId, rel client.InterfaceRelease) error {
	if _, ok := m.Interfaces[ifcId]; !ok {
		return fmt.Errorf("interface %s not found in manifest", ifcId)
	}
	notes := ""
	if rel.Notes != nil {
		notes = *rel.Notes
	}
	manifestRel, ok := m.Interfaces[ifcId].Releases[rel.SemanticVersion]
	if !ok {
		// create new release entry
		m.Interfaces[ifcId].Releases[rel.SemanticVersion] = &rel
	} else {
		manifestNotes := ""
		if manifestRel.Notes != nil {
			manifestNotes = *manifestRel.Notes
		}
		if manifestNotes != notes {
			slog.Warn(fmt.Sprintf("release %s notes changed from %s to %s", manifestRel.SemanticVersion, manifestNotes, notes))
			manifestRel.Notes = &notes
		}
	}
	return nil
}
