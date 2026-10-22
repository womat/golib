//go:build unix

package web

import (
	"io/fs"
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
