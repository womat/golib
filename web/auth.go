package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

var (
	ErrUnauthorized = errors.New("not authorized")
)

type Config struct {
	// Tokens are the bearer tokens WithAuth accepts, as SHA-256 hashes, usually
	// read with ReadTokenFile.
	Tokens []Token
}

// ContextKey is used for storing values in context safely
type ContextKey string

const contextKeyUser ContextKey = "user"

// UserFrom returns who WithAuth authorized the request as: the name of the
// bearer token. ok is false outside WithAuth.
func UserFrom(ctx context.Context) (user string, ok bool) {
	user, ok = ctx.Value(contextKeyUser).(string)
	return user, ok
}

// WithAuth is a middleware that checks if the request is authorized.
//   - If the request is not authorized, it returns a 401 Unauthorized response.
//   - If the request is authorized, it calls the next handler.
//
// A request is authorized when it carries "Authorization: Bearer <token>" and
// the SHA-256 hash of the token is one of config.Tokens. The request then
// carries the name of that token as its user, see UserFrom. With no tokens
// configured, every request is rejected.
func WithAuth(h http.Handler, config Config) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if token, ok := bearerToken(r); ok {
				if name, ok := matchToken(config.Tokens, token); ok {
					recordCaller(r.Context(), name)
					ctx := context.WithValue(r.Context(), contextKeyUser, name)
					h.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// RFC 6750 wants the scheme named in the challenge, so a client
			// learns what to send.
			w.Header().Set("WWW-Authenticate", "Bearer")
			Encode(w, http.StatusUnauthorized, NewApiError(ErrUnauthorized))
		})
}

// bearerToken returns the credentials of an "Authorization: Bearer <token>"
// header. The scheme is matched case-insensitively and may be followed by more
// than one space, as RFC 7235 allows.
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimLeft(token, " ")
	return token, token != ""
}
