//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package credentials

import "os"

// lockFile cannot lock on this system; commands that keep keys at once may lose one.
func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) {}
