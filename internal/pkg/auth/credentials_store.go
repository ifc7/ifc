package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrCredentialsNotFound is returned when no credentials file exists.
var ErrCredentialsNotFound = errors.New("credentials not found")

// CredentialsPath returns the default path for stored API credentials.
func CredentialsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return filepath.Join(configDir, "ifc", "credentials.json"), nil
}

// FileCredentialStore reads and writes credentials as JSON under the user's config directory.
type FileCredentialStore struct {
	path string
}

// NewFileCredentialStore creates a store at the default credentials path.
func NewFileCredentialStore() (*FileCredentialStore, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}
	return &FileCredentialStore{path: path}, nil
}

// NewFileCredentialStoreAt creates a store at an explicit path (for tests).
func NewFileCredentialStoreAt(path string) *FileCredentialStore {
	return &FileCredentialStore{path: path}
}

// Path returns the credentials file path.
func (s *FileCredentialStore) Path() string {
	return s.path
}

// Read loads credentials from disk.
func (s *FileCredentialStore) Read() (*Credentials, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCredentialsNotFound
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	credentials := &Credentials{}
	if err := json.Unmarshal(data, credentials); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	if credentials.AccessToken == "" {
		return nil, fmt.Errorf("parse credentials: access token is empty")
	}
	return credentials, nil
}

// Write persists credentials to disk with restrictive permissions.
func (s *FileCredentialStore) Write(credentials *Credentials) error {
	if credentials == nil {
		return fmt.Errorf("credentials not set")
	}
	if credentials.AccessToken == "" {
		return fmt.Errorf("access token is empty")
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}

	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}
