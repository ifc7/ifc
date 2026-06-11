package project

import (
	"net/http"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/testutils"
)

const testChecksum = "dcdf2064abc0000000000000000000000000000000000000000000000000abcd"

// committedManifest builds a manifest for an interface that has a single
// locally committed revision keyed by its checksum (the placeholder scheme used
// before a server-assigned revision ID exists).
func committedManifest(ifcKey string) *Manifest {
	rev := &client.InterfaceRevision{
		Checksum:      testChecksum,
		Id:            testChecksum,
		Specification: "c3BlYw==",
		CreatedAt:     time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		CreatedBy:     testUserID,
		Notes:         testutils.Ptr("committed revision"),
	}
	return &Manifest{
		Interfaces: map[string]*ManifestInterface{
			ifcKey: {
				Interface: client.Interface{
					Id:             ifcKey,
					Name:           "ifc7-rest",
					Type:           client.OPENAPI,
					Description:    testutils.Ptr("a test interface"),
					LatestRevision: &client.InterfaceRevision{Id: testChecksum, Checksum: testChecksum},
				},
				Revisions: map[string]*client.InterfaceRevision{testChecksum: rev},
				Releases: map[string]*client.InterfaceRelease{
					"1.0.0": {
						SemanticVersion:     "1.0.0",
						InterfaceId:         ifcKey,
						InterfaceRevisionId: testChecksum,
						Summary:             "first release",
					},
				},
			},
		},
	}
}

func TestManifest_reassignRevisionID(t *testing.T) {
	const newID = "revision_01kn3r6n8zf3aa3rrnzmakqncn"
	mft := committedManifest("interface_abc")

	if err := mft.reassignRevisionID("interface_abc", testChecksum, newID); err != nil {
		t.Fatalf("reassignRevisionID returned error: %v", err)
	}

	ifc := mft.Interfaces["interface_abc"]
	if _, ok := ifc.Revisions[testChecksum]; ok {
		t.Fatalf("checksum key %q still present in revisions", testChecksum)
	}
	rev, ok := ifc.Revisions[newID]
	if !ok {
		t.Fatalf("revision not re-keyed to %q", newID)
	}
	if rev.Id != newID {
		t.Fatalf("revision Id = %q, want %q", rev.Id, newID)
	}
	if ifc.LatestRevision.Id != newID {
		t.Fatalf("latest revision Id = %q, want %q", ifc.LatestRevision.Id, newID)
	}
	if got := ifc.Releases["1.0.0"].InterfaceRevisionId; got != newID {
		t.Fatalf("release revision Id = %q, want %q", got, newID)
	}
}

func TestManifest_reassignInterfaceID(t *testing.T) {
	const newID = "interface_01kn3ma93qe59r0p8kw6821y2n"
	mft := committedManifest("ifc7-rest")

	if err := mft.reassignInterfaceID("ifc7-rest", newID); err != nil {
		t.Fatalf("reassignInterfaceID returned error: %v", err)
	}

	if _, ok := mft.Interfaces["ifc7-rest"]; ok {
		t.Fatal("temporary interface key still present")
	}
	ifc, ok := mft.Interfaces[newID]
	if !ok {
		t.Fatalf("interface not re-keyed to %q", newID)
	}
	if ifc.Id != newID {
		t.Fatalf("interface Id = %q, want %q", ifc.Id, newID)
	}
	if got := ifc.Releases["1.0.0"].InterfaceId; got != newID {
		t.Fatalf("release interface Id = %q, want %q", got, newID)
	}
}

func TestManifest_upsertRevision_replacesChecksumPlaceholder(t *testing.T) {
	const newID = "revision_01kn3r6n8zf3aa3rrnzmakqncn"
	mft := committedManifest("interface_abc")

	// Server returns the canonical revision: same content (checksum) but a real
	// revision ID and an authoritative creation time.
	serverRev := client.InterfaceRevision{
		Checksum:      testChecksum,
		Id:            newID,
		Specification: "c3BlYw==",
		CreatedAt:     time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
		CreatedBy:     testUserID,
		Notes:         testutils.Ptr("committed revision"),
	}
	if err := mft.upsertRevision("interface_abc", serverRev); err != nil {
		t.Fatalf("upsertRevision returned error: %v", err)
	}

	ifc := mft.Interfaces["interface_abc"]
	if len(ifc.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(ifc.Revisions))
	}
	if _, ok := ifc.Revisions[testChecksum]; ok {
		t.Fatal("checksum-keyed placeholder was not removed")
	}
	if _, ok := ifc.Revisions[newID]; !ok {
		t.Fatalf("server revision %q not present", newID)
	}
	if ifc.LatestRevision.Id != newID {
		t.Fatalf("latest revision Id = %q, want %q", ifc.LatestRevision.Id, newID)
	}
	if got := ifc.Releases["1.0.0"].InterfaceRevisionId; got != newID {
		t.Fatalf("release revision Id = %q, want %q", got, newID)
	}
}

func TestProject_Push_NewInterfaceRemapsIDs(t *testing.T) {
	const (
		newIfcID = "interface_01kn3ma93qe59r0p8kw6821y2n"
		newRevID = "revision_01kn3r6n8zf3aa3rrnzmakqncn"
	)
	cfg := Config{
		Own: []Owned{{Name: "ifc7-rest", Ref: "", Path: testApiPath}},
	}
	mft := *committedManifest("ifc7-rest")
	// Drop the release so we don't need to mock release creation.
	mft.Interfaces["ifc7-rest"].Releases = map[string]*client.InterfaceRelease{}

	proj, err := projectWithMockClient(t, cfg, mft, func(mock *client.MockClientWithResponsesIfc) {
		expectCurrentUserID(mock)
		mock.EXPECT().
			CreateInterfaceWithResponse(gomock.Any(), gomock.Any()).
			Return(&client.CreateInterfaceResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
				JSON201:      &client.InterfaceDescriptor{Id: newIfcID},
			}, nil).
			AnyTimes()
		mock.EXPECT().
			CreateInterfaceRevisionWithResponse(gomock.Any(), gomock.Eq(client.InterfaceId(newIfcID)), gomock.Any()).
			Return(&client.CreateInterfaceRevisionResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
				JSON201:      &client.InterfaceRevisionDescriptor{Id: newRevID},
			}, nil).
			AnyTimes()
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := proj.Push(t.Context(), PushParams{}); err != nil {
		t.Fatalf("Push returned error: %v", err)
	}

	if _, ok := proj.manifest.Interfaces["ifc7-rest"]; ok {
		t.Fatal("interface still keyed by name after push")
	}
	ifc, ok := proj.manifest.Interfaces[newIfcID]
	if !ok {
		t.Fatalf("interface not re-keyed to %q", newIfcID)
	}
	if ifc.Id != newIfcID {
		t.Fatalf("interface Id = %q, want %q", ifc.Id, newIfcID)
	}
	if _, ok := ifc.Revisions[testChecksum]; ok {
		t.Fatal("revision still keyed by checksum after push")
	}
	if _, ok := ifc.Revisions[newRevID]; !ok {
		t.Fatalf("revision not re-keyed to %q", newRevID)
	}
	// The owned config entry should now reference the server interface ID.
	found := false
	for _, o := range proj.config.Own {
		if o.Name == "ifc7-rest" && o.Ref == newIfcID {
			found = true
		}
	}
	if !found {
		t.Fatalf("owned interface ref not updated to %q: %+v", newIfcID, proj.config.Own)
	}
}
