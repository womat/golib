// Package xlog provides a simple wrapper around slog.Logger with flexible
// output destinations and log levels. It supports:
//
// - stdout, stderr, null (discard) logging
// - file logging with automatic append/create
// - log levels: debug, info (default), warning, error
// - source info, on by default for debug and selectable via WithSource
// - safe file cleanup via Close(), including under concurrent use
//
// # Example usage
//
//  1. Logging to stdout with debug messages and source info:
//     logger, err := xlog.Init("stdout", "debug")
//     if err != nil { panic(err) }
//     defer logger.Close()
//     logger.Debug("Debug message with source info")
//
//  2. Logging to a file with warning level:
//     logger, err := xlog.Init("/tmp/myapp.log", "warning")
//     if err != nil { panic(err) }
//     defer logger.Close()
//     logger.Warn("This is a warning")
//     logger.Info("This info will be ignored due to log level")
//
//  3. Source info at a level other than debug:
//     logger, err := xlog.Init("stdout", "info", xlog.WithSource(true))
//
//  4. Discarding all logs (useful in tests):
//     logger, _ := xlog.Init("null", "debug")
//     logger.Debug("This will not appear anywhere")
//
// After Close() the logger stays usable: what is written to it is discarded
// instead of going to a closed file handle.
package xlog

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// LoggerWrapper wraps a slog.Logger and owns the file handle when logging to a
// file. Call Close() to release it; for stdout, stderr and null it is a no-op.
type LoggerWrapper struct {
	*slog.Logger
	out *output
}

// output is the writer handed to the slog handler. It stays valid for the whole
// life of the logger: Close swaps the destination for io.Discard instead of
// leaving a closed file behind, so a late log call is defined to do nothing
// rather than failing invisibly - slog discards handler write errors.
type output struct {
	mu   sync.Mutex
	dest io.Writer
	file *os.File // nil unless logging to a file
}

// Write implements io.Writer. The lock also serialises writes against Close.
func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dest.Write(p)
}

// close releases the file handle, if any, and discards everything written from
// now on. It is safe to call from several goroutines and does nothing the
// second time.
func (o *output) close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.file == nil {
		return nil
	}

	file := o.file
	o.file = nil
	o.dest = io.Discard
	return file.Close()
}

// Option configures the logger created by Init.
type Option func(*options)

type options struct {
	source    bool
	sourceSet bool
}

// WithSource turns the source file and line on or off explicitly. Without it,
// source info is added for the debug level only.
func WithSource(enabled bool) Option {
	return func(o *options) {
		o.source = enabled
		o.sourceSet = true
	}
}

// Init initializes a slog.Logger with the given output destination and log level.
//
// Parameters:
//   - dest: "stdout", "stderr", "null", or a file path. The three names are
//     matched case-insensitively and ignoring surrounding blanks; anything else
//     is opened as a file, created if missing and appended to otherwise.
//   - logLevel: "debug", "info", "warning", "error" (default: info), matched
//     the same way
//   - opts: optional settings, currently WithSource
//
// Returns a LoggerWrapper and an error if the file cannot be opened.
func Init(dest string, logLevel string, opts ...Option) (*LoggerWrapper, error) {
	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}

	out := &output{}

	switch strings.ToLower(strings.TrimSpace(dest)) {
	case "stdout":
		out.dest = os.Stdout
	case "stderr":
		out.dest = os.Stderr
	case "null":
		out.dest = io.Discard
	default:
		file, err := os.OpenFile(dest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		out.dest = file
		out.file = file
	}

	var level slog.Level
	switch strings.ToLower(strings.TrimSpace(logLevel)) {
	case "debug":
		level = slog.LevelDebug
	case "error":
		level = slog.LevelError
	case "warning", "warn":
		level = slog.LevelWarn
	default:
		level = slog.LevelInfo
	}

	addSource := level == slog.LevelDebug
	if cfg.sourceSet {
		addSource = cfg.source
	}

	logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{
		AddSource: addSource,
		Level:     level}))

	return &LoggerWrapper{Logger: logger, out: out}, nil
}

// Close closes the file handle if logging to a file, and discards everything
// written afterwards.
//
// It is safe to call multiple times and from several goroutines, and it does
// nothing when logging to stdout, stderr or null.
func (l *LoggerWrapper) Close() error {
	return l.out.close()
}
