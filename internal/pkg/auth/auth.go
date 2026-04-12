package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ifc7/ifc/internal"
)

//go:generate oapi-codegen --config=oapi.config.yaml ../../../interfaces/device_auth/device_auth.yaml

// Credentials represents the JWT credentials used to access the IFC API.
type Credentials struct {
	AccessToken  string
	ExpiresAt    time.Time
	RefreshToken *string
}

// CredentialsService manages getting and refreshing JWT credentials.
type CredentialsService struct {
	*ClientWithResponses
	Credentials            *Credentials
	deviceFlowHost         string
	deviceFlowClientId     string
	deviceFlowClientSecret string
	deviceFlowGrantType    string
}

// CredentialsServiceOptions are functional options that can be used to configure a CredentialsService.
type CredentialsServiceOptions func(*CredentialsService) error

// NewCredentialsService creates a new CredentialsService with the given options.
func NewCredentialsService(opts ...CredentialsServiceOptions) (*CredentialsService, error) {
	client := CredentialsService{
		deviceFlowHost:         internal.DeviceFlowURL,
		deviceFlowClientId:     internal.DeviceFlowClientId,
		deviceFlowClientSecret: internal.DeviceFlowClientSecret,
	}
	for _, o := range opts {
		if err := o(&client); err != nil {
			return nil, err
		}
	}
	var clientOpts []ClientOption
	clientOpts = append(clientOpts, WithRequestEditorFn(client.clientHeaderEditor()))
	clientWithResponses, err := NewClientWithResponses(client.deviceFlowHost, clientOpts...)
	if err != nil {
		return nil, err
	}
	client.ClientWithResponses = clientWithResponses
	return &client, nil
}

// ReadCredentials reads API credentials stored locally
func (s *CredentialsService) ReadCredentials() error {
	// TODO: where/how to read will depend on platform
	creds, err := readCredentialsFromProject()
	if err != nil {
		return err
	}
	s.Credentials = creds
	return nil
}

// WriteCredentials writes API credentials to file locally
func (s *CredentialsService) WriteCredentials() error {
	// TODO: where/how to write will depend on platform
	if s.Credentials == nil {
		return fmt.Errorf("credentials not set")
	}
	return writeCredentialsToProject(*s.Credentials)
}

// Login performs the full device flow: request codes, prompt user, poll for tokens.
func (s *CredentialsService) Login(ctx context.Context) error {
	codes, err := s.requestDeviceCodes(ctx)
	if err != nil {
		return err
	}
	if codes.VerificationUriComplete == nil {
		return fmt.Errorf("no verification uri present")
	}

	fmt.Println("\nTo log in, open this URL in your browser:")
	fmt.Println(*codes.VerificationUriComplete)
	fmt.Printf("\nEnter the code: %s\n", codes.UserCode)
	fmt.Println("\nWaiting for authorization...")

	tokens, err := s.pollForTokens(ctx, codes.DeviceCode, codes.Interval)
	if err != nil {
		return err
	}
	s.setCredentials(tokens)
	err = s.WriteCredentials()
	if err != nil {
		return err
	}
	return nil
}

// RefreshTokens exchanges a refresh token for new access tokens via Cognito.
func (s *CredentialsService) RefreshTokens(ctx context.Context) error {
	if s.Credentials == nil {
		return fmt.Errorf("credentials not set")
	}
	if s.Credentials.RefreshToken == nil {
		return fmt.Errorf("no refresh token present")
	}
	refreshClient, err := NewClientWithResponses(internal.CognitoDomain, WithRequestEditorFn(s.clientHeaderEditor()))
	if err != nil {
		return fmt.Errorf("creating refresh client: %w", err)
	}
	response, err := refreshClient.PostOAuth2TokenRefreshWithFormdataBodyWithResponse(ctx, PostOAuth2TokenRefreshFormdataRequestBody{
		ClientId:     s.deviceFlowClientId,
		GrantType:    RefreshToken,
		RefreshToken: *s.Credentials.RefreshToken,
	})
	if err != nil {
		return err
	}
	if response.StatusCode() != http.StatusOK {
		status := fmt.Sprintf("token refresh request failed (status %d)", response.StatusCode())
		if response.JSONDefault != nil {
			if response.JSONDefault.Error == nil {
				status += fmt.Sprintf(", error=%s", *response.JSONDefault.Error)
			}
			if response.JSONDefault.ErrorDescription == nil {
				status += fmt.Sprintf(", description=%s", *response.JSONDefault.ErrorDescription)
			}
		}
		return fmt.Errorf("%s: %s", status, response.Body)
	}

	tokenResponse := AccessTokenResponse{}
	err = json.Unmarshal(response.Body, &tokenResponse)
	if err != nil {
		return fmt.Errorf("parse device auth response: %w", err)
	}
	s.setCredentials(&tokenResponse)
	err = s.WriteCredentials()
	if err != nil {
		return err
	}
	return nil
}

