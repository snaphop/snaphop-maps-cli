//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package credentials

import (
	"os"
	"syscall"
)

// lockFile waits for an exclusive lock on the file. The system releases it if the process ends.
func lockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
}

func unlockFile(file *os.File) {
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
