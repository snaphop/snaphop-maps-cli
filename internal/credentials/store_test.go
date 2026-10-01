package credentials

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const service = "https://maps.snaphop.ai"

func TestDefaultPath(t *testing.T) {
	t.Parallel()
	path, err := DefaultPath(func() (string, error) { return "/home/agent/.config", nil })
	if err != nil || path != filepath.Join("/home/agent/.config", "snaphop-maps", "credentials.json") {
		t.Fatalf("DefaultPath = %q, %v", path, err)
	}
	if _, err := DefaultPath(func() (string, error) { return "", errors.New("no home") }); err == nil ||
		!strings.Contains(err.Error(), "no home") {
		t.Fatalf("DefaultPath error = %v", err)
	}
}

func TestGetFromMissingFile(t *testing.T) {
	t.Parallel()
	store := Store{Path: filepath.Join(t.TempDir(), "absent", "credentials.json")}
	if _, ok, err := store.Get(service); ok || err != nil {
		t.Fatalf("Get = %v, %v", ok, err)
	}
}

func TestPutKeepsEveryServiceApartAndPrivate(t *testing.T) {
	t.Parallel()
	store := Store{Path: filepath.Join(t.TempDir(), "nested", "snaphop-maps", "credentials.json")}
	production := Account{APIKey: "shk_production", KeyID: "k1", ExpiresAt: "2026-10-30T00:00:00Z", Scopes: []string{"READ_MAPS"}, AgentID: "a1", WorkspaceID: "w1", SavedAt: "2026-09-30T00:00:00Z"}
	local := Account{APIKey: "shk_local"}
	if err := store.Put(service, production); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("http://localhost:8080", local); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(service)
	if err != nil || !ok || !reflect.DeepEqual(got, production) {
		t.Fatalf("Get = %+v, %v, %v", got, ok, err)
	}
	got, ok, err = store.Get("http://localhost:8080")
	if err != nil || !ok || got.APIKey != "shk_local" {
		t.Fatalf("Get local = %+v, %v, %v", got, ok, err)
	}
	info, err := os.Stat(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(store.Path))
	if len(entries) != 2 || entries[0].Name() != "credentials.json" || entries[1].Name() != "credentials.json.lock" {
		t.Fatalf("the directory holds %v, want the file and its lock", entries)
	}
}

func TestLoadRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	directory := filepath.Join(dir, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cases := map[string]struct {
		path string
		want string
	}{
		"directory":            {directory, "cannot read"},
		"not json":             {write("garbage.json", "not json"), "is not a credentials file"},
		"null":                 {write("null.json", "null"), "is not a credentials file"},
		"a version in words":   {write("words.json", `{"version": "one"}`), "is not a credentials file"},
		"accounts not by name": {write("list.json", `{"version": 1, "accounts": []}`), "is not a credentials file"},
		"no version":           {write("v0.json", `{"accounts": {}}`), "format version 0"},
		"newer file":           {write("v2.json", `{"version": 2, "accounts": {}}`), "format version 2"},
		"an unreadable account": {write("account.json", `{"version": 1, "accounts": {"`+service+`": "shk_bare"}}`),
			"holds an account for " + service + " that cannot be read"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := Store{Path: tc.path}
			if _, _, err := store.Get(service); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Get error = %v, want %q", err, tc.want)
			}
			if err := store.Put(service, Account{APIKey: "k"}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Put error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadAcceptsAFileWithoutAccounts(t *testing.T) {
	t.Parallel()
	for _, content := range []string{`{"version": 1}`, `{"version": 1, "accounts": null}`} {
		path := filepath.Join(t.TempDir(), "credentials.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		store := Store{Path: path}
		if err := store.Put(service, Account{APIKey: "k"}); err != nil {
			t.Fatal(err)
		}
		if got, ok, _ := store.Get(service); !ok || got.APIKey != "k" {
			t.Fatalf("%s: Get = %+v, %v", content, got, ok)
		}
	}
}

func TestPutReportsEachFailedStep(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	failing := errors.New("disk full")
	cases := map[string]struct {
		store Store
		want  string
	}{
		"directory": {Store{Path: filepath.Join(dir, "d", "credentials.json"),
			mkdirAll: func(string, os.FileMode) error { return failing }}, "cannot create"},
		"directory is a file": {Store{Path: filepath.Join(dir, "f", "credentials.json"),
			mkdirAll: func(string, os.FileMode) error { return nil }}, "cannot lock"},
		"lock": {Store{Path: filepath.Join(dir, "l", "credentials.json"),
			tryLock: func(*os.File, bool) (bool, error) { return false, failing }}, "cannot lock"},
		// A name the file system takes, but not with the lock's suffix added.
		"lock file": {Store{Path: filepath.Join(dir, "n", strings.Repeat("c", 252))}, "cannot lock"},
		"write": {Store{Path: filepath.Join(dir, "w", "credentials.json"),
			write: func(*os.Root, string, []byte, os.FileMode) error { return failing }}, "cannot write"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := tc.store.Put(service, Account{APIKey: "k"}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Put error = %v, want %q", err, tc.want)
			}
			if err := tc.store.Check(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Check error = %v, want %q", err, tc.want)
			}
			if _, err := os.Stat(tc.store.Path); err == nil {
				t.Fatal("a credentials file was written")
			}
		})
	}
}

func TestCheckRewritesTheFileAsItIs(t *testing.T) {
	t.Parallel()
	store := Store{Path: filepath.Join(t.TempDir(), "new", "credentials.json")}
	if err := store.Check(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(store.Path); err != nil || string(data) != "{\n  \"accounts\": {},\n  \"version\": 1\n}\n" {
		t.Fatalf("Check wrote %q, %v", data, err)
	}
	if err := store.Put(service, Account{APIKey: "shk_kept"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Check(); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := store.Get(service); !ok || err != nil || got.APIKey != "shk_kept" {
		t.Fatalf("after Check, Get = %+v, %v, %v", got, ok, err)
	}
}

// TestFieldsOfANewerBuildSurvive is a file a newer build wrote, with fields this one does not know.
func TestFieldsOfANewerBuildSurvive(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "credentials.json")
	const newer = `{"version": 1, "note": "kept",
	  "accounts": {"` + service + `": {"apiKey": "shk_old", "agentId": "a1", "label": "mine"},
	               "https://other.example": {"apiKey": "shk_other", "future": [1, 2]}}}`
	if err := os.WriteFile(path, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path}
	if err := store.Update(service, func(kept Account, _ bool) (Account, error) {
		kept.APIKey = "shk_new"
		return kept, nil
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{`"note": "kept"`, `"label": "mine"`, `"apiKey": "shk_new"`, `"future": [`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("the file lost %s: %s", want, data)
		}
	}
	// Another agent's account keeps nothing of the one it replaces.
	if err := store.Put(service, Account{APIKey: "shk_another", AgentID: "a2"}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "label") {
		t.Fatalf("another agent's account kept the old one's fields: %s", data)
	}
}

// TestALockLinkIsNotFollowed is a lock file that is a link to a file outside the credentials file's
// directory: taking the lock must not create or lock that file.
func TestALockLinkIsNotFollowed(t *testing.T) {
	t.Parallel()
	dir, outside := t.TempDir(), t.TempDir()
	store := Store{Path: filepath.Join(dir, "credentials.json")}
	if err := store.Put(service, Account{APIKey: "shk_kept"}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "victim")
	if err := os.Remove(store.Path + ".lock"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, store.Path+".lock"); err != nil {
		t.Skipf("cannot make a link here: %v", err)
	}
	if err := store.Put(service, Account{APIKey: "shk_new"}); err == nil || !strings.Contains(err.Error(), "cannot lock") {
		t.Fatalf("Put error = %v", err)
	}
	if _, err := os.Lstat(target); err == nil {
		t.Fatal("the link's target was created")
	}
	// A reader that cannot open the lock reads the file as it is: no writer can replace it either.
	if got, ok, err := store.Get(service); !ok || err != nil || got.APIKey != "shk_kept" {
		t.Fatalf("Get = %+v, %v, %v", got, ok, err)
	}
}

// TestALockThatCannotBeOpened is a lock file that is a directory, as a mistake could leave it on any
// system: a writer is refused, and a reader reads the file as it is, since no writer can replace it.
func TestALockThatCannotBeOpened(t *testing.T) {
	t.Parallel()
	store := Store{Path: filepath.Join(t.TempDir(), "credentials.json")}
	if err := store.Put(service, Account{APIKey: "shk_kept"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.Path + ".lock"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.Path+".lock", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(service, Account{APIKey: "shk_new"}); err == nil || !strings.Contains(err.Error(), "cannot lock") {
		t.Fatalf("Put error = %v", err)
	}
	if got, ok, err := store.Get(service); !ok || err != nil || got.APIKey != "shk_kept" {
		t.Fatalf("Get = %+v, %v, %v", got, ok, err)
	}
}

// TestAHeldLockIsWaitedForAWhile holds the lock as a stuck command would.
func TestAHeldLockIsWaitedForAWhile(t *testing.T) {
	t.Parallel()
	store := Store{Path: filepath.Join(t.TempDir(), "credentials.json"), Wait: 50 * time.Millisecond}
	if err := store.Put(service, Account{APIKey: "shk_kept"}); err != nil {
		t.Fatal(err)
	}
	holder, err := os.OpenFile(store.Path+".lock", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if locked, err := tryLockFile(holder, true); !locked || err != nil {
		t.Skipf("this system cannot lock: %v, %v", locked, err)
	}
	for name, call := range map[string]func() error{
		"Put": func() error { return store.Put(service, Account{APIKey: "shk_new"}) },
		"Get": func() error { _, _, err := store.Get(service); return err },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "held its lock for over 50ms") {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	patient := Store{Path: store.Path}
	done := make(chan error)
	go func() { done <- patient.Put(service, Account{APIKey: "shk_new"}) }()
	time.Sleep(100 * time.Millisecond)
	unlockFile(holder)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got, _, _ := patient.Get(service); got.APIKey != "shk_new" {
		t.Fatalf("kept %+v", got)
	}
}

func TestTryLockReportsAFileItCannotLock(t *testing.T) {
	t.Parallel()
	file, err := os.Create(filepath.Join(t.TempDir(), "lock"))
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if locked, err := tryLockFile(file, true); locked || err == nil {
		t.Fatalf("tryLockFile on a closed file = %v, %v", locked, err)
	}
}

func TestUpdateDecidesOnWhatIsKeptNow(t *testing.T) {
	t.Parallel()
	store := Store{Path: filepath.Join(t.TempDir(), "credentials.json")}
	if err := store.Put(service, Account{APIKey: "shk_first", AgentID: "a1"}); err != nil {
		t.Fatal(err)
	}
	err := store.Update(service, func(kept Account, found bool) (Account, error) {
		if !found || kept.APIKey != "shk_first" {
			t.Errorf("Update was given %+v, %v", kept, found)
		}
		kept.APIKey = "shk_second"
		return kept, nil
	})
	if got, _, _ := store.Get(service); err != nil || got.APIKey != "shk_second" || got.AgentID != "a1" {
		t.Fatalf("kept %+v, %v", got, err)
	}
	refused := errors.New("another key is kept")
	err = store.Update(service, func(Account, bool) (Account, error) { return Account{APIKey: "shk_third"}, refused })
	if got, _, _ := store.Get(service); !errors.Is(err, refused) || got.APIKey != "shk_second" {
		t.Fatalf("a refused change kept %+v, %v", got, err)
	}
	err = store.Update("http://localhost:1", func(kept Account, found bool) (Account, error) {
		if found {
			t.Errorf("Update found %+v for another service", kept)
		}
		return Account{APIKey: "shk_local"}, nil
	})
	if got, _, _ := store.Get(service); err != nil || got.APIKey != "shk_second" {
		t.Fatalf("keeping another service's account changed this one: %+v, %v", got, err)
	}
}

// TestCommandsKeepingKeysAtOnceLoseNone is the race that parallel register-agent runs meet: every
// writer reads the file, adds its account and replaces the file. Without the lock, the last to replace
// it drops the accounts the others added.
func TestCommandsKeepingKeysAtOnceLoseNone(t *testing.T) {
	t.Parallel()
	store := Store{Path: filepath.Join(t.TempDir(), "credentials.json")}
	const writers = 16
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Put(fmt.Sprintf("https://service-%d", i), Account{APIKey: fmt.Sprintf("shk_%d", i)}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for i := range writers {
		if got, ok, err := store.Get(fmt.Sprintf("https://service-%d", i)); !ok || err != nil || got.APIKey != fmt.Sprintf("shk_%d", i) {
			t.Errorf("service %d kept %+v, %v, %v", i, got, ok, err)
		}
	}
}

// TestALockFileSaidToBeMissingIsOpenedAgain is macOS answering that a lock file it was asked to create
// does not exist, as it can while other commands create it at once: the open is tried again, and only
// an answer that lasts past the wait refuses the command.
func TestALockFileSaidToBeMissingIsOpenedAgain(t *testing.T) {
	t.Parallel()
	missing := &fs.PathError{Op: "openat", Path: "credentials.json.lock", Err: fs.ErrNotExist}
	opens := 0
	store := Store{Path: filepath.Join(t.TempDir(), "credentials.json"),
		openLock: func(root *os.Root, name string) (*os.File, error) {
			if opens++; opens < 3 {
				return nil, missing
			}
			return openLockFile(root, name)
		}}
	if err := store.Put(service, Account{APIKey: "shk_kept"}); err != nil || opens != 3 {
		t.Fatalf("Put error = %v after %d opens", err, opens)
	}
	if got, ok, err := store.Get(service); !ok || err != nil || got.APIKey != "shk_kept" {
		t.Fatalf("Get = %+v, %v, %v", got, ok, err)
	}
	gone := Store{Path: store.Path, Wait: 50 * time.Millisecond,
		openLock: func(*os.Root, string) (*os.File, error) { return nil, missing }}
	if err := gone.Put(service, Account{APIKey: "shk_new"}); !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "cannot lock") {
		t.Fatalf("Put error = %v", err)
	}
}

// TestAFileSystemThatCannotLockIsStillRead is a credentials file on a file system without locks, such
// as some network ones: no writer can take the lock to replace the file, so a reader reads it as it is.
func TestAFileSystemThatCannotLockIsStillRead(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := (Store{Path: path}).Put(service, Account{APIKey: "shk_kept"}); err != nil {
		t.Fatal(err)
	}
	unsupported := Store{Path: path, tryLock: func(*os.File, bool) (bool, error) { return false, errors.New("no locks available") }}
	if got, ok, err := unsupported.Get(service); !ok || err != nil || got.APIKey != "shk_kept" {
		t.Fatalf("Get = %+v, %v, %v", got, ok, err)
	}
	if err := unsupported.Put(service, Account{APIKey: "shk_new"}); err == nil || !strings.Contains(err.Error(), "no locks available") {
		t.Fatalf("Put error = %v", err)
	}
}

// TestReadersShareTheLock holds the lock as a reader would: other readers read meanwhile, and a writer
// waits for it.
func TestReadersShareTheLock(t *testing.T) {
	t.Parallel()
	store := Store{Path: filepath.Join(t.TempDir(), "credentials.json"), Wait: 50 * time.Millisecond}
	if err := store.Put(service, Account{APIKey: "shk_kept"}); err != nil {
		t.Fatal(err)
	}
	reader, err := os.OpenFile(store.Path+".lock", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if locked, err := tryLockFile(reader, false); !locked || err != nil {
		t.Skipf("this system cannot lock: %v, %v", locked, err)
	}
	if got, ok, err := store.Get(service); !ok || err != nil || got.APIKey != "shk_kept" {
		t.Fatalf("Get = %+v, %v, %v", got, ok, err)
	}
	if err := store.Put(service, Account{APIKey: "shk_new"}); err == nil || !strings.Contains(err.Error(), "held its lock") {
		t.Fatalf("Put error = %v", err)
	}
}

// TestAnAccountRemembersEveryKeyItReplaced: a key replaced two replacements ago, still given in the
// environment, must be recognised as replaced too.
func TestAnAccountRemembersEveryKeyItReplaced(t *testing.T) {
	t.Parallel()
	account := Account{APIKey: "k0", AgentID: "a1", WorkspaceID: "w1"}
	for i := 1; i <= MaxReplacedKeys+1; i++ {
		account.Replace(Account{APIKey: fmt.Sprintf("k%d", i), KeyID: fmt.Sprintf("id%d", i), AgentID: "ignored"}, fmt.Sprintf("d%d", i-1))
	}
	if account.APIKey != fmt.Sprintf("k%d", MaxReplacedKeys+1) || account.AgentID != "a1" || account.WorkspaceID != "w1" ||
		len(account.ReplacedKeysSHA256) != MaxReplacedKeys {
		t.Fatalf("account = %+v", account)
	}
	if !account.Replaced(fmt.Sprintf("d%d", MaxReplacedKeys)) || !account.Replaced("d1") || account.Replaced("d0") {
		t.Fatalf("remembers %v", account.ReplacedKeysSHA256)
	}
}

// TestAnAccountKeptUnderAFormerSpellingIsFoundAndMoved: addresses once kept their scheme's own port,
// as in https://maps.snaphop.ai:443, and now drop it. Such an account is found, and is moved to the
// address's spelling now when it changes, with everything it held, so that no stale key is left.
func TestAnAccountKeptUnderAFormerSpellingIsFoundAndMoved(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "credentials.json")
	const former = `{"version": 1, "accounts": {
	  "http://localhost:80/maps": {"apiKey": "shk_old", "agentId": "a1", "workspaceId": "w1", "label": "mine"},
	  "https://other.example:443": {"apiKey": "shk_other"}}}`
	if err := os.WriteFile(path, []byte(former), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path}
	if got, ok, err := store.Get("http://localhost/maps"); !ok || err != nil || got.APIKey != "shk_old" {
		t.Fatalf("Get = %+v, %v, %v", got, ok, err)
	}
	for _, elsewhere := range []string{"http://localhost/other", "http://localhost:8080/maps", "ftp://localhost/maps", "%zz"} {
		if got, ok, err := store.Get(elsewhere); ok || err != nil {
			t.Fatalf("Get(%s) = %+v, %v, %v", elsewhere, got, ok, err)
		}
	}
	if err := store.Update("http://localhost/maps", func(kept Account, found bool) (Account, error) {
		kept.APIKey = "shk_new"
		return kept, nil
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{`"http://localhost/maps": {`, `"apiKey": "shk_new"`, `"agentId": "a1"`, `"workspaceId": "w1"`, `"label": "mine"`, `"https://other.example:443"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("the file lacks %s: %s", want, data)
		}
	}
	if strings.Contains(string(data), "localhost:80") || strings.Contains(string(data), "shk_old") {
		t.Fatalf("the former spelling was left behind: %s", data)
	}
	// With both spellings kept, the one now is found and changed, and the former is left as it is.
	both := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(both, []byte(`{"version": 1, "accounts": {"https://b.example": {"apiKey": "shk_now"}, "https://b.example:443": {"apiKey": "shk_former"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store = Store{Path: both}
	if got, _, _ := store.Get("https://b.example"); got.APIKey != "shk_now" {
		t.Fatalf("Get = %+v", got)
	}
	if err := store.Put("https://b.example", Account{APIKey: "shk_next"}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(both); !strings.Contains(string(data), "shk_former") || !strings.Contains(string(data), "shk_next") {
		t.Fatalf("the file holds %s", data)
	}
}
