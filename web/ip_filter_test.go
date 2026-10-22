package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// request builds a request coming from remoteAddr.
func requestFrom(remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	return req
}

func TestWithIPFilterEmptyListsArePassThrough(t *testing.T) {
	reached := false
	handler := okHandler(&reached)

	filtered := WithIPFilter(handler, nil, nil)

	rec := httptest.NewRecorder()
	filtered.ServeHTTP(rec, requestFrom("nonsense-without-a-port"))

	if !reached {
		t.Error("with both lists empty the handler must be used unchanged")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWithIPFilter(t *testing.T) {
	tests := []struct {
		name       string
		allowed    []string
		blocked    []string
		remoteAddr string
		wantStatus int
	}{
		{"allowlist hit", []string{"192.168.0.5"}, nil, "192.168.0.5:1234", http.StatusOK},
		{"allowlist miss", []string{"192.168.0.5"}, nil, "192.168.0.6:1234", http.StatusForbidden},
		{"allowlist cidr hit", []string{"192.168.0.0/24"}, nil, "192.168.0.99:1234", http.StatusOK},
		{"allowlist cidr miss", []string{"192.168.0.0/24"}, nil, "192.168.1.1:1234", http.StatusForbidden},
		{"blocklist hit", nil, []string{"10.0.0.1"}, "10.0.0.1:1234", http.StatusForbidden},
		{"blocklist miss", nil, []string{"10.0.0.1"}, "10.0.0.2:1234", http.StatusOK},
		{"blocklist wins over allowlist", []string{"10.0.0.0/8"}, []string{"10.0.0.1"}, "10.0.0.1:1234", http.StatusForbidden},
		{"ipv6 exact", []string{"::1"}, nil, "[::1]:1234", http.StatusOK},
		{"ipv6 cidr", []string{"2001:db8::/32"}, nil, "[2001:db8::1]:1234", http.StatusOK},
		{"ipv4 mapped ipv6", []string{"127.0.0.1"}, nil, "[::ffff:127.0.0.1]:1234", http.StatusOK},
		{"ipv4 mapped ipv6 in cidr", []string{"10.0.0.0/8"}, nil, "[::ffff:10.1.2.3]:1234", http.StatusOK},
		// A link-local client arrives with its zone; the lists name the address alone.
		{"link-local with zone allowed", []string{"fe80::/10"}, nil, "[fe80::1%en0]:1234", http.StatusOK},
		{"link-local with zone blocked", nil, []string{"fe80::1"}, "[fe80::1%en0]:1234", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			rec := httptest.NewRecorder()

			WithIPFilter(okHandler(&reached), tt.allowed, tt.blocked).
				ServeHTTP(rec, requestFrom(tt.remoteAddr))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if reached != (tt.wantStatus == http.StatusOK) {
				t.Errorf("handler reached = %v, want %v", reached, tt.wantStatus == http.StatusOK)
			}
		})
	}
}

func TestWithIPFilterMalformedRemoteAddr(t *testing.T) {
	for name, remoteAddr := range map[string]string{
		"no port":      "no-port-here",
		"host, not ip": "not-an-ip:1234",
	} {
		t.Run(name, func(t *testing.T) {
			reached := false
			rec := httptest.NewRecorder()

			WithIPFilter(okHandler(&reached), []string{"127.0.0.1"}, nil).
				ServeHTTP(rec, requestFrom(remoteAddr))

			if reached {
				t.Error("a request with an unparsable address reached the handler")
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d - an unparsable address is a rejection, not a server fault", rec.Code, http.StatusForbidden)
			}
		})
	}
}

func TestWithIPFilterInvalidEntries(t *testing.T) {
	tests := []struct {
		name       string
		allowed    []string
		blocked    []string
		remoteAddr string
		wantStatus int
	}{
		{
			name:       "invalid cidr in allowlist does not lock out the valid entry",
			allowed:    []string{"192.168.0.0/33", "192.168.0.5"},
			remoteAddr: "192.168.0.5:1234",
			wantStatus: http.StatusOK,
		},
		{
			name:       "hostname in allowlist is not a match",
			allowed:    []string{"localhost"},
			remoteAddr: "127.0.0.1:1234",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "invalid entry in blocklist blocks nothing",
			blocked:    []string{"10.0.0.0/99"},
			remoteAddr: "10.0.0.1:1234",
			wantStatus: http.StatusOK,
		},
		{
			name:       "an allowlist of only invalid entries locks everyone out",
			allowed:    []string{"192.168.0.0/33"},
			remoteAddr: "192.168.0.5:1234",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			rec := httptest.NewRecorder()

			WithIPFilter(okHandler(&reached), tt.allowed, tt.blocked).
				ServeHTTP(rec, requestFrom(tt.remoteAddr))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestValidateIPList(t *testing.T) {
	if err := ValidateIPList([]string{"192.168.0.1", " 10.0.0.0/8 ", "::1", "2001:db8::/32"}); err != nil {
		t.Errorf("valid list: %v", err)
	}
	if err := ValidateIPList(nil); err != nil {
		t.Errorf("empty list: %v", err)
	}

	for _, tt := range []struct{ entry, want string }{
		{"192.168.0.300", `invalid IP address "192.168.0.300"`},
		{"10.0.0.0/99", `invalid CIDR "10.0.0.0/99"`},
		{"localhost", `invalid IP address "localhost"`},
	} {
		err := ValidateIPList([]string{"192.168.0.1", tt.entry})
		if err == nil || err.Error() != tt.want {
			t.Errorf("%q: err = %v, want %s", tt.entry, err, tt.want)
		}
	}
}
