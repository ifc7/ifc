package internal

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	// EnvToken holds an ifc7 access token (for example an agent token from CI).
	// When set, it replaces the stored login for every hub request.
	EnvToken = "IFC_TOKEN"
	// EnvHubURL overrides the base URL of the hub that requests are sent to.
	// It selects a server only; refs in ifc.yaml keep their canonical host.
	EnvHubURL = "IFC_HUB_URL"
	// DefaultHubURL is the hub base URL used when EnvHubURL is unset.
	DefaultHubURL = "https://" + DefaultAPIHost

	apiPath = "/api/v0/"
)

// HubURL returns the hub base URL from $IFC_HUB_URL, or DefaultHubURL.
// The result has no trailing slash.
func HubURL() (*url.URL, error) {
	raw := strings.TrimSpace(os.Getenv(EnvHubURL))
	if raw == "" {
		raw = DefaultHubURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid %s %q: want an http(s) URL such as %s", EnvHubURL, raw, DefaultHubURL)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""
	return u, nil
}

// APIURL returns the hub's REST API base URL, <hub URL>/api/v0/.
func APIURL() (string, error) {
	hub, err := HubURL()
	if err != nil {
		return "", err
	}
	return hub.String() + apiPath, nil
}

// HubURLFromAPIURL recovers the hub base URL from an API base URL built by APIURL.
func HubURLFromAPIURL(apiURL string) (*url.URL, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/")+"/", apiPath)
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}
