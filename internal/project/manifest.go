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
		if manifestIfc.LatestRevision != nil && ifc.LatestRevision != nil &&
			manifestIfc.LatestRevision.Id != ifc.LatestRevision.Id {
			slog.Warn(fmt.Sprintf("interface %s current revision changed from %s to %s", manifestIfc.Id, manifestIfc.LatestRevision.Id, ifc.LatestRevision.Id))
			manifestIfc.LatestRevision = ifc.LatestRevision
		}
	}
	return nil
}

// upsertRevision adds or updates a revision to an interface in the manifest
func (m *Manifest) upsertRevision(ifcId client.InterfaceId, rev client.InterfaceRevision) error {
	ifc, ok := m.Interfaces[ifcId]
	if !ok {
		return fmt.Errorf("interface %s not found in manifest", ifcId)
	}
	// A locally committed revision is temporarily keyed by its checksum until it
	// receives a server-assigned revision ID. If the server returns a revision
	// with matching content, drop the placeholder and redirect any references to
	// the real revision ID so the checksum is never used as a key/ID.
	for key, existing := range ifc.Revisions {
		if key == rev.Id || existing.Checksum != rev.Checksum {
			continue
		}
		delete(ifc.Revisions, key)
		ifc.redirectRevisionRefs(key, existing.Id, rev.Checksum, rev.Id)
	}
	notes := ""
	if rev.Notes != nil {
		notes = *rev.Notes
	}
	manifestRev, ok := ifc.Revisions[rev.Id]
	if !ok {
		// create new revision entry
		ifc.Revisions[rev.Id] = &rev
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
		if manifestRev.Specification != rev.Specification {
			return fmt.Errorf("revision %s specification changed from %s to %s", manifestRev.Id, manifestRev.Specification, rev.Specification)
		}
		// TODO: should we verify the checksum of the revision definition here?
		if manifestRev.CreatedAt != rev.CreatedAt {
			return fmt.Errorf("revision %s created at changed from %s to %s", manifestRev.Id, manifestRev.CreatedAt, rev.CreatedAt)
		}
	}
	return nil
}

// reassignInterfaceID re-keys an interface from a temporary local key (e.g. its
// name) to the server-assigned interface ID and updates every reference to it.
func (m *Manifest) reassignInterfaceID(oldKey string, newID client.InterfaceId) error {
	if oldKey == newID {
		return nil
	}
	ifc, ok := m.Interfaces[oldKey]
	if !ok {
		return fmt.Errorf("interface %s not found in manifest", oldKey)
	}
	if _, exists := m.Interfaces[newID]; exists {
		return fmt.Errorf("interface %s already exists in manifest", newID)
	}
	ifc.Id = newID
	delete(m.Interfaces, oldKey)
	m.Interfaces[newID] = ifc
	for _, rel := range ifc.Releases {
		rel.InterfaceId = newID
	}
	return nil
}

// reassignRevisionID re-keys a revision within an interface from a temporary
// local key (e.g. its checksum) to the server-assigned revision ID and updates
// every reference to it.
func (m *Manifest) reassignRevisionID(ifcId client.InterfaceId, oldKey string, newID client.InterfaceRevisionId) error {
	ifc, ok := m.Interfaces[ifcId]
	if !ok {
		return fmt.Errorf("interface %s not found in manifest", ifcId)
	}
	if oldKey == newID {
		return nil
	}
	if _, ok := ifc.Revisions[oldKey]; !ok {
		return fmt.Errorf("revision %s not found in manifest interface %s", oldKey, ifcId)
	}
	ifc.rekeyRevision(oldKey, newID)
	return nil
}

// rekeyRevision moves a revision from oldKey to newID, updating the revision's
// own Id field and redirecting any references that pointed at it.
func (ifc *ManifestInterface) rekeyRevision(oldKey string, newID client.InterfaceRevisionId) {
	rev, ok := ifc.Revisions[oldKey]
	if !ok || oldKey == newID {
		return
	}
	oldID := rev.Id
	rev.Id = newID
	delete(ifc.Revisions, oldKey)
	ifc.Revisions[newID] = rev
	ifc.redirectRevisionRefs(oldKey, oldID, rev.Checksum, newID)
}

// redirectRevisionRefs points the latest-revision marker and any releases that
// referenced a revision (by its former key, former ID, or checksum) at newID.
func (ifc *ManifestInterface) redirectRevisionRefs(oldKey string, oldID string, checksum string, newID client.InterfaceRevisionId) {
	if ifc.LatestRevision != nil &&
		(ifc.LatestRevision.Id == oldKey || ifc.LatestRevision.Id == oldID || ifc.LatestRevision.Checksum == checksum) {
		ifc.LatestRevision.Id = newID
	}
	for _, rel := range ifc.Releases {
		if rel.InterfaceRevisionId == oldKey || rel.InterfaceRevisionId == oldID {
			rel.InterfaceRevisionId = newID
		}
	}
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
