package credentials

import (
	"os"
	"syscall"
	"unsafe"
)

// kernel32.dll is a known DLL, always loaded from the system directory.
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const lockfileExclusiveLock = 0x2

// lockFile waits for an exclusive lock on the file's first byte. The system releases it if the
// process ends.
func lockFile(file *os.File) error {
	var overlapped syscall.Overlapped
	ok, _, err := procLockFileEx.Call(file.Fd(), lockfileExclusiveLock, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if ok == 0 {
		return err
	}
	return nil
}

func unlockFile(file *os.File) {
	var overlapped syscall.Overlapped
	_, _, _ = procUnlockFileEx.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
}
