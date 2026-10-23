package app

import (
	"log/slog"
	"net/http"

	"github.com/womat/golib/v2/web"
)

// setupRoutes configures all HTTP routes and global middleware for the application:
//   - GET /version - public, no authentication
//   - GET /health - protected, a bearer token from the token file via web.WithAuth
//   - GET /swagger/ - only in builds made with -tags swagger
//
// It wraps the mux in web.WithCORS, web.WithIPFilter and web.WithLogging, in
// that order; the IP filter and the logging write to the same logger. WithCORS
// answers the CORS preflight before the mux, so it never meets the
// authentication middleware; without an "OPTIONS /" route the mux answers 404
// and 405 correctly. It must be called during startup, before the HTTP server
// starts.
func (app *App) setupRoutes() {
	webCfg := app.webConfig()

	mux := http.NewServeMux()

	// Dev-only Swagger documentation (only registered with -tags swagger)
	app.registerSwaggerRoute(mux)

	// Public routes
	mux.Handle("GET /version", app.handleVersion())

	// Protected routes
	mux.Handle("GET /health", web.WithAuth(app.handleHealth(), webCfg))

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

// webConfig is the authentication of the protected routes: the bearer tokens from the token
// file, nothing else.
func (app *App) webConfig() web.Config {
	return web.Config{Tokens: app.config.Webserver.Tokens}
}
