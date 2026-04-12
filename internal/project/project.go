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
	ErrProjectExists     = fmt.Errorf("project already exists")
	ErrInvalidRef        = fmt.Errorf("invalid reference")
	ErrInvalidDefinition = fmt.Errorf("invalid interface definition")
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

// Push pushes local changes to the remote server
func (p *Project) Push(ctx context.Context, params PushParams) error {
	if params.Name != "" {
		for _, o := range p.config.Own {
			if o.Ref == params.Name {
				return p.push(ctx, o)
			}
		}
		return ErrInvalidRef
	}
	for _, o := range p.config.Own {
		err := p.push(ctx, o)
		if err != nil {
			return fmt.Errorf("error committing ref %s: %w", o.Ref, err)
		}
	}
	return nil
}

// -----
// -----

// resolveRef verifies the existence of an interface reference and returns a canonical version
func (p *Project) resolveRef(ctx context.Context, ref string) (string, error) {
	if strings.HasPrefix(ref, internal.DefaultAPIHost) {
		scope, owner, name, err := splitRef(ref)
		if err != nil {
			return "", fmt.Errorf("%w: error parsing ref %s: %w", ErrInvalidRef, ref, err)
		}
		response, err := p.client.GetInterfaceByPathWithResponse(ctx, client.GetInterfaceByPathParamsOwnerScope(scope), owner, name, &client.GetInterfaceByPathParams{})
		if err != nil {
			return "", fmt.Errorf("%w: error fetching interface %s: %w", ErrInvalidRef, ref, err)
		}
		if response.StatusCode() != http.StatusOK {
			return "", fmt.Errorf("%w: error fetching interface %s: HTTP %d", ErrInvalidRef, ref, response.StatusCode())
		}
		return ref, nil
	}
	if strings.HasPrefix(ref, "interface_") {
		i, err := p.client.GetInterfaceWithResponse(ctx, ref, &client.GetInterfaceParams{})
		if err != nil {
			return "", fmt.Errorf("%w: error fetching interface %s: %w", ErrInvalidRef, ref, err)
		}
		if i.StatusCode() != http.StatusOK {
			return "", fmt.Errorf("%w: error fetching interface %s: HTTP %d", ErrInvalidRef, ref, i.StatusCode())
		}
		return ref, nil
	}
	return "", ErrInvalidRef
}

