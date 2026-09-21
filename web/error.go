package web

import (
	"errors"
	"log/slog"
	"net/http"
)

// ErrInternal is what a 5xx response carries instead of the underlying error,
// which stays in the log.
var ErrInternal = errors.New("internal server error")

// ApiError represents a JSON error response.
type ApiError struct {
	Error string `json:"error"`
}

// NewApiError creates a new ApiError from an error.
func NewApiError(err error) ApiError {
	if err == nil {
		return ApiError{"unknown error"}
	}
	return ApiError{Error: err.Error()}
}

// WriteError writes a JSON error response and logs the details.
//
// Optional reasons are logged but never sent to the client. From status 500 on,
// neither is err: the response carries ErrInternal and the underlying error
// stays in the log, so a wrapped database or filesystem error cannot leak
// internals. Below 500 the error is the answer to the caller's own mistake and
// is passed on unchanged.
//
// The log level follows the status: error from 500 on, warn below. Only the
// path is logged, not the query string, which regularly carries tokens.
func WriteError(w http.ResponseWriter, r *http.Request, status int, err error, reason ...error) {

	log := slog.With(
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"error", err)

	if len(reason) > 0 {
		log = log.With("reason", errors.Join(reason...))
	}

	if status >= http.StatusInternalServerError {
		log.Error("API error")
		Encode(w, status, NewApiError(ErrInternal))
		return
	}

	log.Warn("API error")
	Encode(w, status, NewApiError(err))
}
