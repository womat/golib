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

// to generate new self-signed certs for development, run:
// openssl req -x509 -newkey rsa:4096 -keyout dev_key.pem -out dev_cert.pem -days 365 -nodes -subj "/CN=localhost"
// mkdir -p certs
// cd certs
//
// # 1. generate Private Key
// openssl genrsa -out dev_key.pem 2048
//
// # 2. generate Self-signed Certificate
// openssl req -new -x509 -key dev_key.pem -out dev_cert.pem -days 365 -subj "/C=AT/ST=Vienna/L=Vienna/O=Dev/OU=Dev/CN=localhost"

//go:embed certs/dev_cert.pem
var embeddedCertFile []byte

//go:embed certs/dev_key.pem
var embeddedKeyFile []byte

// StartWebServer initializes and starts the HTTPS server asynchronously.
// - Production: uses file-based cert/key from config
// - Development fallback: embedded self-signed cert
// - Non-blocking: runs in goroutine
// - Graceful shutdown on app.ctx cancellation
func (app *App) StartWebServer() error {
	// Set default timeouts
	app.web.ReadTimeout = defaultReadTimeout
	app.web.WriteTimeout = defaultWriteTimeout
	app.web.IdleTimeout = defaultIdleTimeout

	// Load TLS certificate; the embedded development pair is only allowed in DevEnv
	cert, err := loadTLSCert(app.config.Webserver.CertFile, app.config.Webserver.KeyFile, app.config.IsDevEnv())
	if err != nil {
		return fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	// Assign TLS config
	app.web.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12}

	// Create listener
	listener, err := net.Listen("tcp", app.web.Addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	// Channel to report server runtime errors
	serverErrCh := make(chan error, 1)

	// Tracked by app.wg so that a shutdown waits for the listener to be closed;
	// otherwise a restart can race with the next net.Listen on the same port.
	app.wg.Go(func() {
		// ServeTLS blocks until Shutdown is called
		err := app.web.ServeTLS(listener, "", "")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}

		// Close the listener when ServeTLS exits, but ignore the normal
		// already-closed case during graceful shutdown.
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			slog.Error("Failed to close listener", "error", err)
		}
	})

	// Goroutine to monitor runtime errors and handle shutdown
	app.wg.Go(func() {

		select {
		case err := <-serverErrCh:
			slog.Error("Webserver runtime error, stopping the application", "error", err)
			// A service without its web server has no reason to keep running.
			// The shutdown has to happen outside app.wg: shutdownProcedure
			// waits for that very wait group.
			go app.shutdownProcedure(ModeStop)
		case <-app.ctx.Done():
			ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := app.web.Shutdown(ctxShutdown); err != nil {
				slog.Error("Failed to shutdown webserver gracefully", "error", err)
			} else {
				slog.Info("Webserver stopped gracefully")
			}
		}
	})

	return nil
}

// loadTLSCert loads the configured certificate and key.
//
// Only in a development environment does a missing file fall back to the
// embedded self-signed pair. That pair is public - it lives in this repository -
// and is good for localhost and nothing else, so in production a missing file is
// a startup error instead of a silently insecure server.
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
