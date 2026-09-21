package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// maxRequestBody is the largest request body Decode reads. Anything beyond that
// is refused rather than buffered, so a single request cannot exhaust memory.
const maxRequestBody = 1 << 20 // 1 MiB

// Encode writes v as JSON with the given HTTP status code.
// On marshal failure, responds with 500 InternalServerError.
func Encode[T any](w http.ResponseWriter, status int, v T) {

	w.Header().Set("Content-Type", "application/json")

	resp, err := json.Marshal(v)
	if err != nil {
		if resp, err = json.Marshal(NewApiError(err)); err != nil {
			// NewApiError itself failed to marshal – use a static fallback
			// so the client always receives a valid JSON body.
			resp = []byte(`{"error":"internal server error"}`)
		}
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(status)
	}

	_, _ = w.Write(resp)
}

// Decode reads the request body as JSON into T.
//
// The body is limited to maxRequestBody; a larger one is refused instead of
// being read into memory. Data after the JSON value is an error as well, so a
// body of `{"a":1} and then some` does not pass as valid.
//
// The ResponseWriter is only needed for that limit: http.MaxBytesReader uses it
// to let the server answer with 413 instead of dropping the connection.
func Decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err := dec.Decode(&v); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return v, fmt.Errorf("decode json: body larger than %d bytes", maxRequestBody)
		}
		return v, fmt.Errorf("decode json: %w", err)
	}

	if dec.More() {
		return v, errors.New("decode json: unexpected data after the json value")
	}

	return v, nil
}
