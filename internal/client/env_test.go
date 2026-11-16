package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ifc7/ifc/internal"
	"github.com/ifc7/ifc/internal/pkg/auth"
)

const testAgentToken = "ifc_agt_0123456789abcdefghijklmnopqrstuv"

// hubRecorder is a fake hub that records the requests it receives.
type hubRecorder struct {
	mu       sync.Mutex
	requests []*http.Request
	status   int
}

func (h *hubRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.requests = append(h.requests, r.Clone(r.Context()))
	status := h.status
	h.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	switch {
	case status != http.StatusOK:
		_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
	case strings.Contains(r.URL.Path, "/i/"):
		_ = json.NewEncoder(w).Encode(CanonicalInterfaceMeta{Id: "interface_01kn3ma93qe59r0p8kw6821y2n"})
	default:
		_, _ = w.Write([]byte(`[]`))
	}
}

func (h *hubRecorder) last(t *testing.T) *http.Request {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.requests) == 0 {
		t.Fatal("hub received no requests")
	}
	return h.requests[len(h.requests)-1]
}

// isolateUserConfig points the user config dir at an empty temp dir and returns
// the credentials path inside it.
func isolateUserConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := auth.CredentialsPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("credentials path %s is outside temp dir %s", path, dir)
	}
	return path
}

