package web

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

var (
	ErrForbidden = errors.New("forbidden")
)

// WithIPFilter is a middleware that restricts access based on IP address.
// blockedIPs takes priority over allowedIPs.
// If allowedIPs is empty, all IPs are allowed.
// If blockedIPs is empty, no IPs are blocked.
//
// Supported formats for both lists:
//   - 127.0.0.1        (IPv4)
//   - ::1              (IPv6 loopback)
//   - 192.168.0.0/16   (CIDR network)
//   - 10.0.0.0/8       (CIDR network)
func WithIPFilter(h http.Handler, allowedIPs, blockedIPs []string) http.Handler {
	// If both allowedIPs and blockedIPs are empty, no IP filtering is necessary, return the handler as is
	if len(allowedIPs) == 0 && len(blockedIPs) == 0 {
		return h
	}

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			remoteAddr, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				slog.Error("Invalid remote address", "remoteAddress", r.RemoteAddr, "error", err)
				Encode(w, http.StatusInternalServerError, NewApiError(errors.New("invalid remote address")))
				return
			}

			slog.Debug("Checking IP address against IP Filter", "remoteAddress", remoteAddr, "method", r.Method, "path", r.URL.Path)

			if isIPBlocked(remoteAddr, blockedIPs) {
				slog.Warn("IP blocked", "remoteAddress", remoteAddr)
				Encode(w, http.StatusForbidden, NewApiError(ErrForbidden))
				return
			}

			// If the IP is not in the blocklist, check the allowlist
			if !isIPAllowed(remoteAddr, allowedIPs) {
				slog.Warn("IP not allowed", "remoteAddress", remoteAddr)
				Encode(w, http.StatusForbidden, NewApiError(ErrForbidden))
				return
			}

			// If the IP is allowed, proceed with the next handler
			h.ServeHTTP(w, r)
		},
	)
}

// isIPBlocked reports whether ip is in the blocklist.
func isIPBlocked(ip string, blockedIPs []string) bool {

	// If the blockedIPs is empty, nothing is blocked
	if len(blockedIPs) == 0 {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, blockedIP := range blockedIPs {
		if isMatchingIP(parsedIP, blockedIP) {
			return true
		}
	}

	return false
}

// isIPAllowed reports whether ip is in the allowlist.
func isIPAllowed(ip string, allowedIPs []string) bool {

	// If the allowlist is empty, allow all IPs
	if len(allowedIPs) == 0 {
		return true
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Check if the IP is in the allowlist
	for _, allowedIP := range allowedIPs {
		if isMatchingIP(parsedIP, allowedIP) {
			return true
		}
	}

	return false
}

// isMatchingIP reports whether parsedIP matches an IP address or CIDR network entry.
func isMatchingIP(parsedIP net.IP, pattern string) bool {
	if strings.Contains(pattern, "/") {
		if _, ipNet, err := net.ParseCIDR(pattern); err == nil {
			return ipNet.Contains(parsedIP)
		}
		return false
	}

	// Ensure the template IP is valid
	templateIP := net.ParseIP(pattern)
	return templateIP != nil && parsedIP.Equal(templateIP)
}
