package commands

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/ifc7/ifc/internal"
	"github.com/ifc7/ifc/internal/client"
)

const (
	testToken  = "ifc_agt_testtoken0123456789abcdefghijkl"
	testUserID = client.UserId("user_01kn3agent00000000000000000")
)

// fakeHub is an in-memory ifc-server serving the endpoints fetch, commit and push use.
type fakeHub struct {
	mu         sync.Mutex
	interfaces map[string]*fakeInterface // by id
	created    []client.CreateRevisionRequest
	forbidPush map[string]bool // interface ids whose revision POST returns 403
	rejectAll  bool            // every request returns 401
	requested  []string        // "METHOD path" of every request
	nextID     int
}

type fakeInterface struct {
	ifc       client.Interface
	revisions []client.InterfaceRevision
}

func newFakeHub() *fakeHub {
	return &fakeHub{interfaces: map[string]*fakeInterface{}, forbidPush: map[string]bool{}}
}

// addInterface registers interface-7/<slug> with one server revision per spec.
func (h *fakeHub) addInterface(id, slug string, specs ...string) {
	fi := &fakeInterface{ifc: client.Interface{
		Id:           id,
		Name:         slug,
		Slug:         slug,
		Owner:        "org_01kn3interface70000000000000",
		CanonicalUrl: "/i/interface-7/" + slug,
		Type:         client.OPENAPI,
		IsPublic:     true,
	}}
	h.interfaces[id] = fi
	for _, spec := range specs {
		h.appendRevision(fi, []byte(spec), nil)
	}
}

func (h *fakeHub) appendRevision(fi *fakeInterface, spec []byte, notes *string) client.InterfaceRevision {
	h.nextID++
	rev := client.InterfaceRevision{
		Id: fmt.Sprintf("revision_%026d", h.nextID),
		// ifc-server stores the hex sha256 of the raw spec bytes.
		Checksum:      fmt.Sprintf("%x", sha256.Sum256(spec)),
		CreatedAt:     time.Date(2026, time.November, 1, 0, 0, h.nextID, 0, time.UTC),
		CreatedBy:     testUserID,
		Notes:         notes,
		Specification: base64.StdEncoding.EncodeToString(spec),
	}
	fi.revisions = append(fi.revisions, rev)
	latest := rev
	latest.Specification = "" // the interface payload carries no spec body
	fi.ifc.LatestRevision = &latest
	return rev
}

func descriptor(rev client.InterfaceRevision) client.InterfaceRevisionDescriptor {
	return client.InterfaceRevisionDescriptor{
		Id: rev.Id, Checksum: rev.Checksum, CreatedAt: rev.CreatedAt, CreatedBy: rev.CreatedBy, Notes: rev.Notes,
	}
}

func (h *fakeHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requested = append(h.requested, r.Method+" "+r.URL.Path)
	if h.rejectAll || r.Header.Get("Authorization") != "Bearer "+testToken {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 3 && parts[0] == "i" && r.Method == http.MethodGet:
		for _, fi := range h.interfaces {
			if fi.ifc.CanonicalUrl == r.URL.Path {
				writeJSON(w, http.StatusOK, client.CanonicalInterfaceMeta{Id: fi.ifc.Id, Name: fi.ifc.Name, Slug: fi.ifc.Slug})
				return
			}
		}
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "forbidden"})
	case r.URL.Path == "/api/v0/users/current":
		writeJSON(w, http.StatusOK, client.User{Id: testUserID, Name: "CI publisher", Slug: "ci-publisher"})
	case len(parts) >= 4 && parts[0] == "api" && parts[1] == "v0" && parts[2] == "interfaces":
		fi, ok := h.interfaces[parts[3]]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
			return
		}
		h.serveInterface(w, r, fi, parts[4:])
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	}
}

func (h *fakeHub) serveInterface(w http.ResponseWriter, r *http.Request, fi *fakeInterface, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, fi.ifc)
	case len(rest) == 1 && rest[0] == "releases":
		writeJSON(w, http.StatusOK, []client.InterfaceRelease{})
	case len(rest) == 1 && rest[0] == "revisions" && r.Method == http.MethodGet:
		descs := []client.InterfaceRevisionDescriptor{}
		for _, rev := range fi.revisions {
			descs = append(descs, descriptor(rev))
		}
		writeJSON(w, http.StatusOK, descs)
	case len(rest) == 1 && rest[0] == "revisions" && r.Method == http.MethodPost:
		if h.forbidPush[fi.ifc.Id] {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "forbidden"})
			return
		}
		var req client.CreateRevisionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		spec, err := base64.StdEncoding.DecodeString(req.Specification)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		h.created = append(h.created, req)
		writeJSON(w, http.StatusCreated, descriptor(h.appendRevision(fi, spec, req.Notes)))
	case len(rest) == 2 && rest[0] == "revisions" && r.Method == http.MethodGet:
		for _, rev := range fi.revisions {
			if rev.Id == rest[1] {
				writeJSON(w, http.StatusOK, rev)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *fakeHub) createdRevisions() []client.CreateRevisionRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]client.CreateRevisionRequest{}, h.created...)
}

