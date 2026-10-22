package web

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// A WebSocket upgrade hijacks the connection, and http.ResponseController
// reaches the server's writer through Unwrap; both must survive the wrapper.
func TestWithLoggingKeepsHijackerAndUnwrap(t *testing.T) {
	logger, _ := testLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// logResponse has no SetWriteDeadline; only Unwrap gets there.
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
			t.Errorf("SetWriteDeadline: %v", err)
		}
		h, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the ResponseWriter is no longer an http.Hijacker - WebSocket upgrades break")
			return
		}
		conn, buf, err := h.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
		_ = buf.Flush()
	})

	srv := httptest.NewServer(WithLogging(handler, logger))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if body, _ := io.ReadAll(resp.Body); string(body) != "ok" {
		t.Errorf("body = %q, want the one written to the hijacked connection", body)
	}
}

func TestWithLoggingHijackWithoutHijacker(t *testing.T) {
	logger, _ := testLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, _, err := w.(http.Hijacker).Hijack(); err == nil {
			t.Error("Hijack on a writer that cannot be hijacked succeeded")
		}
	})

	// httptest.ResponseRecorder cannot be hijacked.
	WithLogging(handler, logger).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
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

// net/http ignores a WriteHeader after the first Write; the log must show the
// 200 that was sent, not the status that was asked for too late.
func TestWithLoggingIgnoresWriteHeaderAfterWrite(t *testing.T) {
	logger, buf := testLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("sent with 200"))
		w.WriteHeader(http.StatusInternalServerError)
	})

	WithLogging(handler, logger).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(buf.String(), "response.status=200") {
		t.Errorf("want status 200 in the log:\n%s", buf)
	}
}

// An informational 1xx comes before the response; the log must show the final status.
func TestWithLoggingIgnoresInformationalStatus(t *testing.T) {
	logger, buf := testLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		w.WriteHeader(http.StatusNotFound)
	})

	WithLogging(handler, logger).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(buf.String(), "response.status=404") || !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("want status 404 at WARN in the log:\n%s", buf)
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

func TestWithSuccessLevel(t *testing.T) {
	for status, want := range map[int]string{
		http.StatusOK:                  "level=DEBUG",
		http.StatusNotModified:         "level=DEBUG",
		http.StatusUnauthorized:        "level=WARN",
		http.StatusInternalServerError: "level=ERROR",
	} {
		logger, buf := testLogger()
		WithLogging(echoHandler(status, "application/json"), logger, WithSuccessLevel(slog.LevelDebug)).
			ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

		if !strings.Contains(buf.String(), want) {
			t.Errorf("status %d: want %s in\n%s", status, want, buf)
		}
	}

	// WithLogLevel wins.
	logger, buf := testLogger()
	WithLogging(echoHandler(http.StatusUnauthorized, "application/json"), logger,
		WithSuccessLevel(slog.LevelDebug), WithLogLevel(slog.LevelInfo)).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(buf.String(), "level=INFO") {
		t.Errorf("WithLogLevel did not take precedence:\n%s", buf)
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

// failingBody yields data, then fails, like a client that drops the
// connection halfway through its upload.
type failingBody struct{ data *strings.Reader }

func (b failingBody) Read(p []byte) (int, error) {
	if b.data.Len() == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	return b.data.Read(p)
}

func (failingBody) Close() error { return nil }

func TestWithLoggingKeepsTheBodyWhenReadingFails(t *testing.T) {
	logger, _ := testLogger()

	var got string
	var gotErr error
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		got, gotErr = string(b), err
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = failingBody{strings.NewReader(`{"a":`)}

	WithLogging(handler, logger, WithBodyLogging(100)).ServeHTTP(httptest.NewRecorder(), req)

	if got != `{"a":` {
		t.Errorf("handler read %q, want what came before the failure", got)
	}
	if !errors.Is(gotErr, io.ErrUnexpectedEOF) {
		t.Errorf("handler got err %v, want the read error", gotErr)
	}
}