// writeExpiredCredentials stores a stored login whose access token has expired, so any
// attempt to use it would trigger a refresh.
func writeExpiredCredentials(t *testing.T, path string) []byte {
	t.Helper()
	refresh := "stored-refresh-token"
	b, err := json.Marshal(auth.Credentials{
		AccessToken:  "stored-access-token",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshToken: &refresh,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNewAPIClient_EnvToken(t *testing.T) {
	for name, tc := range map[string]struct {
		hubStatus   int
		storedLogin bool
		expErr      error
	}{
		"bearer header uses token": {},
		"expired stored login is ignored and not refreshed": {
			storedLogin: true,
		},
		"401 reports rejected token": {
			hubStatus: http.StatusUnauthorized,
			expErr:    ErrTokenRejected,
		},
	} {
		t.Run(name, func(t *testing.T) {
			credsPath := isolateUserConfig(t)
			var stored []byte
			if tc.storedLogin {
				stored = writeExpiredCredentials(t, credsPath)
			}
			hub := &hubRecorder{status: tc.hubStatus}
			srv := httptest.NewServer(hub)
			t.Cleanup(srv.Close)
			t.Setenv(internal.EnvHubURL, srv.URL)
			t.Setenv(internal.EnvToken, testAgentToken)

			c, err := NewAPIClient(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if c.credentialsService != nil {
				t.Fatal("credentials service was created although IFC_TOKEN is set")
			}
			_, err = c.ListInterfacesWithResponse(t.Context(), nil)
			if tc.expErr != nil {
				if !errors.Is(err, tc.expErr) {
					t.Fatalf("expected %v, got %v", tc.expErr, err)
				}
				if !strings.Contains(err.Error(), "IFC_TOKEN was rejected by the hub (HTTP 401); it may have expired") {
					t.Fatalf("unexpected error text: %v", err)
				}
				if strings.Contains(err.Error(), testAgentToken) {
					t.Fatal("error message leaks the token")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got := hub.last(t).Header.Get("Authorization"); got != "Bearer "+testAgentToken {
				t.Fatalf("Authorization = %q", got)
			}

			after, err := os.ReadFile(credsPath)
			switch {
			case tc.storedLogin && err != nil:
				t.Fatal(err)
			case tc.storedLogin && string(after) != string(stored):
				t.Fatal("stored credentials file was modified")
			case !tc.storedLogin && !os.IsNotExist(err):
				t.Fatalf("credentials file was created: %v", err)
			}
		})
	}
}

func TestNewAPIClient_HubURL(t *testing.T) {
	for name, tc := range map[string]struct {
		hubPath string
	}{
		"root":        {},
		"path prefix": {hubPath: "/hub"},
	} {
		t.Run(name, func(t *testing.T) {
			isolateUserConfig(t)
			hub := &hubRecorder{}
			srv := httptest.NewServer(hub)
			t.Cleanup(srv.Close)
			t.Setenv(internal.EnvHubURL, srv.URL+tc.hubPath+"/")
			t.Setenv(internal.EnvToken, testAgentToken)

			c, err := NewAPIClient(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.ListInterfaceRevisionsWithResponse(t.Context(), "interface_01kn3ma93qe59r0p8kw6821y2n"); err != nil {
				t.Fatal(err)
			}
			if got, want := hub.last(t).URL.Path, tc.hubPath+"/api/v0/interfaces/interface_01kn3ma93qe59r0p8kw6821y2n/revisions"; got != want {
				t.Fatalf("API path = %q, want %q", got, want)
			}

			meta, status, err := c.GetInterfaceByCanonicalPathWithResponse(t.Context(), internal.DefaultAPIHost, "interface-7", "rest-api")
			if err != nil {
				t.Fatal(err)
			}
			if status != http.StatusOK || meta == nil || meta.Id == "" {
				t.Fatalf("unexpected locator response: status %d, meta %+v", status, meta)
			}
			req := hub.last(t)
			if got, want := req.URL.Path, tc.hubPath+"/i/interface-7/rest-api"; got != want {
				t.Fatalf("locator path = %q, want %q", got, want)
			}
			if req.URL.RawQuery != "json" {
				t.Fatalf("locator query = %q", req.URL.RawQuery)
			}
			if got := req.Header.Get("Authorization"); got != "Bearer "+testAgentToken {
				t.Fatalf("locator Authorization = %q", got)
			}
		})
	}
}

func TestNewAPIClient_InvalidHubURL(t *testing.T) {
	for name, value := range map[string]string{
		"no scheme":  "ifc7.dev",
		"bad scheme": "ftp://ifc7.dev",
	} {
		t.Run(name, func(t *testing.T) {
			isolateUserConfig(t)
			t.Setenv(internal.EnvHubURL, value)
			if _, err := NewAPIClient(t.Context()); err == nil || !strings.Contains(err.Error(), internal.EnvHubURL) {
				t.Fatalf("expected an %s error, got %v", internal.EnvHubURL, err)
			}
		})
	}
}

func TestCanonicalLocatorURL(t *testing.T) {
	for name, tc := range map[string]struct {
		apiURL string
		host   string
		exp    string
	}{
		"default host on default hub": {
			apiURL: "https://ifc7.dev/api/v0/",
			host:   "ifc7.dev",
			exp:    "https://ifc7.dev/i/interface-7/rest-api?json",
		},
		"default host follows hub override": {
			apiURL: "https://staging.ifc7.dev/api/v0/",
			host:   "ifc7.dev",
			exp:    "https://staging.ifc7.dev/i/interface-7/rest-api?json",
		},
		"default host follows local hub": {
			apiURL: "http://127.0.0.1:8080/api/v0/",
			host:   "ifc7.dev",
			exp:    "http://127.0.0.1:8080/i/interface-7/rest-api?json",
		},
		"other host is called directly": {
			apiURL: "https://staging.ifc7.dev/api/v0/",
			host:   "dev.ifc7.dev",
			exp:    "https://dev.ifc7.dev/i/interface-7/rest-api?json",
		},
		"localhost uses http": {
			apiURL: "https://ifc7.dev/api/v0/",
			host:   "localhost:8080",
			exp:    "http://localhost:8080/i/interface-7/rest-api?json",
		},
	} {
		t.Run(name, func(t *testing.T) {
			u, err := canonicalLocatorURL(tc.apiURL, tc.host, "interface-7", "rest-api")
			if err != nil {
				t.Fatal(err)
			}
			if u.String() != tc.exp {
				t.Fatalf("got %s, want %s", u, tc.exp)
			}
		})
	}
}
