// Package credentials keeps an agent's API key somewhere that outlasts the conversation that
// obtained it. The key is shown once and is the account's only credential, so the store is a
// file readable only by its owner, written whole and renamed into place under a lock, holding one
// account per service address so that a key is only ever sent to the service that issued it.
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/snaphop/snaphop-maps-cli/internal/atomicfile"
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

	// mkdirAll, write and lock default to os.MkdirAll, atomicfile.Write and lockFile; tests make them fail.
	mkdirAll func(string, os.FileMode) error
	write    func(string, []byte, os.FileMode) error
	lock     func(*os.File) error
}

// DefaultPath is the store's path under the user's configuration directory.
func DefaultPath(configDir func() (string, error)) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", fmt.Errorf("no configuration directory: %w", err)
	}
	return filepath.Join(dir, "snaphop-maps", "credentials.json"), nil
}

// Get returns the account kept for a service address, if there is one. The file is only ever
// replaced whole, so it is read without the lock.
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
	return s.Update(service, func(Account, bool) (Account, error) { return account, nil })
}

// Update changes the account kept for a service address, and leaves every other address's account as
// it was. change is given the account kept now, if any, and returns the one to keep, or an error that
// leaves the file as it was. The file's lock is held from reading the file to replacing it, so that
// commands keeping keys at the same time never lose one, and change decides on what is kept now.
func (s Store) Update(service string, change func(kept Account, found bool) (Account, error)) error {
	mkdirAll, write, lock := os.MkdirAll, atomicfile.Write, lockFile
	if s.mkdirAll != nil {
		mkdirAll, write, lock = s.mkdirAll, s.write, s.lock
	}
	dir := filepath.Dir(s.Path)
	if err := mkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	unlock, err := s.acquire(lock)
	if err != nil {
		return err
	}
	defer unlock()
	f, err := s.load()
	if err != nil {
		return err
	}
	kept, found := f.Accounts[service]
	account, err := change(kept, found)
	if err != nil {
		return err
	}
	f.Accounts[service] = account
	// A struct of strings always encodes.
	data, _ := json.MarshalIndent(f, "", "  ")
	if err := write(s.Path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("cannot write %s: %w", s.Path, err)
	}
	return nil
}

// acquire waits for the lock that every writer holds, on a file beside the credentials file: the
// credentials file itself is replaced, not changed in place, so it cannot hold a lock.
func (s Store) acquire(lock func(*os.File) error) (func(), error) {
	file, err := os.OpenFile(s.Path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot lock %s: %w", s.Path, err)
	}
	if err := lock(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("cannot lock %s: %w", s.Path, err)
	}
	return func() {
		unlockFile(file)
		_ = file.Close()
	}, nil
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
