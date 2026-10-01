// Package atomicfile replaces a file whole. The data goes to a new temporary file beside it, which is
// synced and renamed into place, so that a reader sees the old file or the new one, a crash leaves one
// of them, and a link where the file goes is replaced rather than followed.
package atomicfile

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

// Write puts data in the file at path, readable as perm says less the process's umask, as a file
// os.WriteFile creates would be.
func Write(path string, data []byte, perm os.FileMode) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	return WriteIn(root, filepath.Base(path), data, perm)
}

// WriteIn is Write for a file named relative to root, which neither it nor a link it meets can leave.
func WriteIn(root *os.Root, name string, data []byte, perm os.FileMode) error {
	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	temporary := name + ".tmp-" + hex.EncodeToString(suffix)
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = root.Rename(temporary, name)
	}
	if err != nil {
		_ = root.Remove(temporary)
		return err
	}
	// The rename lasts through a crash once the directory holding it is synced. Not every system can
	// sync a directory, and the file is in place either way.
	if dir, err := root.Open(filepath.Dir(name)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
