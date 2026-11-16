package project

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/ifc7/ifc/internal"
	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/fileio"
	"github.com/ifc7/ifc/internal/pkg/testutils"
	"github.com/ifc7/ifc/internal/tui"
)

// failOnPrompt replaces every TUI prompt with one that fails the test.
func failOnPrompt(t *testing.T) {
	t.Helper()
	origNewInterface := promptNewInterfaceCommit
	origNewRevision := promptNewRevisionCommit
	origOwner := promptInterfaceOwner
	origCreateFile := promptCreateMissingFile
	promptNewInterfaceCommit = func(context.Context, string, client.InterfaceType) (tui.NewInterface, error) {
		t.Fatal("promptNewInterfaceCommit called")
		return tui.NewInterface{}, nil
	}
	promptNewRevisionCommit = func(context.Context, string) (tui.NewRevision, error) {
		t.Fatal("promptNewRevisionCommit called")
		return tui.NewRevision{}, nil
	}
	promptInterfaceOwner = func(context.Context, string, []tui.InterfaceOwnerOption) (tui.InterfaceOwnerOption, error) {
		t.Fatal("promptInterfaceOwner called")
		return tui.InterfaceOwnerOption{}, nil
	}
	promptCreateMissingFile = func(context.Context, string, string) error {
		t.Fatal("promptCreateMissingFile called")
		return nil
	}
	t.Cleanup(func() {
		promptNewInterfaceCommit = origNewInterface
		promptNewRevisionCommit = origNewRevision
		promptInterfaceOwner = origOwner
		promptCreateMissingFile = origCreateFile
	})
}

