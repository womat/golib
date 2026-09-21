// Package app is the service skeleton this template is about: configuration
// with defaults and environment expansion, an App that owns the HTTP server,
// wait group, context and the restart/shutdown channels, signal-driven
// graceful shutdown and config reload, and the route table below.
//
// SetupRoutes registers:
//   - GET /version - public, no authentication
//   - GET /health - protected, API key or JWT via web.WithAuth
//   - OPTIONS / - the CORS preflight, as its own route so it never meets the
//     authentication middleware
//   - GET /swagger/ - only in builds made with -tags swagger
//
// and wraps the mux in web.WithCORS, web.WithIPFilter and web.WithLogging, in
// that order, handing all three the same logger. It must be called during
// startup, before the HTTP server starts.
package app

import (
	"log/slog"
	"net/http"

	"github.com/womat/golib/web"
)

// SetupRoutes configures all HTTP routes and global middleware for the application.
func (app *App) SetupRoutes() {
	webCfg := web.Config{
		ApiKey:    app.config.Webserver.ApiKey,
		JwtSecret: app.config.Webserver.JwtSecret,
		JwtID:     app.config.Webserver.JwtID,
		AppName:   MODULE,
	}

	mux := http.NewServeMux()

	// Preflight CORS requests
	mux.Handle("OPTIONS /", web.HandlePreflight())

	// Dev-only Swagger documentation (only registered with -tags swagger)
	app.registerSwaggerRoute(mux)

	// Public routes
	mux.Handle("GET /version", app.HandleVersion())

	// Protected routes
	mux.Handle("GET /health", web.WithAuth(app.HandleHealth(), webCfg))

	// One logger for the whole chain: WithLogging puts it into the request
	// context, where web.WriteError and the IP filter pick it up at request
	// time. The filter needs it handed over explicitly as well, because it
	// parses its lists here, before there is a request to carry anything.
	logger := slog.Default()

	// Apply global middleware. The last wrapper is the outermost one, so
	// logging sees every request, the ones the IP filter rejects included.
	handler := web.WithCORS(mux)
	handler = web.WithIPFilter(handler, app.config.Webserver.AllowedIPs, app.config.Webserver.BlockedIPs,
		web.WithIPFilterLogger(logger))
	handler = web.WithLogging(handler, logger)
	app.web.Handler = handler
}
