// Package app is the service skeleton this template is about: configuration
// with defaults and environment expansion, an App that owns the HTTP server,
// wait group and context, signal-driven graceful shutdown and config reload,
// and the route table in setupRoutes. One App lives for one configuration;
// cmd/main.go builds a new one on every reload.
//
// Usage:
//
//	cfg, err := app.LoadConfig(file)          // then cfg.Validate()
//	signals := make(chan os.Signal, 1)
//	signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
//	a, err := app.New(cfg, signals, checkReload).Run()
//	<-a.Done()
//	if a.Restarting() { /* build the next App */ } else { /* exit */ }
package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"

	"github.com/womat/golib/v2/web"
)

// VERSION is the application version, following semantic versioning
// as described in https://semver.org/.
//
// It is not maintained in source: the Git tag is the single source of truth and
// the value is injected at build time via -ldflags (see Makefile). The "dev"
// default applies to builds made without them.
var VERSION = "dev"

const MODULE = "demo_app"

// mode is what shutdownProcedure does after the App has stopped.
type mode string

const (
	modeStop    mode = "stop"    // the process exits
	modeRestart mode = "restart" // cmd builds the next App with the reloaded configuration
)

// ErrPermanent marks a start failure that a restart cannot fix, such as a TLS certificate that
// cannot be loaded: it needs someone to repair it. cmd ends demo_app with exit code 78 for it, so
// systemd leaves the service stopped instead of restarting it into the same error.
var ErrPermanent = errors.New("restarting does not help")

// App is where the application is wired up: one App lives for one configuration.
type App struct {
	wg          sync.WaitGroup   // tracks the web server goroutines; add your own here
	config      *Config          // app configuration
	web         *http.Server     // HTTP server
	signals     <-chan os.Signal // OS signals, subscribed once by the caller for all lifecycles
	checkReload func() error     // loads and validates the config file before a SIGHUP restart
	serverErr   chan error       // reports a web server that stopped on its own
	done        chan struct{}    // closed once the App has stopped, see Done
	restarting  bool             // set before done is closed, see Restarting
	ctx         context.Context
	cancelFunc  context.CancelFunc

	// add your additional handler here
}

// New initializes the App struct but does not start services.
//
// signals must already be subscribed (signal.Notify) to SIGHUP, SIGTERM and SIGINT, and stay
// subscribed across restarts: a signal arriving while one App is torn down and the next is
// built then waits in the channel for the next App, instead of hitting the default action,
// which would end the process.
//
// checkReload is called on SIGHUP before anything is torn down. If it reports an error, the
// restart is refused and the App keeps running with its current configuration, so a broken
// config file cannot stop the service. Passing nil skips the check.
func New(config *Config, signals <-chan os.Signal, checkReload func() error) *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		config:      config,
		signals:     signals,
		checkReload: checkReload,
		web: &http.Server{
			Addr: net.JoinHostPort(config.Webserver.ListenHost, strconv.Itoa(config.Webserver.ListenPort)),
		},
		serverErr:  make(chan error, 1),
		done:       make(chan struct{}),
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// Run initializes the application, starts the services and the web server, and sets up OS
// signal handling.
func (app *App) Run() (*App, error) {
	slog.Debug("Initializing application")

	if err := app.setup(); err != nil {
		return app, app.abort(err)
	}

	// here start your services

	slog.Info("Starting web server", "url", app.web.Addr)
	if err := app.startWebServer(); err != nil {
		slog.Error("Web server failed to start", "url", app.web.Addr, "error", err)
		return app, app.abort(err)
	}

	// Handle the OS signals only once nothing can fail any more. A signal that is already
	// waiting would otherwise start a shutdown alongside abort, and be lost to the next App.
	app.handleSignals()

	slog.Info("Module started successfully",
		"module", MODULE,
		"version", VERSION,
		"pid", os.Getpid(),
	)
	return app, nil
}

// setup prepares the application:
// - initializes the services
// - initializes API routes
func (app *App) setup() error {

	// here initialize your services

	// Checked on every start and reload, so a fixed layout stops the warning
	// without a restart.
	if err := web.CheckTokenFile(app.config.Webserver.TokenFile); err != nil {
		slog.Warn("Token file is not protected", "error", err)
	}

	// setupRoutes should always be called at the end
	slog.Debug("Initializing API routes")
	app.setupRoutes()
	app.logTokens()

	return nil
}

