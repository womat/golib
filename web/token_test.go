package web

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewTokenIsRandomHex(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()

	if len(a) != 64 {
		t.Errorf("len = %d, want 64", len(a))
	}
	if _, err := hex.DecodeString(a); err != nil {
		t.Errorf("not hex: %q", a)
	}
	if a == b {
		t.Error("two tokens are equal")
	}
}

// TestHashTokenHashesTheTokenOnly pins the hash to the token as sent, without
// a trailing newline - the mistake "echo token | sha256sum" makes.
func TestHashTokenHashesTheTokenOnly(t *testing.T) {
	// printf %s abc | sha256sum
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

	h := HashToken("abc")
	if got := hex.EncodeToString(h[:]); got != want {
		t.Errorf("HashToken(abc) = %s, want %s", got, want)
	}
}

func TestValidTokenName(t *testing.T) {
	for _, name := range []string{"nodered", "a", "Node-RED_2.test", strings.Repeat("x", 64)} {
		if !ValidTokenName(name) {
			t.Errorf("%q rejected", name)
		}
	}
	for _, name := range []string{"", "-lead", ".lead", "with space", "a:b", "ä", strings.Repeat("x", 65)} {
		if ValidTokenName(name) {
			t.Errorf("%q accepted", name)
		}
	}
}

func hashHex(token string) string {
	h := HashToken(token)
	return hex.EncodeToString(h[:])
}

func TestParseTokens(t *testing.T) {
	in := "# callers of this service\n\n" +
		"nodered " + hashHex("one") + "\n" +
		"  grafana\t" + hashHex("two") + "  \n"

	tokens, err := parseTokens(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 || tokens[0].Name != "nodered" || tokens[1].Name != "grafana" {
		t.Fatalf("got %+v", tokens)
	}
	if tokens[1].Hash != HashToken("two") {
		t.Error("hash of grafana does not match")
	}
}

func TestParseTokensRejectsBrokenLines(t *testing.T) {
	tests := map[string]string{
		"one field":      "nodered\n",
		"three fields":   "nodered " + hashHex("x") + " extra\n",
		"short hash":     "nodered abc123\n",
		"not hex":        "nodered " + strings.Repeat("z", 64) + "\n",
		"invalid name":   "node:red " + hashHex("x") + "\n",
		"duplicate name": "a " + hashHex("x") + "\na " + hashHex("y") + "\n",
		"duplicate hash": "a " + hashHex("x") + "\nb " + hashHex("x") + "\n",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseTokens(strings.NewReader(in)); err == nil {
				t.Error("accepted")
			}
		})
	}
}

func TestMatchToken(t *testing.T) {
	tokens := []Token{
		{Name: "nodered", Hash: HashToken("one")},
		{Name: "grafana", Hash: HashToken("two")},
	}

	if name, ok := matchToken(tokens, "two"); !ok || name != "grafana" {
		t.Errorf("matchToken(two) = %q, %v", name, ok)
	}
	if _, ok := matchToken(tokens, "three"); ok {
		t.Error("unknown token matched")
	}
	if _, ok := matchToken(nil, "one"); ok {
		t.Error("matched against no tokens")
	}
}

func TestAddAndRemoveTokenKeepComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token-hashes")
	start := "# keep me\nnodered " + hashHex("one") // no trailing newline
	if err := os.WriteFile(path, []byte(start), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := AddTokenToFile(path, "grafana", HashToken("two")); err != nil {
		t.Fatal(err)
	}
	if err := AddTokenToFile(path, "grafana", HashToken("three")); !errors.Is(err, ErrTokenExists) {
		t.Errorf("adding a taken name: err = %v, want ErrTokenExists", err)
	}
	if err := AddTokenToFile(path, "influx", HashToken("two")); err == nil {
		t.Error("adding a taken hash under a new name: accepted")
	}

	tokens, err := ReadTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 || tokens[1].Name != "grafana" || tokens[1].Hash != HashToken("two") {
		t.Fatalf("after add: %+v", tokens)
	}

	if err := RemoveTokenFromFile(path, "nodered"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTokenFromFile(path, "nodered"); !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("removing twice: err = %v, want ErrTokenNotFound", err)
	}

	content, _ := os.ReadFile(path)
	want := "# keep me\ngrafana " + hashHex("two") + "\n"
	if string(content) != want {
		t.Errorf("file =\n%s\nwant\n%s", content, want)
	}

	// The mode of the existing file survives the rewrite.
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}

	// No temporary file is left behind.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want 1", len(entries))
	}
}

func TestAddTokenCreatesFileWithMode0640(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token-hashes")

	if err := AddTokenToFile(path, "nodered", HashToken("one")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}
}

