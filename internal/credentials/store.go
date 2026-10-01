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
	"net/url"
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

// MaxReplacedKeys is how many replaced keys an account remembers, newest first. A key replaced longer
// ago than that is forgotten: still given elsewhere, it is sent, and refused.
const MaxReplacedKeys = 64

// Account is what the store keeps for one service address.
type Account struct {
	APIKey      string   `json:"apiKey,omitempty"`
	KeyID       string   `json:"keyId,omitempty"`
	ExpiresAt   string   `json:"expiresAt,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	AgentID     string   `json:"agentId,omitempty"`
	WorkspaceID string   `json:"workspaceId,omitempty"`
	SavedAt     string   `json:"savedAt,omitempty"`
	// ReplacedKeysSHA256 are the SHA-256, in hex, of the keys APIKey replaced, newest first, so that an
	// old key still given elsewhere is recognised as replaced however many replacements ago it was.
	ReplacedKeysSHA256 []string `json:"replacedKeysSha256,omitempty"`
}

// Replaced says whether the account's key replaced the key with this digest, directly or not.
func (a Account) Replaced(digest string) bool {
	for _, replaced := range a.ReplacedKeysSHA256 {
		if replaced == digest {
			return true
		}
	}
	return false
}

// Replace gives the account a new key, remembering the digest of the key it replaces along with
// those that key replaced.
func (a *Account) Replace(next Account, digest string) {
	replaced := append([]string{digest}, a.ReplacedKeysSHA256...)
	a.APIKey, a.KeyID, a.ExpiresAt, a.Scopes, a.SavedAt = next.APIKey, next.KeyID, next.ExpiresAt, next.Scopes, next.SavedAt
	a.ReplacedKeysSHA256 = replaced[:min(len(replaced), MaxReplacedKeys)]
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

	// mkdirAll, write, openLock and tryLock default to os.MkdirAll, atomicfile.WriteIn, openLockFile
	// and tryLockFile; tests make them fail.
	mkdirAll func(string, os.FileMode) error
	write    func(*os.Root, string, []byte, os.FileMode) error
	openLock func(root *os.Root, name string) (*os.File, error)
	tryLock  func(file *os.File, exclusive bool) (bool, error)
}

// DefaultPath is the store's path under the user's configuration directory.
func DefaultPath(configDir func() (string, error)) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", fmt.Errorf("no configuration directory: %w", err)
	}
	return filepath.Join(dir, "snaphop-maps", "credentials.json"), nil
}

// Get returns the account kept for a service address, if there is one. One kept under the address
// with its scheme's own port, as addresses were spelled before that port was dropped from them, is
// found too. It reads under a shared lock when it can take it, so that no writer replaces the file
// while it is open here, which Windows refuses, while other readers read too. Where the lock cannot be
// opened, such as in a directory it may not write to, or cannot be taken, such as on a file system
// that cannot lock, no writer can replace the file either, and it is read as it is.
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
	_, account, found, err := f.find(s.Path, service)
	return account, found, err
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
// agent's. An account found under the address's former spelling, as Get finds it, is moved to its
// spelling now.
func (s Store) Update(service string, change func(kept Account, found bool) (Account, error)) error {
	return s.modify(func(f *file) error {
		spelling, kept, found, err := f.find(s.Path, service)
		if err != nil {
			return err
		}
		account, err := change(kept, found)
		if err != nil {
			return err
		}
		f.put(service, spelling, account, found && account.AgentID == kept.AgentID)
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
	unlock, err := s.acquire(root, true)
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

// acquireToRead takes the lock, shared, for a reader where it can be had, and otherwise lets it read
// as the file is. Only a lock another command holds too long stops it.
func (s Store) acquireToRead() (func(), error) {
	root, err := os.OpenRoot(filepath.Dir(s.Path))
	if err != nil {
		return func() {}, nil
	}
	unlock, err := s.acquire(root, false)
	if errors.Is(err, errUnlockable) {
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

// errUnlockable is a lock that cannot be had here at all, as opposed to one another command holds.
var errUnlockable = errors.New("the lock cannot be had here")

// acquire waits for the lock on a file beside the credentials file, exclusive for a writer and shared
// for a reader: the credentials file itself is replaced, not changed in place, so it cannot hold a
// lock. The lock file is opened inside the credentials file's directory, and a link that leads out of
// it is refused.
func (s Store) acquire(root *os.Root, exclusive bool) (func(), error) {
	wait := DefaultWait
	if s.Wait > 0 {
		wait = s.Wait
	}
	deadline := time.Now().Add(wait)
	file, err := s.open(root, deadline)
	if err != nil {
		return nil, fmt.Errorf("cannot lock %s: %w: %w", s.Path, errUnlockable, err)
	}
	if err := s.wait(file, exclusive, wait, deadline); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("cannot lock %s: %w", s.Path, err)
	}
	return func() {
		unlockFile(file)
		_ = file.Close()
	}, nil
}

// open opens the lock file, creating it if it is missing. macOS can answer that a file it was asked to
// create does not exist while other commands create it and rename the credentials file beside it, so
// that answer is tried again until the deadline: past it, the directory itself is gone.
func (s Store) open(root *os.Root, deadline time.Time) (*os.File, error) {
	openLock := openLockFile
	if s.openLock != nil {
		openLock = s.openLock
	}
	for {
		file, err := openLock(root, filepath.Base(s.Path)+".lock")
		if !errors.Is(err, fs.ErrNotExist) || time.Now().After(deadline) {
			return file, err
		}
		time.Sleep(lockPoll)
	}
}

func openLockFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDWR|os.O_CREATE, 0o600)
}

// wait tries the lock until it is had, or until the deadline has passed: a command that holds it
// longer than wait is stuck, and waiting on would only hang this one.
func (s Store) wait(file *os.File, exclusive bool, wait time.Duration, deadline time.Time) error {
	tryLock := tryLockFile
	if s.tryLock != nil {
		tryLock = s.tryLock
	}
	for {
		locked, err := tryLock(file, exclusive)
		switch {
		case err != nil:
			return fmt.Errorf("%w: %w", errUnlockable, err)
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

// find returns the account kept for a service, and the spelling of the address it is kept under: the
// service's own, or else the one with its scheme's own port.
func (f file) find(path, service string) (string, Account, bool, error) {
	spelling := service
	raw, found := f.accounts[spelling]
	if former := formerSpelling(service); !found && former != "" {
		spelling = former
		raw, found = f.accounts[spelling]
	}
	if !found {
		return service, Account{}, false, nil
	}
	var account Account
	if err := json.Unmarshal(raw, &account); err != nil {
		return "", Account{}, false, fmt.Errorf("%s holds an account for %s that cannot be read: %w", path, spelling, err)
	}
	return spelling, account, true, nil
}

// formerSpelling is a service address with its scheme's own port, as http://localhost:80, or none
// when the address names a port already or is not http(s).
func formerSpelling(service string) string {
	u, err := url.Parse(service)
	if err != nil {
		return ""
	}
	port := map[string]string{"https": ":443", "http": ":80"}[u.Scheme]
	if port == "" || u.Port() != "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + port + u.EscapedPath()
}

// put keeps an account for a service, in place of the one kept under spelling, and with keepUnknown
// the fields of that one that this build does not know.
func (f file) put(service, spelling string, account Account, keepUnknown bool) {
	merged := map[string]json.RawMessage{}
	if keepUnknown {
		var kept map[string]json.RawMessage
		_ = json.Unmarshal(f.accounts[spelling], &kept)
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
	delete(f.accounts, spelling)
	f.accounts[service], _ = json.Marshal(merged)
}

func (f file) encode() []byte {
	f.fields["version"], _ = json.Marshal(FileVersion)
	f.fields["accounts"], _ = json.Marshal(f.accounts)
	data, _ := json.MarshalIndent(f.fields, "", "  ")
	return append(data, '\n')
}