// resolveRefToID resolves a reference returning the associated interface ID
func (p *Project) resolveRefToID(ctx context.Context, ref string) (client.InterfaceId, error) {
	if strings.HasPrefix(ref, internal.DefaultAPIHost) {
		scope, owner, name, err := splitRef(ref)
		if err != nil {
			return "", fmt.Errorf("%w: error parsing ref %s: %w", ErrInvalidRef, ref, err)
		}
		response, err := p.client.GetInterfaceByPathWithResponse(ctx, client.GetInterfaceByPathParamsOwnerScope(scope), owner, name, &client.GetInterfaceByPathParams{})
		if err != nil {
			return "", fmt.Errorf("%w: error fetching interface %s: %w", ErrInvalidRef, ref, err)
		}
		if response.StatusCode() != http.StatusOK {
			return "", fmt.Errorf("%w: error fetching interface %s: HTTP %d", ErrInvalidRef, ref, response.StatusCode())
		}
		return response.JSON200.Id, nil
	}
	if strings.HasPrefix(ref, "interface_") {
		response, err := p.client.GetInterfaceWithResponse(ctx, ref, &client.GetInterfaceParams{})
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
	resp, err := p.client.GetInterfaceWithResponse(ctx, id, &client.GetInterfaceParams{})
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
	for _, rev := range *resp.JSON200 {
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
		newIfc, err := tui.PromptNewInterfaceCommit(ctx, own.Name)
		if err != nil {
			return fmt.Errorf("error prompting for new interface: %w", err)
		}
		revision := client.InterfaceRevision{
			Checksum:   sha,
			CreatedAt:  time.Now(),
			Definition: encoded,
			Notes:      &newIfc.RevisionNotes,
		}
		p.manifest.Interfaces[id] = &ManifestInterface{
			Interface: client.Interface{
				Description:    &newIfc.Description,
				LatestRevision: revision,
				Name:           newIfc.Name,
				Type:           newIfc.Type,
				Id:             id,
			},
			Revisions: map[string]*client.InterfaceRevision{
				// TODO: figure out how to handle IDs before created on server
				sha: &revision,
			},
			Releases: map[string]*client.InterfaceRelease{},
		}
	} else {
		if manifestIfc.LatestRevision.Checksum != sha {
			newRev, err := tui.PromptNewRevisionCommit(ctx)
			if err != nil {
				return fmt.Errorf("error prompting for new revision: %w", err)
			}
			revision := client.InterfaceRevision{
				Checksum:   sha,
				CreatedAt:  time.Now(),
				Definition: encoded,
				Id:         sha,
				Notes:      &newRev.Notes,
			}
			p.manifest.Interfaces[id].Revisions[sha] = &revision
			p.manifest.Interfaces[id].LatestRevision = revision
		}
	}
	return nil
}

// push saves an interface revision or creates a new interface on the server
func (p *Project) push(ctx context.Context, own Owned) error {
	// for each owned interface
	// 1. check if there is an entry checked into the manifest
	// 2. if so, is there a ref in the owned entry?
	//    if not, create new interface on server according to manifest and add ref to config
	//    if so, query the interface from the server and compare it to the manifest
	// if changes in interface, revision, or release, push these changes
	if own.Ref == "" {
		// handle interfaces not yet saved on the server
		manifestEntry, ok := p.manifest.Interfaces[own.Name]
		if !ok {
			// TODO: do we need to handle an error here?
			return nil
		}
		response, err := p.client.CreateInterfaceWithResponse(ctx, client.CreateInterfaceRequest{
			Definition:  manifestEntry.LatestRevision.Definition,
			Description: manifestEntry.Description,
			Name:        manifestEntry.Name,
			Type:        manifestEntry.Type,
		})
		if err != nil {
			return fmt.Errorf("error creating interface %s: %w", own.Name, err)
		}
		if response.StatusCode() != http.StatusCreated {
			return fmt.Errorf("error creating interface %s: HTTP %d", own.Name, response.StatusCode())
		}
		interfaceId := response.JSON201.Id
		// sort revisions by create date earliest first
		revs := slices.Collect(maps.Values(manifestEntry.Revisions))
		slices.SortStableFunc(revs, func(i, j *client.InterfaceRevision) int {
			return i.CreatedAt.Compare(j.CreatedAt)
		})
		for i, rev := range revs {
			// skipping first release because it is created automatically with interface creation
			// TODO: revisit whether this is good behavior or not (maybe not)
			if i == 0 {
				continue
			}
			result, err := p.client.CreateInterfaceRevisionWithResponse(ctx, interfaceId, client.CreateRevisionRequest{
				Definition: rev.Definition,
				Notes:      rev.Notes,
			})
			if err != nil {
				return fmt.Errorf("error creating revision %s: %w", rev.Id, err)
			}
			if result.StatusCode() != http.StatusCreated {
				return fmt.Errorf("error creating revision %s: HTTP %d", rev.Id, result.StatusCode())
			}
		}
		for _, rel := range manifestEntry.Releases {
			result, err := p.client.CreateInterfaceReleaseWithResponse(ctx, interfaceId, client.CreateReleaseRequest{
				InterfaceRevisionId: rel.InterfaceRevisionId,
				Notes:               rel.Notes,
				SemVer:              rel.SemanticVersion,
			})
			if err != nil {
				return fmt.Errorf("error creating release %s: %w", rel.SemanticVersion, err)
			}
			if result.StatusCode() != http.StatusCreated {
				return fmt.Errorf("error creating release %s: HTTP %d", rel.SemanticVersion, result.StatusCode())
			}
		}
	} else {
		// handle interfaces saved on server that might need to be updated
		manifestEntry, ok := p.manifest.Interfaces[own.Name]
		if !ok {
			return nil
		}
		id, err := p.resolveRefToID(ctx, own.Ref)
		if err != nil {
			return fmt.Errorf("error resolving ref %s: %w", own.Ref, err)
		}
		serverIfcResp, err := p.client.GetInterfaceWithResponse(ctx, id, &client.GetInterfaceParams{})
		if err != nil {
			return fmt.Errorf("error fetching interface %s: %w", id, err)
		}
		if serverIfcResp.StatusCode() != http.StatusOK {
			return fmt.Errorf("error fetching interface %s: HTTP %d", id, serverIfcResp.StatusCode())
		}
		serverIfc := *serverIfcResp.JSON200
		serverRevisionsResp, err := p.client.ListInterfaceRevisionsWithResponse(ctx, id)
		if err != nil {
			return fmt.Errorf("error fetching revisions: %w", err)
		}
		if serverRevisionsResp.StatusCode() != http.StatusOK {
			return fmt.Errorf("error fetching revisions: HTTP %d", serverRevisionsResp.StatusCode())
		}
		serverRevisions := *serverRevisionsResp.JSON200
		serverRevisionsMap := make(map[string]*client.InterfaceRevision)
		for _, rev := range serverRevisions {
			serverRevisionsMap[rev.Id] = &rev
		}
		serverReleasesResp, err := p.client.ListInterfaceReleasesWithResponse(ctx, id)
		if err != nil {
			return fmt.Errorf("error fetching releases: %w", err)
		}
		if serverReleasesResp.StatusCode() != http.StatusOK {
			return fmt.Errorf("error fetching releases: HTTP %d", serverReleasesResp.StatusCode())
		}
		serverReleases := *serverReleasesResp.JSON200
		serverReleasesMap := make(map[string]*client.InterfaceRelease)
		for _, rel := range serverReleases {
			serverReleasesMap[rel.SemanticVersion] = &rel
		}
		// compare manifest interface to server & update if necessary
		if manifestEntry.Interface.Name != serverIfc.Name || *manifestEntry.Interface.Description != *serverIfc.Description {
			// TODO: add API endpoint for updating an interface
		}
		// find server revisions not reflected in manifest
		var manifestMissingRevs []*client.InterfaceRevision
		for _, rev := range serverRevisionsMap {
			if _, ok := manifestEntry.Revisions[rev.Id]; !ok {
				manifestMissingRevs = append(manifestMissingRevs, rev)
			}
		}
		if len(manifestMissingRevs) > 0 {
			// TODO: how to handle situation where revs haven't been fetched (sync'ed)
			return fmt.Errorf("revisions out of sync with server %v", manifestMissingRevs)
		}
		var serverMissingRevs []*client.InterfaceRevision
		for _, rev := range manifestEntry.Revisions {
			if _, ok := serverRevisionsMap[rev.Id]; !ok {
				serverMissingRevs = append(serverMissingRevs, rev)
			}
		}
		for _, rev := range serverMissingRevs {
			// TODO: check if revisions existing in server need updating
			// TODO: add API endpoint for updating revisions
			result, err := p.client.CreateInterfaceRevisionWithResponse(ctx, id, client.CreateRevisionRequest{
				Definition: rev.Definition,
				Notes:      rev.Notes,
			})
			if err != nil {
				return fmt.Errorf("error creating revision %s: %w", rev.Id, err)
			}
			if result.StatusCode() != http.StatusCreated {
				return fmt.Errorf("error creating revision %s: HTTP %d", rev.Id, result.StatusCode())
			}
			// TODO: manifest revision should be updated with new revision ID
		}
		// TODO: handle releases
	}

	return nil
}

// splitRef splits an interface reference into its scope, owner, and name components
func splitRef(ref string) (scope string, owner string, name string, err error) {
	prefix := "dev.ifc7.dev/api/v0/ifc/"
	rem, ok := strings.CutPrefix(ref, prefix)
	if !ok {
		return "", "", "", fmt.Errorf("invalid ref %s: missing prefix %s", ref, prefix)
	}
	split := strings.Split(rem, "/")
	if len(split) != 3 {
		return "", "", "", fmt.Errorf("invalid ref %s: expected 3 parts, got %d", ref, len(split))
	}
	return split[0], split[1], split[2], nil
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
	return "", ErrInvalidDefinition
}
