// Package project holds the state of an ifc7 managed project and defines command line operations that
// can be performed on it.
package project

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"gopkg.in/yaml.v3"

	"github.com/ifc7/ifc/internal"
	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/fileio"
	"github.com/ifc7/ifc/internal/tui"
)

var (
	ErrProjectExists        = fmt.Errorf("project already exists")
	ErrInvalidRef           = fmt.Errorf("invalid reference")
	ErrInvalidSpecification = fmt.Errorf("invalid interface specification")

	promptNewInterfaceCommit = tui.PromptNewInterfaceCommit
	promptNewRevisionCommit  = tui.PromptNewRevisionCommit
	promptInterfaceOwner     = tui.PromptInterfaceOwner
	promptCreateMissingFile  = tui.PromptCreateMissingFile
)

// Project holds the state of an ifc7 managed project
type Project struct {
	client   client.ClientWithResponsesIfc
	config   Config
	manifest Manifest
}

// Option is a function that configures a Project
type Option func(*Project)

// New instantiates a new Project struct
func New(opts ...Option) (*Project, error) {
	config := NewConfig()
	manifest := NewManifest()
	proj := &Project{config: *config, manifest: *manifest}
	for _, opt := range opts {
		opt(proj)
	}
	if proj.client == nil {
		apiClient, err := client.NewAPIClient(context.Background())
		if err != nil {
			return nil, err
		}
		proj.client = apiClient
	}
	return proj, nil
}

// Load loads a project from local files
func Load() (*Project, error) {
	config, err := ReadConfig(internal.IfcConfigFile)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}
	manifest, err := ReadManifest(internal.IfcManifestPath)
	if err != nil {
		return nil, fmt.Errorf("error reading manifest file: %w", err)
	}
	project, err := New()
	if err != nil {
		return nil, fmt.Errorf("error creating project: %w", err)
	}
	project.config = *config
	project.manifest = *manifest
	return project, nil
}

// Initialize creates the necessary folders and files for a project if they do not exist in the current folder
func (p *Project) Initialize() error {
	if !fileio.DirExists(internal.IfcFolder) {
		err := os.Mkdir(internal.IfcFolder, 0755)
		if err != nil {
			return err
		}
	}
	err := p.Write()
	if err != nil {
		return fmt.Errorf("error writing project files: %w", err)
	}
	return nil
}

// Write writes project files to disk
func (p *Project) Write() error {
	p.rewriteConfigRefsToCanonical()
	err := p.manifest.Write(internal.IfcManifestPath)
	if err != nil {
		return fmt.Errorf("error writing manifest file: %w", err)
	}
	err = p.config.Write(internal.IfcConfigFile)
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	// TODO: write working copies
	return nil
}

// rewriteConfigRefsToCanonical upgrades interface_… ids in ifc.yaml to host-qualified
// canonical paths when the manifest has CanonicalUrl for that interface.
func (p *Project) rewriteConfigRefsToCanonical() {
	for i, u := range p.config.Use {
		if ref, ok := p.canonicalConfigRefFor(u.Ref); ok {
			p.config.Use[i].Ref = ref
		}
	}
	for i, o := range p.config.Own {
		if o.Ref == "" {
			continue
		}
		if ref, ok := p.canonicalConfigRefFor(o.Ref); ok {
			p.config.Own[i].Ref = ref
		}
	}
}

// canonicalConfigRefFor returns the preferred ifc.yaml ref form for ref when known.
func (p *Project) canonicalConfigRefFor(ref string) (string, bool) {
	if ref == "" {
		return "", false
	}
	if parsed, ok := parsePathRef(ref); ok {
		return parsed.canonical(), true
	}
	if !strings.HasPrefix(ref, "interface_") {
		return "", false
	}
	_, ifc, ok := p.manifest.findInterface(ref)
	if !ok || ifc.CanonicalUrl == "" {
		return "", false
	}
	cfgRef, err := configRefFromCanonicalURL(ifc.CanonicalUrl)
	if err != nil {
		return "", false
	}
	return cfgRef, true
}

// configRefFromCanonicalURL turns an API canonicalUrl into the form stored in
// ifc.yaml (e.g. ifc7.dev/i/acme/petstore).
func configRefFromCanonicalURL(canonicalURL string) (string, error) {
	s := strings.TrimSpace(canonicalURL)
	if s == "" {
		return "", fmt.Errorf("empty canonicalUrl")
	}
	if parsed, ok := parsePathRef(s); ok {
		return parsed.canonical(), nil
	}
	path := s
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	ref := internal.DefaultAPIHost + path
	parsed, ok := parsePathRef(ref)
	if !ok {
		return "", fmt.Errorf("invalid canonicalUrl %q", canonicalURL)
	}
	return parsed.canonical(), nil
}

// UseParams holds parameters that can be passed to the Use method
type UseParams struct {
	Ref string
}

// Use adds a remote interface reference to the project's "use" list
func (p *Project) Use(ctx context.Context, params UseParams) error {
	resolved, err := p.resolveRef(ctx, params.Ref)
	if err != nil {
		return fmt.Errorf("error resolving ref %s: %w", params.Ref, err)
	}
	err = p.config.addUsedInterface(resolved)
	if err != nil {
		return err
	}
	return nil
}

