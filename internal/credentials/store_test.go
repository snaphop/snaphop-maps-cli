package credentials

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/snaphop/snaphop-maps-cli/internal/atomicfile"
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
		"directory":  {directory, "cannot read"},
		"not json":   {write("garbage.json", "not json"), "is not a credentials file"},
		"newer file": {write("v2.json", `{"version": 2, "accounts": {}}`), "format version 2"},
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
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"version": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path}
	if err := store.Put(service, Account{APIKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := store.Get(service); !ok || got.APIKey != "k" {
		t.Fatalf("Get = %+v, %v", got, ok)
	}
}

func TestPutReportsEachFailedStep(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	failing := errors.New("disk full")
	mkdirFails := func(string, os.FileMode) error { return failing }
	writeFails := func(string, []byte, os.FileMode) error { return failing }
	lockFails := func(*os.File) error { return failing }
	cases := map[string]struct {
		store Store
		want  string
	}{
		"directory": {Store{Path: filepath.Join(dir, "d", "credentials.json"),
			mkdirAll: mkdirFails, write: atomicfile.Write, lock: lockFile}, "cannot create"},
		"lock": {Store{Path: filepath.Join(dir, "l", "credentials.json"),
			mkdirAll: os.MkdirAll, write: atomicfile.Write, lock: lockFails}, "cannot lock"},
		// A name the file system takes, but not with the lock's suffix added.
		"lock file": {Store{Path: filepath.Join(dir, "n", strings.Repeat("c", 252))}, "cannot lock"},
		"write": {Store{Path: filepath.Join(dir, "w", "credentials.json"),
			mkdirAll: os.MkdirAll, write: writeFails, lock: lockFile}, "cannot write"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := tc.store.Put(service, Account{APIKey: "k"}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Put error = %v, want %q", err, tc.want)
			}
			if _, err := os.Stat(tc.store.Path); err == nil {
				t.Fatal("a credentials file was written")
			}
		})
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
