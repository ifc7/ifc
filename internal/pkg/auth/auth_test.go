package auth_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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

func testCredentialStore(t *testing.T) *auth.FileCredentialStore {
	t.Helper()
	return auth.NewFileCredentialStoreAt(filepath.Join(t.TempDir(), "ifc", "credentials.json"))
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
	store := testCredentialStore(t)
	refresh := "refresh-token"
	want := &auth.Credentials{
		AccessToken:  "access-token",
		ExpiresAt:    time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		RefreshToken: &refresh,
	}
	if err := store.Write(want); err != nil {
		t.Fatalf("store.Write() error = %v", err)
	}

	service, err := auth.NewCredentialsService(auth.WithCredentialStore(store))
	if err != nil {
		t.Fatalf("NewCredentialsService() error = %v", err)
	}
	if err := service.ReadCredentials(); err != nil {
		t.Fatalf("ReadCredentials() error = %v", err)
	}
	if service.Credentials == nil {
		t.Fatal("Credentials is nil")
	}
	if service.Credentials.AccessToken != want.AccessToken {
		t.Errorf("AccessToken = %q, want %q", service.Credentials.AccessToken, want.AccessToken)
	}
	if service.Credentials.RefreshToken == nil || *service.Credentials.RefreshToken != refresh {
		t.Errorf("RefreshToken = %v, want %q", service.Credentials.RefreshToken, refresh)
	}
}

func TestWriteCredentials(t *testing.T) {
	store := testCredentialStore(t)
	refresh := "refresh-token"

	service, err := auth.NewCredentialsService(auth.WithCredentialStore(store))
	if err != nil {
		t.Fatalf("NewCredentialsService() error = %v", err)
	}
	service.Credentials = &auth.Credentials{
		AccessToken:  "access-token",
		ExpiresAt:    time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		RefreshToken: &refresh,
	}
	if err := service.WriteCredentials(); err != nil {
		t.Fatalf("WriteCredentials() error = %v", err)
	}

	got, err := store.Read()
	if err != nil {
		t.Fatalf("store.Read() error = %v", err)
	}
	if got.AccessToken != service.Credentials.AccessToken {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, service.Credentials.AccessToken)
	}
}

func TestReadCredentials_NotFound(t *testing.T) {
	store := testCredentialStore(t)

	service, err := auth.NewCredentialsService(auth.WithCredentialStore(store))
	if err != nil {
		t.Fatalf("NewCredentialsService() error = %v", err)
	}
	err = service.ReadCredentials()
	if err == nil {
		t.Fatal("ReadCredentials() error = nil, want error")
	}
}
