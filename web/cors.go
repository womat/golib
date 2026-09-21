package web

import (
	"net/http"
	"strings"
)

// Default CORS headers, used when WithCORS is called without options.
const (
	defaultAllowedOrigin  = "*"
	defaultAllowedMethods = "POST, GET, OPTIONS, PUT, PATCH, DELETE"
	defaultAllowedHeaders = "Content-Type, Authorization, X-Api-Key"
	defaultMaxAge         = "86400"
)

// CORSOption configures WithCORS.
type CORSOption func(*corsConfig)

type corsConfig struct {
	origins []string // empty means "*"
	methods string
	headers string
	maxAge  string
}

// WithAllowedOrigins restricts the allowed origins to the given list.
//
// Without it, WithCORS answers every origin with "*", which is the historical
// behaviour and workable for an API guarded by an API key and an IP filter, but
// not for one that relies on browser credentials. With a list, the request's
// Origin header is echoed back only when it is on the list, and "Vary: Origin"
// is set so that a cache in front cannot hand one origin's response to another.
func WithAllowedOrigins(origins ...string) CORSOption {
	return func(c *corsConfig) {
		c.origins = origins
	}
}

// WithAllowedMethods replaces the list of allowed methods.
func WithAllowedMethods(methods ...string) CORSOption {
	return func(c *corsConfig) {
		c.methods = strings.Join(methods, ", ")
	}
}

// WithAllowedHeaders replaces the list of allowed request headers.
func WithAllowedHeaders(headers ...string) CORSOption {
	return func(c *corsConfig) {
		c.headers = strings.Join(headers, ", ")
	}
}

// WithCORS is a middleware that adds CORS headers to the response.
//
// It does not answer preflight requests: OPTIONS is passed on like any other
// method, so register HandlePreflight for it - and register it outside WithAuth,
// because a preflight request carries no credentials and would be rejected.
func WithCORS(next http.Handler, opts ...CORSOption) http.Handler {
	cfg := corsConfig{
		methods: defaultAllowedMethods,
		headers: defaultAllowedHeaders,
		maxAge:  defaultMaxAge,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			if len(cfg.origins) > 0 {
				// The response now depends on the request's Origin header.
				w.Header().Add("Vary", "Origin")
			}
			if origin, ok := cfg.allowedOrigin(r.Header.Get("Origin")); ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Access-Control-Allow-Methods", cfg.methods)
			w.Header().Set("Access-Control-Allow-Headers", cfg.headers)
			w.Header().Set("Access-Control-Max-Age", cfg.maxAge)

			next.ServeHTTP(w, r)
		},
	)
}

// allowedOrigin returns the value for Access-Control-Allow-Origin and whether
// the header should be set at all.
func (c *corsConfig) allowedOrigin(requestOrigin string) (string, bool) {
	if len(c.origins) == 0 {
		return defaultAllowedOrigin, true
	}

	for _, origin := range c.origins {
		if origin == defaultAllowedOrigin {
			return defaultAllowedOrigin, true
		}
		if requestOrigin != "" && origin == requestOrigin {
			return requestOrigin, true
		}
	}

	return "", false
}

// HandlePreflight is a handler for preflight requests.
// see https://developer.mozilla.org/en-US/docs/Glossary/Preflight_request
func HandlePreflight() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	)
}
