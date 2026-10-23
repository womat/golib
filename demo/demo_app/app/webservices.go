package app

import (
	"context"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	defaultReadTimeout  = 10 * time.Second
	defaultWriteTimeout = 15 * time.Second
	defaultIdleTimeout  = 120 * time.Second
)

// The development pair is generated, not checked in: run "make ensure_dev_certs"
// before the first build. It is only used in DevEnv, see loadTLSCert.

//go:embed certs/dev_cert.pem
var embeddedCertFile []byte

//go:embed certs/dev_key.pem
var embeddedKeyFile []byte

// newTLSConfig serves cert with TLS 1.3 only. Every caller in this setup (browsers, Node-RED on
// Node 20, curl) speaks it, and 1.3 leaves no cipher suite to choose badly.
func newTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}
}

// startWebServer initializes and starts the HTTPS server asynchronously.
// - Uses the file-based cert/key from config
// - Development fallback (DevEnv only): embedded self-signed cert
// - Non-blocking: runs in goroutine
// - Graceful shutdown on app.ctx cancellation
// - A server that stops on its own is reported on app.serverErr, which restarts the App
func (app *App) startWebServer() error {
	// Set default timeouts
	app.web.ReadTimeout = defaultReadTimeout
	app.web.WriteTimeout = defaultWriteTimeout
	app.web.IdleTimeout = defaultIdleTimeout

	// A certificate that is missing, unreadable or broken needs someone to fix it.
	cert, err := loadTLSCert(app.config.Webserver.CertFile, app.config.Webserver.KeyFile, app.config.IsDevEnv())
	if err != nil {
		return fmt.Errorf("failed to load TLS certificate: %w (%w)", err, ErrPermanent)
	}

	app.web.TLSConfig = newTLSConfig(cert)

	// An IPv4 listenHost such as 0.0.0.0, and an empty one, listens on IPv4 only. With "tcp"
	// Linux would open a dual-stack socket, and IPv6 clients would bypass blockedIPs and
	// allowedIPs written as IPv4. Set listenHost to "::" to serve IPv6 as well.
	network := "tcp"
	if host := app.config.Webserver.ListenHost; host == "" {
		network = "tcp4"
	} else if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		network = "tcp4"
	}
	listener, err := net.Listen(network, app.web.Addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	// Record the address actually bound, so a test that listens on port 0 learns its port.
	// Nothing in the service reads Addr after this point.
	app.web.Addr = listener.Addr().String()

	// ServeTLS blocks until Shutdown and closes the listener when it returns. A server that
	// stops on its own is reported to the signal handler, which restarts the App; it must not
	// be done from here, because the restart waits for this goroutine via app.wg.
	app.wg.Go(func() {
		if err := app.web.ServeTLS(listener, "", ""); !errors.Is(err, http.ErrServerClosed) {
			app.serverErr <- err
		}
	})

	app.wg.Go(func() {
		<-app.ctx.Done()
		ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.web.Shutdown(ctxShutdown); err != nil {
			slog.Error("Failed to shutdown webserver gracefully", "error", err)
			// Close what is still active, so no request outlives this App and its config.
			_ = app.web.Close()
		} else {
			slog.Info("Webserver stopped gracefully")
		}
	})

	return nil
}

// loadTLSCert loads the configured certificate and key.
//
// Only in a development environment does a missing file fall back to the
// embedded self-signed pair. That pair's private key ships in every binary and
// is good for localhost and nothing else, so in production a missing file is a
// startup error instead of a silently insecure server.
func loadTLSCert(certFile, keyFile string, devEnv bool) (tls.Certificate, error) {
	certMissing, err := isMissing(certFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to read cert file: %w", err)
	}

	keyMissing, err := isMissing(keyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to read key file: %w", err)
	}

	if !certMissing && !keyMissing {
		return tls.LoadX509KeyPair(certFile, keyFile)
	}

	if !devEnv {
		return tls.Certificate{}, fmt.Errorf(
			"TLS certificate or key missing (cert %q, key %q) - the embedded development certificate is only used in %s",
			certFile, keyFile, DevEnv)
	}

	slog.Warn("TLS cert or key file not found, using the embedded development certificate",
		"certFile", certFile, "keyFile", keyFile, "env", DevEnv)
	return tls.X509KeyPair(embeddedCertFile, embeddedKeyFile)
}

// isMissing reports whether the path does not exist. An empty path counts as
// missing; any other stat error is passed on.
func isMissing(path string) (bool, error) {
	if path == "" {
		return true, nil
	}

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}

	return false, nil
}