func (h *fakeHub) requests() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.requested...)
}

// startHub serves hub and points IFC_HUB_URL and IFC_TOKEN at it, with an empty
// user config dir so no stored login is used.
func startHub(t *testing.T, hub *fakeHub) {
	t.Helper()
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)
	t.Setenv(internal.EnvHubURL, srv.URL)
	t.Setenv(internal.EnvToken, testToken)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
}

// freshCheckout writes ifc.yaml and spec files into a temp dir with no .ifc/ folder
// and changes into it.
func freshCheckout(t *testing.T, ifcYAML string, files map[string]string) {
	t.Helper()
	t.Chdir(t.TempDir())
	files[internal.IfcConfigFile] = ifcYAML
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// runIfc runs the ifc root command with args and returns everything written to the
// process's stdout and stderr.
func runIfc(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	resetCommands(t, rootCmd)
	outFile, errFile := captureFile(t, "stdout"), captureFile(t, "stderr")
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outFile, errFile
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	rootCmd.SetArgs(args)
	err = rootCmd.ExecuteContext(t.Context())
	return readCapture(t, outFile), readCapture(t, errFile), err
}

func captureFile(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func readCapture(t *testing.T, f *os.File) string {
	t.Helper()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// resetCommands restores every flag to its default and sets this test's context;
// cobra keeps both on subcommands between runs.
func resetCommands(t *testing.T, cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	cmd.SetContext(t.Context())
	for _, c := range cmd.Commands() {
		resetCommands(t, c)
	}
}

// decodePushReport parses stdout as exactly one push --json document.
func decodePushReport(t *testing.T, stdout string) pushReportJSON {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	var report pushReportJSON
	if err := dec.Decode(&report); err != nil {
		t.Fatalf("stdout is not a push report: %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Fatalf("stdout has more than one JSON document:\n%s", stdout)
	}
	if report.Interfaces == nil {
		t.Fatalf("report has no interfaces array:\n%s", stdout)
	}
	return report
}

// pushReportJSON mirrors the push --json shape that ifc-action's push.sh parses.
type pushReportJSON struct {
	Interfaces []pushResultJSON `json:"interfaces"`
}

type pushResultJSON struct {
	Name        string   `json:"name"`
	Ref         string   `json:"ref,omitempty"`
	Path        string   `json:"path"`
	Status      string   `json:"status"`
	RevisionIDs []string `json:"revisionIds,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Error       string   `json:"error,omitempty"`
}

const (
	restV1    = "openapi: 3.0.0\ninfo:\n  title: rest\n  version: 0.1.0\npaths: {}\n"
	restV2    = "openapi: 3.0.0\ninfo:\n  title: rest\n  version: 0.2.0\npaths: {}\n"
	billingV1 = "openapi: 3.0.0\ninfo:\n  title: billing\n  version: 0.1.0\npaths: {}\n"
	restID    = "interface_01kn3rest0000000000000000000"
	billingID = "interface_01kn3billing000000000000000000"
)

const ciIfcYAML = `use:
    - ref: ifc7.dev/i/someone-else/private-api
own:
    - name: rest-api
      ref: ifc7.dev/i/interface-7/rest-api
      path: api/v0.yaml
    - name: billing-api
      ref: ifc7.dev/i/interface-7/billing-api
      path: api/billing/v0.yaml
    - name: new-api
      ref: ""
      path: api/new.yaml
`

// TestCIFlow runs the exact commands ifc-action runs, on a fresh checkout (ifc.yaml
// and spec files only) against a fake hub.
func TestCIFlow(t *testing.T) {
	hub := newFakeHub()
	hub.addInterface(restID, "rest-api", restV1)
	hub.addInterface(billingID, "billing-api", billingV1)
	startHub(t, hub)
	files := map[string]string{
		"api/v0.yaml":         restV2,    // changed since the last server revision
		"api/billing/v0.yaml": billingV1, // unchanged
		"api/new.yaml":        restV1,    // never pushed
	}
	freshCheckout(t, ciIfcYAML, files)

	ciCycle := func(t *testing.T) pushReportJSON {
		t.Helper()
		if _, stderr, err := runIfc(t, "fetch", "--owned", "--non-interactive"); err != nil {
			t.Fatalf("fetch: %v\n%s", err, stderr)
		}
		_, stderr, err := runIfc(t, "commit", "--non-interactive", "--notes", "Add billing endpoints (abc1234)")
		if err != nil {
			t.Fatalf("commit: %v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, `Skipping "new-api": no ref in ifc.yaml. Run 'ifc push' interactively once to create it.`) {
			t.Fatalf("commit did not warn about new-api on stderr:\n%s", stderr)
		}
		stdout, stderr, err := runIfc(t, "push", "--non-interactive", "--json")
		if err != nil {
			t.Fatalf("push: %v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, `Skipping "new-api"`) {
			t.Fatalf("push messages are not on stderr:\n%s", stderr)
		}
		return decodePushReport(t, stdout)
	}

	report := ciCycle(t)
	created := hub.createdRevisions()
	if len(created) != 1 {
		t.Fatalf("expected exactly one new revision, got %d", len(created))
	}
	if created[0].Notes == nil || *created[0].Notes != "Add billing endpoints (abc1234)" {
		t.Fatalf("revision notes = %v", created[0].Notes)
	}
	if created[0].Specification != base64.StdEncoding.EncodeToString([]byte(restV2)) {
		t.Fatal("pushed specification is not the changed file")
	}
	if created[0].CreatedBy != testUserID {
		t.Fatalf("createdBy = %s", created[0].CreatedBy)
	}
	newRevID := hub.interfaces[restID].ifc.LatestRevision.Id
	expect := []pushResultJSON{
		{Name: "rest-api", Ref: "ifc7.dev/i/interface-7/rest-api", Path: "api/v0.yaml", Status: "pushed", RevisionIDs: []string{newRevID}},
		{Name: "billing-api", Ref: "ifc7.dev/i/interface-7/billing-api", Path: "api/billing/v0.yaml", Status: "up_to_date"},
		{Name: "new-api", Path: "api/new.yaml", Status: "skipped", Reason: "no ref in ifc.yaml"},
	}
	assertReport(t, report, expect)
	for _, req := range hub.requests() {
		if strings.Contains(req, "someone-else") {
			t.Fatalf("fetch --owned requested a use entry: %s", req)
		}
	}

	// A later run on another fresh checkout with the same files pushes nothing:
	// the local checksum matches the server's checksum of the revision it created.
	if err := os.RemoveAll(internal.IfcFolder); err != nil {
		t.Fatal(err)
	}
	report = ciCycle(t)
	if got := len(hub.createdRevisions()); got != 1 {
		t.Fatalf("re-run created %d more revisions", got-1)
	}
	expect[0].Status, expect[0].RevisionIDs = "up_to_date", nil
	assertReport(t, report, expect)
}

func assertReport(t *testing.T, got pushReportJSON, want []pushResultJSON) {
	t.Helper()
	gotJSON, _ := json.Marshal(got.Interfaces)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("push report:\n got %s\nwant %s", gotJSON, wantJSON)
	}
}

func TestPushJSON_Failure(t *testing.T) {
	for name, tc := range map[string]struct {
		setup     func(hub *fakeHub)
		expErr    string
		expReport []pushResultJSON
	}{
		"partial failure reports failed and not attempted": {
			setup: func(hub *fakeHub) { hub.forbidPush[restID] = true },
			// ifc.yaml order: rest-api fails, so billing-api and new-api are not attempted.
			expErr: "HTTP 403",
			expReport: []pushResultJSON{
				{Name: "rest-api", Ref: "ifc7.dev/i/interface-7/rest-api", Path: "api/v0.yaml", Status: "failed", Error: "error creating revision %s: HTTP 403"},
				{Name: "billing-api", Ref: "ifc7.dev/i/interface-7/billing-api", Path: "api/billing/v0.yaml", Status: "not_attempted"},
				{Name: "new-api", Path: "api/new.yaml", Status: "not_attempted"},
			},
		},
		"rejected token": {
			setup:  func(hub *fakeHub) { hub.rejectAll = true },
			expErr: "IFC_TOKEN was rejected by the hub (HTTP 401); it may have expired",
			expReport: []pushResultJSON{
				{Name: "rest-api", Ref: "ifc7.dev/i/interface-7/rest-api", Path: "api/v0.yaml", Status: "failed", Error: "%s"},
				{Name: "billing-api", Ref: "ifc7.dev/i/interface-7/billing-api", Path: "api/billing/v0.yaml", Status: "not_attempted"},
				{Name: "new-api", Path: "api/new.yaml", Status: "not_attempted"},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			hub := newFakeHub()
			hub.addInterface(restID, "rest-api", restV1)
			hub.addInterface(billingID, "billing-api", billingV1)
			startHub(t, hub)
			freshCheckout(t, ciIfcYAML, map[string]string{
				"api/v0.yaml":         restV2,
				"api/billing/v0.yaml": billingV1,
				"api/new.yaml":        restV1,
			})
			for _, args := range [][]string{
				{"fetch", "--owned", "--non-interactive"},
				{"commit", "--non-interactive"},
			} {
				if _, stderr, err := runIfc(t, args...); err != nil {
					t.Fatalf("%s: %v\n%s", args[0], err, stderr)
				}
			}
			tc.setup(hub)

			stdout, _, err := runIfc(t, "push", "--non-interactive", "--json")
			if err == nil || !strings.Contains(err.Error(), tc.expErr) {
				t.Fatalf("expected error containing %q, got %v", tc.expErr, err)
			}
			report := decodePushReport(t, stdout)
			// Fill in error text that depends on run-time values.
			for i := range tc.expReport {
				if tc.expReport[i].Status == "failed" && i < len(report.Interfaces) {
					if strings.Contains(tc.expReport[i].Error, "%s") {
						got := report.Interfaces[i].Error
						if !strings.Contains(got, tc.expErr) {
							t.Fatalf("failed entry error = %q, want it to contain %q", got, tc.expErr)
						}
						tc.expReport[i].Error = got
					}
				}
			}
			assertReport(t, report, tc.expReport)
		})
	}
}

func TestPushJSON_LoadFailure(t *testing.T) {
	startHub(t, newFakeHub())
	t.Chdir(t.TempDir()) // no ifc.yaml
	stdout, _, err := runIfc(t, "push", "--json")
	if err == nil {
		t.Fatal("expected an error without ifc.yaml")
	}
	if report := decodePushReport(t, stdout); len(report.Interfaces) != 0 {
		t.Fatalf("expected no interfaces, got %+v", report.Interfaces)
	}
}

func TestPushHelpMentionsJSON(t *testing.T) {
	// ifc-action detects support by grepping `ifc push --help` for --json.
	stdout, _, err := runIfc(t, "push", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "--json") {
		t.Fatalf("push --help does not mention --json:\n%s", stdout)
	}
}

func TestNonInteractive(t *testing.T) {
	for name, tc := range map[string]struct {
		flag     bool
		ci       string
		terminal bool
		exp      bool
	}{
		"interactive terminal":  {terminal: true, exp: false},
		"flag":                  {flag: true, terminal: true, exp: true},
		"CI=true":               {ci: "true", terminal: true, exp: true},
		"CI=1":                  {ci: "1", terminal: true, exp: true},
		"CI=false":              {ci: "false", terminal: true, exp: false},
		"stdin is not terminal": {terminal: false, exp: true},
	} {
		t.Run(name, func(t *testing.T) {
			orig := stdinIsTerminal
			stdinIsTerminal = func() bool { return tc.terminal }
			t.Cleanup(func() { stdinIsTerminal = orig })
			t.Setenv("CI", tc.ci)
			cmd := &cobra.Command{}
			cmd.Flags().Bool(nonInteractiveFlag, false, "")
			if tc.flag {
				if err := cmd.Flags().Set(nonInteractiveFlag, "true"); err != nil {
					t.Fatal(err)
				}
			}
			if got := nonInteractive(cmd); got != tc.exp {
				t.Fatalf("nonInteractive = %v, want %v", got, tc.exp)
			}
		})
	}
}

func TestNonInteractivePrompts(t *testing.T) {
	for name, tc := range map[string]struct {
		args   []string
		expErr string
	}{
		"add missing file": {
			args:   []string{"add", "api/missing.yaml", "missing", "--non-interactive"},
			expErr: "file api/missing.yaml does not exist",
		},
		"scan": {
			args:   []string{"scan", "--non-interactive"},
			expErr: "ifc scan needs an interactive terminal",
		},
	} {
		t.Run(name, func(t *testing.T) {
			startHub(t, newFakeHub())
			freshCheckout(t, "use: []\nown: []\n", map[string]string{})
			_, _, err := runIfc(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.expErr) {
				t.Fatalf("expected error containing %q, got %v", tc.expErr, err)
			}
			if _, statErr := os.Stat("api/missing.yaml"); !os.IsNotExist(statErr) {
				t.Fatal("missing file was created")
			}
		})
	}
}
