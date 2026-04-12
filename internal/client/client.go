package client

//TODO: make this link robust through version changes
// //go:generate oapi-codegen --config=oapi.config.yaml ../../.ifc/.local/interface_01kn3ma93qe59r0p8kw6821y2n/revision_01kn3r6n8zf3aa3rrnzmakqncn.yaml
// //go:generate oapi-codegen --config=oapi.config.yaml ../../.ifc/ifc7-rest/v0
//go:generate mockgen -source=client.ifc.go -destination=client.mock.go -package=client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ifc7/ifc/internal"
	"github.com/ifc7/ifc/internal/pkg/auth"
)

type Ifc7ApiClient struct {
	*ClientWithResponses
	credentialsService *auth.CredentialsService
}

// NewAPIClient creates an API client with bearer token injection when credentials are provided.
// If no credentials are provided, the client is created without token injection.
func NewAPIClient(ctx context.Context, opts ...ClientOption) (*Ifc7ApiClient, error) {
	client := &Ifc7ApiClient{}

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
	clientWithResponses, err := NewClientWithResponses(internal.DefaultAPIURL, opts...)
	if err != nil {
		return nil, err
	}
	client.ClientWithResponses = clientWithResponses

	return client, nil
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
