//go:build !unix

package web

import (
	"io/fs"
	"os"
)

// copyOwner does nothing where files have no Unix owner and group.
func copyOwner(*os.File, fs.FileInfo) error { return nil }

// fileOwner reports that the platform has no Unix owner and group.
func fileOwner(fs.FileInfo) (uid, gid int, ok bool) { return 0, 0, false }
