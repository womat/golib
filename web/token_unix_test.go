//go:build unix

package web

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckTokenFileRefusesAFileTheServiceOwns(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write every file; the check does not apply")
	}
	path := filepath.Join(t.TempDir(), "token-hashes")
	if err := os.WriteFile(path, nil, 0o440); err != nil {
		t.Fatal(err)
	}

	// Read only, but the owner can chmod it back.
	if err := CheckTokenFile(path); !errors.Is(err, ErrTokenFileWritable) {
		t.Errorf("err = %v, want ErrTokenFileWritable", err)
	}
}

func TestCheckTokenFileAcceptsAFileOfSomeoneElse(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write every file; the check does not apply")
	}
	// Owned by root and not writable by others on every Unix this runs on,
	// and so is its directory.
	const path = "/etc/hosts"
	info, err := os.Stat(path)
	if err != nil {
		t.Skip(err)
	}
	if uid, _, _ := fileOwner(info); uid != 0 || info.Mode().Perm()&0o022 != 0 {
		t.Skipf("%s is not root's alone here", path)
	}

	if err := CheckTokenFile(path); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func TestCheckTokenFileRefusesAWritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write every file; the check does not apply")
	}
	// The file is root's, but whoever can write the directory can replace it.
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	if err := os.Symlink("/etc/hosts", path); err != nil {
		t.Fatal(err)
	}

	err := CheckTokenFile(path)
	if !errors.Is(err, ErrTokenFileWritable) {
		t.Errorf("err = %v, want ErrTokenFileWritable", err)
	}
}

func TestCheckTokenFileMissingFile(t *testing.T) {
	err := CheckTokenFile(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want ErrNotExist", err)
	}
}
