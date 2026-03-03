// Package xlog provides a simple wrapper around slog.Logger with flexible
// output destinations and log levels. It supports:
//
// - stdout, stderr, null (discard) logging
// - file logging with automatic append/create
// - log levels: debug, info (default), warning, error
// - optional source info for debug logs
// - safe file cleanup via Close()
//
// Example usage:
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
//  3. Discarding all logs (useful in tests):
//     logger, _ := xlog.Init("null", "debug")
//     logger.Debug("This will not appear anywhere")
package xlog

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// LoggerWrapper wraps a slog.Logger and optionally keeps a file handle
// for proper cleanup. Use Close() if you log to a file to release resources.
type LoggerWrapper struct {
	*slog.Logger
	*os.File // Optional file handle; nil if not logging to file
}

// Init initializes the slog logger with the given log destinations and log level.
// dest: stdout, stderr, /path/to/logfile
// addSource: add go source file name and line number to log output

// Init initializes a slog.Logger with the given output destination and log level.
//
// Parameters:
//   - dest: "stdout", "stderr", "null", or a file path
//   - logLevel: "debug", "info", "warning", "error" (default: info)
//
// Returns a LoggerWrapper and an error if the file cannot be opened.
func Init(dest string, logLevel string) (*LoggerWrapper, error) {
	var err error
	var writer io.Writer
	var logFile *os.File
	var level slog.Level

	switch strings.ToLower(dest) {
	case "stdout":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	case "null":
		writer = io.Discard
	default:
		if logFile, err = os.OpenFile(dest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
			return nil, err
		}
		writer = logFile
	}

	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "error":
		level = slog.LevelError
	case "warning", "warn":
		level = slog.LevelWarn
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{
		AddSource: level == slog.LevelDebug,
		Level:     level}))
	return &LoggerWrapper{Logger: logger, File: logFile}, nil
}

// Close closes the file handle if logging to a file.
// Safe to call multiple times; does nothing if logging to stdout/stderr/null.
func (l *LoggerWrapper) Close() error {
	if l.File != nil {
		err := l.File.Close()
		l.File = nil
		return err
	}
	return nil
}
