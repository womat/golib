package xlog

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// logFile returns a path in a temp directory that is removed with the test.
func logFilePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.log")
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestFileDestination(t *testing.T) {
	path := logFilePath(t)

	logger, err := Init(path, "warning")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	logger.Debug("debug is below the level")
	logger.Info("info is below the level")
	logger.Warn("warning is written")
	logger.Error("error is written")

	if err = logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	content := readLog(t, path)
	for _, want := range []string{"warning is written", "error is written"} {
		if !strings.Contains(content, want) {
			t.Errorf("log file does not contain %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{"debug is below", "info is below"} {
		if strings.Contains(content, unwanted) {
			t.Errorf("log file contains %q although it is below the level", unwanted)
		}
	}
}

func TestFileDestinationAppends(t *testing.T) {
	path := logFilePath(t)

	for _, msg := range []string{"first run", "second run"} {
		logger, err := Init(path, "info")
		if err != nil {
			t.Fatalf("Init: %v", err)
		}
		logger.Info(msg)
		if err = logger.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	content := readLog(t, path)
	if !strings.Contains(content, "first run") || !strings.Contains(content, "second run") {
		t.Errorf("a second Init truncated the file instead of appending:\n%s", content)
	}
}

func TestWriteAfterCloseIsDiscarded(t *testing.T) {
	path := logFilePath(t)

	logger, err := Init(path, "info")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	logger.Info("before close")

	if err = logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Must neither panic nor reach the file.
	logger.Info("after close")
	logger.Error("also after close")

	content := readLog(t, path)
	if !strings.Contains(content, "before close") {
		t.Errorf("the line written before Close is missing:\n%s", content)
	}
	if strings.Contains(content, "after close") {
		t.Errorf("a line written after Close reached the file:\n%s", content)
	}
}

func TestWriteAfterCloseSucceeds(t *testing.T) {
	// The writer itself must accept and drop the bytes. A closed file handle
	// would return "file already closed" here, and slog would swallow it - the
	// difference is invisible from the outside, which is why it is tested here.
	logger, err := Init(logFilePath(t), "info")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err = logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	line := []byte("written after close\n")
	n, err := logger.out.Write(line)
	if err != nil {
		t.Errorf("Write after Close: %v, want nil", err)
	}
	if n != len(line) {
		t.Errorf("Write after Close wrote %d bytes, want %d", n, len(line))
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	logger, err := Init(logFilePath(t), "info")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err = logger.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err = logger.Close(); err != nil {
		t.Errorf("second Close: %v, want nil", err)
	}
}

func TestCloseIsSafeForConcurrentUse(t *testing.T) {
	// Run under -race: Close and the log calls share the writer.
	logger, err := Init(logFilePath(t), "info")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			logger.Info("concurrent write")
		}()
		go func() {
			defer wg.Done()
			if err := logger.Close(); err != nil {
				t.Errorf("concurrent Close: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestCloseWithoutFile(t *testing.T) {
	for _, dest := range []string{"stdout", "stderr", "null"} {
		logger, err := Init(dest, "info")
		if err != nil {
			t.Fatalf("Init(%q): %v", dest, err)
		}
		if err = logger.Close(); err != nil {
			t.Errorf("Close for %q: %v, want nil", dest, err)
		}
	}
}

func TestDestinationNames(t *testing.T) {
	// Surrounding blanks and capitalisation must not turn a well-known name
	// into a file of that name in the working directory.
	for _, dest := range []string{"STDOUT", " stdout ", "Stderr", "NULL", "\tnull\n"} {
		logger, err := Init(dest, "info")
		if err != nil {
			t.Fatalf("Init(%q): %v", dest, err)
		}
		logger.Close()

		for _, name := range []string{dest, strings.TrimSpace(dest)} {
			if _, err = os.Stat(name); err == nil {
				os.Remove(name)
				t.Errorf("Init(%q) created the file %q", dest, name)
			}
		}
	}
}

func TestInitReportsFileError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "test.log")

	if _, err := Init(path, "info"); err == nil {
		t.Errorf("Init(%q) returned no error although the directory does not exist", path)
	}
}

func TestLevels(t *testing.T) {
	tests := []struct {
		level     string
		wantDebug bool
		wantInfo  bool
		wantWarn  bool
	}{
		{"debug", true, true, true},
		{"info", false, true, true},
		{"warning", false, false, true},
		{"warn", false, false, true},
		{"error", false, false, false},
		{"", false, true, true},         // default is info
		{"nonsense", false, true, true}, // unknown level falls back to info
		{" DEBUG ", true, true, true},   // trimmed and case-insensitive
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			path := logFilePath(t)
			logger, err := Init(path, tt.level)
			if err != nil {
				t.Fatalf("Init: %v", err)
			}

			logger.Debug("dbg")
			logger.Info("inf")
			logger.Warn("wrn")
			if err = logger.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			content := readLog(t, path)
			if got := strings.Contains(content, "dbg"); got != tt.wantDebug {
				t.Errorf("debug logged = %v, want %v", got, tt.wantDebug)
			}
			if got := strings.Contains(content, "inf"); got != tt.wantInfo {
				t.Errorf("info logged = %v, want %v", got, tt.wantInfo)
			}
			if got := strings.Contains(content, "wrn"); got != tt.wantWarn {
				t.Errorf("warn logged = %v, want %v", got, tt.wantWarn)
			}
		})
	}
}

func TestSource(t *testing.T) {
	tests := []struct {
		name       string
		level      string
		opts       []Option
		wantSource bool
	}{
		{"debug implies source", "debug", nil, true},
		{"info has no source", "info", nil, false},
		{"info with WithSource", "info", []Option{WithSource(true)}, true},
		{"debug without source", "debug", []Option{WithSource(false)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := logFilePath(t)
			logger, err := Init(path, tt.level, tt.opts...)
			if err != nil {
				t.Fatalf("Init: %v", err)
			}

			logger.Error("message")
			if err = logger.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			if got := strings.Contains(readLog(t, path), "source="); got != tt.wantSource {
				t.Errorf("source info present = %v, want %v", got, tt.wantSource)
			}
		})
	}
}
