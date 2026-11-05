package credentials

import "io/fs"

// exposed is false: Windows keeps who may read a file in its access control list, which a user's
// profile gives to that user alone, and a file's permission bits do not show it.
func exposed(fs.FileMode) bool { return false }
