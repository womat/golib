package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSDefault(t *testing.T) {
	reached := false
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	WithCORS(okHandler(&reached)).ServeHTTP(rec, req)

	if !reached {
		t.Error("the request did not reach the handler")
	}

	want := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "POST, GET, OPTIONS, PUT, PATCH, DELETE",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
		"Access-Control-Max-Age":       "86400",
	}
	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
}

func TestWithCORSPassesEveryMethodThrough(t *testing.T) {
	// A plain OPTIONS, without Access-Control-Request-Method, is no preflight
	// and belongs to the handler.
	reached := false
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()

	WithCORS(okHandler(&reached)).ServeHTTP(rec, req)

	if !reached {
		t.Error("OPTIONS did not reach the handler")
	}
}

func TestWithCORSAnswersPreflight(t *testing.T) {
	reached := false
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()

	WithCORS(okHandler(&reached)).ServeHTTP(rec, req)

	if reached {
		t.Error("the preflight reached the handler - behind WithAuth it would get 401")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
}

// Without an "OPTIONS /" route the mux tells an unknown path (404) from a
// known one asked with the wrong method (405); WithCORS still answers the
// preflight for both.
func TestWithCORSLetsTheMuxAnswer404And405(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /known", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	handler := WithCORS(mux)

	for _, tt := range []struct {
		method, path string
		preflight    bool
		want         int
	}{
		{http.MethodGet, "/known", false, http.StatusOK},
		{http.MethodGet, "/nope", false, http.StatusNotFound},
		{http.MethodPost, "/known", false, http.StatusMethodNotAllowed},
		{http.MethodOptions, "/known", true, http.StatusNoContent},
	} {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		if tt.preflight {
			req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("%s %s: status = %d, want %d", tt.method, tt.path, rec.Code, tt.want)
		}
	}
}

func TestHandlePreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()

	HandlePreflight().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestWithCORSAllowedOrigins(t *testing.T) {
	tests := []struct {
		name          string
		origins       []string
		requestOrigin string
		wantOrigin    string
		wantVary      bool
	}{
		{"listed origin is echoed", []string{"https://a.example", "https://b.example"}, "https://b.example", "https://b.example", true},
		{"unlisted origin gets no header", []string{"https://a.example"}, "https://evil.example", "", true},
		{"no origin header at all", []string{"https://a.example"}, "", "", true},
		{"explicit wildcard stays wildcard", []string{"*"}, "https://a.example", "*", true},
		{"without the option everything is allowed", nil, "https://a.example", "*", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []CORSOption
			if tt.origins != nil {
				opts = append(opts, WithAllowedOrigins(tt.origins...))
			}

			reached := false
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}
			rec := httptest.NewRecorder()

			WithCORS(okHandler(&reached), opts...).ServeHTTP(rec, req)

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.wantOrigin)
			}
			if got := rec.Header().Get("Vary") == "Origin"; got != tt.wantVary {
				t.Errorf("Vary: Origin present = %v, want %v", got, tt.wantVary)
			}
			if !reached {
				t.Error("the request did not reach the handler")
			}
		})
	}
}

func TestWithCORSMethodsAndHeaders(t *testing.T) {
	reached := false
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	WithCORS(okHandler(&reached),
		WithAllowedMethods("GET", "POST"),
		WithAllowedHeaders("Content-Type", "X-Request-Id"),
	).ServeHTTP(rec, req)

	if got, want := rec.Header().Get("Access-Control-Allow-Methods"), "GET, POST"; got != want {
		t.Errorf("Allow-Methods = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Access-Control-Allow-Headers"), "Content-Type, X-Request-Id"; got != want {
		t.Errorf("Allow-Headers = %q, want %q", got, want)
	}
}
