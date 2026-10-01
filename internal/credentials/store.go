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
	"reflect"
	"strings"
	"time"

	"github.com/snaphop/snaphop-maps-cli/internal/atomicfile"
)

// FileVersion is the store's format.
const FileVersion = 1

// DefaultWait is how long a command waits for another to release the file's lock. Each holds it
// for a moment only, so a longer wait means one is stuck.
const DefaultWait = 10 * time.Second

// lockPoll is how often a command waiting for the lock tries it again.
const lockPoll = 20 * time.Millisecond

// Account is what the store keeps for one service address.
type Account struct {
	APIKey      string   `json:"apiKey,omitempty"`
	KeyID       string   `json:"keyId,omitempty"`
	ExpiresAt   string   `json:"expiresAt,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	AgentID     string   `json:"agentId,omitempty"`
	WorkspaceID string   `json:"workspaceId,omitempty"`
	SavedAt     string   `json:"savedAt,omitempty"`
	// ReplacedKeySHA256 is the SHA-256, in hex, of the key APIKey replaced, so that the old key,
	// still given elsewhere, can be recognised as replaced.
	ReplacedKeySHA256 string `json:"replacedKeySha256,omitempty"`
}

// accountFields are the names Account gives its fields in the file. Any other field an account
// holds, written by a newer build, is kept as it is.
var accountFields = func() map[string]bool {
	names := map[string]bool{}
	for field := range reflect.TypeFor[Account]().Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		names[name] = true
	}
	return names
}()

// file is the credentials file: every field it holds, kept as read so that a field this build does
// not know survives a rewrite, and each account likewise.
type file struct {
	fields   map[string]json.RawMessage
	accounts map[string]json.RawMessage
}

// Store is the credentials file at Path.
type Store struct {
	Path string
	// Wait is how long to wait for another command to release the file's lock; zero is DefaultWait.
	Wait time.Duration

	// mkdirAll, write and tryLock default to os.MkdirAll, atomicfile.WriteIn and tryLockFile; tests
	// make them fail.
	mkdirAll func(string, os.FileMode) error
	write    func(*os.Root, string, []byte, os.FileMode) error
	tryLock  func(*os.File) (bool, error)
}

// DefaultPath is the store's path under the user's configuration directory.
func DefaultPath(configDir func() (string, error)) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", fmt.Errorf("no configuration directory: %w", err)
	}
	return filepath.Join(dir, "snaphop-maps", "credentials.json"), nil
}

// Get returns the account kept for a service address, if there is one. It reads under the lock when
// it can take it, so that no writer replaces the file while it is open here, which Windows refuses.
// Where the lock cannot even be opened, such as in a directory it may not write to, no writer can
// replace the file either, and it is read as it is.
func (s Store) Get(service string) (Account, bool, error) {
	unlock, err := s.acquireToRead()
	if err != nil {
		return Account{}, false, err
	}
	defer unlock()
	f, err := s.load()
	if err != nil {
		return Account{}, false, err
	}
	return f.account(s.Path, service)
}

// Put keeps an account for a service address, replacing any kept for it, and leaves every other
// address's account as it was.
func (s Store) Put(service string, account Account) error {
	return s.Update(service, func(Account, bool) (Account, error) { return account, nil })
}

// Update changes the account kept for a service address, and leaves every other address's account as
// it was. change is given the account kept now, if any, and returns the one to keep, or an error that
// leaves the file as it was. The file's lock is held from reading the file to replacing it, so that
// commands keeping keys at the same time never lose one, and change decides on what is kept now. The
// fields of the kept account this build does not know are kept when the account kept is the same
// agent's.
func (s Store) Update(service string, change func(kept Account, found bool) (Account, error)) error {
	return s.modify(func(f *file) error {
		kept, found, err := f.account(s.Path, service)
		if err != nil {
			return err
		}
		account, err := change(kept, found)
		if err != nil {
			return err
		}
		f.put(service, account, found && account.AgentID == kept.AgentID)
		return nil
	})
}

// Check proves that a key could be kept now: it rewrites the file as it is, under its lock, creating
// the file and its directory when they are missing. It is for before a key is issued, when failing
// costs nothing.
func (s Store) Check() error {
	return s.modify(func(*file) error { return nil })
}

// modify reads the file under its lock, changes it, and replaces it whole.
func (s Store) modify(change func(*file) error) error {
	mkdirAll, write := os.MkdirAll, atomicfile.WriteIn
	if s.mkdirAll != nil {
		mkdirAll = s.mkdirAll
	}
	if s.write != nil {
		write = s.write
	}
	dir := filepath.Dir(s.Path)
	if err := mkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("cannot lock %s: %w", s.Path, err)
	}
	defer root.Close()
	unlock, err := s.acquire(root)
	if err != nil {
		return err
	}
	defer unlock()
	f, err := s.load()
	if err != nil {
		return err
	}
	if err := change(&f); err != nil {
		return err
	}
	if err := write(root, filepath.Base(s.Path), f.encode(), 0o600); err != nil {
		return fmt.Errorf("cannot write %s: %w", s.Path, err)
	}
	return nil
}

// acquireToRead takes the lock for a reader where it can be opened, and otherwise lets it read as
// the file is.
func (s Store) acquireToRead() (func(), error) {
	root, err := os.OpenRoot(filepath.Dir(s.Path))
	if err != nil {
		return func() {}, nil
	}
	unlock, err := s.acquire(root)
	if errors.Is(err, errNoLockFile) {
		unlock, err = func() {}, nil
	}
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return func() {
		unlock()
		_ = root.Close()
	}, nil
}

var errNoLockFile = errors.New("the lock file cannot be opened")

// acquire waits for the lock that every writer holds, on a file beside the credentials file: the
// credentials file itself is replaced, not changed in place, so it cannot hold a lock. The lock file
// is opened inside the credentials file's directory, and a link that leads out of it is refused.
func (s Store) acquire(root *os.Root) (func(), error) {
	file, err := root.OpenFile(filepath.Base(s.Path)+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot lock %s: %w: %w", s.Path, errNoLockFile, err)
	}
	if err := s.wait(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("cannot lock %s: %w", s.Path, err)
	}
	return func() {
		unlockFile(file)
		_ = file.Close()
	}, nil
}

// wait tries the lock until it is had, or until Wait has passed: a command that holds it longer is
// stuck, and waiting on would only hang this one.
func (s Store) wait(file *os.File) error {
	tryLock, wait := tryLockFile, DefaultWait
	if s.tryLock != nil {
		tryLock = s.tryLock
	}
	if s.Wait > 0 {
		wait = s.Wait
	}
	deadline := time.Now().Add(wait)
	for {
		locked, err := tryLock(file)
		switch {
		case err != nil:
			return err
		case locked:
			return nil
		case time.Now().After(deadline):
			return fmt.Errorf("another command has held its lock for over %s", wait)
		}
		time.Sleep(lockPoll)
	}
}

func (s Store) load() (file, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return file{fields: map[string]json.RawMessage{}, accounts: map[string]json.RawMessage{}}, nil
	}
	if err != nil {
		return file{}, fmt.Errorf("cannot read %s: %w", s.Path, err)
	}
	f := file{accounts: map[string]json.RawMessage{}}
	var version int
	err = json.Unmarshal(data, &f.fields)
	if err == nil && f.fields == nil {
		err = errors.New("it is null")
	}
	if raw, ok := f.fields["version"]; err == nil && ok {
		err = json.Unmarshal(raw, &version)
	}
	if raw, ok := f.fields["accounts"]; err == nil && ok {
		err = json.Unmarshal(raw, &f.accounts)
	}
	if err != nil {
		return file{}, fmt.Errorf("%s is not a credentials file: %w", s.Path, err)
	}
	if version != FileVersion {
		return file{}, fmt.Errorf("%s has format version %d; this build reads version %d", s.Path, version, FileVersion)
	}
	if f.accounts == nil {
		f.accounts = map[string]json.RawMessage{}
	}
	return f, nil
}

func (f file) account(path, service string) (Account, bool, error) {
	raw, found := f.accounts[service]
	if !found {
		return Account{}, false, nil
	}
	var account Account
	if err := json.Unmarshal(raw, &account); err != nil {
		return Account{}, false, fmt.Errorf("%s holds an account for %s that cannot be read: %w", path, service, err)
	}
	return account, true, nil
}

// put keeps an account for a service, and with keepUnknown the fields of the one kept now that this
// build does not know.
func (f file) put(service string, account Account, keepUnknown bool) {
	merged := map[string]json.RawMessage{}
	if keepUnknown {
		var kept map[string]json.RawMessage
		_ = json.Unmarshal(f.accounts[service], &kept)
		for name, value := range kept {
			if !accountFields[name] {
				merged[name] = value
			}
		}
	}
	// A struct of strings always encodes, as an object.
	known, _ := json.Marshal(account)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(known, &fields)
	for name, value := range fields {
		merged[name] = value
	}
	f.accounts[service], _ = json.Marshal(merged)
}

func (f file) encode() []byte {
	f.fields["version"], _ = json.Marshal(FileVersion)
	f.fields["accounts"], _ = json.Marshal(f.accounts)
	data, _ := json.MarshalIndent(f.fields, "", "  ")
	return append(data, '\n')
}
