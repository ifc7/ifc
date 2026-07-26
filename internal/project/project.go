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
	"time"

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
	ifc, ok := p.manifest.Interfaces[ref]
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
	Ref  string
}

// Add adds a local interface to the project's "own" list in (ifc.yaml)
func (p *Project) Add(ctx context.Context, params AddParams) error {
	if params.Ref != "" {
		resolved, err := p.resolveRef(ctx, params.Ref)
		if err != nil {
			return fmt.Errorf("error resolving ref %s: %w", params.Ref, err)
		}
		params.Ref = resolved
	}
	return p.config.addOwnedInterface(params.Name, params.Path, params.Ref)
}

// FetchParams holds parameters that can be passed to the Fetch method
type FetchParams struct {
	Ref string
}

// Fetch fetches remote copies of interfaces tracked by the project
func (p *Project) Fetch(ctx context.Context, params FetchParams) error {
	if params.Ref != "" {
		return p.fetch(ctx, params.Ref)
	}
	for _, u := range p.config.Use {
		err := p.fetch(ctx, u.Ref)
		if err != nil {
			return fmt.Errorf("error fetching ref %s: %w", u.Ref, err)
		}
	}
	// TODO: prompt for input if there are discrepancies found in "owned" interfaces
	for _, u := range p.config.Own {
		if u.Ref == "" {
			continue
		}
		err := p.fetch(ctx, u.Ref)
		if err != nil {
			return fmt.Errorf("error fetching ref %s: %w", u.Ref, err)
		}
	}
	return nil
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
	if params.Name != "" {
		for _, o := range p.config.Own {
			if o.Ref == params.Name {
				return p.push(ctx, o)
			}
		}
		return nil, ErrInvalidRef
	}
	if len(p.config.Own) == 0 {
		return []string{"No owned interfaces to push."}, nil
	}
	var messages []string
	for _, o := range p.config.Own {
		msgs, err := p.push(ctx, o)
		if err != nil {
			return messages, fmt.Errorf("error pushing interface %q: %w", o.Name, err)
		}
		messages = append(messages, msgs...)
	}
	return messages, nil
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

// fetch fetches a single interface from the remote server and adds or updates it in the manifest
func (p *Project) fetch(ctx context.Context, ref string) error {
	id, err := p.resolveRefToID(ctx, ref)
	if err != nil {
		return fmt.Errorf("error resolving ref %s: %w", ref, err)
	}
	resp, err := p.client.GetInterfaceWithResponse(ctx, id)
	if err != nil {
		return fmt.Errorf("error fetching interface %s: %w", id, err)
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("error fetching interface %s: HTTP %d", id, resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return fmt.Errorf("error fetching interface %s: unexpected response body", id)
	}
	ifc := *resp.JSON200
	err = p.manifest.upsertInterface(ifc)
	if err != nil {
		return fmt.Errorf("error adding interface to manifest: %w", err)
	}
	err = p.fetchRevisions(ctx, ifc)
	if err != nil {
		return err
	}
	err = p.fetchReleases(ctx, ifc)
	if err != nil {
		return err
	}
	return nil
}

// fetchRevisions retrieves and writes interface revisions to the manifest
func (p *Project) fetchRevisions(ctx context.Context, ifc client.Interface) error {
	resp, err := p.client.ListInterfaceRevisionsWithResponse(ctx, ifc.Id)
	if err != nil {
		return fmt.Errorf("error fetching revisions: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("error fetching revisions: HTTP %d", resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return fmt.Errorf("error fetching revisions: unexpected response body")
	}
	for _, revDescriptor := range *resp.JSON200 {
		getResp, err := p.client.GetInterfaceRevisionWithResponse(ctx, ifc.Id, revDescriptor.Id)
		if err != nil {
			return fmt.Errorf("error fetching revision: %w", err)
		}
		if getResp.StatusCode() != http.StatusOK {
			return fmt.Errorf("error fetching revision: HTTP %d", resp.StatusCode())
		}
		if getResp.JSON200 == nil {
			return fmt.Errorf("error fetching revisions: unexpected response body")
		}
		rev := *getResp.JSON200
		err = p.manifest.upsertRevision(ifc.Id, rev)
		if err != nil {
			return fmt.Errorf("error adding revision to manifest: %w", err)
		}
	}
	return nil
}

// fetchReleases retrieves and writes interface releases to the manifest
func (p *Project) fetchReleases(ctx context.Context, ifc client.Interface) error {
	resp, err := p.client.ListInterfaceReleasesWithResponse(ctx, ifc.Id)
	if err != nil {
		return fmt.Errorf("error fetching releases: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("error fetching releases: HTTP %d", resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return fmt.Errorf("error fetching releases: unexpected response body")
	}
	for _, rel := range *resp.JSON200 {
		err = p.manifest.upsertRelease(ifc.Id, rel)
		if err != nil {
			return fmt.Errorf("error adding release to manifest: %w", err)
		}
	}
	return nil
}

// commit adds local changes to the manifest
func (p *Project) commit(ctx context.Context, own Owned) error {
	// TODO: handle reference for locally owned files that are not managed remotely
	var id string
	var err error
	if own.Ref == "" {
		id = own.Name
	} else {
		id, err = p.resolveRefToID(ctx, own.Ref)
		if err != nil {
			return fmt.Errorf("error resolving ref %s: %w", own.Ref, err)
		}
	}
	b, err := fileio.ReadFile(own.Path)
	if err != nil {
		return fmt.Errorf("error reading local copy %s interface: %w", own.Path, err)
	}
	sha := sha256Checksum(b)
	encoded := base64Encode(b)
	manifestIfc, ok := p.manifest.Interfaces[id]
	if !ok {
		specType, err := DetectSpecificationType(b)
		if err != nil {
			return fmt.Errorf("error detecting interface type for %s: %w", own.Path, err)
		}
		newIfc, err := promptNewInterfaceCommit(ctx, own.Name, specType)
		if err != nil {
			return fmt.Errorf("error prompting for new interface: %w", err)
		}
		userID, err := p.client.CurrentUserID(ctx)
		if err != nil {
			return err
		}
		revision := client.InterfaceRevision{
			Checksum:      sha,
			CreatedAt:     time.Now(),
			CreatedBy:     userID,
			Specification: encoded,
			Notes:         &newIfc.RevisionNotes,
		}
		p.manifest.Interfaces[id] = &ManifestInterface{
			Interface: client.Interface{
				Description:    &newIfc.Description,
				LatestRevision: &revision,
				Name:           newIfc.Name,
				Type:           specType,
				Id:             id,
			},
			Revisions: map[string]*client.InterfaceRevision{
				// TODO: figure out how to handle IDs before created on server
				sha: &revision,
			},
			Releases: map[string]*client.InterfaceRelease{},
		}
	} else {
		if manifestIfc.LatestRevision == nil || manifestIfc.LatestRevision.Checksum != sha {
			newRev, err := promptNewRevisionCommit(ctx, own.Name)
			if err != nil {
				return fmt.Errorf("error prompting for new revision: %w", err)
			}
			userID, err := p.client.CurrentUserID(ctx)
			if err != nil {
				return err
			}
			revision := client.InterfaceRevision{
				Checksum:      sha,
				CreatedAt:     time.Now(),
				CreatedBy:     userID,
				Specification: encoded,
				Id:            sha,
				Notes:         &newRev.Notes,
			}
			p.manifest.Interfaces[id].Revisions[sha] = &revision
			p.manifest.Interfaces[id].LatestRevision = &revision
		}
	}
	return nil
}

// manifestIDForOwned returns the manifest map key for a locally owned interface.
func (p *Project) manifestIDForOwned(ctx context.Context, own Owned) (client.InterfaceId, error) {
	if own.Ref == "" {
		return client.InterfaceId(own.Name), nil
	}
	return p.resolveRefToID(ctx, own.Ref)
}

// push saves an interface revision or creates a new interface on the server.
func (p *Project) push(ctx context.Context, own Owned) ([]string, error) {
	// for each owned interface
	// 1. check if there is an entry checked into the manifest
	// 2. if so, is there a ref in the owned entry?
	//    if not, create new interface on server according to manifest and add ref to config
	//    if so, query the interface from the server and compare it to the manifest
	// if changes in interface, revision, or release, push these changes
	interfaceID, err := p.manifestIDForOwned(ctx, own)
	if err != nil {
		return nil, err
	}
	manifestEntry, ok := p.manifest.Interfaces[interfaceID]
	if !ok {
		return []string{fmt.Sprintf("Skipping %q: no manifest entry for interface %s. Run 'ifc commit' first.", own.Name, interfaceID)}, nil
	}

	var messages []string
	if own.Ref == "" {
		// handle interfaces not yet saved on the server
		userResp, err := p.client.GetCurrentUserWithResponse(ctx)
		if err != nil {
			return nil, fmt.Errorf("error fetching current user: %w", err)
		}
		if userResp.StatusCode() != http.StatusOK {
			return nil, fmt.Errorf("error fetching current user: HTTP %d", userResp.StatusCode())
		}
		if userResp.JSON200 == nil {
			return nil, fmt.Errorf("error fetching current user: unexpected response body")
		}
		user := userResp.JSON200
		userID := user.Id

		orgsResp, err := p.client.ListOrganizationsWithResponse(ctx)
		if err != nil {
			return nil, fmt.Errorf("error listing organizations: %w", err)
		}
		if orgsResp.StatusCode() != http.StatusOK {
			return nil, fmt.Errorf("error listing organizations: HTTP %d", orgsResp.StatusCode())
		}
		if orgsResp.JSON200 == nil {
			return nil, fmt.Errorf("error listing organizations: unexpected response body")
		}

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
			ID:    string(userID),
			Label: userLabel,
			Kind:  "user",
		}}
		for _, org := range *orgsResp.JSON200 {
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

		selectedOwner, err := promptInterfaceOwner(ctx, own.Name, options)
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
		cfgRef, err := configRefFromCanonicalURL(response.JSON201.CanonicalUrl)
		if err != nil {
			return messages, fmt.Errorf("created interface %s missing canonicalUrl: %w", interfaceId, err)
		}
		messages = append(messages, fmt.Sprintf("Created interface %q (%s).", own.Name, cfgRef))
		// Replace the temporary manifest key (the interface name) with the
		// server-assigned interface ID everywhere in the manifest.
		if err := p.manifest.reassignInterfaceID(interfaceID, interfaceId); err != nil {
			return messages, fmt.Errorf("error updating interface ID in manifest: %w", err)
		}
		if created := p.manifest.Interfaces[interfaceId]; created != nil {
			created.CanonicalUrl = response.JSON201.CanonicalUrl
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
			if err := p.recordPushedRevision(interfaceId, revKey, result.JSON201); err != nil {
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
	id := interfaceID
	userID, err := p.client.CurrentUserID(ctx)
	if err != nil {
		return nil, err
	}
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
	serverRevisions := *serverRevisionsResp.JSON200
	serverRevisionsMap := make(map[string]*client.InterfaceRevision)
	for _, revDescriptor := range serverRevisions {
		getResp, err := p.client.GetInterfaceRevisionWithResponse(ctx, id, revDescriptor.Id)
		if err != nil {
			return nil, fmt.Errorf("error fetching revision: %w", err)
		}
		if getResp.StatusCode() != http.StatusOK {
			return nil, fmt.Errorf("error fetching revision: HTTP %d", getResp.StatusCode())
		}
		if getResp.JSON200 == nil {
			return nil, fmt.Errorf("error fetching revisions: unexpected response body")
		}
		rev := *getResp.JSON200
		serverRevisionsMap[rev.Id] = &rev
	}
	serverReleasesResp, err := p.client.ListInterfaceReleasesWithResponse(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("error fetching releases: %w", err)
	}
	if serverReleasesResp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error fetching releases: HTTP %d", serverReleasesResp.StatusCode())
	}
	if serverReleasesResp.JSON200 == nil {
		return nil, fmt.Errorf("error fetching releases: unexpected response body")
	}
	serverReleases := *serverReleasesResp.JSON200
	serverReleasesMap := make(map[string]*client.InterfaceRelease)
	for _, rel := range serverReleases {
		serverReleasesMap[rel.SemanticVersion] = &rel
	}
	_ = serverReleasesMap
	// compare manifest interface to server & update if necessary
	if manifestEntry.Interface.Name != serverIfc.Name || *manifestEntry.Interface.Description != *serverIfc.Description {
		// TODO: add API endpoint for updating an interface
	}
	var manifestMissingRevs []*client.InterfaceRevision
	for _, rev := range serverRevisionsMap {
		if _, ok := manifestEntry.Revisions[rev.Id]; !ok {
			manifestMissingRevs = append(manifestMissingRevs, rev)
		}
	}
	if len(manifestMissingRevs) > 0 {
		return messages, fmt.Errorf("revisions out of sync with server %v", manifestMissingRevs)
	}
	var serverMissingRevKeys []string
	for key, rev := range manifestEntry.Revisions {
		if _, ok := serverRevisionsMap[rev.Id]; !ok {
			serverMissingRevKeys = append(serverMissingRevKeys, key)
		}
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
		if err := p.recordPushedRevision(id, revKey, result.JSON201); err != nil {
			return messages, err
		}
		messages = append(messages, fmt.Sprintf("Pushed revision for %q.", own.Name))
	}
	// TODO: handle releases
	if len(messages) == 0 {
		messages = append(messages, fmt.Sprintf("Interface %q is up to date.", own.Name))
	}
	return messages, nil
}

// recordPushedRevision re-keys a locally committed revision to its server-assigned
// ID and copies server-authoritative metadata into the manifest.
func (p *Project) recordPushedRevision(ifcID client.InterfaceId, revKey string, serverRev *client.InterfaceRevisionDescriptor) error {
	if err := p.manifest.reassignRevisionID(ifcID, revKey, serverRev.Id); err != nil {
		return fmt.Errorf("error updating revision ID in manifest: %w", err)
	}
	if rev := p.manifest.Interfaces[ifcID].Revisions[serverRev.Id]; rev != nil {
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