// requestDeviceCodes requests device and user codes from the device flow token endpoint.
// Sends client_id in both query string and body: ALB may only forward one or the other to Lambda.
//
// We build url-encoded form explicitly: the oapi-codegen PostTokenFormdataRequestBody uses an unexported
// union field so runtime.MarshalForm skips it and would send an empty body.
func (s *CredentialsService) requestDeviceCodes(ctx context.Context) (*DeviceAuthorizationResponse, error) {
	form := url.Values{}
	form.Set("client_id", s.deviceFlowClientId)
	response, err := s.PostTokenWithBodyWithResponse(ctx,
		&PostTokenParams{
			ClientId:   &s.deviceFlowClientId,
			DeviceCode: nil,
			GrantType:  nil,
		},
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("device flow request: %w", err)
	}
	if response.StatusCode() != http.StatusOK {
		status := fmt.Sprintf("device flow request failed (status %d)", response.StatusCode())
		if response.JSON4XX != nil {
			if response.JSON4XX.Error != nil {
				status += fmt.Sprintf(", error=%s", *response.JSON4XX.Error)
			}
			if response.JSON4XX.ErrorDescription != nil {
				status += fmt.Sprintf(", description=%s", *response.JSON4XX.ErrorDescription)
			}
		}
		return nil, fmt.Errorf("%s: %s", status, response.Body)
	}
	deviceAuthResponse := DeviceAuthorizationResponse{}
	err = json.Unmarshal(response.Body, &deviceAuthResponse)
	if err != nil {
		return nil, fmt.Errorf("parse device auth response: %w", err)
	}
	return &deviceAuthResponse, nil
}

// applyDevicePollError maps a device-flow OAuth error to either "sleep and retry" or a fatal error.
func applyDevicePollError(oauthErr *OAuth2ErrorResponse, intervalSec *int) (waitAndRetry bool, err error) {
	if oauthErr == nil || oauthErr.Error == nil || *oauthErr.Error == "" {
		return false, nil
	}
	errMsg := *oauthErr.Error
	if oauthErr.ErrorDescription != nil && *oauthErr.ErrorDescription != "" {
		errMsg = errMsg + ": " + *oauthErr.ErrorDescription
	}
	switch *oauthErr.Error {
	case "authorization_pending":
		return true, nil
	case "slow_down":
		*intervalSec += 5
		return true, nil
	case "expired_token", "invalid_grant":
		return false, fmt.Errorf("device code expired or invalid: %s", errMsg)
	default:
		return false, fmt.Errorf("token request failed: %s", errMsg)
	}
}

// pollForTokens polls the token endpoint until the user authorizes or the device code expires.
// Parameters are duplicated in the query string and form body (see requestDeviceCodes).
func (s *CredentialsService) pollForTokens(ctx context.Context, deviceCode string, interval int) (*AccessTokenResponse, error) {
	intervalSec := interval
	if intervalSec < 1 {
		intervalSec = 5
	}

	for {
		grantType := UrnIetfParamsOauthGrantTypeDeviceCode
		form := url.Values{}
		form.Set("client_id", s.deviceFlowClientId)
		form.Set("device_code", deviceCode)
		form.Set("grant_type", string(UrnIetfParamsOauthGrantTypeDeviceCode))
		response, err := s.PostTokenWithBodyWithResponse(ctx,
			&PostTokenParams{
				ClientId:   &s.deviceFlowClientId,
				DeviceCode: &deviceCode,
				GrantType:  &grantType,
			},
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		if err != nil {
			return nil, fmt.Errorf("poll for tokens: %w", err)
		}
		if response.StatusCode() == http.StatusOK {
			tokenResponse := AccessTokenResponse{}
			err = json.Unmarshal(response.Body, &tokenResponse)
			if err != nil {
				return nil, fmt.Errorf("parse device auth response: %w", err)
			}
			if tokenResponse.AccessToken != "" {
				return &tokenResponse, nil
			}
			var oauthErr OAuth2ErrorResponse
			if json.Unmarshal(response.Body, &oauthErr) == nil {
				wait, oerr := applyDevicePollError(&oauthErr, &intervalSec)
				if oerr != nil {
					return nil, oerr
				}
				if wait {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(time.Duration(intervalSec) * time.Second):
					}
					continue
				}
			}
			return nil, fmt.Errorf("device flow returned no access_token: %s", strings.TrimSpace(string(response.Body)))
		}

		errMsg := string(response.Body)
		if response.JSON4XX != nil {
			wait, oerr := applyDevicePollError(response.JSON4XX, &intervalSec)
			if oerr != nil {
				return nil, oerr
			}
			if wait {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(intervalSec) * time.Second):
				}
				continue
			}
			return nil, fmt.Errorf("token request failed without specific error (status %d): %s", response.StatusCode(), errMsg)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(intervalSec) * time.Second):
			// continue polling
		}
	}
}

// setCredentials sets or updates the Credentials in the client from a tokenResponse.
func (s *CredentialsService) setCredentials(tokens *AccessTokenResponse) {
	if s.Credentials == nil {
		s.Credentials = &Credentials{}
	}
	s.Credentials.AccessToken = tokens.AccessToken
	s.Credentials.ExpiresAt = tokens.expiresTime()
	// refresh token is not returned by the refresh endpoint
	if tokens.RefreshToken != nil {
		s.Credentials.RefreshToken = tokens.RefreshToken
	}
}

// clientHeaderEditor returns a RequestEditorFn that injects the necessary headers.
func (s *CredentialsService) clientHeaderEditor() RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if s.deviceFlowClientSecret != "" {
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(s.deviceFlowClientId+":"+s.deviceFlowClientSecret)))
		}
		return nil
	}
}

// expiresTime returns the time at which the access token expires in a time.Time struct.
func (t *AccessTokenResponse) expiresTime() time.Time {
	return time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
}

// readCredentialsFromProject will read credentials in from a file in the local .ifc folder
func readCredentialsFromProject() (*Credentials, error) {
	file, err := os.ReadFile(internal.IfcCredentialsPath)
	if err != nil {
		return nil, err
	}
	credentials := Credentials{}
	err = json.Unmarshal(file, &credentials)
	if err != nil {
		return nil, err
	}
	return &credentials, nil
}

// writeCredentialsToProject will write credentials in to a file in the local .ifc folder
func writeCredentialsToProject(credentials Credentials) error {
	file, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(internal.IfcCredentialsPath, file, 0600)
}
