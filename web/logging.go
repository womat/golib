package web

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// LogOption configures WithLogging.
type LogOption func(*logConfig)

type logConfig struct {
	bodyLimit int         // 0 disables body logging
	level     *slog.Level // nil follows the status code
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

// WriteHeader captures the status code.
func (lr *logResponse) WriteHeader(status int) {
	if !lr.wroteHeader {
		lr.status = status
		lr.wroteHeader = true
	}
	lr.ResponseWriter.WriteHeader(status)
}

// Write captures size and, up to the limit, the response body.
func (lr *logResponse) Write(b []byte) (int, error) {
	if !lr.wroteHeader {
		// An implicit 200, as net/http would send it.
		lr.status = http.StatusOK
		lr.wroteHeader = true
	}

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

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			start := time.Now()

			// Hand the logger down the chain, so that the handlers and the
			// middleware below this one report to the same place.
			r = r.WithContext(ContextWithLogger(r.Context(), logger))

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
				),
				slog.Group("response",
					slog.Int("status", lr.status),
					slog.Int("size", lr.size),
					slog.Duration("duration", time.Since(start)),
				),
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

			logger.Log(r.Context(), cfg.levelFor(lr.status), "request", attrs...)
		},
	)
}

// readBodyPrefix returns at most limit bytes of the request body and puts the
// whole body back, so the handler still sees all of it.
func readBodyPrefix(r *http.Request, limit int) []byte {
	body, err := io.ReadAll(io.LimitReader(r.Body, int64(limit)))
	if err != nil {
		return nil
	}

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
