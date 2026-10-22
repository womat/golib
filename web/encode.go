package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxRequestBody is the largest request body Decode reads. Anything beyond that
// is refused rather than buffered, so a single request cannot exhaust memory.
const maxRequestBody = 1 << 20 // 1 MiB

// internalErrorBody is the response when v cannot be marshalled: ErrInternal
// as an ApiError, fixed so that it cannot fail itself.
var internalErrorBody = []byte(`{"error":"` + ErrInternal.Error() + `"}`)

// Encode writes v as JSON with the given HTTP status code.
//
// If v cannot be marshalled, it responds with 500 and ErrInternal instead. The
// marshal error is not sent: a custom MarshalJSON may wrap anything, addresses
// and file names included, just like the errors WriteError keeps from 5xx
// responses.
func Encode[T any](w http.ResponseWriter, status int, v T) {

	w.Header().Set("Content-Type", "application/json")

	resp, err := json.Marshal(v)
	if err != nil {
		resp, status = internalErrorBody, http.StatusInternalServerError
	}

	w.WriteHeader(status)
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

	// Anything but the end of the body is trailing data; dec.More would miss a
	// closing bracket.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return v, errors.New("decode json: unexpected data after the json value")
	}

	return v, nil
}
