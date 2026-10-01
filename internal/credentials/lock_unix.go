//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package credentials

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile takes a lock on the file, exclusive or shared, if no one else holds one that conflicts.
// The system releases it if the process ends.
func tryLockFile(file *os.File, exclusive bool) (bool, error) {
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	err := syscall.Flock(int(file.Fd()), how|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(file *os.File) {
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
