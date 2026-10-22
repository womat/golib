//go:build unix

package web

import (
	"io/fs"
	"os"
	"syscall"
)

// fileOwner returns the owner and group of info, where the platform has them.
func fileOwner(info fs.FileInfo) (uid, gid int, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(st.Uid), int(st.Gid), true
}

// wOK is W_OK of access(2), which package syscall does not name everywhere.
const wOK = 0x2

// canChange says why the running process can change path - "" if it cannot.
// Root is not asked: it can change everything.
func canChange(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	euid := os.Geteuid()
	if euid == 0 {
		return "", nil
	}
	if uid, _, ok := fileOwner(info); ok && uid == euid {
		return "belongs to the user the service runs as", nil
	}
	if syscall.Access(path, wOK) == nil {
		return "is writable by the service", nil
	}
	return "", nil
}
