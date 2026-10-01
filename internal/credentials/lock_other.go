//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package credentials

import "os"

// tryLockFile cannot lock on this system; commands that keep keys at once may lose one.
func tryLockFile(*os.File) (bool, error) { return true, nil }

func unlockFile(*os.File) {}
