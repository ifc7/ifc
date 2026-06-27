package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempCredentialStore(t *testing.T) *FileCredentialStore {
	t.Helper()
	return NewFileCredentialStoreAt(filepath.Join(t.TempDir(), "ifc", "credentials.json"))
}

func TestFileCredentialStore_WriteCreatesDirectoryAndFile(t *testing.T) {
	store := tempCredentialStore(t)

	refresh := "refresh-token"
	err := store.Write(&Credentials{
		AccessToken:  "access-token",
		ExpiresAt:    time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		RefreshToken: &refresh,
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(store.Path()))
	if err != nil {
		t.Fatalf("stat credentials directory: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("directory permissions = %04o, want 0700", perm)
	}

	fileInfo, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("stat credentials file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions = %04o, want 0600", perm)
	}
}

func TestFileCredentialStore_ReadWriteRoundTrip(t *testing.T) {
	store := tempCredentialStore(t)
	expiresAt := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	refresh := "refresh-token"

	want := &Credentials{
		AccessToken:  "access-token",
		ExpiresAt:    expiresAt,
		RefreshToken: &refresh,
	}
	if err := store.Write(want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := store.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.AccessToken != want.AccessToken {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, want.AccessToken)
	}
	if !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, want.ExpiresAt)
	}
	if got.RefreshToken == nil || *got.RefreshToken != refresh {
		t.Errorf("RefreshToken = %v, want %q", got.RefreshToken, refresh)
	}
}

func TestFileCredentialStore_ReadMissingFile(t *testing.T) {
	store := tempCredentialStore(t)

	_, err := store.Read()
	if err == nil {
		t.Fatal("Read() error = nil, want ErrCredentialsNotFound")
	}
	if !errors.Is(err, ErrCredentialsNotFound) {
		t.Fatalf("Read() error = %v, want ErrCredentialsNotFound", err)
	}
}

func TestFileCredentialStore_ReadInvalidJSON(t *testing.T) {
	store := tempCredentialStore(t)
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(store.Path(), []byte("{not-json"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := store.Read()
	if err == nil {
		t.Fatal("Read() error = nil, want parse error")
	}
}

func TestFileCredentialStore_WriteRejectsEmptyAccessToken(t *testing.T) {
	store := tempCredentialStore(t)

	err := store.Write(&Credentials{})
	if err == nil {
		t.Fatal("Write() error = nil, want error")
	}
}

func TestFileCredentialStore_WriteUpdatesExistingFile(t *testing.T) {
	store := tempCredentialStore(t)

	if err := store.Write(&Credentials{AccessToken: "first"}); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	if err := store.Write(&Credentials{AccessToken: "second"}); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}

	got, err := store.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.AccessToken != "second" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "second")
	}
}

func TestFileCredentialStore_JSONFieldNames(t *testing.T) {
	store := tempCredentialStore(t)
	refresh := "refresh-token"
	if err := store.Write(&Credentials{
		AccessToken:  "access-token",
		ExpiresAt:    time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		RefreshToken: &refresh,
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, key := range []string{"access_token", "expires_at", "refresh_token"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("credentials JSON missing key %q", key)
		}
	}
}
