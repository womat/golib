//go:build unix

package web

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// owner returns the owner and group of path.
func owner(t *testing.T, path string) (uid, gid int) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	uid, gid, ok := fileOwner(info)
	if !ok {
		t.Fatal("no owner")
	}
	return uid, gid
}

func TestTokenCommandNewFileTakesTheConfigGroup(t *testing.T) {
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(groups, func(g int) bool { return g != os.Getegid() })
	if i < 0 {
		t.Skip("needs a supplementary group to give the config file")
	}
	group := groups[i]

	path := filepath.Join(t.TempDir(), "token-hashes")
	cmd, config, _, stderr := tokenCmd(t, path)
	if err := os.Chown(config, -1, group); err != nil {
		t.Skipf("cannot give the config file group %d: %v", group, err)
	}

	if code := cmd.Run([]string{"add", "nodered"}); code != 0 {
		t.Fatalf("add: exit %d, stderr %s", code, stderr)
	}
	if uid, gid := owner(t, path); uid != os.Geteuid() || gid != group {
		t.Errorf("token file is %d:%d, want %d:%d", uid, gid, os.Geteuid(), group)
	}
}

// A config file that belongs to the service user - an old layout - must not
// hand the token file to that user: whoever owns it can add their own token.
// Run as root (the CI does, with sudo): only root can create a file for
// someone else.
func TestTokenCommandNewFileKeepsItsCreatorAsOwnerAsRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	const service = 65534 // nobody, standing in for the service user

	path := filepath.Join(t.TempDir(), "token-hashes")
	cmd, config, _, stderr := tokenCmd(t, path)
	if err := os.Chown(config, service, service); err != nil {
		t.Fatal(err)
	}

	if code := cmd.Run([]string{"add", "nodered"}); code != 0 {
		t.Fatalf("add: exit %d, stderr %s", code, stderr)
	}
	if uid, gid := owner(t, path); uid != 0 || gid != service {
		t.Errorf("token file is %d:%d, want 0:%d", uid, gid, service)
	}
}
