package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
		{
			name:        "api key as bearer token",
			config:      cfg,
			header:      map[string]string{"Authorization": "Bearer the-api-key"},
			wantStatus:  http.StatusOK,
			wantReached: true,
		},
		{
			name:        "bearer scheme in any case, more than one space",
			config:      cfg,
			header:      map[string]string{"Authorization": "bearer   the-api-key"},
			wantStatus:  http.StatusOK,
			wantReached: true,
		},
		{
			name:       "wrong bearer token of the same length",
			config:     cfg,
			header:     map[string]string{"Authorization": "Bearer the-api-kez"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "api key under another scheme",
			config:     cfg,
			header:     map[string]string{"Authorization": "Basic the-api-key"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty api key never matches an empty bearer token",
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

func TestWithAuthJwt(t *testing.T) {
	cfg := Config{JwtSecret: testSecret, JwtID: testJwtID, AppName: testAppName}

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"valid token", "Bearer " + testToken(t, "alice", time.Hour), http.StatusOK},
		{"expired token", "Bearer " + testToken(t, "alice", -time.Hour), http.StatusUnauthorized},
		{"scheme in any case", "bearer " + testToken(t, "alice", time.Hour), http.StatusOK},
		{"double space", "Bearer  " + testToken(t, "alice", time.Hour), http.StatusOK},
		{"other scheme", "Basic " + testToken(t, "alice", time.Hour), http.StatusUnauthorized},
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
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want Bearer", got)
	}
	if got, want := rec.Body.String(), `{"error":"not authorized"}`; got != want+"\n" && got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// TestWithAuthWarnsAboutXApiKey: a request authorized through X-Api-Key logs the
// deprecation warning once per client, a bearer token never does.
func TestWithAuthWarnsAboutXApiKey(t *testing.T) {
	logger, buf := bufferLogger()
	handler := WithAuth(okHandler(new(bool)), Config{ApiKey: "the-api-key"})

	send := func(remoteAddr string, header, value string) int {
		req := httptest.NewRequest(http.MethodGet, "/health", nil).
			WithContext(ContextWithLogger(context.Background(), logger))
		req.RemoteAddr = remoteAddr
		req.Header.Set(header, value)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	warnings := func() int { return strings.Count(buf.String(), "X-Api-Key is deprecated") }

	if code := send("192.0.2.1:5000", "Authorization", "Bearer the-api-key"); code != http.StatusOK || warnings() != 0 {
		t.Fatalf("bearer: status %d, %d warnings; want 200 and none", code, warnings())
	}
	if code := send("192.0.2.1:5000", "X-Api-Key", "the-api-key"); code != http.StatusOK || warnings() != 1 {
		t.Fatalf("first X-Api-Key: status %d, %d warnings; want 200 and one", code, warnings())
	}
	send("192.0.2.1:5001", "X-Api-Key", "the-api-key") // same client, another port
	if warnings() != 1 {
		t.Errorf("same client warned again: %d warnings", warnings())
	}
	send("192.0.2.2:5000", "X-Api-Key", "the-api-key")
	if warnings() != 2 {
		t.Errorf("second client not warned: %d warnings", warnings())
	}
	send("192.0.2.3:5000", "X-Api-Key", "wrong")
	if warnings() != 2 {
		t.Errorf("a rejected request was warned about: %d warnings", warnings())
	}
	if !strings.Contains(buf.String(), "client=192.0.2.2") {
		t.Errorf("warning does not name the client:\n%s", buf.String())
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