// logTokens names the bearer tokens the protected routes accept, or warns that there are none.
func (app *App) logTokens() {
	names := make([]string, 0, len(app.config.Webserver.Tokens))
	for _, t := range app.config.Webserver.Tokens {
		names = append(names, t.Name)
	}
	if len(names) == 0 {
		slog.Warn("No bearer tokens loaded, every request to /health is rejected",
			"tokenFile", app.config.Webserver.TokenFile)
		return
	}
	slog.Info("Bearer tokens loaded", "tokenFile", app.config.Webserver.TokenFile, "names", names)
}

// Done is closed once the App has stopped and released everything: after SIGHUP, SIGTERM or
// SIGINT, or a web server that stopped on its own. Restarting then tells what comes next.
func (app *App) Done() <-chan struct{} {
	return app.done
}

// Restarting reports, once Done is closed, whether the App stopped to be replaced by one with
// the reloaded configuration (SIGHUP, web server error); false means the process exits.
func (app *App) Restarting() bool {
	return app.restarting
}

// handleSignals handles SIGHUP (restart), SIGTERM and SIGINT (stop) from app.signals, and
// restarts the App when the web server stopped on its own.
//
// The subscription itself belongs to the caller and outlives this App, so nothing here
// stops or resets it; one goroutine per App consumes at most one signal. Being the only
// caller of shutdownProcedure, it also rules out two shutdowns running at once.
func (app *App) handleSignals() {

	go func() {
		slog.Debug("Starting signal handler")

		// Use select instead of a plain channel receive so the goroutine has
		// two exit paths and always terminates cleanly:
		//   - a signal or a server error is received and handled, or
		//   - the context is cancelled externally (e.g. from a concurrent shutdown).
		// Without the second path the goroutine would outlive its App and take
		// the next signal away from the App that replaced it. The loop only
		// continues after a SIGHUP whose config was rejected.
		for {
			select {
			case receivedSignal := <-app.signals:
				slog.Info("Received OS signal", "signal", receivedSignal)
				switch receivedSignal {
				case syscall.SIGHUP:
					if app.checkReload != nil {
						if err := app.checkReload(); err != nil {
							slog.Error("Config reload rejected, keeping the running configuration", "error", err)
							continue
						}
					}
					slog.Info("SIGHUP received, initiating restart")
					app.shutdownProcedure(modeRestart)
				case syscall.SIGTERM, syscall.SIGINT:
					slog.Info("SIGTERM/SIGINT received, stopping")
					app.shutdownProcedure(modeStop)
				}
				return
			case err := <-app.serverErr:
				slog.Error("Web server stopped unexpectedly, initiating restart", "error", err)
				app.shutdownProcedure(modeRestart)
				return
			case <-app.ctx.Done():
				// Context was cancelled externally – exit without triggering
				// a second shutdown procedure.
				slog.Debug("Signal handler: context cancelled, exiting goroutine")
				return
			}
		}
	}()
}

// shutdownProcedure stops the App and closes Done; m decides what Restarting reports. Closing
// instead of sending means it never blocks on a caller that is not listening.
func (app *App) shutdownProcedure(m mode) {
	slog.Info("Initiating shutdown", "mode", m)

	app.stop()

	app.restarting = m == modeRestart
	if app.restarting {
		slog.Info("Shutdown complete, restarting")
	} else {
		slog.Info("Module stopped", "module", MODULE, "version", VERSION, "pid", os.Getpid())
	}
	close(app.done)
}

// cleanup releases application resources.
// It's called when the application is shutdown or restarted.
// Should be used to free up resources.
func (app *App) cleanup() error {
	var errs error

	// here cleanup your service

	return errs
}

// abort undoes a Run that failed half way: it stops the goroutines already started and
// releases what setup acquired, so the caller can start another App with the same port. It
// returns err.
func (app *App) abort(err error) error {
	app.stop()
	return err
}

// stop cancels the App's context, waits for the web server and the other goroutines, and then
// releases the resources.
func (app *App) stop() {
	app.cancelFunc()
	app.wg.Wait()
	if err := app.cleanup(); err != nil {
		slog.Error("Cleanup failed", "error", err)
	}
}
