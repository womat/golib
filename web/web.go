// Package web provides composable http.Handler middleware and JSON helpers for
// small HTTP services. It is middleware, not a framework: build a plain
// http.ServeMux and wrap it.
//
// Authentication accepts a bearer token whose SHA-256 hash is in Config.Tokens,
// usually read from a token file with ReadTokenFile and managed with the
// TokenCommand subcommand; the token's name then appears in the access log.
// There is no other way in: no API key, no X-Api-Key header, no JWT. CORS, IP
// filtering and request logging are independent wrappers, but their order
// matters - see the README for the ordering that is actually useful. Encode
// and Decode handle JSON bodies, WriteError produces a uniform JSON error body
// and logs the details that are not sent to the client.
//
// While serving a request, no part of this package reaches for slog.Default()
// on its own: WithLogging takes the logger and puts it into the request
// context, where WriteError and WithIPFilter pick it up through LoggerFrom.
// Only when nothing seeded the context does that fall back to slog.Default().
// The exception is WithIPFilter while it is built: it reports unusable list
// entries to slog.Default() unless WithIPFilterLogger names a logger.
//
// The access log follows the response status - error from 500, warn from 400,
// info below - so it is visible at the level a service runs at.
//
// # Example usage
//
//	func main() {
//	    logger := slog.Default()
//	    tokens, err := web.ReadTokenFile("/opt/demo/etc/token-hashes")
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    if err := web.CheckTokenFile("/opt/demo/etc/token-hashes"); err != nil {
//	        logger.Warn("Token file is not protected", "error", err)
//	    }
//	    cfg := web.Config{Tokens: tokens}
//
//	    mux := http.NewServeMux()
//	    mux.Handle("GET /health", web.WithAuth(handleHealth(), cfg))
//
//	    // Wrap in this order; the last wrapper is the outermost one, so
//	    // logging sees every request, rejected ones included.
//	    handler := web.WithCORS(mux, web.WithAllowedOrigins("https://example.com"))
//	    handler = web.WithIPFilter(handler, []string{"192.168.0.0/16"}, nil,
//	        web.WithIPFilterLogger(logger))
//	    handler = web.WithLogging(handler, logger)
//
//	    log.Fatal(http.ListenAndServeTLS(":8443", "cert.pem", "key.pem", handler))
//	}
//
//	func handleHealth() http.Handler {
//	    type healthResponse struct {
//	        OK bool `json:"ok"`
//	    }
//
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        web.Encode(w, http.StatusOK, healthResponse{OK: true})
//	    })
//	}
package web
