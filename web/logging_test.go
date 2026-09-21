package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

// echoHandler writes the request body back and reports the given status.
func echoHandler(status int, contentType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0)
		if r.Body != nil {
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			body = buf.Bytes()
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
}

func TestWithLoggingLogsRequestMetadata(t *testing.T) {
	logger, buf := testLogger()

	req := httptest.NewRequest(http.MethodPost, "/meters", strings.NewReader(`{"secret":"hunter2"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	WithLogging(echoHandler(http.StatusCreated, "application/json"), logger).ServeHTTP(rec, req)

	logged := buf.String()
	for _, want := range []string{"POST", "/meters", "201"} {
		if !strings.Contains(logged, want) {
			t.Errorf("log is missing %q:\n%s", want, logged)
		}
	}
}

func TestWithLoggingDoesNotLogBodiesByDefault(t *testing.T) {
	logger, buf := testLogger()

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"password":"hunter2"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	WithLogging(echoHandler(http.StatusOK, "application/json"), logger).ServeHTTP(rec, req)

	if strings.Contains(buf.String(), "hunter2") {
		t.Errorf("the request body was logged although body logging is off:\n%s", buf)
	}
}

func TestWithLoggingBodyLimit(t *testing.T) {
	logger, buf := testLogger()

	body := strings.Repeat("a", 100)
	req := httptest.NewRequest(http.MethodPost, "/data", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	WithLogging(echoHandler(http.StatusOK, "application/json"), logger, WithBodyLogging(16)).
		ServeHTTP(rec, req)

	logged := buf.String()
	if strings.Contains(logged, body) {
		t.Errorf("the body was logged in full despite the limit:\n%s", logged)
	}
	if !strings.Contains(logged, strings.Repeat("a", 16)) {
		t.Errorf("the truncated body is missing from the log:\n%s", logged)
	}

	// The handler must still see the whole body.
	if rec.Body.String() != body {
		t.Errorf("the handler received %d bytes, want %d", rec.Body.Len(), len(body))
	}
}

func TestWithLoggingLogsResponseBody(t *testing.T) {
	logger, buf := testLogger()

	req := httptest.NewRequest(http.MethodPost, "/data", strings.NewReader(`{"echo":"me"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	WithLogging(echoHandler(http.StatusOK, "application/json"), logger, WithBodyLogging(1024)).
		ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), `echo`) {
		t.Errorf("the response body is missing from the log:\n%s", buf)
	}
}

func TestWithLoggingSkipsBinaryBodies(t *testing.T) {
	logger, buf := testLogger()

	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader([]byte{0x00, 0x01, 0x02}))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()

	WithLogging(echoHandler(http.StatusOK, "application/octet-stream"), logger, WithBodyLogging(1024)).
		ServeHTTP(rec, req)

	if strings.Contains(buf.String(), "\x00") {
		t.Errorf("a binary body was written to the log:\n%q", buf.String())
	}
}

func TestWithLoggingKeepsFlusher(t *testing.T) {
	logger, _ := testLogger()

	flushed := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Error("the ResponseWriter is no longer an http.Flusher - streaming handlers break")
			return
		}
		_, _ = w.Write([]byte("event: tick\n\n"))
		f.Flush()
		flushed = true
	})

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()

	WithLogging(handler, logger).ServeHTTP(rec, req)

	if !flushed {
		t.Error("the handler could not flush")
	}
}

func TestWithLoggingRecordsStatusWithoutExplicitWriteHeader(t *testing.T) {
	logger, buf := testLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("implicit 200"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	WithLogging(handler, logger).ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(buf.String(), "200") {
		t.Errorf("the implicit 200 is missing from the log:\n%s", buf)
	}
}

// TestWithLoggingLevelFollowsStatus is the fix for an access log that was
// written at debug level and therefore invisible in production.
func TestWithLoggingLevelFollowsStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{
		{http.StatusOK, "level=INFO"},
		{http.StatusNotFound, "level=WARN"},
		{http.StatusInternalServerError, "level=ERROR"},
	} {
		logger, buf := testLogger()

		WithLogging(echoHandler(tc.status, "application/json"), logger).
			ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

		if !strings.Contains(buf.String(), tc.want) {
			t.Errorf("status %d: want %s, got:\n%s", tc.status, tc.want, buf)
		}
	}
}

// TestWithLoggingAtInfoLevel guards the regression directly: a logger that
// only passes info and above must still see the access log.
func TestWithLoggingAtInfoLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	WithLogging(echoHandler(http.StatusOK, "application/json"), logger).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	if !strings.Contains(buf.String(), "/health") {
		t.Errorf("the access log is invisible at info level:\n%s", buf.String())
	}
}

func TestWithLogLevelPinsTheLevel(t *testing.T) {
	logger, buf := testLogger()

	WithLogging(echoHandler(http.StatusInternalServerError, "application/json"), logger, WithLogLevel(slog.LevelDebug)).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(buf.String(), "level=DEBUG") {
		t.Errorf("WithLogLevel did not pin the level:\n%s", buf)
	}
}

// TestWithLoggingNilLoggerDoesNotPanic covers the case that used to panic on
// the first request rather than when the middleware was built.
func TestWithLoggingNilLoggerDoesNotPanic(t *testing.T) {
	buf := captureLog(t)

	WithLogging(echoHandler(http.StatusOK, "application/json"), nil).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nil-logger", nil))

	if !strings.Contains(buf.String(), "/nil-logger") {
		t.Errorf("a nil logger should fall back to the default one:\n%s", buf)
	}
}

// TestWithLoggingSeedsTheContext is what makes WriteError and WithIPFilter
// report to the same logger.
func TestWithLoggingSeedsTheContext(t *testing.T) {
	logger, buf := testLogger()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		LoggerFrom(r.Context()).Info("from the handler")
	})

	WithLogging(inner, logger).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(buf.String(), "from the handler") {
		t.Errorf("the handler did not find the logger in the context:\n%s", buf)
	}
}
