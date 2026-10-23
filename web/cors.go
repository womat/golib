package web

import (
	"net/http"
	"strings"
)

// Default CORS headers, used when WithCORS is called without options.
const (
	defaultAllowedOrigin  = "*"
	defaultAllowedMethods = "POST, GET, OPTIONS, PUT, PATCH, DELETE"
	defaultAllowedHeaders = "Content-Type, Authorization"
	defaultMaxAge         = "86400"
)

// CORSOption configures WithCORS.
type CORSOption func(*corsConfig)

type corsConfig struct {
	origins []string // empty means "*"
	methods string
	headers string
}

// WithAllowedOrigins restricts the allowed origins to the given list.
//
// Without it, WithCORS answers every origin with "*", which is the historical
// behaviour and workable for an API guarded by bearer tokens and an IP filter, but
// not for one that relies on browser credentials. With a list, the request's
// Origin header is echoed back only when it is on the list, and "Vary: Origin"
// is set so that a cache in front cannot hand one origin's response to another.
//
// An empty list restricts nothing: it means "*", as without the option.
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
// It answers preflight requests itself, with 204 and the headers: an OPTIONS
// request carrying Access-Control-Request-Method never reaches the handler.
// A preflight carries no credentials, so behind WithAuth it would be rejected,
// and a mux needs no "OPTIONS /" route for it - such a route matches every
// path and turns the mux's 404 for an unknown one into 405. Any other OPTIONS
// request is passed on like every other method.
func WithCORS(next http.Handler, opts ...CORSOption) http.Handler {
	cfg := corsConfig{
		methods: defaultAllowedMethods,
		headers: defaultAllowedHeaders,
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
			w.Header().Set("Access-Control-Max-Age", defaultMaxAge)

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

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
//
// Behind WithCORS it is not needed: WithCORS answers the preflight before the
// mux sees it. It stays for services that register "OPTIONS /" without
// WithCORS in front; with WithCORS, drop that route, so the mux answers an
// unknown path with 404 instead of 405.
func HandlePreflight() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	)
}
