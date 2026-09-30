package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
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
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestLoadRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
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
		"directory":  {dir, "cannot read"},
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
	renameFails := func(string, string) error { return failing }
	cases := map[string]struct {
		store Store
		want  string
	}{
		"directory": {Store{Path: filepath.Join(dir, "d", "credentials.json"),
			mkdirAll: mkdirFails, writeFile: os.WriteFile, rename: os.Rename}, "cannot create"},
		"write": {Store{Path: filepath.Join(dir, "w", "credentials.json"),
			mkdirAll: os.MkdirAll, writeFile: writeFails, rename: os.Rename}, "cannot write"},
		"rename": {Store{Path: filepath.Join(dir, "r", "credentials.json"),
			mkdirAll: os.MkdirAll, writeFile: os.WriteFile, rename: renameFails}, "cannot replace"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := tc.store.Put(service, Account{APIKey: "k"}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Put error = %v, want %q", err, tc.want)
			}
			if leftovers, _ := filepath.Glob(tc.store.Path + ".tmp-*"); len(leftovers) != 0 {
				t.Fatalf("temporary files left behind: %v", leftovers)
			}
		})
	}
}
