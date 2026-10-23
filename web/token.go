package web

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Token is one bearer token a service accepts, identified by the name of its
// caller. Only the SHA-256 hash of the token is kept: whoever reads the token
// file learns nothing that lets them call the service.
type Token struct {
	Name string
	Hash [sha256.Size]byte
}

var (
	// ErrTokenExists is returned when a token name is already in the file.
	ErrTokenExists = errors.New("token name already exists")
	// ErrTokenNotFound is returned when a token name is not in the file.
	ErrTokenNotFound = errors.New("token name not found")
	// ErrInvalidTokenName is returned for a name the token file cannot hold.
	ErrInvalidTokenName = errors.New("invalid token name")
	// ErrTokenFileWritable is returned by CheckTokenFile.
	ErrTokenFileWritable = errors.New("token file can be changed by the service")
)

// tokenBytes is the entropy of a generated token. 32 bytes put guessing out of
// reach, which is also why a plain SHA-256 suffices: a slow password hash only
// helps against inputs that can be enumerated.
const tokenBytes = 32

// tokenFileMode is the mode of a newly created token file. The service needs
// to read it and must not be able to write it - whoever can write it can add
// their own token.
const tokenFileMode fs.FileMode = 0o640

var tokenName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// NewToken returns a new random token, 32 bytes as 64 hex characters.
func NewToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hash of token, as stored in a token file.
func HashToken(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

// ValidTokenName reports whether name can be used for a token: 1 to 64
// letters, digits, dots, underscores and hyphens, starting with a letter or
// digit.
func ValidTokenName(name string) bool {
	return tokenName.MatchString(name)
}

// matchToken returns the name of the token whose hash matches token.
//
// Every entry is compared, in constant time, so the time taken does not
// depend on which entry matched.
func matchToken(tokens []Token, token string) (string, bool) {
	sum := HashToken(token)
	name, found := "", false
	for _, t := range tokens {
		if subtle.ConstantTimeCompare(sum[:], t.Hash[:]) == 1 {
			name, found = t.Name, true
		}
	}
	return name, found
}

// ReadTokenFile reads the tokens from a token file.
//
// The format is one token per line, "<name> <sha256 as 64 hex characters>".
// Blank lines and lines starting with # are ignored. A malformed line, or a
// name or hash that appears twice, is an error, so that a broken file stops the
// service at start instead of rejecting every request with 401 or logging the
// wrong caller.
func ReadTokenFile(path string) ([]Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read only, nothing to lose

	tokens, err := parseTokens(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return tokens, nil
}

func parseTokens(r io.Reader) ([]Token, error) {
	var tokens []Token
	seen := make(map[string]int)                   // name -> line
	seenHash := make(map[[sha256.Size]byte]string) // hash -> name

	scanner := bufio.NewScanner(r)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("line %d: want \"<name> <sha256>\", got %d fields", n, len(fields))
		}
		name, hash := fields[0], fields[1]

		if !ValidTokenName(name) {
			return nil, fmt.Errorf("line %d: %w: %q", n, ErrInvalidTokenName, name)
		}
		if first, ok := seen[name]; ok {
			return nil, fmt.Errorf("line %d: name %q already used in line %d", n, name, first)
		}
		seen[name] = n

		b, err := hex.DecodeString(hash)
		if err != nil || len(b) != sha256.Size {
			return nil, fmt.Errorf("line %d: the hash of %q is not 64 hex characters", n, name)
		}

		t := Token{Name: name}
		copy(t.Hash[:], b)

		// One token under two names would be logged as whichever matched last.
		if other, ok := seenHash[t.Hash]; ok {
			return nil, fmt.Errorf("line %d: %q has the same hash as %q in line %d", n, name, other, seen[other])
		}
		seenHash[t.Hash] = name
		tokens = append(tokens, t)
	}

	return tokens, scanner.Err()
}