func TestAddTokenRefusesBrokenFileAndBadName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token-hashes")
	if err := os.WriteFile(path, []byte("broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := AddTokenToFile(path, "nodered", HashToken("one")); err == nil {
		t.Error("added to a broken file")
	}
	if err := AddTokenToFile(filepath.Join(t.TempDir(), "t"), "bad name", HashToken("one")); !errors.Is(err, ErrInvalidTokenName) {
		t.Errorf("err = %v, want ErrInvalidTokenName", err)
	}
	if err := RemoveTokenFromFile(filepath.Join(t.TempDir(), "missing"), "x"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestRemoveTokenRefusesBadName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token-hashes")
	start := "# alt\nnodered " + hashHex("one") + "\n"
	if err := os.WriteFile(path, []byte(start), 0o600); err != nil {
		t.Fatal(err)
	}

	// "#" would match the comment line "# alt", which has two fields as well.
	if err := RemoveTokenFromFile(path, "#"); !errors.Is(err, ErrInvalidTokenName) {
		t.Errorf("err = %v, want ErrInvalidTokenName", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != start {
		t.Errorf("file =\n%s\nwant\n%s", content, start)
	}
}

func TestWithAuthTokens(t *testing.T) {
	cfg := Config{
		Tokens: []Token{{Name: "nodered", Hash: HashToken("the-token")}},
	}

	tests := []struct {
		name       string
		header     map[string]string
		wantStatus int
		wantUser   string
	}{
		{"token", map[string]string{"Authorization": "Bearer the-token"}, http.StatusOK, "nodered"},
		{"wrong token", map[string]string{"Authorization": "Bearer other"}, http.StatusUnauthorized, ""},
		{"token in X-Api-Key", map[string]string{"X-Api-Key": "the-token"}, http.StatusUnauthorized, ""},
		{"no credentials", nil, http.StatusUnauthorized, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var user any
			h := WithAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user = r.Context().Value(userKey{})
			}), cfg)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantUser != "" && user != tt.wantUser {
				t.Errorf("user = %v, want %q", user, tt.wantUser)
			}
		})
	}
}

func TestUserFrom(t *testing.T) {
	cfg := Config{Tokens: []Token{{Name: "nodered", Hash: HashToken("the-token")}}}

	var user string
	var ok bool
	h := WithAuth(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		user, ok = UserFrom(r.Context())
	}), cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer the-token")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !ok || user != "nodered" {
		t.Errorf("UserFrom = %q, %v; want nodered, true", user, ok)
	}

	if user, ok := UserFrom(httptest.NewRequest(http.MethodGet, "/", nil).Context()); ok || user != "" {
		t.Errorf("outside WithAuth: %q, %v", user, ok)
	}
}

// TestFirstAccessGoesToInfo: with WithSuccessLevel(Debug), the first
// successful request of a caller from a client is logged at Info, the next
// ones at Debug, and another client counts as new.
func TestFirstAccessGoesToInfo(t *testing.T) {
	cfg := Config{Tokens: []Token{{Name: "nodered", Hash: HashToken("the-token")}}}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	h := WithLogging(WithAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), cfg),
		logger, WithSuccessLevel(slog.LevelDebug))

	call := func(client, auth string) {
		req := httptest.NewRequest(http.MethodGet, "/data", nil)
		req.RemoteAddr = client + ":40000"
		req.Header.Set("Authorization", auth)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	call("192.0.2.7", "Bearer the-token")
	call("192.0.2.7", "Bearer the-token")
	call("192.0.2.8", "Bearer the-token")
	call("192.0.2.9", "Bearer wrong") // rejected: a warning, not a first access

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines at Info and above, got %d:\n%s", len(lines), buf.String())
	}
	for i, want := range []string{"192.0.2.7", "192.0.2.8"} {
		if !strings.Contains(lines[i], "level=INFO") || !strings.Contains(lines[i], want) ||
			!strings.Contains(lines[i], "firstAccess=true") || !strings.Contains(lines[i], "user=nodered") {
			t.Errorf("line %d: %s", i, lines[i])
		}
	}
	if !strings.Contains(lines[2], "level=WARN") || strings.Contains(lines[2], "firstAccess") {
		t.Errorf("rejected request: %s", lines[2])
	}
}

// TestAccessLogNamesCallerAndClient: the access log written by WithLogging
// carries the client address always and the token name once WithAuth
// accepted the request.
func TestAccessLogNamesCallerAndClient(t *testing.T) {
	cfg := Config{Tokens: []Token{{Name: "nodered", Hash: HashToken("the-token")}}}

	for _, tt := range []struct {
		auth     string
		wantUser bool
	}{
		{"Bearer the-token", true},
		{"Bearer wrong", false},
	} {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		h := WithLogging(WithAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), cfg), logger)

		req := httptest.NewRequest(http.MethodGet, "/data", nil)
		req.RemoteAddr = "192.0.2.7:51234"
		req.Header.Set("Authorization", tt.auth)
		h.ServeHTTP(httptest.NewRecorder(), req)

		line := buf.String()
		if !strings.Contains(line, "request.client=192.0.2.7") {
			t.Errorf("%s: no client in %q", tt.auth, line)
		}
		if got := strings.Contains(line, "user=nodered"); got != tt.wantUser {
			t.Errorf("%s: user logged = %v, want %v: %q", tt.auth, got, tt.wantUser, line)
		}
	}
}
