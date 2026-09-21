package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// bufferLogger returns a logger writing into a buffer at debug level.
func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func TestLoggerFromRoundTrip(t *testing.T) {
	logger, _ := bufferLogger()

	ctx := ContextWithLogger(context.Background(), logger)
	if got := LoggerFrom(ctx); got != logger {
		t.Error("LoggerFrom did not return the logger that was put in")
	}
}

func TestContextWithLoggerIgnoresNil(t *testing.T) {
	ctx := ContextWithLogger(context.Background(), nil)

	// A nil logger must not be stored, so that LoggerFrom keeps falling back
	// instead of handing out a nil pointer that panics on first use.
	if LoggerFrom(ctx) == nil {
		t.Fatal("LoggerFrom returned nil")
	}
	if got := ctx.Value(loggerKey{}); got != nil {
		t.Errorf("a nil logger was stored in the context: %v", got)
	}
}

func TestLoggerFromFallsBackToDefault(t *testing.T) {
	buf := captureLog(t)

	LoggerFrom(context.Background()).Info("fallback reached")

	if !strings.Contains(buf.String(), "fallback reached") {
		t.Errorf("the fallback did not write to the default logger:\n%s", buf)
	}
}

// TestWriteErrorUsesContextLogger is the point of the whole exercise: the
// entry must land in the application's logger, not in the global one.
func TestWriteErrorUsesContextLogger(t *testing.T) {
	defaultBuf := captureLog(t)
	logger, buf := bufferLogger()

	req := httptest.NewRequest(http.MethodGet, "/secret", nil).
		WithContext(ContextWithLogger(context.Background(), logger))

	WriteError(httptest.NewRecorder(), req, http.StatusBadRequest, errors.New("bad request"))

	if !strings.Contains(buf.String(), "/secret") {
		t.Errorf("WriteError did not log to the context logger:\n%s", buf)
	}
	if defaultBuf.Len() != 0 {
		t.Errorf("WriteError also wrote to the default logger:\n%s", defaultBuf)
	}
}

func TestWithIPFilterUsesContextLogger(t *testing.T) {
	defaultBuf := captureLog(t)
	logger, buf := bufferLogger()

	reached := false
	filtered := WithIPFilter(okHandler(&reached), []string{"10.0.0.1"}, nil)

	req := requestFrom("192.168.1.1:1234").
		WithContext(ContextWithLogger(context.Background(), logger))
	filtered.ServeHTTP(httptest.NewRecorder(), req)

	if reached {
		t.Fatal("the handler was reached although the IP is not allowed")
	}
	if !strings.Contains(buf.String(), "IP not allowed") {
		t.Errorf("the rejection did not go to the context logger:\n%s", buf)
	}
	if defaultBuf.Len() != 0 {
		t.Errorf("the rejection also went to the default logger:\n%s", defaultBuf)
	}
}

func TestWithIPFilterLoggerReportsUnusableEntries(t *testing.T) {
	defaultBuf := captureLog(t)
	logger, buf := bufferLogger()

	// Parsing happens here, while the middleware is built - there is no
	// request yet whose context could carry the logger.
	WithIPFilter(http.NotFoundHandler(), []string{"not-an-ip"}, nil, WithIPFilterLogger(logger))

	if !strings.Contains(buf.String(), "not-an-ip") {
		t.Errorf("the unusable entry was not reported to the option's logger:\n%s", buf)
	}
	if defaultBuf.Len() != 0 {
		t.Errorf("the unusable entry also went to the default logger:\n%s", defaultBuf)
	}
}

func TestWithIPFilterParsingFallsBackToDefault(t *testing.T) {
	buf := captureLog(t)

	WithIPFilter(http.NotFoundHandler(), []string{"300.300.300.300"}, nil)

	if !strings.Contains(buf.String(), "300.300.300.300") {
		t.Errorf("without the option the entry should go to the default logger:\n%s", buf)
	}
}
