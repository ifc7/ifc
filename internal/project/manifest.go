package project

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/fileio"
)

var (
	ErrInvalidManifest = fmt.Errorf("invalid manifest")
)

// Manifest is the structure of the local project manifest file (manifest.json)
// This holds local copies and version history of all the interfaces tracked by the project.
type Manifest struct {
	// Interfaces maps canonicalUrl (e.g. "/i/@user/slug") to interface data.
	// Before an interface is created on the server it may be temporarily keyed by
	// its local name (or interface ID, for legacy manifests).
	Interfaces map[string]*ManifestInterface `json:"interfaces"`
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
	manifest.normalizeInterfaceKeys()
	return manifest, nil
}

// Write writes the manifest to a file
func (m *Manifest) Write(path string) error {
	m.normalizeInterfaceKeys()
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

// manifestInterfaceKey returns the preferred Interfaces map key for an interface.
func manifestInterfaceKey(ifc client.Interface) string {
	if ifc.CanonicalUrl != "" {
		return ifc.CanonicalUrl
	}
	if ifc.Id != "" {
		return ifc.Id
	}
	return ifc.Name
}

// canonicalURLPath normalizes a canonicalUrl or config path ref to the API path
// form used as a manifest map key (e.g. "/i/@user/slug").
func canonicalURLPath(refOrURL string) (string, bool) {
	s := strings.TrimSpace(refOrURL)
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "/i/") {
		return s, true
	}
	if parsed, ok := parsePathRef(s); ok {
		return "/i/" + parsed.ownerSeg + "/" + parsed.name, true
	}
	if strings.HasPrefix(s, "i/") {
		return "/" + s, true
	}
	return "", false
}

// findInterface looks up an interface by map key, Id, CanonicalUrl, or Name.
func (m *Manifest) findInterface(key string) (mapKey string, ifc *ManifestInterface, ok bool) {
	if m == nil || m.Interfaces == nil || key == "" {
		return "", nil, false
	}
	if ifc, ok := m.Interfaces[key]; ok {
		return key, ifc, true
	}
	if path, ok := canonicalURLPath(key); ok {
		if ifc, ok := m.Interfaces[path]; ok {
			return path, ifc, true
		}
	}
	for k, ifc := range m.Interfaces {
		if ifc == nil {
			continue
		}
		if ifc.Id == key || ifc.CanonicalUrl == key || ifc.Name == key {
			return k, ifc, true
		}
	}
	return "", nil, false
}

// normalizeInterfaceKeys re-keys entries that have a CanonicalUrl but are still
// stored under a legacy Id (or other temporary key).
func (m *Manifest) normalizeInterfaceKeys() {
	if m == nil || m.Interfaces == nil {
		return
	}
	type move struct {
		from string
		to   string
		ifc  *ManifestInterface
	}
	var moves []move
	for key, ifc := range m.Interfaces {
		if ifc == nil || ifc.CanonicalUrl == "" || key == ifc.CanonicalUrl {
			continue
		}
		moves = append(moves, move{from: key, to: ifc.CanonicalUrl, ifc: ifc})
	}
	for _, mv := range moves {
		if existing, ok := m.Interfaces[mv.to]; ok && existing != mv.ifc {
			continue
		}
		delete(m.Interfaces, mv.from)
		m.Interfaces[mv.to] = mv.ifc
	}
}

// ensureInterfaceKey moves ifc to its preferred map key when CanonicalUrl is set.
func (m *Manifest) ensureInterfaceKey(ifc *ManifestInterface) {
	if m == nil || ifc == nil || ifc.CanonicalUrl == "" {
		return
	}
	want := ifc.CanonicalUrl
	for key, v := range m.Interfaces {
		if v != ifc {
			continue
		}
		if key == want {
			return
		}
		if existing, ok := m.Interfaces[want]; ok && existing != ifc {
			return
		}
		delete(m.Interfaces, key)
		m.Interfaces[want] = ifc
		return
	}
}

// upsertInterface adds or updates an interface in the manifest
func (m *Manifest) upsertInterface(ifc client.Interface) error {
	if m.Interfaces == nil {
		m.Interfaces = map[string]*ManifestInterface{}
	}
	key := manifestInterfaceKey(ifc)
	_, manifestIfc, ok := m.findInterface(ifc.Id)
	if !ok && ifc.CanonicalUrl != "" {
		_, manifestIfc, ok = m.findInterface(ifc.CanonicalUrl)
	}
	if !ok {
		// create new interface entry
		m.Interfaces[key] = &ManifestInterface{
			Interface: ifc,
			Revisions: map[string]*client.InterfaceRevision{},
			Releases:  map[string]*client.InterfaceRelease{},
		}
		return nil
	}
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
	if ifc.Id != "" {
		manifestIfc.Id = ifc.Id
	}
	manifestIfc.applyServerIdentity(ifc.Owner, ifc.Slug, ifc.CanonicalUrl)
	m.ensureInterfaceKey(manifestIfc)
	if manifestIfc.LatestRevision != nil && ifc.LatestRevision != nil &&
		manifestIfc.LatestRevision.Id != ifc.LatestRevision.Id {
		slog.Warn(fmt.Sprintf("interface %s current revision changed from %s to %s", manifestIfc.Id, manifestIfc.LatestRevision.Id, ifc.LatestRevision.Id))
		manifestIfc.LatestRevision = ifc.LatestRevision
	}
	return nil
}

