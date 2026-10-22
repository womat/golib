package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/womat/golib/v2/web"
)

// routedApp returns an App with its routes set up, accepting the bearer token
// "the-token".
func routedApp(t *testing.T, change func(*Config)) *App {
	t.Helper()

	cfg := validConfig()
	cfg.Webserver.Tokens = []web.Token{{Name: "nodered", Hash: web.HashToken("the-token")}}
	if change != nil {
		change(cfg)
	}

	app := New(cfg, t.TempDir(), nil, nil)
	t.Cleanup(app.cancelFunc)
	app.SetupRoutes()
	return app
}

func TestRoutes(t *testing.T) {
	app := routedApp(t, nil)

	tests := []struct {
		name       string
		method     string
		path       string
		token      string
		wantStatus int
	}{
		{"version is public", http.MethodGet, "/version", "", http.StatusOK},
		{"health needs a token", http.MethodGet, "/health", "", http.StatusUnauthorized},
		{"health refuses a wrong token", http.MethodGet, "/health", "other", http.StatusUnauthorized},
		{"health with the token", http.MethodGet, "/health", "the-token", http.StatusOK},
		{"preflight needs no token", http.MethodOptions, "/health", "", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			rec := httptest.NewRecorder()

			app.web.Handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			// WithCORS is outside the mux, so every answer carries it.
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
			}
		})
	}
}

func TestRoutesApplyTheIPFilter(t *testing.T) {
	app := routedApp(t, func(cfg *Config) {
		cfg.Webserver.AllowedIPs = []string{"10.0.0.0/8"}
	})

	req := httptest.NewRequest(http.MethodGet, "/version", nil) // from 192.0.2.1
	rec := httptest.NewRecorder()
	app.web.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
