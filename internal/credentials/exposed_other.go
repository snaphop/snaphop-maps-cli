//go:build !windows

package credentials

import "io/fs"

// exposed says whether others than a file's owner may read or write it, by its permissions.
func exposed(mode fs.FileMode) bool { return mode.Perm()&0o077 != 0 }
