package web

import (
	"context"
	"log/slog"
)

// loggerKey is the context key the request logger is stored under. It is a
// private struct type, so no other package can collide with it.
type loggerKey struct{}

// ContextWithLogger returns a copy of ctx that carries logger.
//
// WithLogging does this for every request it handles, which is how WriteError
// and WithIPFilter find the logger the application configured instead of
// reaching for the global one. Call it directly when a service wants that
// behaviour without WithLogging in the chain.
func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerKey{}, logger)
}

// LoggerFrom returns the logger stored in ctx by ContextWithLogger or
// WithLogging.
//
// It falls back to slog.Default() when there is none, so a handler that is used
// without any of this package's middleware still logs somewhere instead of
// staying silent. Wrap with WithLogging - or seed the context yourself - to
// send these entries to the application's own logger.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
