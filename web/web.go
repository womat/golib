// Package web provides composable http.Handler middleware and JSON helpers for
// small HTTP services. It is middleware, not a framework: build a plain
// http.ServeMux and wrap it.
//
// Authentication accepts either an API key in the X-Api-Key header or a JWT in
// Authorization: Bearer, validated through jwt_util. CORS, IP filtering and
// request logging are independent wrappers that can be combined in any order -
// see the README for the ordering that is actually useful. Encode and Decode
// handle JSON bodies, WriteError produces a uniform JSON error body and logs
// the details that are not sent to the client.
//
// # Example usage
//
//	func main() {
//	    cfg := web.Config{ApiKey: os.Getenv("API_KEY"), AppName: "demo"}
//
//	    mux := http.NewServeMux()
//	    mux.Handle("OPTIONS /", web.HandlePreflight())
//	    mux.Handle("GET /health", web.WithAuth(handleHealth(), cfg))
//
//	    // Wrap in this order; the last wrapper is the outermost one, so
//	    // logging sees every request, rejected ones included.
//	    handler := web.WithCORS(mux, web.WithAllowedOrigins("https://example.com"))
//	    handler = web.WithIPFilter(handler, []string{"192.168.0.0/16"}, nil)
//	    handler = web.WithLogging(handler, slog.Default())
//
//	    log.Fatal(http.ListenAndServe(":8443", handler))
//	}
//
//	func handleHealth() http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        req, err := web.Decode[healthRequest](w, r)
//	        if err != nil {
//	            web.WriteError(w, r, http.StatusBadRequest, err)
//	            return
//	        }
//	        web.Encode(w, http.StatusOK, healthResponse{OK: true, Probe: req.Probe})
//	    })
//	}
package web
