package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ifc7/ifc/internal"
)

// CanonicalInterfaceMeta is the JSON body returned by GET /i/{owner}/{name}?json.
type CanonicalInterfaceMeta struct {
	Id           InterfaceId `json:"id"`
	Name         string      `json:"name"`
	Slug         string      `json:"slug"`
	IsPublic     bool        `json:"isPublic"`
	CanonicalURL string      `json:"canonicalUrl"`
}

// GetInterfaceByCanonicalPathWithResponse resolves a canonical /i/{owner}/{name} locator
// via GET ?json on the ref host (outside /api/v0). The default host is replaced by the
// configured hub URL.
func (c *ClientWithResponses) GetInterfaceByCanonicalPathWithResponse(ctx context.Context, host, ownerPath, name string, reqEditors ...RequestEditorFn) (*CanonicalInterfaceMeta, int, error) {
	underlying, ok := c.ClientInterface.(*Client)
	if !ok {
		return nil, 0, fmt.Errorf("canonical path resolve requires *Client transport")
	}

	u, err := canonicalLocatorURL(underlying.Server, host, ownerPath, name)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")

	for _, e := range underlying.RequestEditors {
		if err := e(ctx, req); err != nil {
			return nil, 0, err
		}
	}
	for _, e := range reqEditors {
		if err := e(ctx, req); err != nil {
			return nil, 0, err
		}
	}

	rsp, err := underlying.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer rsp.Body.Close()
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, rsp.StatusCode, err
	}
	if rsp.StatusCode != http.StatusOK {
		return nil, rsp.StatusCode, nil
	}
	var meta CanonicalInterfaceMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, rsp.StatusCode, err
	}
	return &meta, rsp.StatusCode, nil
}

// canonicalLocatorURL returns the /i/{owner}/{name}?json locator for a ref on host.
// Refs on the default host resolve against the hub serving apiURL ($IFC_HUB_URL), so
// a dev or staging token is never sent to production. Other hosts are called directly.
func canonicalLocatorURL(apiURL, host, ownerPath, name string) (*url.URL, error) {
	u := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     fmt.Sprintf("/i/%s/%s", ownerPath, name),
		RawQuery: "json",
	}
	if strings.HasPrefix(host, "localhost") {
		u.Scheme = "http"
	}
	if host == internal.DefaultAPIHost {
		hub, err := internal.HubURLFromAPIURL(apiURL)
		if err != nil {
			return nil, fmt.Errorf("invalid hub URL %q: %w", apiURL, err)
		}
		u.Scheme, u.Host, u.Path = hub.Scheme, hub.Host, hub.Path+u.Path
	}
	return u, nil
}
