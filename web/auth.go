package web

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/womat/golib/jwt_util"
)

var (
	ErrUnauthorized = errors.New("not authorized")
)

type Config struct {
	// Tokens are the bearer tokens WithAuth accepts, as SHA-256 hashes, usually
	// read with ReadTokenFile. They are checked first.
	Tokens []Token

	// ApiKey is a single key in plain text, accepted as a bearer token or in
	// the deprecated X-Api-Key header. Superseded by Tokens.
	ApiKey    string
	JwtSecret string
	JwtID     string
	AppName   string
}

// ContextKey is used for storing values in context safely
type ContextKey string

const contextKeyUser ContextKey = "user"

// UserFrom returns who WithAuth authorized the request as: the name of the
// bearer token, "apikey" for the API key, or the user of a JWT. ok is false
// outside WithAuth.
func UserFrom(ctx context.Context) (user string, ok bool) {
	user, ok = ctx.Value(contextKeyUser).(string)
	return user, ok
}

// WithAuth is a middleware that checks if the request is authorized.
//   - If the request is not authorized, it returns a 401 Unauthorized response.
//   - If the request is authorized, it calls the next handler.
//
// A bearer token whose SHA-256 hash is one of config.Tokens is accepted first,
// and the request carries the name of that token as its user.
//
// The API key is accepted in two places: as a static bearer token,
// "Authorization: Bearer <key>", which is the preferred form, and in the
// X-Api-Key header, which is deprecated and kept for existing clients. A
// request authorized through X-Api-Key logs a warning, once per client and
// handler, so the clients that still have to move show up in the log. A bearer
// value that is not the API key is then tried as a JWT.
func WithAuth(h http.Handler, config Config) http.Handler {
	warned := newClientSet(maxWarnedClients)

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			if len(config.Tokens) > 0 {
				if token, ok := bearerToken(r); ok {
					if name, ok := matchToken(config.Tokens, token); ok {
						recordCaller(r.Context(), name)
						ctx := context.WithValue(r.Context(), contextKeyUser, name)
						h.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			if len(config.ApiKey) > 0 {
				viaHeader := false
				ok := checkBearerKey(r, config.ApiKey)
				if !ok {
					ok = checkApiKey(r, config.ApiKey)
					viaHeader = ok
				}
				if ok {
					recordCaller(r.Context(), "apikey")
					if viaHeader {
						if client := clientHost(r); warned.add(client) {
							LoggerFrom(r.Context()).Warn("X-Api-Key is deprecated, send the key as Authorization: Bearer",
								"client", client, "path", r.URL.Path)
						}
					}
					// update authenticated user in context and pass it to the next handler
					ctx := context.WithValue(r.Context(), contextKeyUser, "apikey")
					h.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			if len(config.JwtSecret) > 0 && len(config.JwtID) > 0 {
				if claims, ok := checkJwtToken(r, config); ok {
					recordCaller(r.Context(), claims.User)
					// update authenticated user in context and pass it to the next handler
					ctx := context.WithValue(r.Context(), contextKeyUser, claims.User)
					h.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// If neither API Key nor JWT is valid, return Unauthorized. RFC 6750
			// wants the scheme named in the challenge, so a client learns what to send.
			w.Header().Set("WWW-Authenticate", "Bearer")
			Encode(w, http.StatusUnauthorized, NewApiError(ErrUnauthorized))
		})
}

// maxWarnedClients bounds the clients WithAuth remembers per handler for the
// deprecation warning. Beyond it no further client is warned about, so a flood
// of addresses can neither grow the set nor the log.
const maxWarnedClients = 256

// clientSet remembers which clients were already warned about.
type clientSet struct {
	mu   sync.Mutex
	max  int
	seen map[string]struct{}
}

func newClientSet(max int) *clientSet {
	return &clientSet{max: max, seen: make(map[string]struct{})}
}

// add reports whether client is new and was recorded; false once it was seen
// or the set is full.
func (s *clientSet) add(client string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[client]; ok || len(s.seen) >= s.max {
		return false
	}
	s.seen[client] = struct{}{}
	return true
}

// clientHost is the address of the client without its port, or RemoteAddr as
// it is when it cannot be split.
func clientHost(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// checkApiKey validates the X-Api-Key header against the configured key.
//
// The comparison is constant time, so a wrong key does not reveal how many of
// its leading bytes were right. The length of the configured key stays
// observable; see the README.
func checkApiKey(r *http.Request, apiKey string) bool {
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Api-Key")), []byte(apiKey)) == 1
}

// checkBearerKey validates a static bearer token against the configured API key,
// in constant time like checkApiKey.
func checkBearerKey(r *http.Request, apiKey string) bool {
	token, ok := bearerToken(r)
	return ok && subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) == 1
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

// checkJwtToken validates the Bearer token from the Authorization header.
func checkJwtToken(r *http.Request, config Config) (*jwt_util.Claims, bool) {
	token, ok := bearerToken(r)
	if !ok {
		return nil, false
	}

	claims, err := jwt_util.ValidateToken(token, config.AppName, "auth", config.JwtID, config.JwtSecret)
	if err != nil {
		return nil, false
	}
	return claims, true
}
