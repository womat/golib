package app

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

// newSignalApp returns an App whose signal handler listens on the returned channel, buffered
// like the one cmd subscribes. No web server runs.
func newSignalApp(t *testing.T, checkReload func() error) (*App, chan os.Signal) {
	t.Helper()
	signals := make(chan os.Signal, 1)
	app := New(validConfig(), signals, checkReload)
	app.handleSignals()
	return app, signals
}

// SIGHUP restarts the App once the config file has passed the check.
func TestSIGHUPRestarts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		checked := false
		app, signals := newSignalApp(t, func() error { checked = true; return nil })

		signals <- syscall.SIGHUP
		<-app.Done()
		if !app.Restarting() {
			t.Error("SIGHUP stopped the App for good instead of restarting it")
		}
		if !checked {
			t.Error("the config was not checked before the restart")
		}
		if app.ctx.Err() == nil {
			t.Error("the App context was not cancelled by the restart")
		}
	})
}

// A SIGHUP whose config is rejected leaves the App running, and it still reacts to the next
// signal.
func TestRejectedReloadKeepsRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		app, signals := newSignalApp(t, func() error { return errors.New("broken config") })

		signals <- syscall.SIGHUP
		synctest.Wait()
		if app.ctx.Err() != nil {
			t.Fatal("a rejected reload stopped the App")
		}
		select {
		case <-app.Done():
			t.Fatal("a rejected reload stopped the App")
		default:
		}

		signals <- syscall.SIGTERM
		<-app.Done()
	})
}

// SIGTERM and SIGINT stop the App for good; Done is closed, so every later receive returns at
// once.
func TestStopSignals(t *testing.T) {
	for _, sig := range []os.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				app, signals := newSignalApp(t, nil)

				signals <- sig
				<-app.Done()
				if _, open := <-app.Done(); open {
					t.Error("Done is still open after the stop")
				}
				if app.Restarting() {
					t.Errorf("%v reports a restart, want a stop", sig)
				}
				if app.ctx.Err() == nil {
					t.Error("the App context was not cancelled by the stop")
				}
			})
		})
	}
}

// A web server that stopped on its own restarts the App.
func TestServerErrorRestarts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		app, _ := newSignalApp(t, nil)

		app.serverErr <- errors.New("listener gone")
		<-app.Done()
		if !app.Restarting() {
			t.Error("a server error stopped the App for good instead of restarting it")
		}
	})
}

// Once its context is cancelled, the handler of an App leaves the next signal in the channel
// for the App that replaces it.
func TestCancelledHandlerLeavesSignalForNextApp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		app, signals := newSignalApp(t, nil)

		app.cancelFunc()
		synctest.Wait()

		signals <- syscall.SIGTERM
		synctest.Wait()
		if len(signals) != 1 {
			t.Error("the handler of a cancelled App consumed the signal")
		}
	})
}

// The tests below start the real HTTPS server on 127.0.0.1, with the embedded development
// certificate.

// serverConfig is a valid dev configuration listening on port, 0 for any free one.
func serverConfig(t *testing.T, port int) *Config {
	t.Helper()
	cfg := validConfig()
	cfg.Webserver.ListenHost = "127.0.0.1"
	cfg.Webserver.ListenPort = port
	cfg.Webserver.TokenFile = filepath.Join(t.TempDir(), "token-hashes")
	if err := os.WriteFile(cfg.Webserver.TokenFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// serves reports whether the App answers GET /version.
func serves(a *App) bool {
	client := &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,                                  // every request sees whether the server is up right now
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, // the embedded dev certificate
		},
	}
	resp, err := client.Get("https://" + a.web.Addr + "/version")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// A stopped App has closed its listener, so the next App can take the port at once.
func TestStopReleasesThePort(t *testing.T) {
	signals := make(chan os.Signal, 1)
	a, err := New(serverConfig(t, 0), signals, nil).Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !serves(a) {
		t.Fatal("the App does not answer")
	}

	signals <- syscall.SIGTERM
	select {
	case <-a.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("no stop within 5s")
	}

	l, err := net.Listen("tcp", a.web.Addr)
	if err != nil {
		t.Fatalf("port still taken after the stop: %v", err)
	}
	_ = l.Close()
}

// A signal that waits in the channel while a start fails stays there for the next App: the
// failed App must not handle it, because its abort would run alongside the shutdown the
// signal starts.
func TestFailedStartLeavesSignalForNextApp(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()

	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGTERM
	a, err := New(serverConfig(t, busy.Addr().(*net.TCPAddr).Port), signals, nil).Run()
	if err == nil {
		t.Fatal("Run succeeded on a port in use")
	}
	if a.ctx.Err() == nil {
		t.Error("the context of the failed App is still live")
	}

	select {
	case s := <-signals:
		if s != syscall.SIGTERM {
			t.Errorf("signal = %v, want SIGTERM", s)
		}
	default:
		t.Error("the failed App took the signal away from the next one")
	}
}

// A certificate that cannot be loaded does not fix itself: the error is permanent, so cmd ends
// with exit code 78 instead of letting systemd restart into it.
func TestRunMarksCertificateErrorsPermanent(t *testing.T) {
	cfg := serverConfig(t, 0)
	cfg.Env = ProdEnv // no embedded fallback

	_, err := New(cfg, make(chan os.Signal, 1), nil).Run()
	if !errors.Is(err, ErrPermanent) {
		t.Errorf("err = %v, want ErrPermanent", err)
	}
}

// An IPv4 listenHost listens on IPv4 only, so an IPv6 client cannot bypass IP lists written as
// IPv4.
func TestIPv4ListenHostListensOnIPv4Only(t *testing.T) {
	signals := make(chan os.Signal, 1)
	cfg := serverConfig(t, 0)
	cfg.Webserver.ListenHost = "0.0.0.0"
	a, err := New(cfg, signals, nil).Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer func() { signals <- syscall.SIGTERM; <-a.Done() }()

	_, port, _ := net.SplitHostPort(a.web.Addr)
	if conn, err := net.DialTimeout("tcp6", net.JoinHostPort("::1", port), time.Second); err == nil {
		_ = conn.Close()
		t.Error("the server answers on IPv6")
	}
}
