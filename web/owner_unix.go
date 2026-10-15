//go:build unix

package web

import (
	"io/fs"
	"os"
	"syscall"
)

// copyOwner gives f the owner and group recorded in info. Without it a token
// file rewritten by root would end up as root:root, and a service reading it
// through its group would lose access.
func copyOwner(f *os.File, info fs.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(st.Uid) == os.Geteuid() && int(st.Gid) == os.Getegid() {
		return nil // already right, and needs no privilege
	}
	return f.Chown(int(st.Uid), int(st.Gid))
}

// fileOwner returns the owner and group of info, where the platform has them.
func fileOwner(info fs.FileInfo) (uid, gid int, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(st.Uid), int(st.Gid), true
}
