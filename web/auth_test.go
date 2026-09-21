package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/womat/golib/jwt_util"
)

const (
	testAppName = "testapp"
	testJwtID   = "test-id"
	testSecret  = "test-secret-not-a-real-one"
)

// okHandler reports whether it was reached.
func okHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

func testToken(t *testing.T, user string, lifetime time.Duration) string {
	t.Helper()
	token, err := jwt_util.GenerateToken(user, testAppName, "auth", testJwtID, testSecret, lifetime)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return token
}

func TestWithAuth(t *testing.T) {
	cfg := Config{
		ApiKey:    "the-api-key",
		JwtSecret: testSecret,
		JwtID:     testJwtID,
		AppName:   testAppName,
	}

	tests := []struct {
		name        string
		config      Config
		header      map[string]string
		wantStatus  int
		wantReached bool
	}{
		{
			name:        "valid api key",
			config:      cfg,
			header:      map[string]string{"X-Api-Key": "the-api-key"},
			wantStatus:  http.StatusOK,
			wantReached: true,
		},
		{
			name:       "wrong api key",
			config:     cfg,
			header:     map[string]string{"X-Api-Key": "nope"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "api key of the same length",
			config:     cfg,
			header:     map[string]string{"X-Api-Key": "the-api-kez"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no credentials at all",
			config:     cfg,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty config rejects everything",
			config:     Config{},
			header:     map[string]string{"X-Api-Key": ""},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "bearer without token",
			config:     cfg,
			header:     map[string]string{"Authorization": "Bearer"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "garbage token",
			config:     cfg,
			header:     map[string]string{"Authorization": "Bearer not.a.token"},
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

func TestWithAuthJwt(t *testing.T) {
	cfg := Config{JwtSecret: testSecret, JwtID: testJwtID, AppName: testAppName}

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"valid token", "Bearer " + testToken(t, "alice", time.Hour), http.StatusOK},
		{"expired token", "Bearer " + testToken(t, "alice", -time.Hour), http.StatusUnauthorized},
		{"scheme is case sensitive", "bearer " + testToken(t, "alice", time.Hour), http.StatusUnauthorized},
		{"double space", "Bearer  " + testToken(t, "alice", time.Hour), http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", tt.authHeader)
			rec := httptest.NewRecorder()

			WithAuth(okHandler(&reached), cfg).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestWithAuthJwtWrongIssuerOrID(t *testing.T) {
	cfg := Config{JwtSecret: testSecret, JwtID: testJwtID, AppName: testAppName}

	token, err := jwt_util.GenerateToken("alice", "other-app", "auth", "other-id", testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	reached := false
	WithAuth(okHandler(&reached), cfg).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if reached {
		t.Error("a token issued for another app was accepted")
	}
}

func TestWithAuthUnauthorizedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	reached := false
	WithAuth(okHandler(&reached), Config{ApiKey: "k"}).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got, want := rec.Body.String(), `{"error":"not authorized"}`; got != want+"\n" && got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
