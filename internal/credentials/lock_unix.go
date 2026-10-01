//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package credentials

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile takes an exclusive lock on the file if no one else holds it. The system releases it if
// the process ends.
func tryLockFile(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(file *os.File) {
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