// AddParams holds parameters that can be passed to the Add method
type AddParams struct {
	Name string
	Path string
}

// Add adds a local interface to the project's "own" list in (ifc.yaml).
// If the file at Path does not exist, the user is asked to create it
// (including any missing parent directories).
func (p *Project) Add(ctx context.Context, params AddParams) error {
	// Reject name/path conflicts before creating a new file on disk.
	if params.Name != "" && p.config.nameExists(params.Name) {
		return ErrNameExists
	}
	if params.Path != "" && p.config.pathExists(params.Path) {
		return ErrPathExists
	}
	if err := p.ensureOwnedFile(ctx, params); err != nil {
		return err
	}
	return p.config.addOwnedInterface(params.Name, params.Path, "")
}

// ensureOwnedFile creates Path (and parent directories) after confirmation
// when the file does not already exist.
func (p *Project) ensureOwnedFile(ctx context.Context, params AddParams) error {
	if params.Path == "" {
		return nil
	}
	info, err := os.Stat(params.Path)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("path %q is a directory", params.Path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("error checking path %q: %w", params.Path, err)
	}
	if err := promptCreateMissingFile(ctx, params.Path, params.Name); err != nil {
		return fmt.Errorf("error creating missing file: %w", err)
	}
	if err := fileio.WriteFile([]byte{}, params.Path); err != nil {
		return fmt.Errorf("error creating file %q: %w", params.Path, err)
	}
	return nil
}

// FetchParams holds parameters that can be passed to the Fetch method
type FetchParams struct {
	Ref string
}

// fetchConcurrency is the maximum number of in-flight hub requests for one Fetch.
const fetchConcurrency = 8

// Fetch fetches remote copies of interfaces tracked by the project
func (p *Project) Fetch(ctx context.Context, params FetchParams) error {
	sem := semaphore.NewWeighted(fetchConcurrency)
	var mu sync.Mutex
	if params.Ref != "" {
		return p.fetch(ctx, params.Ref, &mu, sem)
	}
	g, ctx := errgroup.WithContext(ctx)
	for _, ref := range trackedFetchRefs(p.config) {
		g.Go(func() error {
			if err := p.fetch(ctx, ref, &mu, sem); err != nil {
				return fmt.Errorf("error fetching ref %s: %w", ref, err)
			}
			return nil
		})
	}
	return g.Wait()
}

