package health

import (
	"runtime"
	"testing"
	"time"
)

func TestGetCurrentHealth(t *testing.T) {
	got := GetCurrentHealth("demo_app", "v1.2.3")

	if got.App != "demo_app" || got.AppVersion != "v1.2.3" {
		t.Errorf("App, AppVersion = %q, %q, want demo_app, v1.2.3", got.App, got.AppVersion)
	}
	if got.OS != runtime.GOOS || got.GoVersion != runtime.Version() {
		t.Errorf("OS, GoVersion = %q, %q, want %q, %q", got.OS, got.GoVersion, runtime.GOOS, runtime.Version())
	}
	if got.Hostname == "" {
		t.Error("Hostname is empty")
	}
	if got.UptimeSeconds < 0 || got.NumGoroutines < 1 {
		t.Errorf("UptimeSeconds = %v, NumGoroutines = %d", got.UptimeSeconds, got.NumGoroutines)
	}

	ts, err := time.Parse(time.RFC3339, got.Timestamp)
	if err != nil {
		t.Fatalf("Timestamp %q is not RFC 3339: %v", got.Timestamp, err)
	}
	if _, offset := ts.Zone(); offset != 0 {
		t.Errorf("Timestamp %q is not UTC", got.Timestamp)
	}
}
