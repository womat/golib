package web

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// FuzzParseTokens checks that whatever a token file holds, parseTokens either
// refuses it or returns tokens that are valid, unique by name and by hash, and
// read back the same once written out again.
func FuzzParseTokens(f *testing.F) {
	f.Add("nodered " + strings.Repeat("ab", 32) + "\n")
	f.Add("# comment\n\n  grafana\t" + strings.Repeat("0F", 32) + "  \r\n")
	f.Add("a " + strings.Repeat("00", 32) + "\nb " + strings.Repeat("00", 32))
	f.Add("x y z\n")

	f.Fuzz(func(t *testing.T, in string) {
		tokens, err := parseTokens(strings.NewReader(in))
		if err != nil {
			return
		}

		names := map[string]bool{}
		hashes := map[[32]byte]bool{}
		var out strings.Builder
		for _, tok := range tokens {
			if !ValidTokenName(tok.Name) {
				t.Fatalf("invalid name %q accepted", tok.Name)
			}
			if names[tok.Name] || hashes[tok.Hash] {
				t.Fatalf("duplicate %q accepted", tok.Name)
			}
			names[tok.Name], hashes[tok.Hash] = true, true
			fmt.Fprintf(&out, "%s %s\n", tok.Name, hex.EncodeToString(tok.Hash[:]))
		}

		again, err := parseTokens(strings.NewReader(out.String()))
		if err != nil || len(again) != len(tokens) {
			t.Fatalf("written out and read back: %v, %d tokens, want %d", err, len(again), len(tokens))
		}
		for i := range again {
			if again[i] != tokens[i] {
				t.Fatalf("token %d changed: %+v, want %+v", i, again[i], tokens[i])
			}
		}
	})
}

// FuzzBearerToken checks that a token is only taken from a Bearer header, is
// never empty and never starts with the blanks that separate it.
func FuzzBearerToken(f *testing.F) {
	f.Add("Bearer abc")
	f.Add("bearer    abc")
	f.Add("Bearer ")
	f.Add("Basic abc")
	f.Add("Bearerabc")

	f.Fuzz(func(t *testing.T, header string) {
		r := &http.Request{Header: http.Header{"Authorization": {header}}}
		token, ok := bearerToken(r)
		if !ok {
			return
		}
		if token == "" || strings.HasPrefix(token, " ") {
			t.Fatalf("token %q from %q", token, header)
		}
		if scheme, _, _ := strings.Cut(header, " "); !strings.EqualFold(scheme, "Bearer") {
			t.Fatalf("token from scheme %q", scheme)
		}
	})
}