func TestLoad_Manifest(t *testing.T) {
	ifcYAML, err := os.ReadFile("ifc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		noConfig    bool
		manifest    *string // nil: no .ifc/ folder at all
		expErr      error
		expManifest *Manifest
	}{
		"missing manifest is empty": {
			expManifest: NewManifest(),
		},
		"empty manifest": {
			manifest:    testutils.Ptr(`{"interfaces": {}}`),
			expManifest: NewManifest(),
		},
		"malformed manifest": {
			manifest: testutils.Ptr(`{"interfaces": [`),
			expErr:   ErrInvalidManifest,
		},
		"missing ifc.yaml": {
			noConfig: true,
			expErr:   ErrInvalidConfig,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if !tc.noConfig {
				if err := os.WriteFile(internal.IfcConfigFile, ifcYAML, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.manifest != nil {
				if err := fileio.WriteFile([]byte(*tc.manifest), internal.IfcManifestPath); err != nil {
					t.Fatal(err)
				}
			}
			mock := client.NewMockClientWithResponsesIfc(gomock.NewController(t))
			proj, err := Load(WithClient(mock))
			if endTest := testutils.CheckErr(t, err, tc.expErr); endTest {
				return
			}
			if !reflect.DeepEqual(&proj.manifest, tc.expManifest) {
				t.Fatalf("manifest = %s, want %s", proj.manifest.String(), tc.expManifest.String())
			}
			if len(proj.config.Use) == 0 {
				t.Fatal("ifc.yaml was not loaded")
			}
			// Writing a fresh checkout creates .ifc/ and leaves ifc.yaml intact.
			if err := proj.Write(); err != nil {
				t.Fatal(err)
			}
			if !fileio.FileExists(internal.IfcManifestPath) {
				t.Fatalf("%s was not written", internal.IfcManifestPath)
			}
			if _, err := ReadConfig(internal.IfcConfigFile); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProject_FetchOwned(t *testing.T) {
	for name, tc := range map[string]struct {
		config    Config
		setupMock func(mock *client.MockClientWithResponsesIfc)
		expIfcs   int
	}{
		"skips use entries and owned entries without a ref": {
			config: Config{
				// gomock fails the test on any call for these refs.
				Use: []Used{{Ref: "ifc7.dev/i/someone-else/private-api"}},
				Own: []Owned{
					{Name: "ifc7-rest", Ref: "ifc7.dev/i/interface7/rest-api", Path: testApiPath},
					{Name: "new-api", Path: "api/new.yaml"},
				},
			},
			setupMock: func(mock *client.MockClientWithResponsesIfc) {
				expectCanonicalResolve(mock)
				mock.EXPECT().
					GetInterfaceWithResponse(gomock.Any(), gomock.Eq("interface_01kn3ma93qe59r0p8kw6821y2n")).
					Return(&client.GetInterfaceResponse{
						HTTPResponse: &http.Response{StatusCode: http.StatusOK},
						JSON200:      manifestInterface,
					}, nil).
					Times(1)
				expectRevisionList(mock)
				expectRevisionGet(mock, manifestRevisions[1])
				expectReleaseList(mock)
			},
			expIfcs: 1,
		},
		"nothing owned with a ref": {
			config: Config{
				Use: []Used{{Ref: "ifc7.dev/i/someone-else/private-api"}},
				Own: []Owned{{Name: "new-api", Path: "api/new.yaml"}},
			},
			expIfcs: 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			proj, err := projectWithMockClient(t, tc.config, *NewManifest(), tc.setupMock)
			if err != nil {
				t.Fatal(err)
			}
			if err := proj.Fetch(t.Context(), FetchParams{Owned: true}); err != nil {
				t.Fatal(err)
			}
			if len(proj.manifest.Interfaces) != tc.expIfcs {
				t.Fatalf("expected %d interfaces, got %d: %s", tc.expIfcs, len(proj.manifest.Interfaces), proj.manifest.String())
			}
		})
	}
}

func TestProject_CommitNonInteractive(t *testing.T) {
	file, err := os.ReadFile(testApiPath)
	if err != nil {
		t.Fatal(err)
	}
	fileSHA := sha256Checksum(file)
	restAPI := Owned{Name: "ifc7-rest", Ref: "ifc7.dev/i/interface7/rest-api", Path: testApiPath}
	newAPI := Owned{Name: "new-api", Path: testApiPath}
	resolve := func(mock *client.MockClientWithResponsesIfc) {
		expectCurrentUserID(mock)
		mock.EXPECT().
			GetInterfaceByCanonicalPathWithResponse(gomock.Any(), gomock.Eq("ifc7.dev"), gomock.Eq("interface7"), gomock.Eq("rest-api")).
			Return(&client.CanonicalInterfaceMeta{Id: "interface_01kn3ma93qe59r0p8kw6821y2n"}, http.StatusOK, nil).
			AnyTimes()
	}

	for name, tc := range map[string]struct {
		config      Config
		manifest    Manifest
		params      CommitParams
		expErr      string
		expNotes    *string // notes on the new revision; nil: no new revision
		expWarnings []string
	}{
		"notes from params": {
			config:   Config{Own: []Owned{restAPI}},
			manifest: mustParseManifest(t, testManifestBytes),
			params:   CommitParams{NonInteractive: true, Notes: testutils.Ptr("Add billing endpoints (abc1234)")},
			expNotes: testutils.Ptr("Add billing endpoints (abc1234)"),
		},
		"notes default to empty": {
			config:   Config{Own: []Owned{restAPI}},
			manifest: mustParseManifest(t, testManifestBytes),
			params:   CommitParams{NonInteractive: true},
			expNotes: testutils.Ptr(""),
		},
		"interactive with notes skips prompt": {
			config:   Config{Own: []Owned{restAPI}},
			manifest: mustParseManifest(t, testManifestBytes),
			params:   CommitParams{Notes: testutils.Ptr("given")},
			expNotes: testutils.Ptr("given"),
		},
		"owned without ref is skipped": {
			config:      Config{Own: []Owned{newAPI, restAPI}},
			manifest:    mustParseManifest(t, testManifestBytes),
			params:      CommitParams{NonInteractive: true},
			expNotes:    testutils.Ptr(""),
			expWarnings: []string{`Skipping "new-api": no ref in ifc.yaml. Run 'ifc push' interactively once to create it.`},
		},
		"owned with ref but not fetched fails": {
			config:   Config{Own: []Owned{restAPI}},
			manifest: *NewManifest(),
			params:   CommitParams{NonInteractive: true},
			expErr:   "run 'ifc fetch --owned' first",
		},
	} {
		t.Run(name, func(t *testing.T) {
			failOnPrompt(t)
			var warnings []string
			tc.params.Warn = func(message string) { warnings = append(warnings, message) }
			proj, err := projectWithMockClient(t, tc.config, tc.manifest, resolve)
			if err != nil {
				t.Fatal(err)
			}
			err = proj.Commit(t.Context(), tc.params)
			if tc.expErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expErr) {
					t.Fatalf("expected error containing %q, got %v", tc.expErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(warnings, tc.expWarnings) {
				t.Fatalf("warnings = %q, want %q", warnings, tc.expWarnings)
			}
			if len(proj.manifest.Interfaces) != 1 {
				t.Fatalf("expected 1 interface, got %d", len(proj.manifest.Interfaces))
			}
			for _, ifc := range proj.manifest.Interfaces {
				rev, ok := ifc.Revisions[fileSHA]
				if tc.expNotes == nil {
					if ok {
						t.Fatal("unexpected new revision")
					}
					continue
				}
				if !ok {
					t.Fatal("no revision committed for the changed file")
				}
				if rev.Notes == nil || *rev.Notes != *tc.expNotes {
					t.Fatalf("notes = %v, want %q", rev.Notes, *tc.expNotes)
				}
				if ifc.LatestRevision == nil || ifc.LatestRevision.Checksum != fileSHA {
					t.Fatal("latest revision was not updated")
				}
			}
		})
	}
}

func TestProject_PushReport(t *testing.T) {
	restAPI := Owned{Name: "ifc7-rest", Ref: "ifc7.dev/i/interface7/rest-api", Path: testApiPath}
	newAPI := Owned{Name: "new-api", Path: "api/new.yaml"}
	laterAPI := Owned{Name: "later-api", Ref: "ifc7.dev/i/interface7/later-api", Path: "api/later.yaml"}
	pushedID := "revision_01kn3localonly0000000001"
	expectCreate := func(status int) func(mock *client.MockClientWithResponsesIfc) {
		return func(mock *client.MockClientWithResponsesIfc) {
			expectRevisionList(mock)
			mock.EXPECT().
				GetCurrentUserWithResponse(gomock.Any()).
				Return(&client.GetCurrentUserResponse{
					HTTPResponse: &http.Response{StatusCode: http.StatusOK},
					JSON200:      &client.User{Id: testUserID},
				}, nil).
				Times(1)
			resp := &client.CreateInterfaceRevisionResponse{HTTPResponse: &http.Response{StatusCode: status}}
			if status == http.StatusCreated {
				resp.JSON201 = &client.InterfaceRevisionDescriptor{
					Id:        pushedID,
					CreatedAt: time.Date(2026, time.May, 2, 0, 0, 0, 0, time.UTC),
				}
			}
			mock.EXPECT().
				CreateInterfaceRevisionWithResponse(gomock.Any(), gomock.Eq(client.InterfaceId("interface_01kn3ma93qe59r0p8kw6821y2n")), gomock.Any()).
				Return(resp, nil).
				Times(1)
		}
	}

	for name, tc := range map[string]struct {
		config    Config
		manifest  Manifest
		params    PushParams
		setupMock func(mock *client.MockClientWithResponsesIfc)
		expErr    bool
		expReport []PushResult
	}{
		"pushed": {
			config:    Config{Own: []Owned{restAPI}},
			manifest:  manifestWithLocalRevision(t),
			params:    PushParams{NonInteractive: true},
			setupMock: expectCreate(http.StatusCreated),
			expReport: []PushResult{
				{Name: "ifc7-rest", Ref: restAPI.Ref, Path: testApiPath, Status: PushStatusPushed, RevisionIDs: []string{pushedID}},
			},
		},
		"up to date": {
			config:    Config{Own: []Owned{restAPI}},
			manifest:  manifestWithIdentity(t),
			params:    PushParams{NonInteractive: true},
			setupMock: expectRevisionList,
			expReport: []PushResult{
				{Name: "ifc7-rest", Ref: restAPI.Ref, Path: testApiPath, Status: PushStatusUpToDate},
			},
		},
		"non-interactive skips owned without ref and never asks for an owner": {
			config:    Config{Own: []Owned{newAPI, restAPI}},
			manifest:  manifestWithIdentity(t),
			params:    PushParams{NonInteractive: true},
			setupMock: expectRevisionList,
			expReport: []PushResult{
				{Name: "new-api", Path: "api/new.yaml", Status: PushStatusSkipped, Reason: "no ref in ifc.yaml"},
				{Name: "ifc7-rest", Ref: restAPI.Ref, Path: testApiPath, Status: PushStatusUpToDate},
			},
		},
		"failure stops the push and reports the rest as not attempted": {
			config:    Config{Own: []Owned{newAPI, restAPI, laterAPI}},
			manifest:  manifestWithLocalRevision(t),
			params:    PushParams{NonInteractive: true},
			setupMock: expectCreate(http.StatusForbidden),
			expErr:    true,
			expReport: []PushResult{
				{Name: "new-api", Path: "api/new.yaml", Status: PushStatusSkipped, Reason: "no ref in ifc.yaml"},
				{Name: "ifc7-rest", Ref: restAPI.Ref, Path: testApiPath, Status: PushStatusFailed, Error: "error creating revision " + testChecksum + ": HTTP 403"},
				{Name: "later-api", Ref: laterAPI.Ref, Path: "api/later.yaml", Status: PushStatusNotAttempted},
			},
		},
		"nothing owned": {
			config:    Config{},
			manifest:  *NewManifest(),
			params:    PushParams{NonInteractive: true},
			expReport: []PushResult{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			failOnPrompt(t)
			proj, err := projectWithMockClient(t, tc.config, tc.manifest, tc.setupMock)
			if err != nil {
				t.Fatal(err)
			}
			report, err := proj.Push(t.Context(), tc.params)
			if tc.expErr != (err != nil) {
				t.Fatalf("expected error: %v, got %v", tc.expErr, err)
			}
			if report == nil {
				t.Fatal("report is nil")
			}
			if !reflect.DeepEqual(report.Interfaces, tc.expReport) {
				t.Fatalf("report = %+v, want %+v", report.Interfaces, tc.expReport)
			}
		})
	}
}

// TestChecksumMatchesServer pins the revision checksum to the hex sha256 of the raw
// spec bytes, which is how ifc-server computes a revision's checksum. If the two
// diverge, every CI run commits and pushes a duplicate revision.
func TestChecksumMatchesServer(t *testing.T) {
	for name, tc := range map[string]struct {
		spec string
		exp  string
	}{
		"empty": {
			spec: "",
			exp:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		"raw yaml bytes, not normalized": {
			spec: "openapi: 3.0.0\n",
			exp:  "344e4b2f7f15b76b5606be45d8031fc43f473f8d63f0e02c61dbf89a97f85e69",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := sha256Checksum([]byte(tc.spec)); got != tc.exp {
				t.Fatalf("sha256Checksum(%q) = %s, want %s", tc.spec, got, tc.exp)
			}
		})
	}
	// Whitespace changes are distinct revisions; nothing is normalized before hashing.
	if sha256Checksum([]byte("openapi: 3.0.0\n")) == sha256Checksum([]byte("openapi:  3.0.0\n")) {
		t.Fatal("checksum ignores whitespace")
	}
}
