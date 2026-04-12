package auth_test

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/ifc7/ifc/internal/pkg/auth"
)

func TestMain(m *testing.M) {
	// run from project root
	_, filename, _, _ := runtime.Caller(0)
	dir := path.Join(path.Dir(filename), "../../..")
	err := os.Chdir(dir)
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestCredentialClient_Login(t *testing.T) {
	t.Skip("skipping test because it requires user interaction")
	client, err := auth.NewCredentialsService()
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("client is nil")
	}
	err = client.Login(t.Context())
	if err != nil {
		t.Fatal(fmt.Errorf("failed to login: %w", err))
	}
	if client.Credentials == nil {
		t.Fatal("credentials are nil")
	}
	if client.Credentials.AccessToken == "" {
		t.Fatal("access token is empty")
	}
	if client.Credentials.RefreshToken == nil {
		t.Fatal("refresh token is nil")
	}
	if *client.Credentials.RefreshToken == "" {
		t.Fatal("refresh token is empty")
	}
	if client.Credentials.ExpiresAt.IsZero() {
		t.Fatal("expiration time is zero")
	}
	err = client.RefreshTokens(t.Context())
	if err != nil {
		t.Fatal(fmt.Errorf("failed to refresh tokens: %w", err))
	}
	if client.Credentials == nil {
		t.Fatal("refresh: credentials are nil")
	}
	if client.Credentials.AccessToken == "" {
		t.Fatal("refresh: access token is empty")
	}
	if client.Credentials.RefreshToken == nil {
		t.Fatal("refresh: refresh token is nil")
	}
	if *client.Credentials.RefreshToken == "" {
		t.Fatal("refresh: refresh token is empty")
	}
	if client.Credentials.ExpiresAt.IsZero() {
		t.Fatal("refresh: expiration time is zero")
	}
}

func TestReadCredentials(t *testing.T) {

}

func TestWriteCredentials(t *testing.T) {

}
