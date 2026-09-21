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

// IPFilterOption configures WithIPFilter.
type IPFilterOption func(*ipFilterConfig)

type ipFilterConfig struct {
	logger *slog.Logger
}

// WithIPFilterLogger reports unusable list entries to logger instead of
// slog.Default().
//
// It exists because the lists are parsed while the middleware is built, before
// there is a request whose context could carry a logger. The rejections at
// request time need no option: they go to the logger WithLogging put into the
// context.
func WithIPFilterLogger(logger *slog.Logger) IPFilterOption {
	return func(c *ipFilterConfig) {
		c.logger = logger
	}
}

// ipList is a parsed allow or block list. Parsing happens once, when the
// middleware is built, not on every request.
type ipList struct {
	addrs []net.IP
	nets  []*net.IPNet
	// configured reports whether the caller passed any entry at all, so that a
	// list of nothing but invalid entries can be told from an empty one.
	configured bool
}

// parseIPList turns the configured strings into addresses and networks.
// An entry that is neither is reported and skipped: a typo must not silently
// turn into a rule that matches nothing.
func parseIPList(log *slog.Logger, name string, entries []string) ipList {
	list := ipList{configured: len(entries) > 0}

	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)

		if strings.Contains(trimmed, "/") {
			if _, ipNet, err := net.ParseCIDR(trimmed); err == nil {
				list.nets = append(list.nets, ipNet)
				continue
			}
			log.Error("Invalid CIDR in IP filter, entry ignored", "list", name, "entry", entry)
			continue
		}

		if ip := net.ParseIP(trimmed); ip != nil {
			list.addrs = append(list.addrs, ip)
			continue
		}
		log.Error("Invalid IP address in IP filter, entry ignored", "list", name, "entry", entry)
	}

	if list.configured && len(list.addrs) == 0 && len(list.nets) == 0 {
		log.Error("IP filter list has no usable entry left", "list", name, "entries", entries)
	}

	return list
}

// empty reports whether the list holds no usable entry.
func (l ipList) empty() bool {
	return len(l.addrs) == 0 && len(l.nets) == 0
}

// contains reports whether ip matches one of the addresses or networks.
func (l ipList) contains(ip net.IP) bool {
	for _, addr := range l.addrs {
		if addr.Equal(ip) {
			return true
		}
	}
	for _, ipNet := range l.nets {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

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
//
// Both lists are parsed once, here. An unusable entry is logged and skipped; if
// that leaves a configured allowlist without a single usable entry, everything
// is rejected - the safe direction, and loud enough to be noticed.
//
// The filter looks at r.RemoteAddr only. Behind a reverse proxy that is the
// proxy's address, which makes the filter a no-op; see the README.
//
// Rejections are logged to the logger WithLogging put into the request context;
// the entries written while the lists are parsed go to slog.Default() unless
// WithIPFilterLogger names one.
func WithIPFilter(h http.Handler, allowedIPs, blockedIPs []string, opts ...IPFilterOption) http.Handler {
	// If both allowedIPs and blockedIPs are empty, no IP filtering is necessary, return the handler as is
	if len(allowedIPs) == 0 && len(blockedIPs) == 0 {
		return h
	}

	cfg := ipFilterConfig{logger: slog.Default()}
	for _, opt := range opts {
		opt(&cfg)
	}

	allowed := parseIPList(cfg.logger, "allowed", allowedIPs)
	blocked := parseIPList(cfg.logger, "blocked", blockedIPs)

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			log := LoggerFrom(r.Context())

			remoteAddr, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				log.Warn("Invalid remote address, request rejected", "remoteAddress", r.RemoteAddr, "error", err)
				Encode(w, http.StatusForbidden, NewApiError(ErrForbidden))
				return
			}

			ip := net.ParseIP(remoteAddr)
			if ip == nil {
				log.Warn("Unparsable remote address, request rejected", "remoteAddress", remoteAddr)
				Encode(w, http.StatusForbidden, NewApiError(ErrForbidden))
				return
			}

			log.Debug("Checking IP address against IP Filter", "remoteAddress", remoteAddr, "method", r.Method, "path", r.URL.Path)

			if blocked.contains(ip) {
				log.Warn("IP blocked", "remoteAddress", remoteAddr)
				Encode(w, http.StatusForbidden, NewApiError(ErrForbidden))
				return
			}

			// An allowlist that was configured but holds nothing usable rejects
			// everything; one that was never configured allows everything.
			if !allowed.empty() && !allowed.contains(ip) ||
				allowed.empty() && allowed.configured {
				log.Warn("IP not allowed", "remoteAddress", remoteAddr)
				Encode(w, http.StatusForbidden, NewApiError(ErrForbidden))
				return
			}

			// If the IP is allowed, proceed with the next handler
			h.ServeHTTP(w, r)
		},
	)
}
