//go:build !unix

package web

import (
	"io/fs"
	"os"
)

// fileOwner reports that the platform has no Unix owner and group.
func fileOwner(fs.FileInfo) (uid, gid int, ok bool) { return 0, 0, false }

// canChange checks nothing where files have no Unix permissions; it only
// reports a file that cannot be stat'ed.
func canChange(path string) (string, error) {
	_, err := os.Stat(path)
	return "", err
}
