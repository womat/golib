package web

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Encode writes v as JSON with the given HTTP status code.
// On marshal failure, responds with 500 InternalServerError.
func Encode[T any](w http.ResponseWriter, status int, v T) {

	w.Header().Set("Content-Type", "application/json")

	resp, err := json.Marshal(v)
	if err != nil {
		resp, _ = json.Marshal(NewApiError(err))
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(status)
	}

	_, _ = w.Write(resp)
}

// Decode reads the request body as JSON into T.
func Decode[T any](r *http.Request) (T, error) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return v, fmt.Errorf("decode json: %w", err)
	}
	return v, nil
}
