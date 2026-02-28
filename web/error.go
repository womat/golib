package web

import (
	"log/slog"
	"net/http"
)

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
// Optional reason is logged but not included in the response.
func WriteError(w http.ResponseWriter, r *http.Request, status int, err error, reason ...error) {

	log := slog.With(
		"method", r.Method,
		"path", r.URL.Path,
		"query", r.URL.Query(),
		"status", status,
		"error", err)

	if len(reason) > 0 {
		log = log.With("reason", reason[0])
	}

	log.Debug("API error")
	Encode(w, status, NewApiError(err))
}
