package web

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
)

// logResponse wraps http.ResponseWriter to capture status code and response body for logging.
//   - Status is the HTTP status code.
//   - Body is the response body.
//   - ResponseWriter is the original http.ResponseWriter.
type logResponse struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

// WriteHeader captures the status code.
func (lr *logResponse) WriteHeader(status int) {
	lr.status = status
	lr.ResponseWriter.WriteHeader(status)
}

// Write captures the response body.
func (lr *logResponse) Write(b []byte) (int, error) {
	lr.body.Write(b)
	return lr.ResponseWriter.Write(b)
}

// WithLogging is a middleware that logs the request and response.
//   - It logs the request method, URL, and body.
//   - It logs the response status and body.
func WithLogging(h http.Handler, logger *slog.Logger) http.Handler {

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			// Read and restore request body before handler consumes it
			var reqBody bytes.Buffer
			if r.Body != nil {
				_, _ = io.Copy(&reqBody, r.Body)
				r.Body = io.NopCloser(bytes.NewBuffer(reqBody.Bytes()))
			}

			lr := &logResponse{ResponseWriter: w, status: http.StatusOK}
			h.ServeHTTP(lr, r)

			// Log the request and response.
			logger.Debug("request",
				slog.Group("request",
					slog.String("method", r.Method),
					slog.String("url", r.URL.String()),
					slog.String("body", reqBody.String()),
				),
				slog.Group("response",
					slog.Int("status", lr.status),
					slog.String("body", lr.body.String()),
				),
			)
		},
	)
}
