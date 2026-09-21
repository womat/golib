package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLog installs a logger writing into buf as the default logger for the
// duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &buf
}

func TestNewApiError(t *testing.T) {
	if got := NewApiError(nil); got.Error != "unknown error" {
		t.Errorf("NewApiError(nil) = %q, want %q", got.Error, "unknown error")
	}
	if got := NewApiError(errors.New("boom")); got.Error != "boom" {
		t.Errorf("NewApiError = %q, want %q", got.Error, "boom")
	}
}

func TestWriteErrorClientError(t *testing.T) {
	log := captureLog(t)
	req := httptest.NewRequest(http.MethodGet, "/data?token=secret-value", nil)
	rec := httptest.NewRecorder()

	WriteError(rec, req, http.StatusNotFound, errors.New("meter not found"))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var body ApiError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body.Error != "meter not found" {
		t.Errorf("body error = %q, want %q", body.Error, "meter not found")
	}

	if strings.Contains(log.String(), "secret-value") {
		t.Errorf("the query string was logged:\n%s", log)
	}
	if !strings.Contains(log.String(), "level=WARN") {
		t.Errorf("a 4xx should be logged at warn level:\n%s", log)
	}
}

func TestWriteErrorServerErrorHidesDetails(t *testing.T) {
	log := captureLog(t)
	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	rec := httptest.NewRecorder()

	internal := errors.New("dial tcp 10.0.0.9:5432: connection refused")
	WriteError(rec, req, http.StatusInternalServerError, internal, errors.New("while loading meters"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.9") {
		t.Errorf("the response leaks internals: %q", rec.Body.String())
	}

	logged := log.String()
	if !strings.Contains(logged, "10.0.0.9") {
		t.Errorf("the underlying error is missing from the log:\n%s", logged)
	}
	if !strings.Contains(logged, "while loading meters") {
		t.Errorf("the reason is missing from the log:\n%s", logged)
	}
	if !strings.Contains(logged, "level=ERROR") {
		t.Errorf("a 5xx should be logged at error level:\n%s", logged)
	}
}