// applyServerIdentity copies Owner, Slug, and CanonicalUrl from a server response
// whenever those fields are returned (non-empty).
func (ifc *ManifestInterface) applyServerIdentity(owner client.InterfaceOwner, slug, canonicalURL string) {
	if ifc == nil {
		return
	}
	if owner != "" && ifc.Owner != owner {
		if ifc.Owner != "" {
			slog.Warn(fmt.Sprintf("interface %s owner changed from %s to %s", ifc.Id, ifc.Owner, owner))
		}
		ifc.Owner = owner
	}
	if slug != "" && ifc.Slug != slug {
		if ifc.Slug != "" {
			slog.Warn(fmt.Sprintf("interface %s slug changed from %s to %s", ifc.Id, ifc.Slug, slug))
		}
		ifc.Slug = slug
	}
	if canonicalURL != "" && ifc.CanonicalUrl != canonicalURL {
		if ifc.CanonicalUrl != "" {
			slog.Warn(fmt.Sprintf("interface %s canonicalUrl changed from %s to %s", ifc.Id, ifc.CanonicalUrl, canonicalURL))
		}
		ifc.CanonicalUrl = canonicalURL
	}
}

// upsertRevision adds or updates a revision to an interface in the manifest
func (m *Manifest) upsertRevision(ifcKey string, rev client.InterfaceRevision) error {
	_, ifc, ok := m.findInterface(ifcKey)
	if !ok {
		return fmt.Errorf("interface %s not found in manifest", ifcKey)
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
		if !manifestRev.CreatedAt.Equal(rev.CreatedAt) {
			slog.Warn(fmt.Sprintf("revision %s created at changed from %s to %s", manifestRev.Id, manifestRev.CreatedAt, rev.CreatedAt))
			manifestRev.CreatedAt = rev.CreatedAt
		}
	}
	return nil
}

// reassignInterfaceKey re-keys an interface from a temporary local key (e.g. its
// name) to its canonicalUrl and records the server-assigned interface ID.
func (m *Manifest) reassignInterfaceKey(oldKey string, newID client.InterfaceId, canonicalURL string) error {
	if canonicalURL == "" {
		return fmt.Errorf("canonicalUrl required to re-key interface %s", oldKey)
	}
	mapKey, ifc, ok := m.findInterface(oldKey)
	if !ok {
		return fmt.Errorf("interface %s not found in manifest", oldKey)
	}
	newKey := canonicalURL
	if mapKey != newKey {
		if existing, exists := m.Interfaces[newKey]; exists && existing != ifc {
			return fmt.Errorf("interface %s already exists in manifest", newKey)
		}
		delete(m.Interfaces, mapKey)
		m.Interfaces[newKey] = ifc
	}
	ifc.Id = newID
	ifc.CanonicalUrl = canonicalURL
	for _, rel := range ifc.Releases {
		rel.InterfaceId = newID
	}
	return nil
}

// reassignRevisionID re-keys a revision within an interface from a temporary
// local key (e.g. its checksum) to the server-assigned revision ID and updates
// every reference to it.
func (m *Manifest) reassignRevisionID(ifcKey string, oldKey string, newID client.InterfaceRevisionId) error {
	_, ifc, ok := m.findInterface(ifcKey)
	if !ok {
		return fmt.Errorf("interface %s not found in manifest", ifcKey)
	}
	if oldKey == newID {
		return nil
	}
	if _, ok := ifc.Revisions[oldKey]; !ok {
		return fmt.Errorf("revision %s not found in manifest interface %s", oldKey, ifcKey)
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
func (m *Manifest) upsertRelease(ifcKey string, rel client.InterfaceRelease) error {
	mapKey, ifc, ok := m.findInterface(ifcKey)
	if !ok {
		return fmt.Errorf("interface %s not found in manifest", ifcKey)
	}
	notes := ""
	if rel.Notes != nil {
		notes = *rel.Notes
	}
	manifestRel, ok := ifc.Releases[rel.SemanticVersion]
	if !ok {
		// create new release entry
		m.Interfaces[mapKey].Releases[rel.SemanticVersion] = &rel
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