// trackedFetchRefs returns use refs, then owned refs, skipping blanks and duplicates.
func trackedFetchRefs(cfg Config) []string {
	seen := make(map[string]struct{})
	var refs []string
	add := func(ref string) {
		if ref == "" {
			return
		}
		if _, ok := seen[ref]; ok {
			return
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	for _, u := range cfg.Use {
		add(u.Ref)
	}
	for _, u := range cfg.Own {
		add(u.Ref)
	}
	return refs
}

// CommitParams holds parameters that can be passed to the Commit method
type CommitParams struct {
	Ref string
}

// Commit adds local changes to owned interfaces to the manifest
func (p *Project) Commit(ctx context.Context, params CommitParams) error {
	if params.Ref != "" {
		for _, o := range p.config.Own {
			if o.Ref == params.Ref {
				return p.commit(ctx, o)
			}
		}
		return ErrInvalidRef
	}
	for _, o := range p.config.Own {
		err := p.commit(ctx, o)
		if err != nil {
			return fmt.Errorf("error committing ref %s: %w", o.Ref, err)
		}
	}
	return nil
}

// PushParams holds parameters that can be passed to the Push method
type PushParams struct {
	Name string
}

// Push pushes local changes to the remote server. It returns user-facing status messages.
func (p *Project) Push(ctx context.Context, params PushParams) ([]string, error) {
	targets, err := p.ownedToPush(params)
	if err != nil {
		return nil, err
	}
	if targets == nil {
		return []string{"No owned interfaces to push."}, nil
	}
	session := &pushSession{}
	for _, o := range targets {
		if o.Ref == "" {
			if err := session.loadOwnerChoices(ctx, p); err != nil {
				return nil, err
			}
			break
		}
	}
	var messages []string
	for _, o := range targets {
		msgs, err := p.push(ctx, o, session)
		if err != nil {
			if params.Name != "" {
				return messages, err
			}
			return messages, fmt.Errorf("error pushing interface %q: %w", o.Name, err)
		}
		messages = append(messages, msgs...)
	}
	return messages, nil
}

// ownedToPush returns the owned interfaces selected by params.
// A nil slice means the project has nothing to push.
func (p *Project) ownedToPush(params PushParams) ([]Owned, error) {
	if params.Name != "" {
		for _, o := range p.config.Own {
			if o.Ref == params.Name {
				return []Owned{o}, nil
			}
		}
		return nil, ErrInvalidRef
	}
	if len(p.config.Own) == 0 {
		return nil, nil
	}
	return p.config.Own, nil
}

// -----
// -----

// resolveRef verifies the existence of an interface reference and returns a canonical version
func (p *Project) resolveRef(ctx context.Context, ref string) (string, error) {
	if parsed, ok := parsePathRef(ref); ok {
		_, status, err := p.client.GetInterfaceByCanonicalPathWithResponse(ctx, parsed.host, parsed.ownerPath(), parsed.name)
		if err != nil {
			return "", fmt.Errorf("%w: error fetching interface %s: %w", ErrInvalidRef, ref, err)
		}
		if status != http.StatusOK {
			return "", fmt.Errorf("%w: error fetching interface %s: HTTP %d", ErrInvalidRef, ref, status)
		}
		return parsed.canonical(), nil
	}
	if strings.HasPrefix(ref, "interface_") {
		i, err := p.client.GetInterfaceWithResponse(ctx, ref)
		if err != nil {
			return "", fmt.Errorf("%w: error fetching interface %s: %w", ErrInvalidRef, ref, err)
		}
		if i.StatusCode() != http.StatusOK {
			return "", fmt.Errorf("%w: error fetching interface %s: HTTP %d", ErrInvalidRef, ref, i.StatusCode())
		}
		if i.JSON200 == nil {
			return "", fmt.Errorf("%w: error fetching interface %s: unexpected response body", ErrInvalidRef, ref)
		}
		cfgRef, err := configRefFromCanonicalURL(i.JSON200.CanonicalUrl)
		if err != nil {
			return "", fmt.Errorf("%w: interface %s missing canonicalUrl: %w", ErrInvalidRef, ref, err)
		}
		return cfgRef, nil
	}
	return "", ErrInvalidRef
}

// resolveRefToID resolves a reference returning the associated interface ID
func (p *Project) resolveRefToID(ctx context.Context, ref string) (client.InterfaceId, error) {
	if parsed, ok := parsePathRef(ref); ok {
		meta, status, err := p.client.GetInterfaceByCanonicalPathWithResponse(ctx, parsed.host, parsed.ownerPath(), parsed.name)
		if err != nil {
			return "", fmt.Errorf("%w: error fetching interface %s: %w", ErrInvalidRef, ref, err)
		}
		if status != http.StatusOK {
			return "", fmt.Errorf("%w: error fetching interface %s: HTTP %d", ErrInvalidRef, ref, status)
		}
		if meta == nil || meta.Id == "" {
			return "", fmt.Errorf("%w: error fetching interface %s: response body is nil", ErrInvalidRef, ref)
		}
		return meta.Id, nil
	}
	if strings.HasPrefix(ref, "interface_") {
		response, err := p.client.GetInterfaceWithResponse(ctx, ref)
		if err != nil {
			return "", fmt.Errorf("%w: error fetching interface %s: %w", ErrInvalidRef, ref, err)
		}
		if response.StatusCode() != http.StatusOK {
			return "", fmt.Errorf("%w: error fetching interface %s: HTTP %d", ErrInvalidRef, ref, response.StatusCode())
		}
		return response.JSON200.Id, nil
	}
	return ref, ErrInvalidRef
}

// fetch fetches a single interface from the remote server and adds or updates it in the manifest.
// mu guards manifest writes. sem limits in-flight hub requests shared with the rest of this Fetch.
func (p *Project) fetch(ctx context.Context, ref string, mu *sync.Mutex, sem *semaphore.Weighted) error {
	ifc, err := p.loadInterface(ctx, ref, sem)
	if err != nil {
		return err
	}

	mu.Lock()
	err = p.manifest.upsertInterface(ifc)
	skipRevisions := err == nil && manifestRevisionsCurrent(&p.manifest, ifc)
	mu.Unlock()
	if err != nil {
		return fmt.Errorf("error adding interface to manifest: %w", err)
	}

	g, gctx := errgroup.WithContext(ctx)
	var revisions []client.InterfaceRevision
	if !skipRevisions {
		g.Go(func() error {
			revs, err := p.collectRevisions(gctx, ifc, mu, sem)
			if err != nil {
				return err
			}
			revisions = revs
			return nil
		})
	}
	var releases []client.InterfaceRelease
	g.Go(func() error {
		rels, err := p.loadReleases(gctx, ifc.Id, sem)
		if err != nil {
			return err
		}
		releases = rels
		return nil
	})
	if err := g.Wait(); err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	key := manifestInterfaceKey(ifc)
	for _, rev := range revisions {
		if err := p.manifest.upsertRevision(key, rev); err != nil {
			return fmt.Errorf("error adding revision to manifest: %w", err)
		}
	}
	for _, rel := range releases {
		if err := p.manifest.upsertRelease(key, rel); err != nil {
			return fmt.Errorf("error adding release to manifest: %w", err)
		}
	}
	return nil
}

// loadInterface resolves ref and returns the full interface. An interface id is used directly.
func (p *Project) loadInterface(ctx context.Context, ref string, sem *semaphore.Weighted) (client.Interface, error) {
	var id client.InterfaceId
	var err error
	if strings.HasPrefix(ref, "interface_") {
		id = ref
	} else {
		id, err = withPermit(ctx, sem, func() (client.InterfaceId, error) {
			return p.resolveRefToID(ctx, ref)
		})
		if err != nil {
			return client.Interface{}, fmt.Errorf("error resolving ref %s: %w", ref, err)
		}
	}
	return withPermit(ctx, sem, func() (client.Interface, error) {
		resp, err := p.client.GetInterfaceWithResponse(ctx, id)
		if err != nil {
			return client.Interface{}, fmt.Errorf("error fetching interface %s: %w", id, err)
		}
		if resp.StatusCode() != http.StatusOK {
			return client.Interface{}, fmt.Errorf("error fetching interface %s: HTTP %d", id, resp.StatusCode())
		}
		if resp.JSON200 == nil {
			return client.Interface{}, fmt.Errorf("error fetching interface %s: unexpected response body", id)
		}
		return *resp.JSON200, nil
	})
}

// collectRevisions lists revision descriptors and downloads specifications the manifest does not already have.
func (p *Project) collectRevisions(ctx context.Context, ifc client.Interface, mu *sync.Mutex, sem *semaphore.Weighted) ([]client.InterfaceRevision, error) {
	resp, err := withPermit(ctx, sem, func() (*client.ListInterfaceRevisionsResponse, error) {
		return p.client.ListInterfaceRevisionsWithResponse(ctx, ifc.Id)
	})
	if err != nil {
		return nil, fmt.Errorf("error fetching revisions: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error fetching revisions: HTTP %d", resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("error fetching revisions: unexpected response body")
	}

	mu.Lock()
	var ready []client.InterfaceRevision
	var missing []client.InterfaceRevisionDescriptor
	key := manifestInterfaceKey(ifc)
	for _, desc := range *resp.JSON200 {
		if rev, ok := cachedRevision(&p.manifest, key, desc, ifc.LatestRevision); ok {
			ready = append(ready, rev)
			continue
		}
		missing = append(missing, desc)
	}
	mu.Unlock()

	if len(missing) == 0 {
		return ready, nil
	}

	fetched := make([]client.InterfaceRevision, len(missing))
	g, ctx := errgroup.WithContext(ctx)
	for i, desc := range missing {
		g.Go(func() error {
			rev, err := p.loadRevision(ctx, ifc.Id, desc.Id, sem)
			if err != nil {
				return err
			}
			fetched[i] = rev
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return append(ready, fetched...), nil
}

func (p *Project) loadRevision(ctx context.Context, interfaceID client.InterfaceId, revisionID client.InterfaceRevisionId, sem *semaphore.Weighted) (client.InterfaceRevision, error) {
	return withPermit(ctx, sem, func() (client.InterfaceRevision, error) {
		getResp, err := p.client.GetInterfaceRevisionWithResponse(ctx, interfaceID, revisionID)
		if err != nil {
			return client.InterfaceRevision{}, fmt.Errorf("error fetching revision: %w", err)
		}
		if getResp.StatusCode() != http.StatusOK {
			return client.InterfaceRevision{}, fmt.Errorf("error fetching revision: HTTP %d", getResp.StatusCode())
		}
		if getResp.JSON200 == nil {
			return client.InterfaceRevision{}, fmt.Errorf("error fetching revisions: unexpected response body")
		}
		return *getResp.JSON200, nil
	})
}

func (p *Project) loadReleases(ctx context.Context, interfaceID client.InterfaceId, sem *semaphore.Weighted) ([]client.InterfaceRelease, error) {
	resp, err := withPermit(ctx, sem, func() (*client.ListInterfaceReleasesResponse, error) {
		return p.client.ListInterfaceReleasesWithResponse(ctx, interfaceID)
	})
	if err != nil {
		return nil, fmt.Errorf("error fetching releases: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error fetching releases: HTTP %d", resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("error fetching releases: unexpected response body")
	}
	return *resp.JSON200, nil
}

// manifestRevisionsCurrent reports whether the manifest already stores the remote latest revision.
// Revisions are immutable, so an unchanged latest id means the stored history is complete.
func manifestRevisionsCurrent(m *Manifest, remote client.Interface) bool {
	if remote.LatestRevision == nil || remote.LatestRevision.Id == "" {
		return false
	}
	_, local, ok := m.findInterface(manifestInterfaceKey(remote))
	if !ok || local.LatestRevision == nil || local.LatestRevision.Id != remote.LatestRevision.Id {
		return false
	}
	if remote.LatestRevision.Checksum != "" && local.LatestRevision.Checksum != remote.LatestRevision.Checksum {
		return false
	}
	rev, ok := local.Revisions[remote.LatestRevision.Id]
	if !ok || rev == nil || rev.Specification == "" {
		return false
	}
	if remote.LatestRevision.Checksum != "" && rev.Checksum != remote.LatestRevision.Checksum {
		return false
	}
	return true
}

// cachedRevision returns a full revision without a hub get when the manifest or the
// interface payload already has the specification for desc.
func cachedRevision(m *Manifest, ifcKey string, desc client.InterfaceRevisionDescriptor, latest *client.InterfaceRevision) (client.InterfaceRevision, bool) {
	_, ifc, found := m.findInterface(ifcKey)
	if found {
		if existing, ok := ifc.Revisions[desc.Id]; ok && existing != nil {
			if existing.Checksum != desc.Checksum {
				return client.InterfaceRevision{}, false
			}
			if existing.Specification != "" {
				return revisionFromDescriptor(desc, existing.Specification), true
			}
		}
	}
	if latest != nil && latest.Id == desc.Id && latest.Specification != "" && latest.Checksum == desc.Checksum {
		return *latest, true
	}
	if found {
		for _, existing := range ifc.Revisions {
			if existing == nil || existing.Specification == "" || existing.Checksum != desc.Checksum {
				continue
			}
			return revisionFromDescriptor(desc, existing.Specification), true
		}
	}
	return client.InterfaceRevision{}, false
}

func revisionFromDescriptor(desc client.InterfaceRevisionDescriptor, spec client.InterfaceSpecification) client.InterfaceRevision {
	return client.InterfaceRevision{
		Checksum:      desc.Checksum,
		CreatedAt:     desc.CreatedAt,
		CreatedBy:     desc.CreatedBy,
		FileFormat:    desc.FileFormat,
		Id:            desc.Id,
		Notes:         desc.Notes,
		Specification: spec,
	}
}

func withPermit[T any](ctx context.Context, sem *semaphore.Weighted, fn func() (T, error)) (T, error) {
	if err := sem.Acquire(ctx, 1); err != nil {
		var zero T
		return zero, err
	}
	defer sem.Release(1)
	return fn()
}

// commit adds local changes to the manifest
func (p *Project) commit(ctx context.Context, own Owned) error {
	key, err := p.manifestKeyForOwned(ctx, own)
	if err != nil {
		return err
	}
	b, err := fileio.ReadFile(own.Path)
	if err != nil {
		return fmt.Errorf("error reading local copy %s interface: %w", own.Path, err)
	}
	sha := sha256Checksum(b)
	encoded := base64Encode(b)
	_, manifestIfc, ok := p.manifest.findInterface(key)
	if !ok {
		specType, err := DetectSpecificationType(b)
		if err != nil {
			return fmt.Errorf("error detecting interface type for %s: %w", own.Path, err)
		}
		userID, err := p.client.CurrentUserID(ctx)
		if err != nil {
			return fmt.Errorf("not authenticated: please run 'ifc login' first")
		}
		newIfc, err := promptNewInterfaceCommit(ctx, own.Name, specType)
		if err != nil {
			return fmt.Errorf("error prompting for new interface: %w", err)
		}
		revision := client.InterfaceRevision{
			Checksum:      sha,
			CreatedAt:     time.Now(),
			CreatedBy:     userID,
			Specification: encoded,
			Notes:         &newIfc.RevisionNotes,
		}
		entry := &ManifestInterface{
			Interface: client.Interface{
				Description:    &newIfc.Description,
				LatestRevision: &revision,
				Name:           newIfc.Name,
				Type:           specType,
				Id:             key,
			},
			Revisions: map[string]*client.InterfaceRevision{
				// TODO: figure out how to handle IDs before created on server
				sha: &revision,
			},
			Releases: map[string]*client.InterfaceRelease{},
		}
		if path, ok := canonicalURLPath(own.Ref); ok {
			entry.CanonicalUrl = path
			key = path
			entry.Id = ""
		} else if path, ok := canonicalURLPath(key); ok {
			entry.CanonicalUrl = path
			key = path
		}
		p.manifest.Interfaces[key] = entry
	} else {
		if path, ok := canonicalURLPath(own.Ref); ok && manifestIfc.CanonicalUrl == "" {
			manifestIfc.CanonicalUrl = path
			p.manifest.ensureInterfaceKey(manifestIfc)
		}
		if manifestIfc.LatestRevision == nil || manifestIfc.LatestRevision.Checksum != sha {
			userID, err := p.client.CurrentUserID(ctx)
			if err != nil {
				return fmt.Errorf("not authenticated: please run 'ifc login' first")
			}
			newRev, err := promptNewRevisionCommit(ctx, own.Name)
			if err != nil {
				return fmt.Errorf("error prompting for new revision: %w", err)
			}
			revision := client.InterfaceRevision{
				Checksum:      sha,
				CreatedAt:     time.Now(),
				CreatedBy:     userID,
				Specification: encoded,
				Id:            sha,
				Notes:         &newRev.Notes,
			}
			manifestIfc.Revisions[sha] = &revision
			manifestIfc.LatestRevision = &revision
		}
	}
	return nil
}

// manifestKeyForOwned returns the manifest map key for a locally owned interface.
func (p *Project) manifestKeyForOwned(ctx context.Context, own Owned) (string, error) {
	if key, ok := p.localManifestKeyForOwned(own); ok {
		return key, nil
	}
	if own.Ref == "" {
		return own.Name, nil
	}
	if path, ok := canonicalURLPath(own.Ref); ok {
		id, err := p.resolveRefToID(ctx, own.Ref)
		if err != nil {
			return "", err
		}
		// Legacy manifests may still be keyed by interface ID.
		if mapKey, _, ok := p.manifest.findInterface(id); ok {
			return mapKey, nil
		}
		return path, nil
	}
	id, err := p.resolveRefToID(ctx, own.Ref)
	if err != nil {
		return "", err
	}
	if mapKey, _, ok := p.manifest.findInterface(id); ok {
		return mapKey, nil
	}
	return string(id), nil
}

// pushSession caches the current user and organization list for one Push.
// Owner choices load once when any targeted interface has no ref. The user
// loads once, and only when a revision is created, unless choices were already loaded.
type pushSession struct {
	user          *client.User
	orgs          []client.Organization
	choicesLoaded bool
}

func (s *pushSession) ensureUser(ctx context.Context, p *Project) error {
	if s.user != nil {
		return nil
	}
	userResp, err := p.client.GetCurrentUserWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("error fetching current user: %w", err)
	}
	if userResp.StatusCode() != http.StatusOK {
		return fmt.Errorf("error fetching current user: HTTP %d", userResp.StatusCode())
	}
	if userResp.JSON200 == nil {
		return fmt.Errorf("error fetching current user: unexpected response body")
	}
	s.user = userResp.JSON200
	return nil
}

func (s *pushSession) loadOwnerChoices(ctx context.Context, p *Project) error {
	if err := s.ensureUser(ctx, p); err != nil {
		return err
	}
	if s.choicesLoaded {
		return nil
	}
	orgsResp, err := p.client.ListOrganizationsWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("error listing organizations: %w", err)
	}
	if orgsResp.StatusCode() != http.StatusOK {
		return fmt.Errorf("error listing organizations: HTTP %d", orgsResp.StatusCode())
	}
	if orgsResp.JSON200 == nil {
		return fmt.Errorf("error listing organizations: unexpected response body")
	}
	s.orgs = *orgsResp.JSON200
	s.choicesLoaded = true
	return nil
}

func (s *pushSession) userID(ctx context.Context, p *Project) (client.UserId, error) {
	if err := s.ensureUser(ctx, p); err != nil {
		return "", err
	}
	return s.user.Id, nil
}

func (s *pushSession) ownerOptions() []tui.InterfaceOwnerOption {
	user := s.user
	userLabel := user.Name
	if userLabel == "" {
		userLabel = user.Slug
	}
	if userLabel == "" {
		userLabel = "You (current user)"
	} else {
		userLabel = fmt.Sprintf("You (%s)", userLabel)
	}
	options := []tui.InterfaceOwnerOption{{
		ID:    string(user.Id),
		Label: userLabel,
		Kind:  "user",
	}}
	for _, org := range s.orgs {
		label := org.Name
		if label == "" {
			label = org.Slug
		}
		if label == "" {
			label = string(org.Id)
		}
		options = append(options, tui.InterfaceOwnerOption{
			ID:    string(org.Id),
			Label: label,
			Kind:  "org",
		})
	}
	return options
}

func interfaceIdentityComplete(ifc client.Interface) bool {
	return ifc.Owner != "" && ifc.Slug != "" && ifc.CanonicalUrl != ""
}

// push saves an interface revision or creates a new interface on the server.
func (p *Project) push(ctx context.Context, own Owned, session *pushSession) ([]string, error) {
	// for each owned interface
	// 1. check if there is an entry checked into the manifest
	// 2. if so, is there a ref in the owned entry?
	//    if not, create new interface on server according to manifest and add ref to config
	//    if so, query the interface from the server and compare it to the manifest
	// if changes in interface, revision, or release, push these changes
	manifestKey, err := p.manifestKeyForOwned(ctx, own)
	if err != nil {
		return nil, err
	}
	_, manifestEntry, ok := p.manifest.findInterface(manifestKey)
	if !ok {
		return []string{fmt.Sprintf("Skipping %q: no manifest entry for interface %s. Run 'ifc commit' first.", own.Name, manifestKey)}, nil
	}

	var messages []string
	if own.Ref == "" {
		// handle interfaces not yet saved on the server
		if err := session.loadOwnerChoices(ctx, p); err != nil {
			return nil, err
		}
		userID := session.user.Id
		selectedOwner, err := promptInterfaceOwner(ctx, own.Name, session.ownerOptions())
		if err != nil {
			return nil, fmt.Errorf("error prompting for interface owner: %w", err)
		}

		response, err := p.client.CreateInterfaceWithResponse(ctx, client.CreateInterfaceRequest{
			Description: manifestEntry.Description,
			Name:        manifestEntry.Name,
			Type:        manifestEntry.Type,
			Owner:       client.InterfaceOwner(selectedOwner.ID),
			IsPublic:    true, // TODO: be able to set this
		})
		if err != nil {
			return nil, fmt.Errorf("error creating interface %s: %w", own.Name, err)
		}
		if response.StatusCode() != http.StatusCreated {
			return nil, fmt.Errorf("error creating interface %s: HTTP %d", own.Name, response.StatusCode())
		}
		if response.JSON201 == nil {
			return nil, fmt.Errorf("error creating interface %s: unexpected response body", own.Name)
		}
		interfaceId := response.JSON201.Id
		canonicalURL := response.JSON201.CanonicalUrl
		cfgRef, err := configRefFromCanonicalURL(canonicalURL)
		if err != nil {
			return messages, fmt.Errorf("created interface %s missing canonicalUrl: %w", interfaceId, err)
		}
		messages = append(messages, fmt.Sprintf("Created interface %q (%s).", own.Name, cfgRef))
		// Replace the temporary manifest key (the interface name) with the
		// server-assigned canonicalUrl and record the interface ID.
		if err := p.manifest.reassignInterfaceKey(manifestKey, interfaceId, canonicalURL); err != nil {
			return messages, fmt.Errorf("error updating interface key in manifest: %w", err)
		}
		if created := p.manifest.Interfaces[canonicalURL]; created != nil {
			created.applyServerIdentity(response.JSON201.Owner, response.JSON201.Slug, canonicalURL)
		}
		// Record the canonical ref so future commits and pushes resolve to this interface.
		if err := p.config.updateOwnedInterfaceRef(own.Name, cfgRef); err != nil {
			return messages, fmt.Errorf("error updating owned interface ref: %w", err)
		}
		revKeys := slices.Collect(maps.Keys(manifestEntry.Revisions))
		slices.SortStableFunc(revKeys, func(a, b string) int {
			return manifestEntry.Revisions[a].CreatedAt.Compare(manifestEntry.Revisions[b].CreatedAt)
		})
		for _, revKey := range revKeys {
			rev := manifestEntry.Revisions[revKey]
			result, err := p.client.CreateInterfaceRevisionWithResponse(ctx, interfaceId, client.CreateRevisionRequest{
				CreatedBy:     userID,
				Specification: rev.Specification,
				Notes:         rev.Notes,
			})
			if err != nil {
				return messages, fmt.Errorf("error creating revision %s: %w", rev.Id, err)
			}
			if result.StatusCode() != http.StatusCreated {
				return messages, fmt.Errorf("error creating revision %s: HTTP %d", rev.Id, result.StatusCode())
			}
			if result.JSON201 == nil {
				return messages, fmt.Errorf("error creating revision %s: unexpected response body", rev.Id)
			}
			if err := p.recordPushedRevision(canonicalURL, revKey, result.JSON201); err != nil {
				return messages, err
			}
			messages = append(messages, fmt.Sprintf("Pushed revision for %q.", own.Name))
		}
		for _, rel := range manifestEntry.Releases {
			result, err := p.client.CreateInterfaceReleaseWithResponse(ctx, interfaceId, client.CreateReleaseRequest{
				InterfaceRevisionId: rel.InterfaceRevisionId,
				Notes:               rel.Notes,
				SemVer:              rel.SemanticVersion,
				Summary:             rel.Summary,
			})
			if err != nil {
				return messages, fmt.Errorf("error creating release %s: %w", rel.SemanticVersion, err)
			}
			if result.StatusCode() != http.StatusCreated {
				return messages, fmt.Errorf("error creating release %s: HTTP %d", rel.SemanticVersion, result.StatusCode())
			}
			messages = append(messages, fmt.Sprintf("Pushed release %s for %q.", rel.SemanticVersion, own.Name))
		}
		return messages, nil
	}

	// handle interfaces saved on server that might need to be updated
	id := manifestEntry.Id
	if id == "" {
		id, err = p.resolveRefToID(ctx, own.Ref)
		if err != nil {
			return nil, err
		}
	}
	if !interfaceIdentityComplete(manifestEntry.Interface) {
		serverIfcResp, err := p.client.GetInterfaceWithResponse(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("error fetching interface %s: %w", id, err)
		}
		if serverIfcResp.StatusCode() != http.StatusOK {
			return nil, fmt.Errorf("error fetching interface %s: HTTP %d", id, serverIfcResp.StatusCode())
		}
		if serverIfcResp.JSON200 == nil {
			return nil, fmt.Errorf("error fetching interface %s: unexpected response body", id)
		}
		serverIfc := *serverIfcResp.JSON200
		manifestEntry.applyServerIdentity(serverIfc.Owner, serverIfc.Slug, serverIfc.CanonicalUrl)
		p.manifest.ensureInterfaceKey(manifestEntry)
		// compare manifest interface to server & update if necessary
		if manifestEntry.Interface.Name != serverIfc.Name || *manifestEntry.Interface.Description != *serverIfc.Description {
			// TODO: add API endpoint for updating an interface
		}
	}
	serverRevisionsResp, err := p.client.ListInterfaceRevisionsWithResponse(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("error fetching revisions: %w", err)
	}
	if serverRevisionsResp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error fetching revisions: HTTP %d", serverRevisionsResp.StatusCode())
	}
	if serverRevisionsResp.JSON200 == nil {
		return nil, fmt.Errorf("error fetching revisions: unexpected response body")
	}
	serverIDs := make(map[string]struct{}, len(*serverRevisionsResp.JSON200))
	var manifestMissingRevs []client.InterfaceRevisionId
	for _, desc := range *serverRevisionsResp.JSON200 {
		serverIDs[desc.Id] = struct{}{}
		if _, ok := manifestEntry.Revisions[desc.Id]; !ok {
			manifestMissingRevs = append(manifestMissingRevs, desc.Id)
		}
	}
	if len(manifestMissingRevs) > 0 {
		return messages, fmt.Errorf("revisions out of sync with server %v", manifestMissingRevs)
	}
	var serverMissingRevKeys []string
	for key, rev := range manifestEntry.Revisions {
		if rev == nil {
			continue
		}
		if _, ok := serverIDs[rev.Id]; !ok {
			serverMissingRevKeys = append(serverMissingRevKeys, key)
		}
	}
	slices.SortStableFunc(serverMissingRevKeys, func(a, b string) int {
		return manifestEntry.Revisions[a].CreatedAt.Compare(manifestEntry.Revisions[b].CreatedAt)
	})
	if len(serverMissingRevKeys) > 0 {
		userID, err := session.userID(ctx, p)
		if err != nil {
			return nil, err
		}
		for _, revKey := range serverMissingRevKeys {
			rev := manifestEntry.Revisions[revKey]
			// TODO: check if revisions existing in server need updating
			// TODO: add API endpoint for updating revisions
			result, err := p.client.CreateInterfaceRevisionWithResponse(ctx, id, client.CreateRevisionRequest{
				CreatedBy:     userID,
				Specification: rev.Specification,
				Notes:         rev.Notes,
			})
			if err != nil {
				return messages, fmt.Errorf("error creating revision %s: %w", rev.Id, err)
			}
			if result.StatusCode() != http.StatusCreated {
				return messages, fmt.Errorf("error creating revision %s: HTTP %d", rev.Id, result.StatusCode())
			}
			if result.JSON201 == nil {
				return messages, fmt.Errorf("error creating revision %s: unexpected response body", rev.Id)
			}
			if err := p.recordPushedRevision(manifestInterfaceKey(manifestEntry.Interface), revKey, result.JSON201); err != nil {
				return messages, err
			}
			messages = append(messages, fmt.Sprintf("Pushed revision for %q.", own.Name))
		}
	}
	// TODO: handle releases
	if len(messages) == 0 {
		messages = append(messages, fmt.Sprintf("Interface %q is up to date.", own.Name))
	}
	return messages, nil
}

// recordPushedRevision re-keys a locally committed revision to its server-assigned
// ID and copies server-authoritative metadata into the manifest.
func (p *Project) recordPushedRevision(ifcKey string, revKey string, serverRev *client.InterfaceRevisionDescriptor) error {
	if err := p.manifest.reassignRevisionID(ifcKey, revKey, serverRev.Id); err != nil {
		return fmt.Errorf("error updating revision ID in manifest: %w", err)
	}
	_, ifc, ok := p.manifest.findInterface(ifcKey)
	if !ok {
		return fmt.Errorf("interface %s not found in manifest", ifcKey)
	}
	if rev := ifc.Revisions[serverRev.Id]; rev != nil {
		rev.CreatedAt = serverRev.CreatedAt
	}
	return nil
}

// pathRef is a parsed owner-path interface reference.
type pathRef struct {
	host     string
	ownerSeg string // "@user" or "org-slug" as it appears in the URL
	name     string
	ver      string // optional vX.Y.Z
}

func (p pathRef) ownerPath() string { return p.ownerSeg }

func (p pathRef) canonical() string {
	path := p.host + "/i/" + p.ownerSeg + "/" + p.name
	if p.ver != "" {
		path += "/" + p.ver
	}
	return path
}

var knownAPIHosts = []string{
	internal.DefaultAPIHost,
	"staging.ifc7.dev",
	"dev.ifc7.dev",
	"localhost",
	"localhost:8080",
}

// parsePathRef parses canonical /i/{owner}/{name}[/version] refs.
func parsePathRef(ref string) (pathRef, bool) {
	ref = strings.TrimPrefix(ref, "https://")
	ref = strings.TrimPrefix(ref, "http://")
	for _, host := range knownAPIHosts {
		if parsed, ok := parsePathRefWithHost(ref, host); ok {
			return parsed, true
		}
	}
	return pathRef{}, false
}

func parsePathRefWithHost(ref, host string) (pathRef, bool) {
	if !strings.HasPrefix(ref, host+"/") && ref != host {
		return pathRef{}, false
	}
	rem := strings.TrimPrefix(ref, host)
	rem = strings.TrimPrefix(rem, "/")

	after, ok := strings.CutPrefix(rem, "i/")
	if !ok {
		return pathRef{}, false
	}
	parts := strings.Split(after, "/")
	if len(parts) != 2 && len(parts) != 3 {
		return pathRef{}, false
	}
	ownerSeg := parts[0]
	if ownerSeg == "" || ownerSeg == "@" {
		return pathRef{}, false
	}
	name := parts[1]
	if name == "" {
		return pathRef{}, false
	}
	ver := ""
	if len(parts) == 3 {
		ver = parts[2]
	}
	return pathRef{host: host, ownerSeg: ownerSeg, name: name, ver: ver}, true
}

// sha256Checksum calculates the SHA256 checksum of a byte slice
func sha256Checksum(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// base64Encode encodes a byte slice into base64 format
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// base64Decode decodes a base64-encoded string into a byte slice
func base64Decode(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

// getExt will try to determine the file extension of the file contents
func getExt(b []byte) (string, error) {
	if json.Valid(b) {
		return ".json", nil
	}
	var i any
	if yaml.Unmarshal(b, &i) == nil {
		return ".yaml", nil
	}
	return "", ErrInvalidSpecification
}
