package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteReplacesTheFileWhole(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.json")
	for _, content := range []string{"first", "second"} {
		if err := Write(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if data, err := os.ReadFile(path); err != nil || string(data) != content {
			t.Fatalf("file holds %q, %v; want %q", data, err, content)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestWriteSetsTheModeAskedFor(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := Write(path, []byte("skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestWriteReplacesALinkInsteadOfFollowingIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("cannot make a link here: %v", err)
	}
	if err := Write(link, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(victim); string(data) != "keep me" {
		t.Fatalf("the link's target now holds %q", data)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the link is still a link: %v", err)
	}
}

func TestWriteInCannotLeaveItsRoot(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	inside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(inside, "away")); err != nil {
		t.Skipf("cannot make a link here: %v", err)
	}
	root, err := os.OpenRoot(inside)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := WriteIn(root, filepath.Join("away", "file"), []byte("x"), 0o600); err == nil {
		t.Fatal("wrote through a link that leaves the root")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote outside the root: %v", entries)
	}
}

func TestWriteReportsEachFailure(t *testing.T) {
	t.Parallel()
	cases := map[string]func(dir string) string{
		// No directory to write in.
		"no directory": func(dir string) string { return filepath.Join(dir, "missing", "file") },
		// A name the file system takes, but not with the temporary file's suffix added.
		"name too long": func(dir string) string { return filepath.Join(dir, strings.Repeat("c", 240)) },
		// A directory in the way, which a file cannot be renamed over.
		"directory in the way": func(dir string) string {
			if err := os.MkdirAll(filepath.Join(dir, "occupied", "child"), 0o700); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(dir, "occupied")
		},
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := Write(path(dir), []byte("x"), 0o600); err == nil {
				t.Fatal("no error")
			}
			if leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*")); len(leftovers) != 0 {
				t.Fatalf("temporary files left behind: %v", leftovers)
			}
		})
	}
}
