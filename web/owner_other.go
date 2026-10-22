//go:build !unix

package web

import "io/fs"

// fileOwner reports that the platform has no Unix owner and group.
func fileOwner(fs.FileInfo) (uid, gid int, ok bool) { return 0, 0, false }
