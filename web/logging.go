package web

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LogOption configures WithLogging.
type LogOption func(*logConfig)

type logConfig struct {
	bodyLimit int         // 0 disables body logging
	level     *slog.Level // nil follows the status code
	success   *slog.Level // level below status 400; nil means Info
}

// WithBodyLogging adds the request and response body to the log entry, cut off
// at maxBytes.
//
// It is off by default and should stay off outside of debugging: bodies carry
// passwords, tokens and personal data, all of which end up in the log in clear
// text. Only textual payloads are logged (application/json, application/xml
// and text/*); anything else is reported by size only.
func WithBodyLogging(maxBytes int) LogOption {
	return func(c *logConfig) {
		c.bodyLimit = maxBytes
	}
}

// WithLogLevel writes every entry at the given level instead of letting the
// response status decide.
//
// Use it to put the access log back on a single level - slog.LevelDebug, for
// instance, which is what this middleware did before it started following the
// status.
func WithLogLevel(level slog.Level) LogOption {
	return func(c *logConfig) {
		c.level = &level
	}
}

// WithSuccessLevel writes entries for responses below 400 at the given level;
// from 400 on the status still decides (Warn, from 500 Error).
//
// For a service that is polled - a web page asking every few seconds -
// slog.LevelDebug keeps the successful requests out of the normal log while a
// rejected one, with its client, still shows as a warning. The first
// successful request of each authenticated caller from each client is still
// written at Info, marked firstAccess=true, so the normal log shows who uses
// the service. Up to 256 such pairs are remembered per WithLogging; a service
// that rebuilds its handler on reload logs them again. WithLogLevel takes
// precedence over it.
func WithSuccessLevel(level slog.Level) LogOption {
	return func(c *logConfig) {
		c.success = &level
	}
}

// levelFor reports the level one entry is written at: the configured one, or
// else the one the status code calls for.
func (c logConfig) levelFor(status int) slog.Level {
	if c.level != nil {
		return *c.level
	}

	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	case c.success != nil:
		return *c.success
	default:
		return slog.LevelInfo
	}
}

// logResponse wraps http.ResponseWriter to capture the status code, the
// response size and, if asked for, the beginning of the body.
//
// Unwrap, Flush and Hijack are passed through so that streaming responses and
// WebSocket upgrades keep working when this middleware is in the chain.
type logResponse struct {
	http.ResponseWriter
	status      int
	size        int
	body        bytes.Buffer
	bodyLimit   int
	wroteHeader bool
}

// WriteHeader captures the status code. An informational 1xx goes out ahead of
// the response and is not the status the log reports.
func (lr *logResponse) WriteHeader(status int) {
	if !lr.wroteHeader && status >= http.StatusOK {
		lr.status = status
		lr.wroteHeader = true
	}
	lr.ResponseWriter.WriteHeader(status)
}

// Write captures size and, up to the limit, the response body.
func (lr *logResponse) Write(b []byte) (int, error) {
	// The first Write sends the header, with the status set so far - 200 if
	// there was no WriteHeader; a WriteHeader after it no longer counts.
	lr.wroteHeader = true

	if room := lr.bodyLimit - lr.body.Len(); room > 0 {
		lr.body.Write(b[:min(room, len(b))])
	}

	n, err := lr.ResponseWriter.Write(b)
	lr.size += n
	return n, err
}

// Unwrap gives http.ResponseController access to the underlying writer.
func (lr *logResponse) Unwrap() http.ResponseWriter {
	return lr.ResponseWriter
}

