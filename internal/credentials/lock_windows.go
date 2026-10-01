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

const (
	lockfileFailImmediately = 0x1
	lockfileExclusiveLock   = 0x2
	// errorLockViolation is ERROR_LOCK_VIOLATION: another handle holds the lock.
	errorLockViolation syscall.Errno = 33
)

// tryLockFile takes a lock on the file's first byte, exclusive or shared, if no one else holds one
// that conflicts. The system releases it if the process ends.
func tryLockFile(file *os.File, exclusive bool) (bool, error) {
	var flags uintptr = lockfileFailImmediately
	if exclusive {
		flags |= lockfileExclusiveLock
	}
	var overlapped syscall.Overlapped
	ok, _, err := procLockFileEx.Call(file.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if ok != 0 {
		return true, nil
	}
	if err == errorLockViolation {
		return false, nil
	}
	return false, err
}

func unlockFile(file *os.File) {
	var overlapped syscall.Overlapped
	_, _, _ = procUnlockFileEx.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
}
