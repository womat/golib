package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler reports whether it was reached.
func okHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestWithAuth(t *testing.T) {
	cfg := Config{Tokens: []Token{{Name: "nodered", Hash: HashToken("the-token")}}}

	tests := []struct {
		name        string
		config      Config
		header      map[string]string
		wantStatus  int
		wantReached bool
	}{
		{
			name:        "valid token",
			config:      cfg,
			header:      map[string]string{"Authorization": "Bearer the-token"},
			wantStatus:  http.StatusOK,
			wantReached: true,
		},
		{
			name:        "bearer scheme in any case, more than one space",
			config:      cfg,
			header:      map[string]string{"Authorization": "bearer   the-token"},
			wantStatus:  http.StatusOK,
			wantReached: true,
		},
		{
			name:       "wrong token of the same length",
			config:     cfg,
			header:     map[string]string{"Authorization": "Bearer the-tokem"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no credentials at all",
			config:     cfg,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "bearer without token",
			config:     cfg,
			header:     map[string]string{"Authorization": "Bearer"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token under another scheme",
			config:     cfg,
			header:     map[string]string{"Authorization": "Basic the-token"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			// v1 accepted an API key here; v2 has no other way in than a token.
			name:       "token in the X-Api-Key header",
			config:     cfg,
			header:     map[string]string{"X-Api-Key": "the-token"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty config rejects everything",
			config:     Config{},
			header:     map[string]string{"Authorization": "Bearer the-token"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty config never matches an empty bearer token",
			config:     Config{},
			header:     map[string]string{"Authorization": "Bearer "},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			WithAuth(okHandler(&reached), tt.config).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if reached != tt.wantReached {
				t.Errorf("handler reached = %v, want %v", reached, tt.wantReached)
			}
		})
	}
}

func TestWithAuthUnauthorizedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	reached := false
	WithAuth(okHandler(&reached), Config{Tokens: []Token{{Name: "a", Hash: HashToken("k")}}}).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want Bearer", got)
	}
	if got, want := rec.Body.String(), `{"error":"not authorized"}`; got != want+"\n" && got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestClientSetIsBounded(t *testing.T) {
	s := newClientSet(2)
	if !s.add("a") || !s.add("b") {
		t.Fatal("first two clients not recorded")
	}
	if s.add("a") {
		t.Error("a client was recorded twice")
	}
	if s.add("c") {
		t.Error("a full set recorded another client")
	}
}