// Flush passes through to the underlying writer, if it can flush.
func (lr *logResponse) Flush() {
	if f, ok := lr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack passes through to the underlying writer, if it can be hijacked.
func (lr *logResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := lr.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("web: the underlying ResponseWriter cannot be hijacked")
}

// WithLogging is a middleware that logs one entry per request.
//
//   - It logs the request method and path.
//   - It logs the response status, size and duration.
//   - Bodies are logged only with WithBodyLogging, and only up to its limit.
//
// The level follows the response status - error from 500, warn from 400, info
// below - so the access log is visible at the level a service normally runs at.
// WithLogLevel pins it to one level instead.
//
// The logger is put into the request context, which is where WriteError and
// WithIPFilter look for it. Wrap with this middleware outermost and those
// entries go to the same logger; see the README for the ordering.
//
// A nil logger means slog.Default(), so the middleware never panics on a
// request; passing one explicitly is the point of the parameter.
func WithLogging(h http.Handler, logger *slog.Logger, opts ...LogOption) http.Handler {
	var cfg logConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	if logger == nil {
		logger = slog.Default()
	}

	// Callers already logged at Info; see WithSuccessLevel.
	seen := newClientSet(maxSeenClients)

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			start := time.Now()

			// Hand the logger down the chain, so that the handlers and the
			// middleware below this one report to the same place.
			r = r.WithContext(ContextWithLogger(r.Context(), logger))

			// WithAuth runs inside this handler and cannot hand a context
			// back out; it records the authenticated caller here instead.
			caller := new(requestCaller)
			r = r.WithContext(context.WithValue(r.Context(), callerKey{}, caller))

			// Read and restore the request body before the handler consumes it,
			// but never more than the limit.
			var reqBody []byte
			if cfg.bodyLimit > 0 && r.Body != nil && isTextual(r.Header.Get("Content-Type")) {
				reqBody = readBodyPrefix(r, cfg.bodyLimit)
			}

			// Capture the response body unconditionally; whether it may be
			// logged is decided afterwards, when the handler has set its
			// Content-Type.
			lr := &logResponse{ResponseWriter: w, status: http.StatusOK, bodyLimit: cfg.bodyLimit}

			h.ServeHTTP(lr, r)

			attrs := []any{
				slog.Group("request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("client", clientHost(r)),
				),
				slog.Group("response",
					slog.Int("status", lr.status),
					slog.Int("size", lr.size),
					slog.Duration("duration", time.Since(start)),
				),
			}

			level := cfg.levelFor(lr.status)
			if caller.name != "" {
				attrs = append(attrs, slog.String("user", caller.name))

				// With successful requests below Info, the first one of each caller
				// from each client still goes to Info, so the normal log shows who
				// uses the service without a line per poll.
				if level < slog.LevelInfo && cfg.level == nil && lr.status < http.StatusBadRequest &&
					seen.add(caller.name+" "+clientHost(r)) {
					level = slog.LevelInfo
					attrs = append(attrs, slog.Bool("firstAccess", true))
				}
			}

			if cfg.bodyLimit > 0 {
				responseBody := lr.body.String()
				if !isTextual(lr.Header().Get("Content-Type")) {
					responseBody = ""
				}
				attrs = append(attrs, slog.Group("body",
					slog.String("request", string(reqBody)),
					slog.String("response", responseBody),
				))
			}

			logger.Log(r.Context(), level, "request", attrs...)
		},
	)
}

// requestCaller is where WithAuth records who a request was authorized as,
// for WithLogging to put into the access log.
type requestCaller struct {
	name string
}

// callerKey is the context key the requestCaller is stored under.
type callerKey struct{}

// recordCaller notes name as the authenticated caller of the request, if
// WithLogging is in the chain to log it.
func recordCaller(ctx context.Context, name string) {
	if c, ok := ctx.Value(callerKey{}).(*requestCaller); ok {
		c.name = name
	}
}

// readBodyPrefix returns at most limit bytes of the request body and puts the
// whole body back, so the handler still sees all of it. What was read before a
// read error is put back as well; the handler meets the error itself, on the
// rest of the body.
func readBodyPrefix(r *http.Request, limit int) []byte {
	body, _ := io.ReadAll(io.LimitReader(r.Body, int64(limit)))

	r.Body = struct {
		io.Reader
		io.Closer
	}{
		Reader: io.MultiReader(bytes.NewReader(body), r.Body),
		Closer: r.Body,
	}

	return body
}

// isTextual reports whether a body of that content type is safe to put into a
// log line.
func isTextual(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.TrimSpace(strings.ToLower(mediaType))

	return mediaType == "application/json" ||
		mediaType == "application/xml" ||
		strings.HasPrefix(mediaType, "text/")
}

// maxSeenClients bounds the callers WithLogging remembers per handler for the
// first-access line. Beyond it no further caller is promoted to Info, so a
// flood of addresses can neither grow the set nor the log.
const maxSeenClients = 256

// clientSet remembers which clients were already seen.
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
