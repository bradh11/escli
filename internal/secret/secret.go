// Package secret stores API keys in the operating system's credential store:
// the macOS Keychain, the Windows Credential Manager or the Linux Secret
// Service. When that store is unavailable, as on a headless Linux server, it
// falls back to a credentials file that only the owner can read.
package secret

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bradh11/escli/internal/config"
	"github.com/zalando/go-keyring"
)

const (
	service  = "escli"
	fileName = "credentials.json"
)

// ErrNotFound means that no API key is stored for the profile.
var ErrNotFound = errors.New("no API key stored")

// Store reads and writes the API key of each profile.
type Store struct {
	dir string
}

// New returns a Store whose fallback file lives in the configuration directory dir.
func New(dir string) *Store {
	return &Store{dir: dir}
}

// Get returns the API key of profile.
func (s *Store) Get(profile string) (string, error) {
	key, kerr := keyring.Get(service, profile)
	if kerr == nil {
		return key, nil
	}
	creds, err := s.readFile()
	if err != nil {
		return "", err
	}
	if key, ok := creds[profile]; ok {
		return key, nil
	}
	if errors.Is(kerr, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return "", fmt.Errorf("%w (the system keychain is unavailable: %w)", ErrNotFound, kerr)
}

// Set stores the API key of profile and returns a description of where it went.
func (s *Store) Set(profile, key string) (string, error) {
	kerr := keyring.Set(service, profile, key)
	if kerr == nil {
		// Remove a plain-text copy left from a time when the keychain was unavailable.
		if _, err := s.deleteFromFile(profile); err != nil {
			return "", err
		}
		return "the system keychain", nil
	}
	creds, err := s.readFile()
	if err != nil {
		return "", err
	}
	creds[profile] = key
	if err := s.writeFile(creds); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s (plain text that only your user can read, because the system keychain is unavailable: %v)", s.path(), kerr), nil
}

// Delete removes the API key of profile from both the keychain and the file.
// It returns ErrNotFound when neither held a key.
func (s *Store) Delete(profile string) error {
	kerr := keyring.Delete(service, profile)
	removed, err := s.deleteFromFile(profile)
	if err != nil {
		return err
	}
	switch {
	case kerr == nil || removed:
		return nil
	case errors.Is(kerr, keyring.ErrNotFound):
		return ErrNotFound
	default:
		return kerr
	}
}

func (s *Store) path() string {
	return filepath.Join(s.dir, fileName)
}

func (s *Store) readFile() (map[string]string, error) {
	creds := map[string]string{}
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return creds, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("read %s: %w", s.path(), err)
	}
	return creds, nil
}

func (s *Store) writeFile(creds map[string]string) error {
	if len(creds) == 0 {
		err := os.Remove(s.path())
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(s.path(), append(data, '\n'))
}

func (s *Store) deleteFromFile(profile string) (bool, error) {
	creds, err := s.readFile()
	if err != nil {
		return false, err
	}
	if _, ok := creds[profile]; !ok {
		return false, nil
	}
	delete(creds, profile)
	return true, s.writeFile(creds)
}
