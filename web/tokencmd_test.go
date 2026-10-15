package web

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tokenCmd returns a TokenCommand whose config file names tokenFile, and the
// buffers it writes to.
func tokenCmd(t *testing.T, tokenFile string) (TokenCommand, string, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	config := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(config, []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	return TokenCommand{
		App:           "demo",
		DefaultConfig: config,
		TokenFile: func(configPath string) (string, error) {
			if configPath != config {
				return "", errors.New("unexpected config " + configPath)
			}
			return tokenFile, nil
		},
		Stdout: &stdout,
		Stderr: &stderr,
	}, config, &stdout, &stderr
}

func TestTokenCommandAddListRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens")
	cmd, _, stdout, stderr := tokenCmd(t, path)

	if code := cmd.Run([]string{"add", "nodered"}); code != 0 {
		t.Fatalf("add: exit %d, stderr %s", code, stderr)
	}

	// stdout holds the token and nothing else, so it can be captured.
	token := strings.TrimSpace(stdout.String())
	tokens, err := ReadTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].Name != "nodered" || tokens[0].Hash != HashToken(token) {
		t.Fatalf("file holds %+v, token %q", tokens, token)
	}
	if !strings.Contains(stderr.String(), "Restart demo") {
		t.Errorf("stderr does not ask for a restart: %s", stderr)
	}

	stdout.Reset()
	if code := cmd.Run([]string{"list"}); code != 0 || stdout.String() != "nodered\n" {
		t.Errorf("list: exit %d, stdout %q", code, stdout)
	}

	if code := cmd.Run([]string{"add", "nodered"}); code != 1 {
		t.Errorf("adding a taken name: exit %d, want 1", code)
	}

	if code := cmd.Run([]string{"remove", "nodered"}); code != 0 {
		t.Fatalf("remove: exit %d, stderr %s", code, stderr)
	}
	if tokens, _ := ReadTokenFile(path); len(tokens) != 0 {
		t.Errorf("after remove: %+v", tokens)
	}
}

// TestTokenCommandFlagsAnywhere: -config and -dry-run work before, between
// and after the positional arguments.
func TestTokenCommandFlagsAnywhere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens")
	cmd, config, _, _ := tokenCmd(t, path)
	cmd.DefaultConfig = "/nonexistent/config.yaml"

	for _, args := range [][]string{
		{"-config", config, "add", "a"},
		{"add", "-config", config, "b"},
		{"add", "c", "-config", config},
	} {
		if code := cmd.Run(args); code != 0 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	if tokens, _ := ReadTokenFile(path); len(tokens) != 3 {
		t.Errorf("file holds %d tokens, want 3", len(tokens))
	}
}

func TestTokenCommandDryRunWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens")
	cmd, _, stdout, stderr := tokenCmd(t, path)

	if code := cmd.Run([]string{"add", "nodered", "-dry-run"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	token := strings.TrimSpace(stdout.String())
	if !strings.Contains(stderr.String(), "nodered "+hashHex(token)) {
		t.Errorf("stderr lacks the line for the file: %s", stderr)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("dry run created the token file")
	}

	// remove -dry-run leaves an existing entry in place.
	if err := AddTokenToFile(path, "grafana", HashToken("x")); err != nil {
		t.Fatal(err)
	}
	if code := cmd.Run([]string{"remove", "grafana", "-dry-run"}); code != 0 {
		t.Errorf("remove -dry-run: exit %d", code)
	}
	if tokens, _ := ReadTokenFile(path); len(tokens) != 1 {
		t.Error("remove -dry-run removed the token")
	}
	if code := cmd.Run([]string{"remove", "missing", "-dry-run"}); code != 1 {
		t.Errorf("remove -dry-run of a missing name: exit %d, want 1", code)
	}
}

// TestTokenCommandShowsNoTokenWhenItCannotWrite: a token that is printed but
// stored nowhere would be useless, so add fails before printing it.
func TestTokenCommandShowsNoTokenWhenItCannotWrite(t *testing.T) {
	for name, tokenFile := range map[string]string{
		"no token file configured": "",
		"directory does not exist": filepath.Join(t.TempDir(), "missing", "tokens"),
	} {
		t.Run(name, func(t *testing.T) {
			cmd, _, stdout, stderr := tokenCmd(t, tokenFile)

			if code := cmd.Run([]string{"add", "nodered"}); code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			if stdout.Len() != 0 {
				t.Errorf("printed a token that was not stored: %q", stdout)
			}
			if tokenFile == "" && !strings.Contains(stderr.String(), "-dry-run") {
				t.Errorf("no hint at -dry-run: %s", stderr)
			}
		})
	}
}

func TestTokenCommandUsage(t *testing.T) {
	cmd, _, _, _ := tokenCmd(t, filepath.Join(t.TempDir(), "tokens"))

	for _, args := range [][]string{nil, {"add"}, {"list", "x"}, {"rotate", "x"}, {"add", "x", "-unknown"}} {
		if code := cmd.Run(args); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	if code := cmd.Run([]string{"add", "bad name"}); code != 1 {
		t.Errorf("invalid name: exit %d, want 1", code)
	}
}

func TestTokenCommandListWithoutFile(t *testing.T) {
	cmd, _, stdout, stderr := tokenCmd(t, filepath.Join(t.TempDir(), "tokens"))

	if code := cmd.Run([]string{"list"}); code != 0 {
		t.Errorf("exit %d", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "No token file yet") {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}
}
