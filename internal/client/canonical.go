package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
// via GET ?json on the API host (outside /api/v0).
func (c *ClientWithResponses) GetInterfaceByCanonicalPathWithResponse(ctx context.Context, host, ownerPath, name string, reqEditors ...RequestEditorFn) (*CanonicalInterfaceMeta, int, error) {
	underlying, ok := c.ClientInterface.(*Client)
	if !ok {
		return nil, 0, fmt.Errorf("canonical path resolve requires *Client transport")
	}

	scheme := "https"
	if strings.HasPrefix(host, "localhost") {
		scheme = "http"
	}
	u := &url.URL{
		Scheme:   scheme,
		Host:     host,
		Path:     fmt.Sprintf("/i/%s/%s", ownerPath, name),
		RawQuery: "json",
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
