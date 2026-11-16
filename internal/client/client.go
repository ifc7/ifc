package client

//TODO: make this link robust through version changes
// //go:generate oapi-codegen --config=oapi.config.yaml ../../.ifc/v0.yaml
//go:generate mockgen -source=client.ifc.go -destination=client.mock.go -package=client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ifc7/ifc/internal"
	"github.com/ifc7/ifc/internal/pkg/auth"
)

type Ifc7ApiClient struct {
	*ClientWithResponses
	credentialsService *auth.CredentialsService
}

// ErrTokenRejected is returned for any hub response with HTTP 401 while $IFC_TOKEN is in use.
var ErrTokenRejected = fmt.Errorf("%s was rejected by the hub (HTTP 401); it may have expired", internal.EnvToken)

// NewAPIClient creates a client for the hub API at $IFC_HUB_URL (default https://ifc7.dev).
// A non-empty $IFC_TOKEN is sent as the bearer token on every request; the stored login
// is then never read, written or refreshed. Otherwise the stored login is used when present.
func NewAPIClient(ctx context.Context, opts ...ClientOption) (*Ifc7ApiClient, error) {
	apiURL, err := internal.APIURL()
	if err != nil {
		return nil, err
	}
	client := &Ifc7ApiClient{}

	envToken := os.Getenv(internal.EnvToken)
	if envToken != "" {
		opts = append(opts, WithRequestEditorFn(staticBearerTokenEditor(envToken)))
	} else {
		credsService, err := auth.NewCredentialsService()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize credentials service: %w", err)
		}
		client.credentialsService = credsService

		err = client.credentialsService.ReadCredentials()
		if err != nil {
			slog.Warn("failed to read client credentials", "error", err.Error())
		} else {
			opts = append(opts, WithRequestEditorFn(client.bearerTokenEditor()))
		}
	}
	clientWithResponses, err := NewClientWithResponses(apiURL, opts...)
	if err != nil {
		return nil, err
	}
	if envToken != "" {
		if underlying, ok := clientWithResponses.ClientInterface.(*Client); ok {
			underlying.Client = rejectUnauthorized{doer: underlying.Client}
		}
	}
	client.ClientWithResponses = clientWithResponses

	return client, nil
}

// staticBearerTokenEditor sets a fixed bearer token on each request.
func staticBearerTokenEditor(token string) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// rejectUnauthorized turns HTTP 401 responses into ErrTokenRejected.
type rejectUnauthorized struct {
	doer HttpRequestDoer
}

func (r rejectUnauthorized) Do(req *http.Request) (*http.Response, error) {
	rsp, err := r.doer.Do(req)
	if err != nil {
		return nil, err
	}
	if rsp.StatusCode == http.StatusUnauthorized {
		_ = rsp.Body.Close()
		return nil, ErrTokenRejected
	}
	return rsp, nil
}

// CurrentUserID returns the ID of the authenticated user.
func (c *ClientWithResponses) CurrentUserID(ctx context.Context) (UserId, error) {
	resp, err := c.GetCurrentUserWithResponse(ctx)
	if err != nil {
		return "", fmt.Errorf("error fetching current user: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("error fetching current user: HTTP %d", resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return "", fmt.Errorf("error fetching current user: unexpected response body")
	}
	return resp.JSON200.Id, nil
}

// bearerTokenEditor returns a RequestEditorFn that injects a valid bearer token.
func (c *Ifc7ApiClient) bearerTokenEditor() RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		token, err := c.getValidToken(ctx)
		if err != nil {
			return err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return nil
	}
}

// getValidToken returns a valid access token, refreshing if expired.
func (c *Ifc7ApiClient) getValidToken(ctx context.Context) (string, error) {
	if c.credentialsService.Credentials == nil {
		// try reading config
		err := c.credentialsService.ReadCredentials()
		if err != nil {
			return "", fmt.Errorf("unable to read credentials: %w", err)
		}
		if c.credentialsService.Credentials == nil {
			return "", fmt.Errorf("no credentials set")
		}
	}
	if c.credentialsService.Credentials.ExpiresAt.Before(time.Now()) {
		err := c.credentialsService.RefreshTokens(ctx)
		if err != nil {
			return "", fmt.Errorf("unable to refresh token: %w", err)
		}
	}
	if c.credentialsService.Credentials.AccessToken == "" {
		return "", fmt.Errorf("no valid access token")
	}
	return c.credentialsService.Credentials.AccessToken, nil
}