// AddTokenToFile adds a token to the token file at path, creating the file if
// it does not exist. It fails with ErrTokenExists if the name is taken, and
// with an error if the hash is already in the file under another name.
//
// Comments and the order of the other lines are kept. The file is replaced
// atomically and keeps its mode, owner and group; a new file gets mode 0640.
func AddTokenToFile(path, name string, hash [sha256.Size]byte) error {
	if !ValidTokenName(name) {
		return fmt.Errorf("%w: %q", ErrInvalidTokenName, name)
	}

	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	tokens, err := parseTokens(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, t := range tokens {
		if t.Name == name {
			return fmt.Errorf("%w: %q", ErrTokenExists, name)
		}
		// Written anyway, the file would stop the service at its next start.
		if t.Hash == hash {
			return fmt.Errorf("%q has the same hash as %q", name, t.Name)
		}
	}

	if len(content) > 0 && !bytes.HasSuffix(content, []byte("\n")) {
		content = append(content, '\n')
	}
	content = fmt.Appendf(content, "%s %s\n", name, hex.EncodeToString(hash[:]))

	return replaceFile(path, content)
}

// RemoveTokenFromFile removes the token with the given name from the token
// file at path. It fails with ErrInvalidTokenName for a name no token can
// have, and with ErrTokenNotFound if there is none.
//
// Comments and the order of the other lines are kept; the file is replaced
// atomically and keeps its mode, owner and group.
func RemoveTokenFromFile(path, name string) error {
	if !ValidTokenName(name) {
		return fmt.Errorf("%w: %q", ErrInvalidTokenName, name)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := parseTokens(bytes.NewReader(content)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	var out bytes.Buffer
	removed := false
	for line := range strings.Lines(string(content)) {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == name {
			removed = true
			continue
		}
		out.WriteString(line)
	}

	if !removed {
		return fmt.Errorf("%w: %q", ErrTokenNotFound, name)
	}
	return replaceFile(path, out.Bytes())
}

// replaceFile writes content to path through a temporary file in the same
// directory and a rename, so a reader sees either the old or the new file,
// never a half written one. An existing file passes its mode, owner and group
// on to the new one.
func replaceFile(path string, content []byte) error {
	mode := tokenFileMode
	info, err := os.Stat(path)
	switch {
	case err == nil:
		mode = info.Mode().Perm()
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	// Gone after a successful rename; otherwise the half written file goes.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if err := fillAndClose(tmp, content, mode, info); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// fillAndClose writes content to f, gives it mode and the owner recorded in
// info (if any), flushes it to disk and closes it. f is closed in every case;
// the first error is returned.
func fillAndClose(f *os.File, content []byte, mode fs.FileMode, info fs.FileInfo) error {
	err := func() error {
		if _, err := f.Write(content); err != nil {
			return err
		}
		if err := f.Chmod(mode); err != nil {
			return err
		}
		if info != nil {
			if err := copyOwner(f, info); err != nil {
				return err
			}
		}
		return f.Sync()
	}()

	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// copyOwner gives f the owner and group recorded in info. Without it a token
// file rewritten by root would end up as root:root, and a service reading it
// through its group would lose access.
func copyOwner(f *os.File, info fs.FileInfo) error {
	uid, gid, ok := fileOwner(info)
	if !ok || uid == os.Geteuid() && gid == os.Getegid() {
		return nil // nothing to copy, or already right and needs no privilege
	}
	return f.Chown(uid, gid)
}

// CheckTokenFile reports, with ErrTokenFileWritable, whether the running
// process could change the token file at path - and so add a token of its own.
// It could if it owns the file or its directory, which lets it chmod them, or
// if it may write either of them; whoever can write the directory can replace
// the file. The same holds for every directory above, up to the root, because
// whoever can change one of them can swap the directory below. A service calls it at start with the token file it reads, and
// logs a failure as a warning: the service still works, but its layout lets
// a compromised service grant itself access.
//
// It checks nothing when the process runs as root, which can change every
// file anyway, and nothing on platforms without Unix permissions. A file that
// cannot be stat'ed is reported with that error. For a token file that is a
// symlink it checks the directories of the link, not of its target; see the
// README.
func CheckTokenFile(path string) error {
	for _, target := range []struct{ path, what string }{
		{path, "the file"},
		{filepath.Dir(path), "its directory"},
	} {
		reason, err := canChange(target.path)
		if err != nil {
			return err
		}
		if reason != "" {
			return fmt.Errorf("%s: %w: %s %s", path, ErrTokenFileWritable, target.what, reason)
		}
	}

	// Whoever can change a directory further up can swap the one below it.
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return err
	}
	for dir != filepath.Dir(dir) {
		dir = filepath.Dir(dir)
		reason, err := canChange(dir)
		if err != nil {
			return err
		}
		if reason != "" {
			return fmt.Errorf("%s: %w: directory %s %s", path, ErrTokenFileWritable, dir, reason)
		}
	}
	return nil
}
