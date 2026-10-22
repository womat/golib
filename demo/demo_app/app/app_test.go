package app

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// These tests start the real HTTPS server on a free port of 127.0.0.1, with
// the embedded development certificate. The signals are sent into the channel
// the App reads, not to the process.

const lifecycleTimeout = 5 * time.Second

// freePort returns a port nothing listens on at the moment.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// testConfig is a valid dev configuration listening on port.
func testConfig(t *testing.T, port int) *Config {
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

// running starts an App and returns it with the channel it reads signals from.
func running(t *testing.T, checkReload func() error) (*App, chan os.Signal) {
	t.Helper()
	signals := make(chan os.Signal, 1)
	a, err := New(testConfig(t, freePort(t)), t.TempDir(), signals, checkReload).Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return a, signals
}

// stop ends a running App and waits until it is gone.
func stop(t *testing.T, a *App, signals chan<- os.Signal) {
	t.Helper()
	signals <- syscall.SIGTERM
	expect(t, a.Shutdown(), "shutdown")
}

// expect waits for ch to deliver.
func expect(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(lifecycleTimeout):
		t.Fatalf("no %s within %v", what, lifecycleTimeout)
	}
}

// serves reports whether the App answers GET /version.
func serves(a *App) bool {
	client := &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // the embedded dev certificate
		},
	}
	resp, err := client.Get("https://" + a.web.Addr + "/version")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func TestSIGTERMStopsAndReleasesThePort(t *testing.T) {
	a, signals := running(t, nil)
	if !serves(a) {
		t.Fatal("the App does not answer")
	}

	stop(t, a, signals)

	l, err := net.Listen("tcp", a.web.Addr)
	if err != nil {
		t.Fatalf("port still taken after shutdown: %v", err)
	}
	_ = l.Close()
}

func TestSIGHUPRestarts(t *testing.T) {
	a, signals := running(t, func() error { return nil })

	signals <- syscall.SIGHUP

	expect(t, a.Restart(), "restart")
}

func TestSIGHUPWithABrokenConfigKeepsRunning(t *testing.T) {
	checked := make(chan struct{}, 1)
	a, signals := running(t, func() error {
		checked <- struct{}{}
		return errors.New("broken config")
	})

	signals <- syscall.SIGHUP
	expect(t, checked, "config check")

	select {
	case <-a.Restart():
		t.Fatal("restarted with a config that failed its check")
	case <-time.After(200 * time.Millisecond):
	}
	if !serves(a) {
		t.Error("the App stopped serving after a rejected reload")
	}

	// It still listens for signals.
	stop(t, a, signals)
}

func TestServerErrorRestarts(t *testing.T) {
	a, _ := running(t, nil)

	a.serverErr <- errors.New("listener gone")

	expect(t, a.Restart(), "restart")
}

func TestFailedRunReleasesWhatItAcquired(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()
	port := busy.Addr().(*net.TCPAddr).Port

	signals := make(chan os.Signal, 1)
	a, err := New(testConfig(t, port), t.TempDir(), signals, nil).Run()
	if err == nil {
		t.Fatal("Run on a busy port succeeded")
	}
	if a.ctx.Err() == nil {
		t.Error("the context of the failed App is still live")
	}

	// The signal handler of the failed App is gone: a signal waits for the
	// next App instead of being taken by the dead one.
	signals <- syscall.SIGTERM
	time.Sleep(100 * time.Millisecond)
	if len(signals) != 1 {
		t.Error("the failed App consumed a signal")
	}
}

// The port of a restarted App is free for the next one: the restart waits for
// the listener to close.
func TestRestartReleasesThePort(t *testing.T) {
	a, signals := running(t, func() error { return nil })
	addr := a.web.Addr

	signals <- syscall.SIGHUP
	expect(t, a.Restart(), "restart")

	_, portText, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portText)
	next, err := New(testConfig(t, port), t.TempDir(), signals, nil).Run()
	if err != nil {
		t.Fatalf("next App on the same port: %v", err)
	}
	stop(t, next, signals)
}
