// Package credentials keeps an agent's API key somewhere that outlasts the conversation that
// obtained it. The key is shown once and is the account's only credential, so the store is a
// file readable only by its owner, written whole and renamed into place, holding one account per
// service address so that a key is only ever sent to the service that issued it.
package credentials

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileVersion is the store's format.
const FileVersion = 1

// Account is what the store keeps for one service address.
type Account struct {
	APIKey      string   `json:"apiKey,omitempty"`
	KeyID       string   `json:"keyId,omitempty"`
	ExpiresAt   string   `json:"expiresAt,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	AgentID     string   `json:"agentId,omitempty"`
	WorkspaceID string   `json:"workspaceId,omitempty"`
	SavedAt     string   `json:"savedAt,omitempty"`
}

type file struct {
	Version  int                `json:"version"`
	Accounts map[string]Account `json:"accounts"`
}

// Store is the credentials file at Path.
type Store struct {
	Path string

	// mkdirAll, writeFile and rename default to the os package's; tests make them fail.
	mkdirAll  func(string, os.FileMode) error
	writeFile func(string, []byte, os.FileMode) error
	rename    func(string, string) error
}

// DefaultPath is the store's path under the user's configuration directory.
func DefaultPath(configDir func() (string, error)) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", fmt.Errorf("no configuration directory: %w", err)
	}
	return filepath.Join(dir, "snaphop-maps", "credentials.json"), nil
}

// Get returns the account kept for a service address, if there is one.
func (s Store) Get(service string) (Account, bool, error) {
	f, err := s.load()
	if err != nil {
		return Account{}, false, err
	}
	account, ok := f.Accounts[service]
	return account, ok, nil
}

// Put keeps an account for a service address, replacing any kept for it, and leaves every other
// address's account as it was.
func (s Store) Put(service string, account Account) error {
	f, err := s.load()
	if err != nil {
		return err
	}
	f.Accounts[service] = account
	// A struct of strings always encodes.
	data, _ := json.MarshalIndent(f, "", "  ")
	mkdirAll, writeFile, rename := os.MkdirAll, os.WriteFile, os.Rename
	if s.mkdirAll != nil {
		mkdirAll, writeFile, rename = s.mkdirAll, s.writeFile, s.rename
	}
	dir := filepath.Dir(s.Path)
	if err := mkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	temporary := s.Path + ".tmp-" + hex.EncodeToString(suffix)
	if err := writeFile(temporary, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("cannot write %s: %w", temporary, err)
	}
	if err := rename(temporary, s.Path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("cannot replace %s: %w", s.Path, err)
	}
	return nil
}

func (s Store) load() (file, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return file{Version: FileVersion, Accounts: map[string]Account{}}, nil
	}
	if err != nil {
		return file{}, fmt.Errorf("cannot read %s: %w", s.Path, err)
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return file{}, fmt.Errorf("%s is not a credentials file: %w", s.Path, err)
	}
	if f.Version != FileVersion {
		return file{}, fmt.Errorf("%s has format version %d; this build reads version %d", s.Path, f.Version, FileVersion)
	}
	if f.Accounts == nil {
		f.Accounts = map[string]Account{}
	}
	return f, nil
}
